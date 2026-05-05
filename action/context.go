package action

import (
	"context"
	"io"
)

// CtxOption configures a context created by NewCtx.
type CtxOption func(*baseCtx)

// WithOutput sets the streaming output writer for the context.
// When not set, Output() returns io.Discard.
func WithOutput(w io.Writer) CtxOption {
	return func(c *baseCtx) {
		if w != nil {
			c.output = w
		}
	}
}

// WithEmit sets the event emitter for the context.
// When not set, Emit() is a no-op.
func WithEmit(fn func(Event)) CtxOption {
	return func(c *baseCtx) {
		if fn != nil {
			c.emit = fn
		}
	}
}

// NewCtx creates an action.Ctx from a context.Context and optional
// configuration. This is the standard way to construct contexts for action
// and tool execution.
func NewCtx(ctx context.Context, opts ...CtxOption) Ctx {
	if ctx == nil {
		ctx = context.Background()
	}
	c := &baseCtx{Context: ctx}
	for _, opt := range opts {
		if opt != nil {
			opt(c)
		}
	}
	return c
}

// BaseCtx is a minimal Ctx implementation that embeds context.Context and
// provides no-op defaults for Output and Emit. Embed it in test stubs or
// simple context types to satisfy the Ctx interface without boilerplate.
type BaseCtx struct {
	context.Context
}

func (BaseCtx) Output() io.Writer { return io.Discard }
func (BaseCtx) Emit(Event)        {}

// baseCtx is the unexported implementation returned by NewCtx.
type baseCtx struct {
	context.Context
	output io.Writer
	emit   func(Event)
}

func (c *baseCtx) Output() io.Writer {
	if c.output == nil {
		return io.Discard
	}
	return c.output
}

func (c *baseCtx) Emit(event Event) {
	if c.emit != nil {
		c.emit(event)
	}
}
