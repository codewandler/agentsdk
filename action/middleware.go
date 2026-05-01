package action

// Middleware wraps an Action, intercepting its execution lifecycle.
type Middleware interface {
	Wrap(Action) Action
}

// MiddlewareFunc adapts a function into Middleware.
type MiddlewareFunc func(Action) Action

func (f MiddlewareFunc) Wrap(next Action) Action { return f(next) }

// HandlerMiddlewareFunc adapts the older handler-wrapper shape into Middleware.
type HandlerMiddlewareFunc func(Handler) Handler

func (f HandlerMiddlewareFunc) Wrap(next Action) Action {
	if next == nil {
		return nil
	}
	return New(next.Spec(), f(next.Execute))
}

// CallState is per-call mutable state shared across hook phases within one
// middleware. Each middleware gets its own CallState per Execute call.
type CallState map[string]any

// Hooks defines action middleware injection points. Hooks operate on Go-native
// action inputs and results; projection-specific concerns such as LLM guidance
// remain outside the action package.
type Hooks interface {
	// OnSpec receives the inner action spec at wrap time. Return a modified spec
	// to expose metadata changes, or the input spec unchanged to pass through.
	OnSpec(inner Action, spec Spec) Spec

	// OnInput runs before the inner action executes. It may transform the input
	// and continue, or short-circuit by returning shortCircuit=true with a Result.
	OnInput(ctx Ctx, inner Action, input any, state CallState) (nextInput any, result Result, shortCircuit bool)

	// OnContext runs after OnInput when execution continues. It may return a
	// modified context and a cleanup function deferred until after OnResult.
	OnContext(ctx Ctx, state CallState) (Ctx, func())

	// OnIntent runs during intent extraction. It may amend, enrich, or replace the
	// declared intent without executing the action.
	OnIntent(ctx Ctx, inner Action, intent Intent, state CallState) Intent

	// OnResult runs after the inner action returns or after OnInput short-circuits.
	// It may inspect, transform, or replace the result.
	OnResult(ctx Ctx, inner Action, input any, result Result, state CallState) Result
}

// HooksBase provides no-op defaults for action hooks.
type HooksBase struct{}

func (HooksBase) OnSpec(_ Action, spec Spec) Spec { return spec }
func (HooksBase) OnInput(_ Ctx, _ Action, input any, _ CallState) (any, Result, bool) {
	return input, Result{}, false
}
func (HooksBase) OnContext(ctx Ctx, _ CallState) (Ctx, func()) { return ctx, nil }
func (HooksBase) OnIntent(_ Ctx, _ Action, intent Intent, _ CallState) Intent {
	return intent
}
func (HooksBase) OnResult(_ Ctx, _ Action, _ any, result Result, _ CallState) Result {
	return result
}

// HooksMiddleware creates Middleware from Hooks.
func HooksMiddleware(hooks Hooks) Middleware {
	return MiddlewareFunc(func(inner Action) Action {
		if inner == nil {
			return nil
		}
		if hooks == nil {
			return inner
		}
		return &hookedAction{
			inner: inner,
			hooks: hooks,
			spec:  hooks.OnSpec(inner, inner.Spec()),
		}
	})
}

type hookedAction struct {
	inner Action
	hooks Hooks
	spec  Spec
}

func (a *hookedAction) Spec() Spec { return a.spec }

// Unwrap exposes the inner action for introspection.
func (a *hookedAction) Unwrap() Action { return a.inner }

func (a *hookedAction) Execute(ctx Ctx, input any) Result {
	state := make(CallState)

	transformed, result, shortCircuit := a.hooks.OnInput(ctx, a.inner, input, state)
	if shortCircuit {
		return a.hooks.OnResult(ctx, a.inner, transformed, result, state)
	}

	ctx, cleanup := a.hooks.OnContext(ctx, state)
	if cleanup != nil {
		defer cleanup()
	}

	result = a.inner.Execute(ctx, transformed)
	return a.hooks.OnResult(ctx, a.inner, transformed, result, state)
}

// Unwrap returns the immediate inner action if a is wrapped.
func Unwrap(a Action) Action {
	if w, ok := a.(interface{ Unwrap() Action }); ok {
		return w.Unwrap()
	}
	return nil
}

// Innermost returns the deepest unwrapped action by repeatedly calling Unwrap.
func Innermost(a Action) Action {
	for {
		inner := Unwrap(a)
		if inner == nil {
			return a
		}
		a = inner
	}
}

func hookLayers(a Action) []*hookedAction {
	var layers []*hookedAction
	cur := a
	for {
		if ht, ok := cur.(*hookedAction); ok {
			layers = append(layers, ht)
			cur = ht.inner
			continue
		}
		break
	}
	for i, j := 0, len(layers)-1; i < j; i, j = i+1, j-1 {
		layers[i], layers[j] = layers[j], layers[i]
	}
	return layers
}

// Apply applies middleware to a. First supplied middleware is innermost; last
// supplied middleware is outermost and runs first.
func Apply(a Action, middleware ...Middleware) Action {
	for _, m := range middleware {
		if m == nil || a == nil {
			continue
		}
		a = m.Wrap(a)
	}
	return a
}

var (
	_ Middleware = MiddlewareFunc(nil)
	_ Middleware = HandlerMiddlewareFunc(nil)
	_ Action     = (*hookedAction)(nil)
)
