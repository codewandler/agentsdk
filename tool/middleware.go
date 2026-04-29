package tool

import (
	"encoding/json"

	"github.com/invopop/jsonschema"
)

// Middleware wraps a Tool, intercepting its lifecycle at defined hook points.
type Middleware interface {
	// Wrap takes an inner Tool and returns a new Tool that decorates it.
	Wrap(inner Tool) Tool
}

// MiddlewareFunc is a convenience adapter for functions that implement Middleware.
type MiddlewareFunc func(inner Tool) Tool

func (f MiddlewareFunc) Wrap(inner Tool) Tool { return f(inner) }

// Apply wraps a tool with one or more middlewares.
// Apply(tool, m1, m2, m3) → m3(m2(m1(tool)))
// First applied = innermost. Last applied = outermost (runs first on call).
func Apply(t Tool, middlewares ...Middleware) Tool {
	for _, m := range middlewares {
		t = m.Wrap(t)
	}
	return t
}

// Unwrap returns the immediate inner tool if t is a wrapped tool.
// Returns nil if t is not wrapped.
func Unwrap(t Tool) Tool {
	if w, ok := t.(interface{ Unwrap() Tool }); ok {
		return w.Unwrap()
	}
	return nil
}

// Innermost returns the deepest unwrapped tool by repeatedly calling Unwrap.
func Innermost(t Tool) Tool {
	for {
		inner := Unwrap(t)
		if inner == nil {
			return t
		}
		t = inner
	}
}

// CallState is per-call mutable state shared across hook phases within
// one middleware. Each middleware gets its own CallState per Execute call.
// It is NOT shared across stacked middlewares.
type CallState map[string]any

// Hooks defines the injection points a middleware can implement.
// All methods have no-op defaults via HooksBase.
//
// Hook methods receive the inner Tool for introspection (e.g. type-asserting
// to IntentProvider). They must not call inner.Execute — that is handled
// by the wrapper.
type Hooks interface {
	// ── Metadata (called once at wrap time, cached) ──

	// OnName returns a replacement name, or "" to keep the inner tool's name.
	OnName(inner Tool) string

	// OnDescription returns a replacement description, or "" to keep inner's.
	OnDescription(inner Tool) string

	// OnGuidance returns replacement guidance, or "" to keep inner's.
	// To append rather than replace, concatenate with inner.Guidance() yourself.
	OnGuidance(inner Tool) string

	// OnSchema receives the inner tool's schema and returns an extended schema.
	// Return nil to keep the inner schema unchanged.
	// The returned schema is cached — OnSchema is called once per Wrap.
	OnSchema(inner Tool) *jsonschema.Schema

	// ── Per-call hooks (called on every Execute) ──

	// OnInput is called with the raw JSON arguments from the LLM.
	// It may:
	//   - Return (modified_input, nil, nil) to transform and continue.
	//   - Return (_, result, nil) to short-circuit with a result (skip Execute).
	//   - Return (_, nil, err) to short-circuit with an error (skip Execute).
	//   - Return (input, nil, nil) to pass through unchanged.
	//
	// Use state to pass parsed values to later hooks (OnContext, OnResult).
	OnInput(ctx Ctx, inner Tool, input json.RawMessage, state CallState) (json.RawMessage, Result, error)

	// OnContext is called after OnInput succeeds (no short-circuit).
	// Returns a (possibly modified) Ctx and a cleanup function.
	// The cleanup is deferred immediately — it runs after Execute + OnResult.
	OnContext(ctx Ctx, state CallState) (Ctx, func())

	// OnIntent is called during intent extraction (ExtractIntent). It may
	// amend, enrich, or replace the intent. For example:
	//   - A locality-aware middleware can upgrade Locality from "unknown"
	//     to "sensitive" based on deployment context.
	//   - A middleware that writes audit logs can append its own operations.
	//   - A middleware can downgrade Confidence if it detects uncertainty.
	//
	// Return the (possibly modified) intent. Return the input intent
	// unchanged to pass through.
	OnIntent(ctx Ctx, inner Tool, intent Intent, state CallState) Intent

	// OnResult is called after the inner tool returns (or after context
	// expiry). It may inspect, log, transform, or replace the result/error.
	OnResult(ctx Ctx, inner Tool, input json.RawMessage, result Result, err error, state CallState) (Result, error)
}

