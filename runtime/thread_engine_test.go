package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestCreateThreadEngineCreatesDurableEngine(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	client := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "created"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
	}}}

	engine, stored, err := CreateThreadEngine(ctx, store, thread.CreateParams{
		ID:       "thread_engine_create",
		Metadata: map[string]string{"title": "created"},
		Source:   thread.EventSource{Type: "test", SessionID: "session_create"},
	}, client, registry, WithModel("model-create"))
	require.NoError(t, err)
	require.Equal(t, thread.ID("thread_engine_create"), stored.ID)
	require.Equal(t, "created", stored.Metadata["title"])
	require.NotNil(t, engine.Session())
	require.NotNil(t, engine.ThreadRuntime())

	_, err = engine.RunTurn(ctx, "hello")
	require.NoError(t, err)
	require.Equal(t, "model-create", client.requests[0].Model)

	after, err := store.Read(ctx, thread.ReadParams{ID: "thread_engine_create"})
	require.NoError(t, err)
	requireEventCountRuntime(t, after.Events, thread.EventThreadCreated, 1)
	requireEventCountRuntime(t, after.Events, conversation.EventConversationUserMessage, 1)
	requireEventCountRuntime(t, after.Events, conversation.EventConversationAssistantMessage, 1)
}

func TestOpenThreadEngineCreatesMissingThreadAndResumesExistingThread(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	createClient := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "opened"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
	}}}

	created, stored, err := OpenThreadEngine(ctx, store, thread.CreateParams{
		ID:       "thread_engine_open",
		Metadata: map[string]string{"title": "open"},
	}, createClient, registry, WithModel("model-open"))
	require.NoError(t, err)
	require.Equal(t, thread.ID("thread_engine_open"), stored.ID)
	_, err = created.RunTurn(ctx, "first")
	require.NoError(t, err)

	resumed, stored, err := OpenThreadEngine(ctx, store, thread.CreateParams{
		ID:       "thread_engine_open",
		Metadata: map[string]string{"title": "ignored"},
	}, &fakeClient{}, registry, WithModel("model-open"))
	require.NoError(t, err)
	require.Equal(t, "open", stored.Metadata["title"])
	messages, err := resumed.Session().Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	requireTextMessage(t, messages[0], "first")
	requireTextMessage(t, messages[1], "opened")

	after, err := store.Read(ctx, thread.ReadParams{ID: "thread_engine_open"})
	require.NoError(t, err)
	requireEventCountRuntime(t, after.Events, thread.EventThreadCreated, 1)
	requireEventCountRuntime(t, after.Events, conversation.EventConversationUserMessage, 1)
	requireEventCountRuntime(t, after.Events, conversation.EventConversationAssistantMessage, 1)
}

func TestOpenThreadEngineUsesContextProvidersAndRestoresRecords(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	provider := runtimeContextProvider{
		key: "env",
		fragments: []agentcontext.ContextFragment{{
			Key:       "env/pwd",
			Content:   "pwd: /repo",
			Authority: agentcontext.AuthorityUser,
		}},
	}
	client := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "ok"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
	}}}

	engine, _, err := OpenThreadEngine(ctx, store, thread.CreateParams{ID: "thread_engine_context_provider"}, client, registry,
		WithContextProviders(provider),
	)
	require.NoError(t, err)
	_, err = engine.RunTurn(ctx, "hello")
	require.NoError(t, err)
	requireMessageContaining(t, client.requests[0], "pwd: /repo")

	stored, err := store.Read(ctx, thread.ReadParams{ID: "thread_engine_context_provider"})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, EventContextRenderCommitted, 1)

	resumed, _, err := ResumeThreadEngine(ctx, store, thread.ResumeParams{ID: "thread_engine_context_provider"}, &fakeClient{}, registry,
		WithContextProviders(provider),
	)
	require.NoError(t, err)
	records := resumed.ThreadRuntime().ContextManager().Records()
	require.Contains(t, records, agentcontext.ProviderKey("env"))
	require.NotEmpty(t, records["env"].Fragments)
}

func TestCreateThreadEngineUsesProvidedContextManager(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	manager, err := agentcontext.NewManager(runtimeContextProvider{
		key: "policy",
		fragments: []agentcontext.ContextFragment{{
			Key:       "policy/rule",
			Content:   "custom policy context",
			Authority: agentcontext.AuthorityDeveloper,
		}},
	})
	require.NoError(t, err)
	client := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "ok"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
	}}}

	engine, _, err := CreateThreadEngine(ctx, store, thread.CreateParams{ID: "thread_engine_context_manager"}, client, registry,
		WithThreadContextManager(manager),
	)
	require.NoError(t, err)
	_, err = engine.RunTurn(ctx, "hello")
	require.NoError(t, err)
	requireInstructionContaining(t, client.requests[0], "custom policy context")
}

