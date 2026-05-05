package runtime

import (
	"context"
	"io"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/toolactivation"
)

type ToolContext struct {
	context.Context
	output    io.Writer
	emit      func(action.Event)
	workDir   string
	agentID   string
	sessionID string
	extra     map[string]any
}

type ToolContextOption func(*ToolContext)

func NewToolContext(ctx context.Context, opts ...ToolContextOption) *ToolContext {
	if ctx == nil {
		ctx = context.Background()
	}
	out := &ToolContext{Context: ctx, extra: map[string]any{}}
	for _, opt := range opts {
		if opt != nil {
			opt(out)
		}
	}
	return out
}

func WithToolWorkDir(workDir string) ToolContextOption {
	return func(c *ToolContext) { c.workDir = workDir }
}

func WithToolAgentID(agentID string) ToolContextOption {
	return func(c *ToolContext) { c.agentID = agentID }
}

func WithToolSessionID(sessionID string) ToolContextOption {
	return func(c *ToolContext) { c.sessionID = sessionID }
}

func WithToolExtra(key string, value any) ToolContextOption {
	return func(c *ToolContext) {
		if c.extra == nil {
			c.extra = map[string]any{}
		}
		c.extra[key] = value
	}
}

// WithToolOutput sets the streaming output writer for tool execution.
func WithToolOutput(w io.Writer) ToolContextOption {
	return func(c *ToolContext) { c.output = w }
}

// WithToolEmit sets the event emitter for tool execution.
func WithToolEmit(fn func(action.Event)) ToolContextOption {
	return func(c *ToolContext) { c.emit = fn }
}

func WithToolActivation(state toolactivation.State) ToolContextOption {
	return WithToolExtra(toolactivation.ContextKey, state)
}

func WithToolSkillActivation(state *skill.ActivationState) ToolContextOption {
	return WithToolExtra(skill.ContextKey, state)
}

func (c *ToolContext) WorkDir() string { return c.workDir }
func (c *ToolContext) AgentID() string { return c.agentID }
func (c *ToolContext) SessionID() string {
	return c.sessionID
}
func (c *ToolContext) Extra() map[string]any {
	if c.extra == nil {
		c.extra = map[string]any{}
	}
	return c.extra
}

func (c *ToolContext) Output() io.Writer {
	if c.output == nil {
		return io.Discard
	}
	return c.output
}

func (c *ToolContext) Emit(event action.Event) {
	if c.emit != nil {
		c.emit(event)
	}
}

func (c *ToolContext) Deadline() (time.Time, bool) {
	if c.Context == nil {
		return time.Time{}, false
	}
	return c.Context.Deadline()
}

func (c *ToolContext) Done() <-chan struct{} {
	if c.Context == nil {
		return nil
	}
	return c.Context.Done()
}

func (c *ToolContext) Err() error {
	if c.Context == nil {
		return nil
	}
	return c.Context.Err()
}

func (c *ToolContext) Value(key any) any {
	if c.Context == nil {
		return nil
	}
	return c.Context.Value(key)
}
