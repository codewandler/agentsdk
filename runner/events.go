package runner

import (
	"encoding/json"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/llmadapter/unified"
)

type Event interface{}

type TextDeltaEvent struct {
	Step int
	Text string
}

type ReasoningDeltaEvent struct {
	Step int
	Text string
}

type ToolCallEvent struct {
	Step int
	Call unified.ToolCall
}

type ToolCallArgsDeltaEvent struct {
	Step  int
	Index int
	ID    string
	Name  string
	Delta string
}

type ToolResultEvent struct {
	CallID  string
	Name    string
	Output  string
	IsError bool
}

type UsageEvent struct {
	Step             int
	Model            string
	ProviderIdentity conversation.ProviderIdentity
	Usage            unified.Usage
}

type CompletedEvent struct {
	Step         int
	FinishReason unified.FinishReason
}

type StepStartEvent struct {
	Step     int
	MaxSteps int
	Model    string
}

type StepDoneEvent struct {
	Step             int
	MaxSteps         int
	Model            string
	ProviderIdentity conversation.ProviderIdentity
	Usage            unified.Usage
	FinishReason     unified.FinishReason
}

type WarningEvent struct {
	Step    int
	Warning unified.WarningEvent
}

type RawEvent struct {
	Step int
	Raw  unified.RawEvent
}

type RouteEvent struct {
	Step             int
	Route            unified.RouteEvent
	ProviderIdentity conversation.ProviderIdentity
}

type ProviderExecutionEvent struct {
	Step      int
	Execution unified.ProviderExecutionEvent
}

type ErrorEvent struct {
	Err error
}

type EventHandler func(Event)

// EventHandlerContext carries per-turn metadata for event handler factories.
// It lives in runner so that presentation layers (such as terminal/ui) can
// accept this type without importing the agent package.
type EventHandlerContext struct {
	SessionID string
	TurnID    int
	Model     string
}

func ToolCallArgsMap(call unified.ToolCall) (map[string]any, error) {
	if len(call.Arguments) == 0 {
		return nil, nil
	}
	var out map[string]any
	if err := json.Unmarshal(call.Arguments, &out); err != nil {
		return nil, err
	}
	return out, nil
}