func TestNewRegistersContextProvidersOnExistingThreadRuntime(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_existing_runtime_context"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	provider := runtimeContextProvider{
		key: "env",
		fragments: []agentcontext.ContextFragment{{
			Key:       "env/shell",
			Content:   "shell: bash",
			Authority: agentcontext.AuthorityUser,
		}},
	}
	client := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "ok"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
	}}}

	engine, err := New(client, WithThreadRuntime(threadRuntime), WithContextProviders(provider))
	require.NoError(t, err)
	_, err = engine.RunTurn(ctx, "hello")
	require.NoError(t, err)
	requireMessageContaining(t, client.requests[0], "shell: bash")
}

func TestOpenThreadEngineReturnsMissingBranchErrorForExistingThread(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	_, err := store.Create(ctx, thread.CreateParams{ID: "thread_engine_missing_branch"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry()
	require.NoError(t, err)

	_, _, err = OpenThreadEngine(ctx, store, thread.CreateParams{
		ID:       "thread_engine_missing_branch",
		BranchID: "missing",
	}, &fakeClient{}, registry)
	require.ErrorIs(t, err, thread.ErrNotFound)
}

func TestThreadStoreTypedErrors(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()

	_, err := store.Resume(ctx, thread.ResumeParams{ID: "missing"})
	require.ErrorIs(t, err, thread.ErrNotFound)
	_, err = store.Create(ctx, thread.CreateParams{ID: "thread_errors"})
	require.NoError(t, err)
	_, err = store.Create(ctx, thread.CreateParams{ID: "thread_errors"})
	require.ErrorIs(t, err, thread.ErrAlreadyExists)
	require.True(t, errors.Is(err, thread.ErrAlreadyExists))
}

func TestResumeThreadEngineCreatesSessionWhenConversationMissing(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_engine_new"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	client := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "ok"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
	}}}

	engine, stored, err := ResumeThreadEngine(ctx, store, thread.ResumeParams{ID: live.ID()}, client, registry, WithModel("model-a"))
	require.NoError(t, err)
	require.Equal(t, live.ID(), stored.ID)
	require.NotNil(t, engine.Session())
	require.NotNil(t, engine.ThreadRuntime())

	_, err = engine.RunTurn(ctx, "hello")
	require.NoError(t, err)
	require.Len(t, client.requests, 1)
	require.Equal(t, "model-a", client.requests[0].Model)

	after, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, after.Events, conversation.EventConversationUserMessage, 1)
	requireEventCountRuntime(t, after.Events, conversation.EventConversationAssistantMessage, 1)

	resumed, _, err := ResumeThreadEngine(ctx, store, thread.ResumeParams{ID: live.ID()}, &fakeClient{}, registry, WithModel("model-a"))
	require.NoError(t, err)
	messages, err := resumed.Session().Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	requireTextMessage(t, messages[0], "hello")
	requireTextMessage(t, messages[1], "ok")
}

func TestResumeThreadEngineRestoresConversationCapabilitiesAndContextRecords(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_engine_resume"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	_, err = threadRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)
	applyPlanActions(t, ctx, threadRuntime, `{"actions":[
		{"action":"create_plan","plan":{"id":"plan_1","title":"Resume engine"}},
		{"action":"add_step","step":{"id":"step_1","title":"Restore all state","status":"in_progress"}}
	]}`)
	_, err = threadRuntime.renderAndCommitContext(ctx, agentcontext.BuildRequest{
		ThreadID: string(live.ID()),
		BranchID: string(live.BranchID()),
		Reason:   agentcontext.RenderTurn,
	})
	require.NoError(t, err)

	session := conversation.New(conversation.WithStore(conversation.NewThreadEventStore(store, live)))
	_, err = session.AddUser("previous")
	require.NoError(t, err)

	engine, _, err := ResumeThreadEngine(ctx, store, thread.ResumeParams{ID: live.ID()}, &fakeClient{}, registry)
	require.NoError(t, err)
	requireToolSpec(t, unified.Request{Tools: tool.UnifiedToolsFrom(engine.ThreadRuntime().Tools())}, "plan")
	records := engine.ThreadRuntime().ContextManager().Records()
	require.Contains(t, records, agentcontext.ProviderKey("capabilities"))
	require.NotEmpty(t, records["capabilities"].Fragments)
	messages, err := engine.Session().Messages()
	require.NoError(t, err)
	require.Len(t, messages, 1)
	requireTextMessage(t, messages[0], "previous")
}
