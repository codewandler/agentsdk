package runner

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestRunTurnCommitsOnlyAfterFinalResponse(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			routeWithContinuation("openai", "openai.responses", "openai.responses", "public", "gpt-test", unified.ContinuationPreviousResponseID),
			unified.ProviderExecutionEvent{InternalContinuation: unified.ContinuationPreviousResponseID, Transport: unified.TransportHTTPSSE},
			unified.TextDeltaEvent{Text: "hello"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_1"},
		},
	)
	sess := newTestHistory("")

	result, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build(),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "test", APIKind: "responses"}),
	)
	require.NoError(t, err)
	requireEventType[StepStartEvent](t, result.Events)
	requireEventType[StepDoneEvent](t, result.Events)
	requireEventType[TextDeltaEvent](t, result.Events)
	requireEventType[CompletedEvent](t, result.Events)
	route := requireEventType[RouteEvent](t, result.Events)
	require.Equal(t, "openai", route.ProviderIdentity.ProviderName)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, unified.RoleUser, messages[0].Role)
	require.Equal(t, unified.RoleAssistant, messages[1].Role)
	require.Empty(t, messages[1].ID)

	continuation, ok, err := conversation.ContinuationAtHead(sess.Tree(), sess.Branch(), conversation.ProviderIdentity{ProviderName: "openai"})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_1", continuation.ResponseID)
	require.Equal(t, "gpt-test", continuation.NativeModel)
	require.Equal(t, unified.ContinuationPreviousResponseID, continuation.ConsumerContinuation)
	require.Equal(t, unified.TransportHTTPSSE, continuation.Transport)
	execution := requireEventType[ProviderExecutionEvent](t, result.Events)
	require.Equal(t, unified.TransportHTTPSSE, execution.Execution.Transport)
}

func TestRunTurnUsesNativeContinuationProjection(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("next", "resp_2"))
	sess := newTestHistory("")
	fragment := conversation.NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{
		Role:    unified.RoleUser,
		Content: []unified.ContentPart{unified.TextPart{Text: "hello"}},
	})
	fragment.SetAssistantMessage(unified.Message{
		Role:    unified.RoleAssistant,
		Content: []unified.ContentPart{unified.TextPart{Text: "hi"}},
	})
	continuation := conversation.NewProviderContinuation(
		conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"},
		"resp_1",
		unified.Extensions{},
	)
	continuation.ConsumerContinuation = unified.ContinuationPreviousResponseID
	continuation.InternalContinuation = unified.ContinuationPreviousResponseID
	fragment.AddContinuation(continuation)
	fragment.Complete(unified.FinishReasonStop)
	_, err := sess.CommitFragment(fragment)
	require.NoError(t, err)

	_, err = RunTurn(context.Background(), sess, client, conversation.NewRequest().User("again").Build(),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "openai", APIKind: "openai.responses", NativeModel: "gpt-test"}),
	)
	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
	require.Len(t, client.RequestAt(0).Messages, 1)
	previousResponseID, ok, err := unified.GetExtension[string](client.RequestAt(0).Extensions, unified.ExtOpenAIPreviousResponseID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_1", previousResponseID)
}

func TestRunTurnKeepsCodexProjectionAsReplay(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			routeWithContinuation("codex_responses", "codex.responses", "codex.responses", "public", "gpt-test", unified.ContinuationReplay),
			unified.ProviderExecutionEvent{InternalContinuation: unified.ContinuationPreviousResponseID, Transport: unified.TransportWebSocket},
			unified.TextDeltaEvent{Text: "hello"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_codex"},
		},
		runnertest.TextStream("next", "resp_next"),
	)
	sess := newTestHistory("thread_codex")

	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build(),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "codex_responses", APIKind: "codex.responses", NativeModel: "gpt-test"}),
	)
	require.NoError(t, err)
	_, err = RunTurn(context.Background(), sess, client, conversation.NewRequest().User("again").Build(),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "codex_responses", APIKind: "codex.responses", NativeModel: "gpt-test"}),
	)
	require.NoError(t, err)
	require.Len(t, client.Requests(), 2)
	require.Greater(t, len(client.RequestAt(1).Messages), 1)
	require.False(t, client.RequestAt(1).Extensions.Has(unified.ExtOpenAIPreviousResponseID))
	codex, warnings := unified.CodexExtensionsFrom(client.RequestAt(1).Extensions)
	require.Empty(t, warnings)
	require.Equal(t, unified.InteractionSession, codex.InteractionMode)
	require.Equal(t, "thread_codex", codex.SessionID)
}

