package runtime

import (
	"context"
	"fmt"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

// Engine is the low-level execution engine for model/tool turns over history.
type Engine struct {
	client           unified.Client
	history          *History
	historyOptions   []HistoryOption
	request          conversation.Request
	tools            []tool.Tool
	maxSteps         int
	toolCtx          tool.Ctx
	toolCtxFactory   func(context.Context) tool.Ctx
	toolTimeout      time.Duration
	toolExecutor     runner.ToolExecutor
	providerIdentity conversation.ProviderIdentity
	requestPreparer  runner.RequestPreparer
	threadRuntime    *ThreadRuntime
	threadContexts   *agentcontext.Manager
	contextProviders []agentcontext.Provider
	capabilitySpecs  []capability.AttachSpec
	onEvent          runner.EventHandler
	onRequest        runner.RequestObserver
}

func New(client unified.Client, opts ...Option) (*Engine, error) {
	if client == nil {
		return nil, fmt.Errorf("runtime: client is required")
	}
	engine := &Engine{
		client:   client,
		maxSteps: 8,
		request:  conversation.Request{Stream: true},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(engine)
		}
	}
	if err := engine.applyThreadContextOptions(); err != nil {
		return nil, err
	}
	if engine.history == nil {
		engine.history = NewHistory(engine.historyOptions...)
	}
	if engine.threadRuntime != nil && engine.threadRuntime.Live() != nil && engine.history.live == nil {
		engine.history.live = engine.threadRuntime.Live()
		engine.history.branch = conversation.BranchID(engine.threadRuntime.Live().BranchID())
	}
	return engine, nil
}

func HistoryOptions(opts ...Option) []HistoryOption {
	engine := &Engine{
		maxSteps: 8,
		request:  conversation.Request{Stream: true},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(engine)
		}
	}
	return append([]HistoryOption(nil), engine.historyOptions...)
}

func (e *Engine) History() *History {
	if e == nil {
		return nil
	}
	return e.history
}

func (e *Engine) ThreadRuntime() *ThreadRuntime {
	if e == nil {
		return nil
	}
	return e.threadRuntime
}

// RegisterContextProviders adds providers to the engine's active context manager
// for future turns.
func (e *Engine) RegisterContextProviders(providers ...agentcontext.Provider) error {
	if e == nil {
		return fmt.Errorf("runtime: engine is nil")
	}
	if len(providers) == 0 {
		return nil
	}
	if e.threadRuntime != nil {
		manager := e.threadRuntime.ContextManager()
		if manager == nil {
			return fmt.Errorf("runtime: thread runtime has no context manager")
		}
		return manager.Register(providers...)
	}
	if e.threadContexts == nil {
		manager, err := agentcontext.NewManager()
		if err != nil {
			return err
		}
		e.threadContexts = manager
	}
	return e.threadContexts.Register(providers...)
}

// ContextState returns a human-readable summary of the last committed context
// manager render state.
func (e *Engine) ContextState() string {
	if e == nil {
		return "context: unavailable"
	}
	if e.threadRuntime != nil {
		return e.threadRuntime.ContextState()
	}
	if e.threadContexts != nil {
		return e.threadContexts.LastRenderState()
	}
	return "context: unavailable"
}

func (e *Engine) applyThreadContextOptions() error {
	if e == nil {
		return nil
	}
	if e.threadRuntime != nil {
		if e.threadContexts != nil && e.threadContexts != e.threadRuntime.ContextManager() {
			return fmt.Errorf("runtime: thread context manager cannot replace an existing thread runtime context manager")
		}
		if len(e.contextProviders) == 0 {
			return nil
		}
		manager := e.threadRuntime.ContextManager()
		if manager == nil {
			return fmt.Errorf("runtime: thread runtime has no context manager")
		}
		return manager.Register(e.contextProviders...)
	}
	if e.threadContexts == nil && len(e.contextProviders) > 0 {
		manager, err := agentcontext.NewManager()
		if err != nil {
			return err
		}
		e.threadContexts = manager
	}
	if e.threadContexts == nil || len(e.contextProviders) == 0 {
		return nil
	}
	return e.threadContexts.Register(e.contextProviders...)
}

func (e *Engine) ResetHistory(opts ...HistoryOption) *History {
	if e == nil {
		return nil
	}
	historyOptions := append([]HistoryOption(nil), e.historyOptions...)
	historyOptions = append(historyOptions, opts...)
	e.history = NewHistory(historyOptions...)
	return e.history
}

func (e *Engine) Compact(ctx context.Context, summary string, replaces ...conversation.NodeID) (conversation.NodeID, error) {
	if e == nil {
		return "", fmt.Errorf("runtime: engine is nil")
	}
	if e.history == nil {
		return "", fmt.Errorf("runtime: history is required")
	}
	if e.threadRuntime != nil {
		return e.threadRuntime.Compact(ctx, e.history, summary, replaces...)
	}
	return e.history.CompactContext(ctx, summary, replaces...)
}

func (e *Engine) RunTurn(ctx context.Context, user string, opts ...TurnOption) (runner.Result, error) {
	if e == nil {
		return runner.Result{}, fmt.Errorf("runtime: engine is nil")
	}
	cfg := e.turnConfig()
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
	if e.threadRuntime != nil {
		if len(e.capabilitySpecs) > 0 {
			if err := e.threadRuntime.EnsureCapabilities(ctx, e.capabilitySpecs...); err != nil {
				return runner.Result{}, err
			}
		}
		if err := cfg.addThreadRuntime(e.threadRuntime); err != nil {
			return runner.Result{}, err
		}
	} else if e.threadContexts != nil {
		cfg.addContextManager(e.threadContexts)
	}
	return runner.RunTurn(ctx, e.history, e.client, cfg.Request, cfg.runnerOptions()...)
}

func (e *Engine) turnConfig() TurnConfig {
	return TurnConfig{
		Request:          cloneRequest(e.request),
		MaxSteps:         e.maxSteps,
		Tools:            append([]tool.Tool(nil), e.tools...),
		ToolCtx:          e.toolCtx,
		ToolCtxFactory:   e.toolCtxFactory,
		ToolTimeout:      e.toolTimeout,
		ToolExecutor:     e.toolExecutor,
		ProviderIdentity: e.providerIdentity,
		RequestPreparer:  e.requestPreparer,
		OnEvent:          e.onEvent,
		OnRequest:        e.onRequest,
	}
}

func (c TurnConfig) runnerOptions() []runner.Option {
	opts := []runner.Option{
		runner.WithMaxSteps(c.MaxSteps),
		runner.WithTools(c.Tools),
		runner.WithToolCtx(c.ToolCtx),
		runner.WithToolTimeout(c.ToolTimeout),
		runner.WithProviderIdentity(c.ProviderIdentity),
		runner.WithRequestPreparer(c.RequestPreparer),
		runner.WithEventHandler(c.OnEvent),
		runner.WithRequestObserver(c.OnRequest),
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
	req.Items = append([]conversation.Item(nil), req.Items...)
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
