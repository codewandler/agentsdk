package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestThreadRuntimeInjectsPlannerToolsContextAndResumes(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{
		ID:     "thread_runtime",
		Source: thread.EventSource{Type: "session", SessionID: "session_1"},
	})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry, WithThreadRuntimeSource(thread.EventSource{Type: "session", SessionID: "session_1"}))
	require.NoError(t, err)
	_, err = threadRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)

	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallDoneEvent{Index: 0, ID: "call_plan", Name: "plan", Args: json.RawMessage(`{"actions":[
				{"action":"create_plan","plan":{"id":"plan_1","title":"Runtime plan"}},
				{"action":"add_step","step":{"id":"step_1","title":"Persist planner state","status":"in_progress"}}
			]}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
		{
			unified.TextDeltaEvent{Text: "done"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
		},
	}}
	engine, err := New(client,
		WithThreadRuntime(threadRuntime),
		WithMaxSteps(2),
		WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
	)
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "create a plan")
	require.NoError(t, err)
	require.Len(t, client.requests, 2)
	requireToolSpec(t, client.requests[0], "plan")
	requireNoMessageContaining(t, client.requests[0], "Runtime plan")
	requireMessageContaining(t, client.requests[1], "Plan \"Runtime plan\" has 1 step(s).")
	requireMessageContaining(t, client.requests[1], "title: Persist planner state")
	sessionMessages, err := engine.History().Messages()
	require.NoError(t, err)
	requireNoStoredMessageContaining(t, sessionMessages, "Runtime plan")

	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, capability.EventAttached, 1)
	requireEventCountRuntime(t, stored.Events, capability.EventStateEventDispatched, 2)
	requireEventCountRuntime(t, stored.Events, EventContextFragmentRecorded, 2)
	requireEventCountRuntime(t, stored.Events, EventContextSnapshotRecorded, 2)
	requireEventCountRuntime(t, stored.Events, EventContextRenderCommitted, 2)

	resumedRuntime, _, err := ResumeThreadRuntime(ctx, store, thread.ResumeParams{
		ID:     live.ID(),
		Source: thread.EventSource{Type: "session", SessionID: "session_2"},
	}, registry)
	require.NoError(t, err)
	resumedClient := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "resumed"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
	}}}
	resumedEngine, err := New(resumedClient,
		WithHistory(NewHistory(WithHistorySessionID("session_2"))),
		WithThreadRuntime(resumedRuntime),
	)
	require.NoError(t, err)

	_, err = resumedEngine.RunTurn(ctx, "continue")
	require.NoError(t, err)
	require.Len(t, resumedClient.requests, 1)
	requireToolSpec(t, resumedClient.requests[0], "plan")
	requireMessageContaining(t, resumedClient.requests[0], "Plan \"Runtime plan\" has 1 step(s).")
	requireMessageContaining(t, resumedClient.requests[0], "title: Persist planner state")
}

func TestThreadRuntimeAutoAttachesConfiguredCapabilities(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_auto_capability"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	client := &fakeClient{}
	engine, err := New(client,
		WithThreadRuntime(threadRuntime),
		WithCapabilities(capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"}),
	)
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "hi")
	require.NoError(t, err)
	requireToolSpec(t, client.requests[0], "plan")
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, capability.EventAttached, 1)

	_, err = engine.RunTurn(ctx, "again")
	require.NoError(t, err)
	stored, err = store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, capability.EventAttached, 1)
}

func TestThreadRuntimeSendsContextDiffOnlyForNativeContinuation(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_native_context"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	_, err = threadRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)

	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallDoneEvent{Index: 0, ID: "call_plan", Name: "plan", Args: json.RawMessage(`{"actions":[
				{"action":"create_plan","plan":{"id":"plan_1","title":"Native context"}},
				{"action":"add_step","step":{"id":"step_1","title":"Render once","status":"in_progress"}}
			]}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
		{
			unified.RouteEvent{
				ProviderName:         "openai",
				TargetAPI:            "openai.responses",
				NativeModel:          "gpt-test",
				ConsumerContinuation: unified.ContinuationPreviousResponseID,
				InternalContinuation: unified.ContinuationPreviousResponseID,
				Transport:            unified.TransportHTTPSSE,
			},
			unified.ProviderExecutionEvent{
				InternalContinuation: unified.ContinuationPreviousResponseID,
				Transport:            unified.TransportHTTPSSE,
			},
			unified.TextDeltaEvent{Text: "done"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_1"},
		},
		{
			unified.TextDeltaEvent{Text: "continued"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_2"},
		},
	}}
	engine, err := New(client,
		WithThreadRuntime(threadRuntime),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"}),
		WithMaxSteps(2),
	)
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "create a plan")
	require.NoError(t, err)
	require.Len(t, client.requests, 2)
	requireMessageContaining(t, client.requests[1], "Plan \"Native context\" has 1 step(s).")
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, runner.EventProviderRouteSelected, 1)
	requireEventCountRuntime(t, stored.Events, runner.EventProviderExecutionMetadataRecorded, 1)

	_, err = engine.RunTurn(ctx, "continue")
	require.NoError(t, err)
	require.Len(t, client.requests, 3)
	requireNoMessageContaining(t, client.requests[2], "Native context")
	require.True(t, client.requests[2].Extensions.Has(unified.ExtOpenAIPreviousResponseID))
}

