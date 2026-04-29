package tool

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/require"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

// fakeTool is a minimal Tool for testing.
type fakeTool struct {
	name    string
	desc    string
	guid    string
	schema  *jsonschema.Schema
	handler func(ctx Ctx, input json.RawMessage) (Result, error)
}

func (t *fakeTool) Name() string               { return t.name }
func (t *fakeTool) Description() string        { return t.desc }
func (t *fakeTool) Guidance() string           { return t.guid }
func (t *fakeTool) Schema() *jsonschema.Schema { return t.schema }
func (t *fakeTool) Execute(ctx Ctx, input json.RawMessage) (Result, error) {
	if t.handler != nil {
		return t.handler(ctx, input)
	}
	return Text("ok"), nil
}

type fakeCtx struct {
	context.Context
}

func (c fakeCtx) WorkDir() string       { return "/tmp" }
func (c fakeCtx) AgentID() string       { return "test-agent" }
func (c fakeCtx) SessionID() string     { return "test-session" }
func (c fakeCtx) Extra() map[string]any { return nil }

func testCtx() Ctx {
	return fakeCtx{Context: context.Background()}
}

// ── Apply / Unwrap / Innermost ────────────────────────────────────────────────

func TestApply_NoMiddleware(t *testing.T) {
	base := &fakeTool{name: "base"}
	result := Apply(base)
	require.Equal(t, base, result, "Apply with no middlewares should return the same tool")
}

func TestApply_SingleMiddleware(t *testing.T) {
	base := &fakeTool{name: "base"}
	m := HooksMiddleware(&renameHooks{newName: "wrapped"})
	result := Apply(base, m)

	require.Equal(t, "wrapped", result.Name())
	require.Equal(t, base, Unwrap(result))
}

func TestApply_MultipleMiddlewares_Order(t *testing.T) {
	base := &fakeTool{name: "base", desc: "original"}

	m1 := HooksMiddleware(&renameHooks{newName: "m1"})
	m2 := HooksMiddleware(&renameHooks{newName: "m2"})

	// Apply(t, m1, m2) → m2(m1(t))
	// m2 is outermost, so its name wins.
	result := Apply(base, m1, m2)
	require.Equal(t, "m2", result.Name())

	// Unwrap once → m1's layer
	inner := Unwrap(result)
	require.NotNil(t, inner)
	require.Equal(t, "m1", inner.Name())

	// Unwrap again → base
	innermost := Unwrap(inner)
	require.NotNil(t, innermost)
	require.Equal(t, "base", innermost.Name())

	// Innermost goes all the way
	require.Equal(t, base, Innermost(result))
}

func TestUnwrap_UnwrappedTool(t *testing.T) {
	base := &fakeTool{name: "base"}
	require.Nil(t, Unwrap(base))
}

func TestInnermost_UnwrappedTool(t *testing.T) {
	base := &fakeTool{name: "base"}
	require.Equal(t, base, Innermost(base))
}

// ── Metadata hooks ────────────────────────────────────────────────────────────

func TestHooks_MetadataPassthrough(t *testing.T) {
	base := &fakeTool{
		name: "base",
		desc: "base desc",
		guid: "base guidance",
		schema: &jsonschema.Schema{
			Type: "object",
		},
	}

	// HooksBase returns "" / nil for all metadata → inner values pass through.
	m := HooksMiddleware(&HooksBase{})
	wrapped := Apply(base, m)

	require.Equal(t, "base", wrapped.Name())
	require.Equal(t, "base desc", wrapped.Description())
	require.Equal(t, "base guidance", wrapped.Guidance())
	require.Equal(t, base.Schema(), wrapped.Schema())
}

func TestHooks_MetadataOverride(t *testing.T) {
	base := &fakeTool{
		name:   "base",
		desc:   "base desc",
		guid:   "base guidance",
		schema: &jsonschema.Schema{Type: "object"},
	}

	newSchema := &jsonschema.Schema{Type: "object", Description: "extended"}
	m := HooksMiddleware(&fullMetadataHooks{
		name:   "new-name",
		desc:   "new desc",
		guid:   "new guidance",
		schema: newSchema,
	})
	wrapped := Apply(base, m)

	require.Equal(t, "new-name", wrapped.Name())
	require.Equal(t, "new desc", wrapped.Description())
	require.Equal(t, "new guidance", wrapped.Guidance())
	require.Equal(t, newSchema, wrapped.Schema())
}

// ── OnInput: passthrough ──────────────────────────────────────────────────────

func TestHooks_OnInput_Passthrough(t *testing.T) {
	called := false
	base := &fakeTool{
		name: "base",
		handler: func(ctx Ctx, input json.RawMessage) (Result, error) {
			called = true
			require.Equal(t, `{"key":"value"}`, string(input))
			return Text("done"), nil
		},
	}

	m := HooksMiddleware(&HooksBase{})
	wrapped := Apply(base, m)

	res, err := wrapped.Execute(testCtx(), json.RawMessage(`{"key":"value"}`))
	require.NoError(t, err)
	require.True(t, called)
	require.Equal(t, "done", res.String())
}

