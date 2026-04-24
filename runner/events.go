package runner

import (
	"github.com/codewandler/llmadapter/unified"
)

type Event interface{}

type TextDeltaEvent struct {
	Text string
}

type ReasoningDeltaEvent struct {
	Text string
}

type ToolCallEvent struct {
	Call unified.ToolCall
}

type ToolResultEvent struct {
	CallID  string
	Name    string
	Output  string
	IsError bool
}

type UsageEvent struct {
	Usage unified.Usage
}

type CompletedEvent struct {
	FinishReason unified.FinishReason
}

type ErrorEvent struct {
	Err error
}

type EventHandler func(Event)
