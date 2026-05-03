package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
)

// CreateThreadEngine creates a new thread, initializes its ThreadRuntime, and
// returns a runtime Engine backed by that thread's event store.
func CreateThreadEngine(ctx context.Context, store thread.Store, params thread.CreateParams, client unified.Client, registry capability.Registry, opts ...Option) (*Engine, thread.Stored, error) {
	if err := validateThreadEngineInputs(store, client, registry); err != nil {
		return nil, thread.Stored{}, err
	}
	live, err := store.Create(ctx, params)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	runtimeOpts, err := threadRuntimeOptionsFromEngineOptions(params.Source, opts...)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	threadRuntime, err := NewThreadRuntime(live, registry, runtimeOpts...)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		return nil, thread.Stored{}, err
	}
	engine, err := newThreadEngine(ctx, store, threadRuntime, client, opts...)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	return engine, stored, nil
}

// OpenThreadEngine resumes params.ID when it exists, otherwise creates it. When
// params.ID is empty it always creates a new thread.
func OpenThreadEngine(ctx context.Context, store thread.Store, params thread.CreateParams, client unified.Client, registry capability.Registry, opts ...Option) (*Engine, thread.Stored, error) {
	if err := validateThreadEngineInputs(store, client, registry); err != nil {
		return nil, thread.Stored{}, err
	}
	if params.ID == "" {
		return CreateThreadEngine(ctx, store, params, client, registry, opts...)
	}
	if _, err := store.Read(ctx, thread.ReadParams{ID: params.ID}); err != nil {
		if errors.Is(err, thread.ErrNotFound) {
			return CreateThreadEngine(ctx, store, params, client, registry, opts...)
		}
		return nil, thread.Stored{}, err
	}
	return ResumeThreadEngine(ctx, store, thread.ResumeParams{
		ID:       params.ID,
		BranchID: params.BranchID,
		Source:   params.Source,
	}, client, registry, opts...)
}

// ResumeThreadEngine resumes an existing thread branch and returns an Engine
// whose session, capabilities, and context diffs are rebuilt from thread events.
func ResumeThreadEngine(ctx context.Context, store thread.Store, params thread.ResumeParams, client unified.Client, registry capability.Registry, opts ...Option) (*Engine, thread.Stored, error) {
	if err := validateThreadEngineInputs(store, client, registry); err != nil {
		return nil, thread.Stored{}, err
	}
	runtimeOpts, err := threadRuntimeOptionsFromEngineOptions(thread.EventSource{}, opts...)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	threadRuntime, stored, err := ResumeThreadRuntime(ctx, store, params, registry, runtimeOpts...)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	engine, err := newThreadEngine(ctx, store, threadRuntime, client, opts...)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	return engine, stored, nil
}

func newThreadEngine(ctx context.Context, store thread.Store, threadRuntime *ThreadRuntime, client unified.Client, opts ...Option) (*Engine, error) {
	if threadRuntime == nil || threadRuntime.Live() == nil {
		return nil, fmt.Errorf("runtime: thread runtime is required")
	}
	historyOptions := append(historyOptions(opts...), WithHistoryLiveThread(threadRuntime.Live()))
	history, err := ResumeHistoryFromThread(ctx, store, threadRuntime.Live(), historyOptions...)
	if err != nil {
		return nil, err
	}
	engineOptions := append([]Option(nil), opts...)
	engineOptions = append(engineOptions, clearThreadContextOptions(), WithHistory(history), WithThreadRuntime(threadRuntime))
	engine, err := New(client, engineOptions...)
	if err != nil {
		return nil, err
	}
	return engine, nil
}

func validateThreadEngineInputs(store thread.Store, client unified.Client, registry capability.Registry) error {
	if store == nil {
		return fmt.Errorf("runtime: thread store is required")
	}
	if client == nil {
		return fmt.Errorf("runtime: client is required")
	}
	if registry == nil {
		return fmt.Errorf("runtime: capability registry is required")
	}
	return nil
}

func clearThreadContextOptions() Option {
	return func(e *Engine) {
		e.threadContexts = nil
		e.contextProviders = nil
	}
}

func threadRuntimeOptionsFromEngineOptions(source thread.EventSource, opts ...Option) ([]ThreadRuntimeOption, error) {
	engine := &Engine{}
	for _, opt := range opts {
		if opt != nil {
			opt(engine)
		}
	}
	runtimeOpts := threadRuntimeSourceOptions(source)
	manager := engine.threadContexts
	if len(engine.contextProviders) > 0 {
		if manager == nil {
			var err error
			manager, err = agentcontext.NewManager(engine.contextProviders...)
			if err != nil {
				return nil, err
			}
		} else if err := manager.Register(engine.contextProviders...); err != nil {
			return nil, err
		}
	}
	if manager != nil {
		runtimeOpts = append(runtimeOpts, WithContextManager(manager))
	}
	return runtimeOpts, nil
}

func threadRuntimeSourceOptions(source thread.EventSource) []ThreadRuntimeOption {
	if source.Type == "" && source.ID == "" && source.SessionID == "" {
		return nil
	}
	return []ThreadRuntimeOption{WithThreadRuntimeSource(source)}
}
