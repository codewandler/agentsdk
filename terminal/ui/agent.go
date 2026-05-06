package ui

import (
	"io"
	"strconv"

	"github.com/codewandler/agentsdk/runner"
)

// AgentEventHandlerFactory adapts terminal event rendering to agent turn events.
// The agent remains responsible for recording usage and route state; this
// factory only renders events at the terminal boundary.
func AgentEventHandlerFactory(out io.Writer, opts ...EventDisplayOption) func(runner.EventHandlerContext) runner.EventHandler {
	return func(ctx runner.EventHandlerContext) runner.EventHandler {
		if out == nil {
			return nil
		}
		allOpts := append([]EventDisplayOption{
			WithTurnID(strconv.Itoa(ctx.TurnID)),
			WithSessionID(ctx.SessionID),
			WithFallbackModel(ctx.Model),
		}, opts...)
		return NewEventDisplay(out, allOpts...).Handler()
	}
}