func TestRunTurnPreservesReasoningSignatureForReplay(t *testing.T) {
	client := runnertest.NewClient(runnertest.ReasoningTextStream("think", "sig", "hello"))
	sess := newTestHistory("")

	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build())
	require.NoError(t, err)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Len(t, messages[1].Content, 2)
	reasoning, ok := messages[1].Content[0].(unified.ReasoningPart)
	require.True(t, ok)
	require.Equal(t, "think", reasoning.Text)
	require.Equal(t, "sig", reasoning.Signature)
}

func TestRunTurnExecutesToolAndCommitsWholeTranscript(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.ToolCallStream("resp_tool", runnertest.ToolCall("echo", "call_1", 0, `{"text":"ok"}`)),
		runnertest.TextStream("done", "resp_final"),
	)
	echo := tool.New("echo", "echo text", func(_ tool.Ctx, p struct {
		Text string `json:"text"`
	}) (tool.Result, error) {
		return tool.Text(p.Text), nil
	})
	sess := newTestHistory("")

	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("use echo").Build(), WithTools([]tool.Tool{echo}))
	require.NoError(t, err)
	require.Len(t, client.Requests(), 2)
	require.Len(t, client.RequestAt(1).Messages, 3)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 4)
	require.Len(t, messages[1].ToolCalls, 1)
	require.Len(t, messages[2].ToolResults, 1)
}

func TestRunTurnAccumulatesToolArgsDeltas(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			unified.ToolCallStartEvent{Index: 0, ID: "call_1", Name: "echo"},
			unified.ToolCallArgsDeltaEvent{Index: 0, Delta: `{"text"`},
			unified.ToolCallArgsDeltaEvent{Index: 0, Delta: `:"ok"}`},
			unified.ToolCallDoneEvent{Index: 0},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
	)
	echo := tool.New("echo", "echo text", func(_ tool.Ctx, p struct {
		Text string `json:"text"`
	}) (tool.Result, error) {
		return tool.Text(p.Text), nil
	})

	result, err := RunTurn(context.Background(), newTestHistory(""), client, conversation.NewRequest().User("use echo").Build(),
		WithTools([]tool.Tool{echo}),
		WithMaxSteps(1),
	)
	require.ErrorIs(t, err, ErrMaxStepsReached)
	var toolResult ToolResultEvent
	for _, event := range result.Events {
		if ev, ok := event.(ToolResultEvent); ok {
			toolResult = ev
		}
	}
	require.Equal(t, "ok", toolResult.Output)
	require.False(t, toolResult.IsError)
}

func TestRunTurnToolTimeoutEmitsTimedOutResult(t *testing.T) {
	client := runnertest.NewClient(runnertest.ToolCallStream("resp_tool", runnertest.ToolCall("slow", "call_1", 0, `{}`)))
	slow := tool.New("slow", "slow tool", func(ctx tool.Ctx, _ struct{}) (tool.Result, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	sess := newTestHistory("")

	result, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("use slow").Build(),
		WithTools([]tool.Tool{slow}),
		WithToolTimeout(time.Millisecond),
		WithMaxSteps(1),
	)
	require.ErrorIs(t, err, ErrMaxStepsReached)
	requireToolResult(t, result.Events, "[Timed out]", true)
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Len(t, messages, 3)
	require.Len(t, messages[1].ToolCalls, 1)
	require.Len(t, messages[2].ToolResults, 1)
	require.True(t, messages[2].ToolResults[0].IsError)
}

func TestRunTurnCancellationEmitsCanceledForRemainingToolCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := runnertest.NewClient(runnertest.ToolCallStream("resp_tool",
		runnertest.ToolCall("first", "call_1", 0, `{}`),
		runnertest.ToolCall("second", "call_2", 1, `{}`),
	))
	executor := ToolExecutorFunc(func(_ context.Context, call unified.ToolCall) unified.ToolResult {
		if call.Name == "first" {
			cancel()
			return toolResult(call, "[Canceled]", true)
		}
		return toolResult(call, "should not run", false)
	})
	sess := newTestHistory("")

	result, err := RunTurn(ctx, sess, client, conversation.NewRequest().User("use tools").Build(), WithToolExecutor(executor))
	require.ErrorIs(t, err, context.Canceled)
	var outputs []string
	for _, event := range result.Events {
		if ev, ok := event.(ToolResultEvent); ok {
			outputs = append(outputs, ev.Output)
		}
	}
	require.Equal(t, []string{"[Canceled]", "[Canceled]"}, outputs)
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Len(t, messages, 3)
	require.Len(t, messages[1].ToolCalls, 2)
	require.Len(t, messages[2].ToolResults, 2)
}

