package tool

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/stretchr/testify/require"
)

type actionToolInput struct {
	Name string `json:"name" jsonschema:"required"`
}

type actionToolOutput struct {
	Greeting string `json:"greeting"`
}

func TestFromActionExposesActionAsTool(t *testing.T) {
	a := action.NewTyped[actionToolInput, actionToolOutput](action.Spec{
		Name:        "greet",
		Description: "greet someone",
	}, func(_ action.Ctx, input actionToolInput) (actionToolOutput, error) {
		return actionToolOutput{Greeting: "hello " + input.Name}, nil
	})

	tl := FromAction(a, WithActionGuidance("say hello"))

	require.Equal(t, "greet", tl.Name())
	require.Equal(t, "greet someone", tl.Description())
	require.Equal(t, "say hello", tl.Guidance())
	require.NotNil(t, tl.Schema())

	res, err := tl.Execute(minimalCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`{"name":"Ada"}`))
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.JSONEq(t, `{"greeting":"hello Ada"}`, res.String())
}

func TestFromActionMapsActionErrorToToolErrorResult(t *testing.T) {
	want := errors.New("boom")
	a := action.NewTyped[actionToolInput, actionToolOutput](action.Spec{Name: "fail"}, func(action.Ctx, actionToolInput) (actionToolOutput, error) {
		return actionToolOutput{}, want
	})

	res, err := FromAction(a).Execute(minimalCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`{"name":"Ada"}`))
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Equal(t, "boom", res.String())
}

func TestFromActionReturnsParseErrorForInvalidInput(t *testing.T) {
	a := action.NewTyped[actionToolInput, actionToolOutput](action.Spec{Name: "greet"}, func(action.Ctx, actionToolInput) (actionToolOutput, error) {
		return actionToolOutput{}, nil
	})

	_, err := FromAction(a).Execute(minimalCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`{"name":123}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse greet input")
}

func TestFromActionPreservesToolResultData(t *testing.T) {
	a := action.New(action.Spec{Name: "tool_result"}, func(action.Ctx, any) action.Result {
		return action.Result{Data: Error("denied")}
	})

	res, err := FromAction(a).Execute(minimalCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Equal(t, "denied", res.String())
}

func TestFromActionMapsExplicitFailedStatusToToolErrorResult(t *testing.T) {
	a := action.New(action.Spec{Name: "fail_status"}, func(action.Ctx, any) action.Result {
		return action.Failed(nil)
	})

	res, err := FromAction(a).Execute(minimalCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, nil)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Equal(t, "action failed", res.String())
}

func TestToActionAdaptsLegacyTool(t *testing.T) {
	tl := New("echo", "echo text", func(_ Ctx, p struct {
		Text string `json:"text" jsonschema:"required"`
	}) (Result, error) {
		return Text(p.Text), nil
	})

	a := ToAction(tl)
	spec := a.Spec()
	require.Equal(t, "echo", spec.Name)
	require.Equal(t, "echo text", spec.Description)
	require.Equal(t, reflect.TypeOf(json.RawMessage{}), spec.Input.GoType)
	require.NotNil(t, spec.Input.Schema)

	res := a.Execute(minimalCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, map[string]any{"text": "hi"})
	require.NoError(t, res.Error)
	toolRes, ok := res.Data.(Result)
	require.True(t, ok)
	require.Equal(t, "hi", toolRes.String())
}

func TestToActionRequiresToolCtx(t *testing.T) {
	tl := New("echo", "echo text", func(_ Ctx, _ struct{}) (Result, error) {
		return Text("ok"), nil
	})

	res := ToAction(tl).Execute(action.NewCtx(context.Background()), json.RawMessage(`{}`))
	require.Error(t, res.Error)
	require.Contains(t, res.Error.Error(), "requires tool.Ctx")
}

func TestFromActionAppliesActionMiddlewareOption(t *testing.T) {
	a := action.NewTyped[string, string](action.Spec{Name: "echo"}, func(_ action.Ctx, input string) (string, error) {
		return input, nil
	})

	tl := FromAction(a, WithActionMiddleware(action.HooksMiddleware(prefixHook{prefix: "option:"})))
	res, err := tl.Execute(minimalCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`"value"`))
	require.NoError(t, err)
	require.JSONEq(t, `"option:value"`, res.String())
}

func TestApplyActionAppliesMiddlewareToActionBackedTool(t *testing.T) {
	a := action.NewTyped[string, string](action.Spec{Name: "echo"}, func(_ action.Ctx, input string) (string, error) {
		return input, nil
	})

	tl := ApplyAction(FromAction(a), action.HooksMiddleware(prefixHook{prefix: "apply:"}))
	require.Implements(t, (*ActionBacked)(nil), tl)
	res, err := tl.Execute(minimalCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`"value"`))
	require.NoError(t, err)
	require.JSONEq(t, `"apply:value"`, res.String())
}

func TestApplyActionLeavesLegacyToolUnchanged(t *testing.T) {
	legacy := New("legacy", "legacy tool", func(Ctx, struct{}) (Result, error) {
		return Text("ok"), nil
	})

	got := ApplyAction(legacy, action.HooksMiddleware(prefixHook{prefix: "ignored:"}))
	require.Same(t, legacy, got)
}

type prefixHook struct {
	action.HooksBase
	prefix string
}

func (h prefixHook) OnInput(_ action.Ctx, _ action.Action, input any, _ action.CallState) (any, action.Result, bool) {
	return h.prefix + input.(string), action.Result{}, false
}
