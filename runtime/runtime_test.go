package runtime

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/runner"
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
	events := []unified.Event{unified.CompletedEvent{FinishReason: unified.FinishReasonStop}}
	if len(c.events) > 0 {
		events = c.events[0]
		c.events = c.events[1:]
	}
	out := make(chan unified.Event, len(events))
	for _, event := range events {
		out <- event
	}
	close(out)
	return out, nil
}

func TestRunTurnAppliesDefaultsAndCommits(t *testing.T) {
	client := &fakeClient{events: [][]unified.Event{{
		unified.RouteEvent{ProviderName: "test", TargetAPI: "responses", NativeModel: "native"},
		unified.TextDeltaEvent{Text: "ok"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_1"},
	}}}
	agent, err := New(client,
		WithModel("public"),
		WithMaxOutputTokens(100),
		WithTemperature(0.2),
	)
	require.NoError(t, err)

	result, err := agent.RunTurn(context.Background(), "hello")
	require.NoError(t, err)
	require.Len(t, client.requests, 1)
	require.Equal(t, "public", client.requests[0].Model)
	require.Equal(t, 100, *client.requests[0].MaxOutputTokens)
	require.Equal(t, 0.2, *client.requests[0].Temperature)
	require.Len(t, client.requests[0].Messages, 1)
	requireEventType[runner.RouteEvent](t, result.Events)

	messages, err := agent.Session().Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
}

func TestRunTurnCanOverrideToolsPerTurn(t *testing.T) {
	client := &fakeClient{events: [][]unified.Event{
		{
			unified.ToolCallDoneEvent{Index: 0, ID: "call_1", Name: "echo", Args: json.RawMessage(`{"text":"ok"}`)},
			unified.CompletedEvent{FinishReason: unified.FinishReasonToolCall},
		},
		{
			unified.TextDeltaEvent{Text: "done"},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
		},
	}}
	echo := tool.New("echo", "echo text", func(_ tool.Ctx, p struct {
		Text string `json:"text"`
	}) (tool.Result, error) {
		return tool.Text(p.Text), nil
	})
	agent, err := New(client, WithMaxSteps(2))
	require.NoError(t, err)

	_, err = agent.RunTurn(context.Background(), "use echo", WithTurnTools([]tool.Tool{echo}))
	require.NoError(t, err)
	require.Len(t, client.requests, 2)
	require.Len(t, client.requests[0].Tools, 1)
	require.Len(t, client.requests[1].Tools, 1)
}

func requireEventType[T runner.Event](t *testing.T, events []runner.Event) T {
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