func TestRunTurnPassesThroughWarningsAndRawEvents(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			unified.WarningEvent{Code: "dropped", Message: "field dropped"},
			unified.RawEvent{APIKind: "test", Type: "raw"},
			unified.TextDeltaEvent{Text: "ok"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
		},
	)
	result, err := RunTurn(context.Background(), newTestHistory(""), client, conversation.NewRequest().User("hi").Build())
	require.NoError(t, err)
	requireEventType[WarningEvent](t, result.Events)
	requireEventType[RawEvent](t, result.Events)
}

func TestRunTurnProviderErrorDoesNotCommit(t *testing.T) {
	client := runnertest.NewClient(runnertest.ErrorStream(errors.New("boom")))
	sess := newTestHistory("")
	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build())
	require.ErrorContains(t, err, "boom")
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Empty(t, messages)
}

func TestRunTurnProviderErrorRecordsThreadDiagnostic(t *testing.T) {
	ctx := context.Background()
	client := runnertest.NewClient([]unified.Event{
		unified.ToolCallStartEvent{Index: 0, ID: "call_1", Name: "partial"},
		unified.ToolCallArgsDeltaEvent{Index: 0, Delta: `{"a"`},
		unified.ErrorEvent{Err: errors.New("boom"), Recoverable: true},
	})
	sess, store := newTestThreadHistory(t, ctx)

	_, err := RunTurn(ctx, sess, client, conversation.NewRequest().User("hi").Build(),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "test", APIKind: "responses"}),
	)
	require.ErrorContains(t, err, "boom")
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Empty(t, messages)

	payload := requireProviderStreamFailedPayload(t, ctx, store, sess.live.ID())
	require.Equal(t, 1, payload.Step)
	require.Equal(t, "test", payload.ProviderIdentity.ProviderName)
	require.Equal(t, "boom", payload.Error)
	require.True(t, payload.Recoverable)
	require.Equal(t, "error", payload.Facts.LastEvent)
	require.Len(t, payload.Facts.ToolCalls, 1)
	require.Equal(t, "call_1", payload.Facts.ToolCalls[0].ID)
	require.Equal(t, "partial", payload.Facts.ToolCalls[0].Name)
	require.Equal(t, len(`{"a"`), payload.Facts.ToolCalls[0].ArgsBytes)
}

func TestRunTurnIncompleteStreamDoesNotCommit(t *testing.T) {
	client := runnertest.NewClient(runnertest.IncompleteTextStream("partial"))
	sess := newTestHistory("")
	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build())
	require.ErrorContains(t, err, "without completed")
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Empty(t, messages)
}

func TestRunTurnIncompleteStreamRecordsThreadDiagnostic(t *testing.T) {
	ctx := context.Background()
	client := runnertest.NewClient(runnertest.IncompleteTextStream("partial"))
	sess, store := newTestThreadHistory(t, ctx)

	_, err := RunTurn(ctx, sess, client, conversation.NewRequest().Model("gpt-test").User("hi").Build())
	require.ErrorContains(t, err, "without completed")
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Empty(t, messages)

	payload := requireProviderStreamFailedPayload(t, ctx, store, sess.live.ID())
	require.Equal(t, 1, payload.Step)
	require.Equal(t, "gpt-test", payload.Model)
	require.Equal(t, "runner: stream ended without completed event", payload.Error)
	require.Equal(t, "stream_closed", payload.Facts.LastEvent)
	require.Equal(t, len("partial"), payload.Facts.TextBytes)
	require.False(t, payload.Facts.SawCompleted)
}

func requireToolResult(t *testing.T, events []Event, output string, isError bool) {
	t.Helper()
	for _, event := range events {
		if ev, ok := event.(ToolResultEvent); ok {
			require.Equal(t, output, ev.Output)
			require.Equal(t, isError, ev.IsError)
			return
		}
	}
	require.Fail(t, "missing tool result event")
}

