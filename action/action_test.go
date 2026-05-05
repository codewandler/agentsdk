package action

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/require"
)

type typedInput struct {
	Name string `json:"name" jsonschema:"required"`
	Age  int    `json:"age"`
}

type typedOutput struct {
	Greeting string `json:"greeting"`
}

func TestNewTypedExecutesAndInfersTypes(t *testing.T) {
	a := NewTyped[typedInput, typedOutput](Spec{
		Name:        "greet",
		Description: "Greet a person.",
	}, func(ctx Ctx, in typedInput) (typedOutput, error) {
		require.NotNil(t, ctx)
		return typedOutput{Greeting: "hello " + in.Name}, nil
	})

	spec := a.Spec()
	require.Equal(t, "greet", spec.Name)
	require.Equal(t, reflect.TypeOf(typedInput{}), spec.Input.GoType)
	require.Equal(t, reflect.TypeOf(typedOutput{}), spec.Output.GoType)
	require.NotNil(t, spec.Input.Schema)
	require.NotNil(t, spec.Output.Schema)

	result := a.Execute(NewCtx(context.Background()), typedInput{Name: "Ada", Age: 42})
	require.NoError(t, result.Error)
	require.Equal(t, typedOutput{Greeting: "hello Ada"}, result.Data)
}

func TestNewTypedMapsHandlerErrorToResult(t *testing.T) {
	want := errors.New("boom")
	a := NewTyped[typedInput, typedOutput](Spec{Name: "fail"}, func(Ctx, typedInput) (typedOutput, error) {
		return typedOutput{}, want
	})

	result := a.Execute(NewCtx(context.Background()), typedInput{Name: "Ada"})
	require.ErrorIs(t, result.Error, want)
	require.Nil(t, result.Data)
}

func TestNewTypedInvalidInputReturnsResultError(t *testing.T) {
	a := NewTyped[typedInput, typedOutput](Spec{Name: "greet"}, func(Ctx, typedInput) (typedOutput, error) {
		return typedOutput{}, nil
	})

	result := a.Execute(NewCtx(context.Background()), "not input")
	require.Error(t, result.Error)
	var invalid ErrInvalidInput
	require.ErrorAs(t, result.Error, &invalid)
	require.Equal(t, reflect.TypeOf(typedInput{}), invalid.Expected)
	require.Equal(t, reflect.TypeOf(""), invalid.Actual)
}

func TestResultContracts(t *testing.T) {
	ok := OK("done", "event")
	require.Equal(t, StatusOK, ok.Status)
	require.False(t, ok.IsError())
	require.NoError(t, ok.Err())
	require.Equal(t, []Event{"event"}, ok.Events)

	failed := Failed(nil)
	require.Equal(t, StatusError, failed.Status)
	require.True(t, failed.IsError())
	require.EqualError(t, failed.Err(), "action failed")
}

func TestTypeDecodeJSONValidatesWhenSchemaPresent(t *testing.T) {
	typ := TypeOf[typedInput]()
	require.NotNil(t, typ.Schema)

	decoded, err := typ.DecodeJSON([]byte(`{"name":"Ada","age":42}`))
	require.NoError(t, err)
	require.Equal(t, typedInput{Name: "Ada", Age: 42}, decoded)

	_, err = typ.DecodeJSON([]byte(`{"age":42}`))
	require.Error(t, err)
}

func TestTypeSchemaNilForNonSerializableInput(t *testing.T) {
	typ := TypeOf[chan int]()
	require.Equal(t, reflect.TypeOf((chan int)(nil)), typ.GoType)
	require.Nil(t, typ.Schema)
}

func TestRegistryRegistersAndResolvesActions(t *testing.T) {
	a := New(Spec{Name: "one"}, func(Ctx, any) Result { return Result{Data: "ok"} })
	reg := NewRegistry()
	require.NoError(t, reg.Register(a))

	got, ok := reg.Get("one")
	require.True(t, ok)
	require.Same(t, a, got)
	require.Equal(t, []Action{a}, reg.All())
	require.ErrorAs(t, reg.Register(a), &ErrDuplicate{})
}

func TestHandlerMiddlewareWrapsExecution(t *testing.T) {
	a := New(Spec{Name: "base"}, func(Ctx, any) Result { return Result{Data: "base"} })
	wrapped := Apply(a, HandlerMiddlewareFunc(func(next Handler) Handler {
		return func(ctx Ctx, input any) Result {
			result := next(ctx, input)
			result.Events = append(result.Events, "wrapped")
			return result
		}
	}))

	result := wrapped.Execute(NewCtx(context.Background()), nil)
	require.Equal(t, "base", result.Data)
	require.Equal(t, []Event{"wrapped"}, result.Events)
}

func TestHooksMiddlewarePhases(t *testing.T) {
	cleaned := false
	a := New(Spec{Name: "base"}, func(_ Ctx, input any) Result {
		require.Equal(t, "transformed", input)
		return Result{Data: "base"}
	})

	wrapped := Apply(a, HooksMiddleware(&testHooks{cleaned: &cleaned}))
	require.Equal(t, "wrapped", wrapped.Spec().Name)

	result := wrapped.Execute(NewCtx(context.Background()), "original")
	require.Equal(t, "base wrapped", result.Data)
	require.Equal(t, []Event{"input", "result"}, result.Events)
	require.True(t, cleaned)
}

func TestHooksMiddlewareShortCircuit(t *testing.T) {
	a := New(Spec{Name: "base"}, func(Ctx, any) Result {
		t.Fatal("inner action should not execute")
		return Result{}
	})

	wrapped := Apply(a, HooksMiddleware(&shortCircuitHooks{}))
	result := wrapped.Execute(NewCtx(context.Background()), "original")
	require.Equal(t, "denied observed", result.Data)
}

