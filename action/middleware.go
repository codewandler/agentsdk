package action

// Middleware wraps an action handler.
type Middleware interface {
	Wrap(Handler) Handler
}

// MiddlewareFunc adapts a function into Middleware.
type MiddlewareFunc func(Handler) Handler

func (f MiddlewareFunc) Wrap(next Handler) Handler { return f(next) }

// ApplyHandler applies middleware to h. First supplied middleware is innermost;
// last supplied middleware is outermost and runs first.
func ApplyHandler(h Handler, middleware ...Middleware) Handler {
	for _, m := range middleware {
		if m == nil {
			continue
		}
		h = m.Wrap(h)
	}
	return h
}

// Apply returns an action with middleware applied to its Execute path.
func Apply(a Action, middleware ...Middleware) Action {
	if a == nil {
		return nil
	}
	h := ApplyHandler(a.Execute, middleware...)
	return New(a.Spec(), h)
}