func requireEventType[T Event](t *testing.T, events []Event) T {
	t.Helper()
	for _, event := range events {
		if ev, ok := event.(T); ok {
			return ev
		}
	}
	var zero T
	require.Failf(t, "missing event type", "%T", zero)
	return zero
}

type testHistory struct {
	sessionID string
	branch    conversation.BranchID
	tree      *conversation.Tree
}

type testThreadHistory struct {
	*testHistory
	live thread.Live
}

func newTestThreadHistory(t *testing.T, ctx context.Context) (*testThreadHistory, *thread.MemoryStore) {
	t.Helper()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_runner_diagnostics"})
	require.NoError(t, err)
	return &testThreadHistory{testHistory: newTestHistory("session_runner_diagnostics"), live: live}, store
}

func (h *testThreadHistory) ThreadEventsEnabled() bool { return h.live != nil }

func (h *testThreadHistory) AppendThreadEvents(ctx context.Context, events ...thread.Event) error {
	return h.live.Append(ctx, events...)
}

func (h *testThreadHistory) CommitFragmentWithThreadEvents(ctx context.Context, fragment *conversation.TurnFragment, events ...thread.Event) ([]conversation.NodeID, error) {
	if len(events) > 0 {
		if err := h.live.Append(ctx, events...); err != nil {
			return nil, err
		}
	}
	return h.CommitFragment(fragment)
}

func requireProviderStreamFailedPayload(t *testing.T, ctx context.Context, store *thread.MemoryStore, id thread.ID) providerStreamFailedPayload {
	t.Helper()
	stored, err := store.Read(ctx, thread.ReadParams{ID: id})
	require.NoError(t, err)
	var payloads []providerStreamFailedPayload
	for _, event := range stored.Events {
		if event.Kind != EventProviderStreamFailed {
			continue
		}
		var payload providerStreamFailedPayload
		require.NoError(t, json.Unmarshal(event.Payload, &payload))
		payloads = append(payloads, payload)
	}
	require.Len(t, payloads, 1)
	return payloads[0]
}

func newTestHistory(sessionID string) *testHistory {
	if sessionID == "" {
		sessionID = "test_session"
	}
	return &testHistory{
		sessionID: sessionID,
		branch:    conversation.MainBranch,
		tree:      conversation.NewTree(),
	}
}

func (h *testHistory) SessionID() string             { return h.sessionID }
func (h *testHistory) Branch() conversation.BranchID { return h.branch }
func (h *testHistory) Tree() *conversation.Tree      { return h.tree }

func (h *testHistory) Messages() ([]unified.Message, error) {
	return conversation.ProjectMessages(h.tree, h.branch)
}

func (h *testHistory) BuildRequestForProvider(req conversation.Request, identity conversation.ProviderIdentity) (unified.Request, error) {
	items, err := conversation.ProjectItems(h.tree, h.branch)
	if err != nil {
		return unified.Request{}, err
	}
	pending := append([]unified.Message(nil), req.Messages...)
	projection, err := conversation.DefaultProjectionPolicy().Project(conversation.ProjectionInput{
		Tree:                    h.tree,
		Branch:                  h.branch,
		ProviderIdentity:        identity,
		Items:                   items,
		PendingItems:            conversation.ItemsFromMessages(pending),
		Extensions:              req.Extensions,
		AllowNativeContinuation: true,
	})
	if err != nil {
		return unified.Request{}, err
	}
	out := unified.Request{
		Model:      req.Model,
		Messages:   projection.Messages,
		Stream:     req.Stream,
		Extensions: projection.Extensions,
	}
	if conversation.IsCodexResponsesIdentity(identity) {
		if err := conversation.AddCodexSessionHints(&out, identity, string(h.sessionID), h.tree, h.branch); err != nil {
			return unified.Request{}, err
		}
	}
	return out, nil
}

func (h *testHistory) CommitFragment(fragment *conversation.TurnFragment) ([]conversation.NodeID, error) {
	payloads, err := fragment.Payloads()
	if err != nil {
		return nil, err
	}
	return h.tree.AppendMany(h.branch, payloads...)
}

func routeWithContinuation(providerName string, api string, family string, publicModel string, nativeModel string, consumer unified.ContinuationMode) unified.RouteEvent {
	route := runnertest.Route(providerName, api, family, publicModel, nativeModel)
	route.ConsumerContinuation = consumer
	route.InternalContinuation = consumer
	route.Transport = unified.TransportHTTPSSE
	return route
}