type testHooks struct {
	HooksBase
	cleaned *bool
}

func (h *testHooks) OnSpec(_ Action, spec Spec) Spec {
	spec.Name = "wrapped"
	return spec
}

func (h *testHooks) OnInput(_ Ctx, _ Action, _ any, state CallState) (any, Result, bool) {
	state["events"] = []Event{"input"}
	return "transformed", Result{}, false
}

func (h *testHooks) OnContext(ctx Ctx, state CallState) (Ctx, func()) {
	state["context"] = true
	return ctx, func() { *h.cleaned = true }
}

func (h *testHooks) OnResult(_ Ctx, _ Action, _ any, result Result, state CallState) Result {
	if !state["context"].(bool) {
		result.Error = errors.New("context hook did not run")
		return result
	}
	result.Data = result.Data.(string) + " wrapped"
	result.Events = append(state["events"].([]Event), "result")
	return result
}

type shortCircuitHooks struct{ HooksBase }

func (shortCircuitHooks) OnInput(Ctx, Action, any, CallState) (any, Result, bool) {
	return "original", Result{Data: "denied"}, true
}

func (shortCircuitHooks) OnResult(_ Ctx, _ Action, _ any, result Result, _ CallState) Result {
	result.Data = result.Data.(string) + " observed"
	return result
}

type customSchemaParam string

func (customSchemaParam) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{Type: "string", Description: "custom value"}
}

func TestSchemaForAppliesRequiredTagFix(t *testing.T) {
	type Params struct {
		Path customSchemaParam `json:"path" jsonschema:"description=Accepts a string\\, an array\\, or a glob.,required"`
		Name string            `json:"name" jsonschema:"description=Simple name,required"`
	}

	s := SchemaFor[Params]()
	require.NotNil(t, s)
	require.Contains(t, s.Required, "path")
	require.Contains(t, s.Required, "name")

	pathProp, ok := s.Properties.Get("path")
	require.True(t, ok)
	require.Contains(t, pathProp.Description, "string, an array, or a glob")
}

func TestSchemaForDoesNotInjectNumericExamples(t *testing.T) {
	type Params struct {
		Limit int `json:"limit"`
	}

	raw, err := json.Marshal(SchemaFor[Params]())
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(raw, &parsed))
	props := parsed["properties"].(map[string]any)
	limit := props["limit"].(map[string]any)
	require.Equal(t, "integer", limit["type"])
	_, ok := limit["examples"]
	require.False(t, ok)
}

func TestExtractIntentFromActionProviderAndMiddleware(t *testing.T) {
	base := intentAction{Action: New(Spec{Name: "read_file"}, func(Ctx, any) Result { return Result{} })}
	wrapped := Apply(base, HooksMiddleware(&intentAmendingHooks{}))

	intent := ExtractIntent(wrapped, NewCtx(context.Background()), "input")
	require.Equal(t, "read_file", intent.Action)
	require.Equal(t, "filesystem_read", intent.Class)
	require.Equal(t, []string{"filesystem_read", "audited"}, intent.Behaviors)
	require.Len(t, intent.Operations, 2)
}

func TestExtractIntentOpaqueFallback(t *testing.T) {
	a := New(Spec{Name: "unknown"}, func(Ctx, any) Result { return Result{} })
	intent := ExtractIntent(a, NewCtx(context.Background()), nil)
	require.Equal(t, "unknown", intent.Action)
	require.Equal(t, "unknown", intent.Class)
	require.True(t, intent.Opaque)
	require.Equal(t, "low", intent.Confidence)
}

func TestIntentNormalizeMirrorsActionAndToolFields(t *testing.T) {
	intent := Intent{Class: "filesystem_read"}.Normalize("read_file")
	require.Equal(t, "read_file", intent.Action)
	require.Equal(t, "read_file", intent.Tool)
	require.Equal(t, "filesystem_read", intent.Class)
	require.Equal(t, "filesystem_read", intent.ToolClass)
	require.Equal(t, "low", intent.Confidence)

	legacy := Intent{Tool: "legacy_tool", ToolClass: "network_access", Confidence: "high"}.Normalize("")
	require.Equal(t, "legacy_tool", legacy.Action)
	require.Equal(t, "legacy_tool", legacy.Tool)
	require.Equal(t, "network_access", legacy.Class)
	require.Equal(t, "network_access", legacy.ToolClass)
	require.Equal(t, "high", legacy.Confidence)
}

type intentAction struct{ Action }

func (a intentAction) DeclareIntent(Ctx, any) (Intent, error) {
	return Intent{
		Action:     a.Spec().Name,
		Class:      "filesystem_read",
		Behaviors:  []string{"filesystem_read"},
		Confidence: "high",
		Operations: []IntentOperation{{
			Resource:  IntentResource{Category: "file", Value: "README.md", Locality: "workspace"},
			Operation: "read",
			Certain:   true,
		}},
	}, nil
}

type intentAmendingHooks struct{ HooksBase }

func (intentAmendingHooks) OnIntent(_ Ctx, _ Action, intent Intent, _ CallState) Intent {
	intent.Behaviors = append(intent.Behaviors, "audited")
	intent.Operations = append(intent.Operations, IntentOperation{
		Resource:  IntentResource{Category: "file", Value: "audit.log", Locality: "workspace"},
		Operation: "write",
		Certain:   true,
	})
	return intent
}
