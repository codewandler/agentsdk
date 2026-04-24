package runner

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

type fakeClient struct {
	requests []unified.Request
	events   [][]unified.Event
}

func (c *fakeClient) Request(_ context.Context, req unified.Request) (<-chan unified.Event, error) {
	c.requests = append(c.requests, req)
	if len(c.events) == 0 {
		out := make(chan unified.Event)
		close(out)
		return out, nil
	}
	out := make(chan unified.Event, len(c.events[0]))
	for _, event := range c.events[0] {
		out <- event
	}
	close(out)
	c.events = c.events[1:]
	return out, nil
}

func TestRunTurnCommitsOnlyAfterFinalResponse(t *testing.T) {
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.TextDeltaEvent{Text: "hello"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_1"},
		},
	}}
	sess := conversation.New()

	result, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build(),
		WithProviderIdentity(conversation.ProviderIdentity{ProviderName: "test", APIKind: "responses"}),
	)
	require.NoError(t, err)
	requireEventType[StepStartEvent](t, result.Events)
	requireEventType[StepDoneEvent](t, result.Events)
	requireEventType[TextDeltaEvent](t, result.Events)
	requireEventType[CompletedEvent](t, result.Events)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, unified.RoleUser, messages[0].Role)
	require.Equal(t, unified.RoleAssistant, messages[1].Role)

	continuation, ok, err := conversation.ContinuationAtHead(sess.Tree(), sess.Branch(), conversation.ProviderIdentity{ProviderName: "test"})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_1", continuation.ResponseID)
}

func TestRunTurnExecutesToolAndCommitsWholeTranscript(t *testing.T) {
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallDoneEvent{Index: 0, ID: "call_1", Name: "echo", Args: json.RawMessage(`{"text":"ok"}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall, MessageID: "resp_tool"},
		},
		{
			unified.TextDeltaEvent{Text: "done"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_final"},
		},
	}}
	echo := tool.New("echo", "echo text", func(_ tool.Ctx, p struct {
		Text string `json:"text"`
	}) (tool.Result, error) {
		return tool.Text(p.Text), nil
	})
	sess := conversation.New(conversation.WithTools(tool.UnifiedToolsFrom([]tool.Tool{echo})))

	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("use echo").Build(), WithTools([]tool.Tool{echo}))
	require.NoError(t, err)
	require.Len(t, client.requests, 2)
	require.Len(t, client.requests[1].Messages, 3)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 4)
	require.Len(t, messages[1].ToolCalls, 1)
	require.Len(t, messages[2].ToolResults, 1)
}

func TestRunTurnAccumulatesToolArgsDeltas(t *testing.T) {
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallStartEvent{Index: 0, ID: "call_1", Name: "echo"},
			unified.ToolCallArgsDeltaEvent{Index: 0, Delta: `{"text"`},
			unified.ToolCallArgsDeltaEvent{Index: 0, Delta: `:"ok"}`},
			unified.ToolCallDoneEvent{Index: 0},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
	}}
	echo := tool.New("echo", "echo text", func(_ tool.Ctx, p struct {
		Text string `json:"text"`
	}) (tool.Result, error) {
		return tool.Text(p.Text), nil
	})

	result, err := RunTurn(context.Background(), conversation.New(), client, conversation.NewRequest().User("use echo").Build(),
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
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallDoneEvent{Index: 0, ID: "call_1", Name: "slow", Args: json.RawMessage(`{}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
	}}
	slow := tool.New("slow", "slow tool", func(ctx tool.Ctx, _ struct{}) (tool.Result, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})

	result, err := RunTurn(context.Background(), conversation.New(), client, conversation.NewRequest().User("use slow").Build(),
		WithTools([]tool.Tool{slow}),
		WithToolTimeout(time.Millisecond),
		WithMaxSteps(1),
	)
	require.ErrorIs(t, err, ErrMaxStepsReached)
	requireToolResult(t, result.Events, "[Timed out]", true)
}

func TestRunTurnCancellationEmitsCanceledForRemainingToolCalls(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallDoneEvent{Index: 0, ID: "call_1", Name: "first", Args: json.RawMessage(`{}`)},
			unified.ToolCallDoneEvent{Index: 1, ID: "call_2", Name: "second", Args: json.RawMessage(`{}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
	}}
	executor := ToolExecutorFunc(func(_ context.Context, call unified.ToolCall) unified.ToolResult {
		if call.Name == "first" {
			cancel()
			return toolResult(call, "[Canceled]", true)
		}
		return toolResult(call, "should not run", false)
	})

	result, err := RunTurn(ctx, conversation.New(), client, conversation.NewRequest().User("use tools").Build(), WithToolExecutor(executor))
	require.ErrorIs(t, err, context.Canceled)
	var outputs []string
	for _, event := range result.Events {
		if ev, ok := event.(ToolResultEvent); ok {
			outputs = append(outputs, ev.Output)
		}
	}
	require.Equal(t, []string{"[Canceled]", "[Canceled]"}, outputs)
}

func TestRunTurnPassesThroughWarningsAndRawEvents(t *testing.T) {
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.WarningEvent{Code: "dropped", Message: "field dropped"},
			unified.RawEvent{APIKind: "test", Type: "raw"},
			unified.TextDeltaEvent{Text: "ok"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
		},
	}}
	result, err := RunTurn(context.Background(), conversation.New(), client, conversation.NewRequest().User("hi").Build())
	require.NoError(t, err)
	requireEventType[WarningEvent](t, result.Events)
	requireEventType[RawEvent](t, result.Events)
}

func TestRunTurnProviderErrorDoesNotCommit(t *testing.T) {
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.TextDeltaEvent{Text: "partial"},
			unified.ErrorEvent{Err: errors.New("boom")},
		},
	}}
	sess := conversation.New()
	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build())
	require.ErrorContains(t, err, "boom")
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Empty(t, messages)
}

func TestRunTurnIncompleteStreamDoesNotCommit(t *testing.T) {
	client := &fakeClient{events: [][]unified.Event{
		{unified.TextDeltaEvent{Text: "partial"}},
	}}
	sess := conversation.New()
	_, err := RunTurn(context.Background(), sess, client, conversation.NewRequest().User("hi").Build())
	require.ErrorContains(t, err, "without completed")
	messages, msgErr := sess.Messages()
	require.NoError(t, msgErr)
	require.Empty(t, messages)
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