// HooksBase provides no-op defaults for all hooks.
// Embed this in concrete middleware structs.
type HooksBase struct{}

func (HooksBase) OnName(Tool) string        { return "" }
func (HooksBase) OnDescription(Tool) string { return "" }
func (HooksBase) OnGuidance(Tool) string    { return "" }
func (HooksBase) OnSchema(Tool) *jsonschema.Schema {
	return nil
}
func (HooksBase) OnInput(_ Ctx, _ Tool, input json.RawMessage, _ CallState) (json.RawMessage, Result, error) {
	return input, nil, nil
}
func (HooksBase) OnContext(ctx Ctx, _ CallState) (Ctx, func()) { return ctx, nil }
func (HooksBase) OnIntent(_ Ctx, _ Tool, intent Intent, _ CallState) Intent {
	return intent
}
func (HooksBase) OnResult(_ Ctx, _ Tool, _ json.RawMessage, res Result, err error, _ CallState) (Result, error) {
	return res, err
}

// HooksMiddleware creates a Middleware from a Hooks implementation.
func HooksMiddleware(hooks Hooks) Middleware {
	return MiddlewareFunc(func(inner Tool) Tool {
		t := &hookedTool{inner: inner, hooks: hooks}
		// Cache metadata at wrap time — these don't change per call.
		t.name = hooks.OnName(inner)
		t.desc = hooks.OnDescription(inner)
		t.guid = hooks.OnGuidance(inner)
		t.schema = hooks.OnSchema(inner)
		return t
	})
}

// hookedTool wraps an inner Tool with hook-based interception.
// It satisfies the Tool interface and supports Unwrap for introspection.
type hookedTool struct {
	inner  Tool
	hooks  Hooks
	name   string             // cached; "" means use inner
	desc   string             // cached; "" means use inner
	guid   string             // cached; "" means use inner
	schema *jsonschema.Schema // cached; nil means use inner
}

// Unwrap exposes the inner tool for introspection.
func (t *hookedTool) Unwrap() Tool { return t.inner }

func (t *hookedTool) Name() string {
	if t.name != "" {
		return t.name
	}
	return t.inner.Name()
}

func (t *hookedTool) Description() string {
	if t.desc != "" {
		return t.desc
	}
	return t.inner.Description()
}

func (t *hookedTool) Guidance() string {
	if t.guid != "" {
		return t.guid
	}
	return t.inner.Guidance()
}

func (t *hookedTool) Schema() *jsonschema.Schema {
	if t.schema != nil {
		return t.schema
	}
	return t.inner.Schema()
}

// onIntent delegates to the hooks' OnIntent. Called by ExtractIntent
// as it walks the middleware chain inside-out.
func (t *hookedTool) onIntent(ctx Ctx, intent Intent, state CallState) Intent {
	return t.hooks.OnIntent(ctx, t.inner, intent, state)
}

func (t *hookedTool) Execute(ctx Ctx, input json.RawMessage) (Result, error) {
	state := make(CallState)

	// 1. OnInput — may transform or short-circuit
	transformed, earlyResult, err := t.hooks.OnInput(ctx, t.inner, input, state)
	if err != nil {
		return nil, err
	}
	if earlyResult != nil {
		return earlyResult, nil
	}

	// 2. OnContext — may modify context (deadline, values, etc.)
	ctx, cleanup := t.hooks.OnContext(ctx, state)
	if cleanup != nil {
		defer cleanup()
	}

	// 3. Execute inner tool
	result, execErr := t.inner.Execute(ctx, transformed)

	// 4. OnResult — may transform/replace
	return t.hooks.OnResult(ctx, t.inner, transformed, result, execErr, state)
}

// Compile-time checks.
var (
	_ Middleware = MiddlewareFunc(nil)
	_ Tool       = (*hookedTool)(nil)
)