func TestThreadRuntimeReplaysCommittedContextRecordsOnResume(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_context_resume"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	_, err = threadRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)
	applyPlanActions(t, ctx, threadRuntime, `{"actions":[
		{"action":"create_plan","plan":{"id":"plan_1","title":"Resume context"}},
		{"action":"add_step","step":{"id":"step_1","title":"Do not resend","status":"pending"}}
	]}`)
	history := NewHistory()
	commitNativeContinuation(t, history, "resp_existing")
	firstClient := &fakeClient{events: [][]unified.Event{{
		unified.RouteEvent{
			ProviderName:         "openai",
			TargetAPI:            "openai.responses",
			NativeModel:          "gpt-test",
			ConsumerContinuation: unified.ContinuationPreviousResponseID,
			InternalContinuation: unified.ContinuationPreviousResponseID,
			Transport:            unified.TransportHTTPSSE,
		},
		unified.TextDeltaEvent{Text: "ok"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_1"},
	}}}
	firstEngine, err := New(firstClient,
		WithHistory(history),
		WithThreadRuntime(threadRuntime),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"}),
	)
	require.NoError(t, err)
	_, err = firstEngine.RunTurn(ctx, "render context once")
	require.NoError(t, err)
	requireMessageContaining(t, firstClient.requests[0], "Resume context")

	resumedRuntime, _, err := ResumeThreadRuntime(ctx, store, thread.ResumeParams{ID: live.ID()}, registry)
	require.NoError(t, err)
	resumedClient := &fakeClient{events: [][]unified.Event{{
		unified.TextDeltaEvent{Text: "ok"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_2"},
	}}}
	resumedEngine, err := New(resumedClient,
		WithHistory(history),
		WithThreadRuntime(resumedRuntime),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"}),
	)
	require.NoError(t, err)
	_, err = resumedEngine.RunTurn(ctx, "after resume")
	require.NoError(t, err)
	requireNoMessageContaining(t, resumedClient.requests[0], "Resume context")
}

func TestThreadRuntimeRollsBackContextRenderWhenProviderRequestFails(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_rollback"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	_, err = threadRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)
	applyPlanActions(t, ctx, threadRuntime, `{"actions":[
		{"action":"create_plan","plan":{"id":"plan_1","title":"Rollback context"}},
		{"action":"add_step","step":{"id":"step_1","title":"Retry render","status":"pending"}}
	]}`)

	history := NewHistory()
	commitNativeContinuation(t, history, "resp_existing")
	client := &fakeClient{
		errors: []error{errFakeRequest, nil},
		events: [][]unified.Event{{
			unified.TextDeltaEvent{Text: "ok"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_next"},
		}},
	}
	engine, err := New(client,
		WithHistory(history),
		WithThreadRuntime(threadRuntime),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"}),
	)
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "first attempt")
	require.ErrorIs(t, err, errFakeRequest)
	require.Len(t, client.requests, 1)
	requireMessageContaining(t, client.requests[0], "Rollback context")

	_, err = engine.RunTurn(ctx, "retry")
	require.NoError(t, err)
	require.Len(t, client.requests, 2)
	requireMessageContaining(t, client.requests[1], "Rollback context")
}

func TestThreadRuntimeSendsTombstoneForRemovedFragmentWithNativeContinuation(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_tombstone"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	_, err = threadRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)
	applyPlanActions(t, ctx, threadRuntime, `{"actions":[
		{"action":"create_plan","plan":{"id":"plan_1","title":"Tombstone context"}},
		{"action":"add_step","step":{"id":"step_1","title":"Remove me","status":"pending"}}
	]}`)
	_, err = threadRuntime.ContextManager().Build(ctx, agentcontextBuildRequest(live.ID(), live.BranchID()))
	require.NoError(t, err)

	history := NewHistory()
	commitNativeContinuation(t, history, "resp_existing")
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallDoneEvent{Index: 0, ID: "call_plan", Name: "plan", Args: json.RawMessage(`{"actions":[{"action":"remove_step","step_id":"step_1"}]}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
		{
			unified.TextDeltaEvent{Text: "ok"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_next"},
		},
	}}
	engine, err := New(client,
		WithHistory(history),
		WithThreadRuntime(threadRuntime),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"}),
		WithMaxSteps(2),
	)
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "remove step")
	require.NoError(t, err)
	require.Len(t, client.requests, 2)
	requireNoMessageContaining(t, client.requests[0], "Remove me")
	requireMessageContaining(t, client.requests[1], "Context fragment removed: planner_1/planner/step/step_1")
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, EventContextFragmentRemoved, 1)
}

func TestThreadRuntimeReplaysCapabilitiesForSelectedBranch(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	mainLive, err := store.Create(ctx, thread.CreateParams{ID: "thread_branch_runtime"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	mainRuntime, err := NewThreadRuntime(mainLive, registry)
	require.NoError(t, err)
	_, err = mainRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)
	applyPlanActions(t, ctx, mainRuntime, `{"actions":[
		{"action":"create_plan","plan":{"id":"plan_1","title":"Branch plan"}},
		{"action":"add_step","step":{"id":"step_before","title":"Before fork","status":"completed"}}
	]}`)

	altLive, err := store.Fork(ctx, thread.ForkParams{ID: mainLive.ID(), FromBranchID: thread.MainBranch, ToBranchID: "alt"})
	require.NoError(t, err)
	applyPlanActions(t, ctx, mainRuntime, `{"actions":[{"action":"add_step","step":{"id":"step_main","title":"Main only","status":"pending"}}]}`)

	altRuntime, _, err := ResumeThreadRuntime(ctx, store, thread.ResumeParams{ID: mainLive.ID(), BranchID: altLive.BranchID()}, registry)
	require.NoError(t, err)
	applyPlanActions(t, ctx, altRuntime, `{"actions":[{"action":"add_step","step":{"id":"step_alt","title":"Alt only","status":"pending"}}]}`)

	resumedMain, _, err := ResumeThreadRuntime(ctx, store, thread.ResumeParams{ID: mainLive.ID(), BranchID: thread.MainBranch}, registry)
	require.NoError(t, err)
	resumedAlt, _, err := ResumeThreadRuntime(ctx, store, thread.ResumeParams{ID: mainLive.ID(), BranchID: "alt"}, registry)
	require.NoError(t, err)

	mainContext, err := resumedMain.ContextManager().Build(ctx, agentcontextBuildRequest(mainLive.ID(), thread.MainBranch))
	require.NoError(t, err)
	altContext, err := resumedAlt.ContextManager().Build(ctx, agentcontextBuildRequest(mainLive.ID(), "alt"))
	require.NoError(t, err)
	requireContextContains(t, mainContext, "Main only")
	requireContextNotContains(t, mainContext, "Alt only")
	requireContextContains(t, altContext, "Alt only")
	requireContextNotContains(t, altContext, "Main only")
	requireContextContains(t, altContext, "Before fork")
}

func TestThreadRuntimeRejectsCapabilityToolNameConflict(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_tool_conflict"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	_, err = threadRuntime.AttachCapability(ctx, capability.AttachSpec{CapabilityName: planner.CapabilityName, InstanceID: "planner_1"})
	require.NoError(t, err)
	conflicting := tool.New("plan", "conflict", func(tool.Ctx, struct{}) (tool.Result, error) {
		return tool.Text("conflict"), nil
	})
	engine, err := New(&fakeClient{}, WithThreadRuntime(threadRuntime), WithTools([]tool.Tool{conflicting}))
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "hi")
	require.ErrorContains(t, err, `duplicate tool "plan"`)
}

func TestThreadRuntimeRendersDeveloperAuthorityAsInstruction(t *testing.T) {
	ctx := context.Background()
	contexts, err := agentcontext.NewManager(runtimeContextProvider{
		key: "policy",
		fragments: []agentcontext.ContextFragment{{
			Key:       "policy/rule",
			Content:   "Always preserve durable state.",
			Authority: agentcontext.AuthorityDeveloper,
		}},
	})
	require.NoError(t, err)
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_authority"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry(planner.Factory{})
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry, WithContextManager(contexts))
	require.NoError(t, err)
	client := &fakeClient{}
	engine, err := New(client, WithThreadRuntime(threadRuntime))
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "hi")
	require.NoError(t, err)
	require.Len(t, client.requests, 1)
	require.Len(t, client.requests[0].Instructions, 1)
	require.Equal(t, unified.InstructionDeveloper, client.requests[0].Instructions[0].Kind)
	requireInstructionContaining(t, client.requests[0], "Always preserve durable state.")
	requireNoMessageContaining(t, client.requests[0], "Always preserve durable state.")
}

func TestThreadRuntimePropagatesEphemeralCacheControlForUnstableContextFragments(t *testing.T) {
	ctx := context.Background()
	contexts, err := agentcontext.NewManager(runtimeContextProvider{
		key: "dynamic",
		fragments: []agentcontext.ContextFragment{{
			Key:       "dynamic/current",
			Content:   "Current volatile state.",
			Authority: agentcontext.AuthorityUser,
			CachePolicy: agentcontext.CachePolicy{
				Scope: agentcontext.CacheTurn,
			},
		}},
	})
	require.NoError(t, err)
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_cache_control"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry, WithContextManager(contexts))
	require.NoError(t, err)
	client := &fakeClient{}
	engine, err := New(client, WithThreadRuntime(threadRuntime))
	require.NoError(t, err)

	_, err = engine.RunTurn(ctx, "hi")
	require.NoError(t, err)
	require.Len(t, client.requests, 1)
	requireMessageContaining(t, client.requests[0], "Current volatile state.")
	for _, message := range client.requests[0].Messages {
		for _, part := range message.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, "Current volatile state.") {
				require.NotNil(t, text.CacheControl)
				require.Equal(t, unified.CacheControlEphemeral, text.CacheControl.Type)
				return
			}
		}
	}
	t.Fatal("missing context text part")
}

func TestThreadRuntimeCompactsConversationAndCommitsContextRender(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_compaction"})
	require.NoError(t, err)
	recorder := &recordingContextProvider{
		key: "policy",
		fragments: []agentcontext.ContextFragment{{
			Key:       "policy/rule",
			Content:   "Keep current plan state available.",
			Authority: agentcontext.AuthorityDeveloper,
		}},
	}
	contexts, err := agentcontext.NewManager(recorder)
	require.NoError(t, err)
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry, WithContextManager(contexts))
	require.NoError(t, err)
	history := NewHistory(WithHistorySessionID("session_compaction"), WithHistoryLiveThread(live))
	oldOne, err := history.AddUser("old one")
	require.NoError(t, err)
	oldTwo, err := history.AddUser("old two")
	require.NoError(t, err)
	keep, err := history.AddUser("keep")
	require.NoError(t, err)

	compaction, err := threadRuntime.Compact(ctx, history, "summary of old messages", oldOne, oldTwo)
	require.NoError(t, err)
	require.NotEmpty(t, compaction)
	require.Len(t, recorder.requests, 1)
	require.Equal(t, agentcontext.RenderCompaction, recorder.requests[0].Reason)
	require.Equal(t, agentcontext.PreferFull, recorder.requests[0].Preference)
	require.Equal(t, string(live.ID()), recorder.requests[0].ThreadID)
	require.Equal(t, string(live.BranchID()), recorder.requests[0].BranchID)

	messages, err := history.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	requireTextMessage(t, messages[0], "summary of old messages")
	requireTextMessage(t, messages[1], "keep")
	_, ok := history.Tree().Node(oldOne)
	require.True(t, ok)
	_, ok = history.Tree().Node(oldTwo)
	require.True(t, ok)
	_, ok = history.Tree().Node(keep)
	require.True(t, ok)

	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, eventConversationUserMessage, 3)
	requireEventCountRuntime(t, stored.Events, eventConversationCompaction, 1)
	requireEventCountRuntime(t, stored.Events, EventContextRenderCommitted, 1)

	resumedSession, err := ResumeHistoryFromThread(ctx, store, live, WithHistorySessionID("session_compaction"))
	require.NoError(t, err)
	resumedMessages, err := resumedSession.Messages()
	require.NoError(t, err)
	require.Len(t, resumedMessages, 2)
	requireTextMessage(t, resumedMessages[0], "summary of old messages")
	requireTextMessage(t, resumedMessages[1], "keep")

	resumedProvider := &recordingContextProvider{key: "policy"}
	resumedContexts, err := agentcontext.NewManager(resumedProvider)
	require.NoError(t, err)
	resumedRuntime, _, err := ResumeThreadRuntime(ctx, store, thread.ResumeParams{ID: live.ID()}, registry, WithContextManager(resumedContexts))
	require.NoError(t, err)
	records := resumedRuntime.ContextManager().Records()
	require.Contains(t, records, agentcontext.ProviderKey("policy"))
	require.NotEmpty(t, records["policy"].Fragments)
}

func TestEngineCompactUsesThreadRuntimeWhenConfigured(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_engine_compaction"})
	require.NoError(t, err)
	registry, err := capability.NewRegistry()
	require.NoError(t, err)
	threadRuntime, err := NewThreadRuntime(live, registry)
	require.NoError(t, err)
	history := NewHistory(WithHistoryLiveThread(live))
	old, err := history.AddUser("old")
	require.NoError(t, err)
	engine, err := New(&fakeClient{}, WithHistory(history), WithThreadRuntime(threadRuntime))
	require.NoError(t, err)

	_, err = engine.Compact(ctx, "summary", old)
	require.NoError(t, err)

	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	require.NoError(t, err)
	requireEventCountRuntime(t, stored.Events, eventConversationUserMessage, 1)
	requireEventCountRuntime(t, stored.Events, eventConversationCompaction, 1)
	requireEventCountRuntime(t, stored.Events, EventContextRenderCommitted, 1)
}

func requireToolSpec(t *testing.T, req unified.Request, name string) {
	t.Helper()
	for _, spec := range req.Tools {
		if spec.Name == name {
			return
		}
	}
	t.Fatalf("missing tool spec %q in %#v", name, req.Tools)
}

func requireInstructionContaining(t *testing.T, req unified.Request, want string) {
	t.Helper()
	for _, instruction := range req.Instructions {
		for _, part := range instruction.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				return
			}
		}
	}
	t.Fatalf("missing instruction containing %q in %#v", want, req.Instructions)
}

func applyPlanActions(t *testing.T, ctx context.Context, runtime *ThreadRuntime, input string) {
	t.Helper()
	planTool := requireRuntimeTool(t, runtime, "plan")
	_, err := planTool.Execute(NewToolContext(ctx), json.RawMessage(input))
	require.NoError(t, err)
}

func requireRuntimeTool(t *testing.T, runtime *ThreadRuntime, name string) tool.Tool {
	t.Helper()
	for _, candidate := range runtime.Tools() {
		if candidate.Name() == name {
			return candidate
		}
	}
	t.Fatalf("missing runtime tool %q", name)
	return nil
}

func commitNativeContinuation(t *testing.T, history *History, responseID string) {
	t.Helper()
	fragment := conversation.NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{Role: unified.RoleUser, Content: []unified.ContentPart{unified.TextPart{Text: "previous"}}})
	fragment.SetAssistantMessage(unified.Message{Role: unified.RoleAssistant, Content: []unified.ContentPart{unified.TextPart{Text: "ok"}}})
	continuation := conversation.NewProviderContinuation(
		conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		responseID,
		unified.Extensions{},
	)
	continuation.ConsumerContinuation = unified.ContinuationPreviousResponseID
	continuation.InternalContinuation = unified.ContinuationPreviousResponseID
	continuation.Transport = unified.TransportHTTPSSE
	fragment.AddContinuation(continuation)
	fragment.Complete(unified.FinishReasonStop)
	_, err := history.CommitFragment(fragment)
	require.NoError(t, err)
}

func agentcontextBuildRequest(threadID thread.ID, branchID thread.BranchID) agentcontext.BuildRequest {
	return agentcontext.BuildRequest{ThreadID: string(threadID), BranchID: string(branchID)}
}

func requireContextContains(t *testing.T, result agentcontext.BuildResult, want string) {
	t.Helper()
	for _, fragment := range result.Active {
		if strings.Contains(fragment.Content, want) {
			return
		}
	}
	t.Fatalf("missing context containing %q in %#v", want, result.Active)
}

func requireContextNotContains(t *testing.T, result agentcontext.BuildResult, want string) {
	t.Helper()
	for _, fragment := range result.Active {
		if strings.Contains(fragment.Content, want) {
			t.Fatalf("unexpected context containing %q in %#v", want, result.Active)
		}
	}
}

type runtimeContextProvider struct {
	key       agentcontext.ProviderKey
	fragments []agentcontext.ContextFragment
}

func (p runtimeContextProvider) Key() agentcontext.ProviderKey { return p.key }

func (p runtimeContextProvider) GetContext(context.Context, agentcontext.Request) (agentcontext.ProviderContext, error) {
	return agentcontext.ProviderContext{Fragments: append([]agentcontext.ContextFragment(nil), p.fragments...)}, nil
}

type recordingContextProvider struct {
	key       agentcontext.ProviderKey
	fragments []agentcontext.ContextFragment
	requests  []agentcontext.Request
}

func (p *recordingContextProvider) Key() agentcontext.ProviderKey { return p.key }

func (p *recordingContextProvider) GetContext(_ context.Context, req agentcontext.Request) (agentcontext.ProviderContext, error) {
	p.requests = append(p.requests, req)
	return agentcontext.ProviderContext{Fragments: append([]agentcontext.ContextFragment(nil), p.fragments...)}, nil
}

func requireTextMessage(t *testing.T, msg unified.Message, want string) {
	t.Helper()
	require.Len(t, msg.Content, 1)
	text, ok := msg.Content[0].(unified.TextPart)
	require.True(t, ok)
	require.Equal(t, want, text.Text)
}

func requireMessageContaining(t *testing.T, req unified.Request, want string) {
	t.Helper()
	for _, message := range req.Messages {
		for _, part := range message.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				return
			}
		}
	}
	t.Fatalf("missing message containing %q in %#v", want, req.Messages)
}

func requireNoMessageContaining(t *testing.T, req unified.Request, want string) {
	t.Helper()
	for _, message := range req.Messages {
		for _, part := range message.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				t.Fatalf("unexpected message containing %q in %#v", want, req.Messages)
			}
		}
	}
}

func requireNoStoredMessageContaining(t *testing.T, messages []unified.Message, want string) {
	t.Helper()
	for _, message := range messages {
		for _, part := range message.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				t.Fatalf("unexpected stored message containing %q in %#v", want, messages)
			}
		}
	}
}

func requireEventCountRuntime(t *testing.T, events []thread.Event, kind thread.EventKind, want int) {
	t.Helper()
	var got int
	for _, event := range events {
		if event.Kind == kind {
			got++
		}
	}
	require.Equal(t, want, got, "event count for %q", kind)
}
