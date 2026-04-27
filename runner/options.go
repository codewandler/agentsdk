package runner

import (
	"context"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

type Options struct {
	MaxSteps         int
	Tools            []tool.Tool
	ToolCtx          tool.Ctx
	ToolTimeout      time.Duration
	ToolExecutor     ToolExecutor
	ProviderIdentity conversation.ProviderIdentity
	RequestPreparer  RequestPreparer
	OnEvent          EventHandler
}

type Option func(*Options)

type RequestPrepareMeta struct {
	Step               int
	ProviderIdentity   conversation.ProviderIdentity
	NativeContinuation bool
}

type PreparedRequest struct {
	Request  conversation.Request
	Commit   func(context.Context) error
	Rollback func(context.Context)
}

type RequestPreparer func(context.Context, RequestPrepareMeta, conversation.Request) (PreparedRequest, error)

type ToolExecutor interface {
	ExecuteTool(ctx context.Context, call unified.ToolCall) unified.ToolResult
}

type ToolExecutorFunc func(ctx context.Context, call unified.ToolCall) unified.ToolResult

func (f ToolExecutorFunc) ExecuteTool(ctx context.Context, call unified.ToolCall) unified.ToolResult {
	return f(ctx, call)
}

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

func WithToolTimeout(timeout time.Duration) Option {
	return func(o *Options) {
		if timeout > 0 {
			o.ToolTimeout = timeout
		}
	}
}

func WithToolExecutor(executor ToolExecutor) Option {
	return func(o *Options) {
		o.ToolExecutor = executor
	}
}

func WithProviderIdentity(identity conversation.ProviderIdentity) Option {
	return func(o *Options) {
		o.ProviderIdentity = identity
	}
}

func WithRequestPreparer(preparer RequestPreparer) Option {
	return func(o *Options) {
		o.RequestPreparer = preparer
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
