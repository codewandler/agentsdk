package conversation

import "github.com/codewandler/llmadapter/unified"

type PayloadKind string

const (
	PayloadMessage       PayloadKind = "message"
	PayloadAssistantTurn PayloadKind = "assistant_turn"
	PayloadCompaction    PayloadKind = "compaction"
	PayloadAnnotation    PayloadKind = "annotation"
)

type Payload interface {
	Kind() PayloadKind
}

type MessageEvent struct {
	Message unified.Message `json:"message"`
}

func (MessageEvent) Kind() PayloadKind { return PayloadMessage }

type AssistantTurnEvent struct {
	Message       unified.Message        `json:"message"`
	FinishReason  unified.FinishReason   `json:"finish_reason,omitempty"`
	Usage         unified.Usage          `json:"usage,omitempty"`
	Continuations []ProviderContinuation `json:"continuations,omitempty"`
}

func (AssistantTurnEvent) Kind() PayloadKind { return PayloadAssistantTurn }

type CompactionEvent struct {
	Summary  string   `json:"summary"`
	Replaces []NodeID `json:"replaces,omitempty"`
}

func (CompactionEvent) Kind() PayloadKind { return PayloadCompaction }

type AnnotationEvent struct {
	Text string         `json:"text,omitempty"`
	Meta map[string]any `json:"meta,omitempty"`
}

func (AnnotationEvent) Kind() PayloadKind { return PayloadAnnotation }