// ── OnInput: transform ────────────────────────────────────────────────────────

func TestHooks_OnInput_Transform(t *testing.T) {
	base := &fakeTool{
		name: "base",
		handler: func(ctx Ctx, input json.RawMessage) (Result, error) {
			require.Equal(t, `{"transformed":true}`, string(input))
			return Text("ok"), nil
		},
	}

	m := HooksMiddleware(&transformInputHooks{
		replacement: `{"transformed":true}`,
	})
	wrapped := Apply(base, m)

	res, err := wrapped.Execute(testCtx(), json.RawMessage(`{"original":true}`))
	require.NoError(t, err)
	require.Equal(t, "ok", res.String())
}

// ── OnInput: short-circuit with result ────────────────────────────────────────

func TestHooks_OnInput_ShortCircuitResult(t *testing.T) {
	innerCalled := false
	base := &fakeTool{
		name: "base",
		handler: func(ctx Ctx, input json.RawMessage) (Result, error) {
			innerCalled = true
			return Text("should not reach"), nil
		},
	}

	m := HooksMiddleware(&shortCircuitResultHooks{
		result: Error("denied: too dangerous"),
	})
	wrapped := Apply(base, m)

	res, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.False(t, innerCalled)
	require.True(t, res.IsError())
	require.Equal(t, "denied: too dangerous", res.String())
}

// ── OnInput: short-circuit with error ─────────────────────────────────────────

func TestHooks_OnInput_ShortCircuitError(t *testing.T) {
	innerCalled := false
	base := &fakeTool{
		name: "base",
		handler: func(ctx Ctx, input json.RawMessage) (Result, error) {
			innerCalled = true
			return nil, nil
		},
	}

	m := HooksMiddleware(&shortCircuitErrorHooks{
		err: errors.New("infrastructure failure"),
	})
	wrapped := Apply(base, m)

	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.Error(t, err)
	require.Equal(t, "infrastructure failure", err.Error())
	require.False(t, innerCalled)
}

// ── OnContext ─────────────────────────────────────────────────────────────────

func TestHooks_OnContext_CleanupCalled(t *testing.T) {
	cleanedUp := false
	base := &fakeTool{name: "base"}

	m := HooksMiddleware(&contextHooks{
		cleanup: func() { cleanedUp = true },
	})
	wrapped := Apply(base, m)

	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, cleanedUp)
}

func TestHooks_OnContext_CleanupCalledAfterOnResult(t *testing.T) {
	var order []string
	base := &fakeTool{name: "base"}

	m := HooksMiddleware(&orderTrackingHooks{
		order:   &order,
		cleanup: func() { order = append(order, "cleanup") },
	})
	wrapped := Apply(base, m)

	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	// OnResult runs before cleanup (cleanup is deferred)
	require.Equal(t, []string{"onresult", "cleanup"}, order)
}

// ── OnResult ──────────────────────────────────────────────────────────────────

func TestHooks_OnResult_Transform(t *testing.T) {
	base := &fakeTool{
		name: "base",
		handler: func(ctx Ctx, input json.RawMessage) (Result, error) {
			return Text("original"), nil
		},
	}

	m := HooksMiddleware(&transformResultHooks{
		transform: func(res Result, err error) (Result, error) {
			return Text("modified: " + res.String()), nil
		},
	})
	wrapped := Apply(base, m)

	res, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "modified: original", res.String())
}

func TestHooks_OnResult_ReceivesInnerError(t *testing.T) {
	innerErr := errors.New("inner failed")
	base := &fakeTool{
		name: "base",
		handler: func(ctx Ctx, input json.RawMessage) (Result, error) {
			return nil, innerErr
		},
	}

	var receivedErr error
	m := HooksMiddleware(&transformResultHooks{
		transform: func(res Result, err error) (Result, error) {
			receivedErr = err
			return res, err
		},
	})
	wrapped := Apply(base, m)

	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.Error(t, err)
	require.Equal(t, innerErr, receivedErr)
}

// ── CallState: data flows between hooks ───────────────────────────────────────

func TestHooks_CallState_FlowsBetweenPhases(t *testing.T) {
	base := &fakeTool{name: "base"}

	m := HooksMiddleware(&stateFlowHooks{t: t})
	wrapped := Apply(base, m)

	res, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "state-value", res.String())
}

// ── Stacking: multiple middlewares compose correctly ──────────────────────────

func TestHooks_Stacking_ExecutionOrder(t *testing.T) {
	var order []string
	base := &fakeTool{
		name: "base",
		handler: func(ctx Ctx, input json.RawMessage) (Result, error) {
			order = append(order, "execute")
			return Text("ok"), nil
		},
	}

	makeTracker := func(label string) *executionOrderHooks {
		return &executionOrderHooks{label: label, order: &order}
	}

	// Apply(base, inner, outer) → outer wraps inner wraps base
	wrapped := Apply(base,
		HooksMiddleware(makeTracker("inner")),
		HooksMiddleware(makeTracker("outer")),
	)

	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)

	// Outer runs first (outermost), then inner, then execute, then inner result, then outer result
	require.Equal(t, []string{
		"outer:input",
		"inner:input",
		"execute",
		"inner:result",
		"outer:result",
	}, order)
}

