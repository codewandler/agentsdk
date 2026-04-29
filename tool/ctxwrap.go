package tool

import (
	"context"
	"time"
)

// WrapCtx returns a new Ctx that uses newCtx for deadline/cancellation/values
// but preserves all Ctx metadata (WorkDir, AgentID, SessionID, Extra).
//
// This is useful when a middleware needs to set a tighter deadline or inject
// context values without losing the tool execution metadata.
func WrapCtx(base Ctx, newCtx context.Context) Ctx {
	return &wrappedCtx{Ctx: base, ctx: newCtx}
}

type wrappedCtx struct {
	Ctx
	ctx context.Context
}

func (c *wrappedCtx) Deadline() (time.Time, bool) { return c.ctx.Deadline() }
func (c *wrappedCtx) Done() <-chan struct{}        { return c.ctx.Done() }
func (c *wrappedCtx) Err() error                   { return c.ctx.Err() }
func (c *wrappedCtx) Value(key any) any            { return c.ctx.Value(key) }

// Compile-time check.
var _ Ctx = (*wrappedCtx)(nil)
