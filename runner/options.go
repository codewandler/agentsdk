package runner

import (
	"context"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/tool"
)

type Options struct {
	MaxSteps         int
	Tools            []tool.Tool
	ToolCtx          tool.Ctx
	ProviderIdentity conversation.ProviderIdentity
	OnEvent          EventHandler
}

type Option func(*Options)

func WithMaxSteps(max int) Option {
	return func(o *Options) {
		if max > 0 {
			o.MaxSteps = max
		}
	}
}

func WithTools(tools []tool.Tool) Option {
	return func(o *Options) {
		o.Tools = append([]tool.Tool(nil), tools...)
	}
}

func WithToolCtx(ctx tool.Ctx) Option {
	return func(o *Options) {
		o.ToolCtx = ctx
	}
}

func WithProviderIdentity(identity conversation.ProviderIdentity) Option {
	return func(o *Options) {
		o.ProviderIdentity = identity
	}
}

func WithEventHandler(handler EventHandler) Option {
	return func(o *Options) {
		o.OnEvent = handler
	}
}

func applyOptions(opts []Option) Options {
	out := Options{MaxSteps: 8}
	for _, opt := range opts {
		if opt != nil {
			opt(&out)
		}
	}
	return out
}

type basicToolCtx struct {
	context.Context
	workDir   string
	agentID   string
	sessionID string
	extra     map[string]any
}

func (c *basicToolCtx) WorkDir() string       { return c.workDir }
func (c *basicToolCtx) AgentID() string       { return c.agentID }
func (c *basicToolCtx) SessionID() string     { return c.sessionID }
func (c *basicToolCtx) Extra() map[string]any { return c.extra }