// ── OnIntent: passthrough ────────────────────────────────────────────────────

func TestHooksBase_OnIntent_Passthrough(t *testing.T) {
	var h HooksBase
	intent := Intent{
		Tool:       "test",
		ToolClass:  "filesystem_read",
		Confidence: "high",
		Operations: []IntentOperation{{
			Resource:  IntentResource{Category: "file", Value: "/tmp/x", Locality: "workspace"},
			Operation: "read",
			Certain:   true,
		}},
	}

	got := h.OnIntent(testCtx(), &fakeTool{name: "test"}, intent, make(CallState))
	require.Equal(t, intent, got)
}
// ── MiddlewareFunc ────────────────────────────────────────────────────────────

func TestMiddlewareFunc(t *testing.T) {
	base := &fakeTool{name: "base"}
	m := MiddlewareFunc(func(inner Tool) Tool {
		return &fakeTool{
			name: "func-wrapped",
			handler: func(ctx Ctx, input json.RawMessage) (Result, error) {
				return inner.Execute(ctx, input)
			},
		}
	})

	wrapped := m.Wrap(base)
	require.Equal(t, "func-wrapped", wrapped.Name())

	res, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ok", res.String())
}

// ── Hook implementations for tests ───────────────────────────────────────────

type renameHooks struct {
	HooksBase
	newName string
}

func (h *renameHooks) OnName(Tool) string { return h.newName }

type fullMetadataHooks struct {
	HooksBase
	name, desc, guid string
	schema           *jsonschema.Schema
}

func (h *fullMetadataHooks) OnName(Tool) string               { return h.name }
func (h *fullMetadataHooks) OnDescription(Tool) string        { return h.desc }
func (h *fullMetadataHooks) OnGuidance(Tool) string           { return h.guid }
func (h *fullMetadataHooks) OnSchema(Tool) *jsonschema.Schema { return h.schema }

type transformInputHooks struct {
	HooksBase
	replacement string
}

func (h *transformInputHooks) OnInput(_ Ctx, _ Tool, _ json.RawMessage, _ CallState) (json.RawMessage, Result, error) {
	return json.RawMessage(h.replacement), nil, nil
}

type shortCircuitResultHooks struct {
	HooksBase
	result Result
}

func (h *shortCircuitResultHooks) OnInput(_ Ctx, _ Tool, _ json.RawMessage, _ CallState) (json.RawMessage, Result, error) {
	return nil, h.result, nil
}

type shortCircuitErrorHooks struct {
	HooksBase
	err error
}

func (h *shortCircuitErrorHooks) OnInput(_ Ctx, _ Tool, _ json.RawMessage, _ CallState) (json.RawMessage, Result, error) {
	return nil, nil, h.err
}

type contextHooks struct {
	HooksBase
	cleanup func()
}

func (h *contextHooks) OnContext(ctx Ctx, _ CallState) (Ctx, func()) {
	return ctx, h.cleanup
}

type orderTrackingHooks struct {
	HooksBase
	order   *[]string
	cleanup func()
}

func (h *orderTrackingHooks) OnContext(ctx Ctx, _ CallState) (Ctx, func()) {
	return ctx, h.cleanup
}

func (h *orderTrackingHooks) OnResult(_ Ctx, _ Tool, _ json.RawMessage, res Result, err error, _ CallState) (Result, error) {
	*h.order = append(*h.order, "onresult")
	return res, err
}

type transformResultHooks struct {
	HooksBase
	transform func(Result, error) (Result, error)
}

func (h *transformResultHooks) OnResult(_ Ctx, _ Tool, _ json.RawMessage, res Result, err error, _ CallState) (Result, error) {
	return h.transform(res, err)
}

type stateFlowHooks struct {
	HooksBase
	t *testing.T
}

func (h *stateFlowHooks) OnInput(_ Ctx, _ Tool, input json.RawMessage, state CallState) (json.RawMessage, Result, error) {
	state["parsed"] = "state-value"
	return input, nil, nil
}

func (h *stateFlowHooks) OnResult(_ Ctx, _ Tool, _ json.RawMessage, _ Result, _ error, state CallState) (Result, error) {
	val, ok := state["parsed"].(string)
	require.True(h.t, ok, "state should contain 'parsed' key")
	return Text(val), nil
}

type executionOrderHooks struct {
	HooksBase
	label string
	order *[]string
}

func (h *executionOrderHooks) OnInput(_ Ctx, _ Tool, input json.RawMessage, _ CallState) (json.RawMessage, Result, error) {
	*h.order = append(*h.order, h.label+":input")
	return input, nil, nil
}

func (h *executionOrderHooks) OnResult(_ Ctx, _ Tool, _ json.RawMessage, res Result, err error, _ CallState) (Result, error) {
	*h.order = append(*h.order, h.label+":result")
	return res, err
}
