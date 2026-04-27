package runtime

import (
	"context"
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
	requireEventCountRuntime(t, after.Events, conversation.EventConversationStored, 2)

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
