package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

type Agent struct {
	client           unified.Client
	session          *conversation.Session
	sessionOptions   []conversation.Option
	request          conversation.Request
	tools            []tool.Tool
	maxSteps         int
	toolCtx          tool.Ctx
	toolCtxFactory   func(context.Context) tool.Ctx
	toolTimeout      time.Duration
	toolExecutor     runner.ToolExecutor
	providerIdentity conversation.ProviderIdentity
	onEvent          runner.EventHandler
}

func New(client unified.Client, opts ...Option) (*Agent, error) {
	if client == nil {
		return nil, fmt.Errorf("runtime: client is required")
	}
	agent := &Agent{
		client:   client,
		maxSteps: 8,
		request:  conversation.Request{Stream: true},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(agent)
		}
	}
	if agent.session == nil {
		agent.session = conversation.New(agent.sessionOptions...)
	}
	return agent, nil
}

func Must(client unified.Client, opts ...Option) *Agent {
	agent, err := New(client, opts...)
	if err != nil {
		panic(err)
	}
	return agent
}

func (a *Agent) Session() *conversation.Session {
	if a == nil {
		return nil
	}
	return a.session
}

func (a *Agent) ResetSession(opts ...conversation.Option) *conversation.Session {
	if a == nil {
		return nil
	}
	sessionOptions := append([]conversation.Option(nil), a.sessionOptions...)
	sessionOptions = append(sessionOptions, opts...)
	a.session = conversation.New(sessionOptions...)
	return a.session
}

func (a *Agent) RunTurn(ctx context.Context, user string, opts ...TurnOption) (runner.Result, error) {
	if a == nil {
		return runner.Result{}, fmt.Errorf("runtime: agent is nil")
	}
	cfg := a.turnConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if user != "" {
		cfg.Request.Messages = append(cfg.Request.Messages, unified.Message{
			Role:    unified.RoleUser,
			Content: []unified.ContentPart{unified.TextPart{Text: user}},
		})
	}
	if cfg.ToolCtx == nil && cfg.ToolCtxFactory != nil {
		cfg.ToolCtx = cfg.ToolCtxFactory(ctx)
	}
	return runner.RunTurn(ctx, a.session, a.client, cfg.Request, cfg.runnerOptions()...)
}

func (a *Agent) turnConfig() TurnConfig {
	return TurnConfig{
		Request:          cloneRequest(a.request),
		MaxSteps:         a.maxSteps,
		Tools:            append([]tool.Tool(nil), a.tools...),
		ToolCtx:          a.toolCtx,
		ToolCtxFactory:   a.toolCtxFactory,
		ToolTimeout:      a.toolTimeout,
		ToolExecutor:     a.toolExecutor,
		ProviderIdentity: a.providerIdentity,
		OnEvent:          a.onEvent,
	}
}

func (c TurnConfig) runnerOptions() []runner.Option {
	opts := []runner.Option{
		runner.WithMaxSteps(c.MaxSteps),
		runner.WithTools(c.Tools),
		runner.WithToolCtx(c.ToolCtx),
		runner.WithToolTimeout(c.ToolTimeout),
		runner.WithProviderIdentity(c.ProviderIdentity),
		runner.WithEventHandler(c.OnEvent),
	}
	if c.ToolExecutor != nil {
		opts = append(opts, runner.WithToolExecutor(c.ToolExecutor))
	}
	return opts
}

func cloneRequest(req conversation.Request) conversation.Request {
	req.Stop = append([]string(nil), req.Stop...)
	req.Instructions = append([]unified.Instruction(nil), req.Instructions...)
	req.Tools = append([]unified.Tool(nil), req.Tools...)
	req.Messages = append([]unified.Message(nil), req.Messages...)
	req.Extensions = cloneExtensions(req.Extensions)
	return req
}

func cloneExtensions(ext unified.Extensions) unified.Extensions {
	var out unified.Extensions
	for _, key := range ext.Keys() {
		_ = out.SetRaw(key, ext.Raw(key))
	}
	return out
}
