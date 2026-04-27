package runtime

import (
	"context"
	"errors"
	"fmt"

	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
)

func ResumeThreadEngine(ctx context.Context, store thread.Store, params thread.ResumeParams, client unified.Client, registry capability.Registry, opts ...Option) (*Engine, thread.Stored, error) {
	if store == nil {
		return nil, thread.Stored{}, fmt.Errorf("runtime: thread store is required")
	}
	if client == nil {
		return nil, thread.Stored{}, fmt.Errorf("runtime: client is required")
	}
	if registry == nil {
		return nil, thread.Stored{}, fmt.Errorf("runtime: capability registry is required")
	}
	threadRuntime, stored, err := ResumeThreadRuntime(ctx, store, params, registry)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	eventStore := conversation.NewThreadEventStore(store, threadRuntime.Live())
	sessionOptions := append(SessionOptions(opts...), conversation.WithStore(eventStore))
	session, err := conversation.Resume(ctx, eventStore, "", sessionOptions...)
	if err != nil {
		if !errors.Is(err, conversation.ErrNoEvents) {
			return nil, thread.Stored{}, err
		}
		session = conversation.New(sessionOptions...)
	}
	engineOptions := append([]Option(nil), opts...)
	engineOptions = append(engineOptions, WithSession(session), WithThreadRuntime(threadRuntime))
	engine, err := New(client, engineOptions...)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	return engine, stored, nil
}
