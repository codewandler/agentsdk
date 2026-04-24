package runner

import (
	"context"
	"encoding/json"
	"testing"

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
	require.Len(t, result.Events, 2)

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
