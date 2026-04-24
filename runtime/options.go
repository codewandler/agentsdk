package runtime

import (
	"context"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

type Option func(*Agent)

func WithSession(session *conversation.Session) Option {
	return func(a *Agent) { a.session = session }
}

func WithSessionOptions(opts ...conversation.Option) Option {
	return func(a *Agent) { a.sessionOptions = append(a.sessionOptions, opts...) }
}

func WithModel(model string) Option {
	return func(a *Agent) {
		a.request.Model = model
		a.sessionOptions = append(a.sessionOptions, conversation.WithModel(model))
	}
}

func WithMaxOutputTokens(max int) Option {
	return func(a *Agent) {
		a.request.MaxOutputTokens = &max
		a.sessionOptions = append(a.sessionOptions, conversation.WithMaxOutputTokens(max))
	}
}

func WithTemperature(value float64) Option {
	return func(a *Agent) {
		a.request.Temperature = &value
		a.sessionOptions = append(a.sessionOptions, conversation.WithTemperature(value))
	}
}

func WithSystem(text string) Option {
	return func(a *Agent) { a.sessionOptions = append(a.sessionOptions, conversation.WithSystem(text)) }
}

func WithReasoning(reasoning unified.ReasoningConfig) Option {
	return func(a *Agent) {
		a.request.Reasoning = &reasoning
		a.sessionOptions = append(a.sessionOptions, conversation.WithReasoning(reasoning))
	}
}

func WithTools(tools []tool.Tool) Option {
	return func(a *Agent) {
		a.tools = append([]tool.Tool(nil), tools...)
		unifiedTools := tool.UnifiedToolsFrom(tools)
		a.request.Tools = unifiedTools
		a.sessionOptions = append(a.sessionOptions, conversation.WithTools(unifiedTools))
	}
}

func WithToolChoice(choice unified.ToolChoice) Option {
	return func(a *Agent) {
		a.request.ToolChoice = &choice
		a.sessionOptions = append(a.sessionOptions, conversation.WithToolChoice(choice))
	}
}

func WithStream(stream bool) Option {
	return func(a *Agent) { a.request.Stream = stream }
}

func WithRequestDefaults(req conversation.Request) Option {
	return func(a *Agent) { a.request = cloneRequest(req) }
}

func WithMaxSteps(max int) Option {
	return func(a *Agent) {
		if max > 0 {
			a.maxSteps = max
		}
	}
}

func WithToolCtx(ctx tool.Ctx) Option {
	return func(a *Agent) { a.toolCtx = ctx }
}

func WithToolContextFactory(factory func(context.Context) tool.Ctx) Option {
	return func(a *Agent) { a.toolCtxFactory = factory }
}

func WithToolTimeout(timeout time.Duration) Option {
	return func(a *Agent) {
		if timeout > 0 {
			a.toolTimeout = timeout
		}
	}
}

func WithToolExecutor(executor runner.ToolExecutor) Option {
	return func(a *Agent) { a.toolExecutor = executor }
}

func WithProviderIdentity(identity conversation.ProviderIdentity) Option {
	return func(a *Agent) { a.providerIdentity = identity }
}

func WithEventHandler(handler runner.EventHandler) Option {
	return func(a *Agent) { a.onEvent = handler }
}

type TurnOption func(*TurnConfig)

type TurnConfig struct {
	Request          conversation.Request
	MaxSteps         int
	Tools            []tool.Tool
	ToolCtx          tool.Ctx
	ToolTimeout      time.Duration
	ToolExecutor     runner.ToolExecutor
	ToolCtxFactory   func(context.Context) tool.Ctx
	ProviderIdentity conversation.ProviderIdentity
	OnEvent          runner.EventHandler
}

func WithTurnRequest(req conversation.Request) TurnOption {
	return func(c *TurnConfig) { c.Request = req }
}

func WithTurnTools(tools []tool.Tool) TurnOption {
	return func(c *TurnConfig) {
		c.Tools = append([]tool.Tool(nil), tools...)
		c.Request.Tools = tool.UnifiedToolsFrom(tools)
	}
}

func WithTurnToolCtx(ctx tool.Ctx) TurnOption {
	return func(c *TurnConfig) { c.ToolCtx = ctx }
}

func WithTurnToolContextFactory(factory func(context.Context) tool.Ctx) TurnOption {
	return func(c *TurnConfig) { c.ToolCtxFactory = factory }
}

func WithTurnEventHandler(handler runner.EventHandler) TurnOption {
	return func(c *TurnConfig) { c.OnEvent = handler }
}

func WithTurnProviderIdentity(identity conversation.ProviderIdentity) TurnOption {
	return func(c *TurnConfig) { c.ProviderIdentity = identity }
}

func WithTurnMaxSteps(max int) TurnOption {
	return func(c *TurnConfig) {
		if max > 0 {
			c.MaxSteps = max
		}
	}
}
