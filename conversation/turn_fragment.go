package conversation

import (
	"fmt"

	"github.com/codewandler/llmadapter/unified"
)

type TurnFragment struct {
	requestMessages []unified.Message
	assistant       unified.Message
	finishReason    unified.FinishReason
	usage           unified.Usage
	continuations   []ProviderContinuation
	completed       bool
	err             error
}

func NewTurnFragment() *TurnFragment {
	return &TurnFragment{}
}

func (f *TurnFragment) AddRequestMessages(messages ...unified.Message) {
	f.requestMessages = append(f.requestMessages, messages...)
}

func (f *TurnFragment) SetAssistantMessage(message unified.Message) {
	f.assistant = message
}

func (f *TurnFragment) SetUsage(usage unified.Usage) {
	f.usage = usage
}

func (f *TurnFragment) AddContinuation(continuation ProviderContinuation) {
	if continuation.ResponseID != "" {
		f.continuations = append(f.continuations, continuation)
	}
}

func (f *TurnFragment) Complete(reason unified.FinishReason) {
	f.finishReason = reason
	f.completed = true
}

func (f *TurnFragment) Fail(err error) {
	f.err = err
}

func (f *TurnFragment) Payloads() ([]Payload, error) {
	if f.err != nil {
		return nil, fmt.Errorf("conversation: turn fragment failed: %w", f.err)
	}
	if !f.completed {
		return nil, fmt.Errorf("conversation: turn fragment is incomplete")
	}
	payloads := make([]Payload, 0, len(f.requestMessages)+1)
	for _, message := range f.requestMessages {
		payloads = append(payloads, MessageEvent{Message: message})
	}
	if !isZeroMessage(f.assistant) || len(f.continuations) > 0 || f.finishReason != "" || f.usage.HasTokens() || f.usage.Costs.Total() != 0 {
		payloads = append(payloads, AssistantTurnEvent{
			Message:       f.assistant,
			FinishReason:  f.finishReason,
			Usage:         f.usage,
			Continuations: append([]ProviderContinuation(nil), f.continuations...),
		})
	}
	if len(payloads) == 0 {
		return nil, fmt.Errorf("conversation: turn fragment has no payloads")
	}
	return payloads, nil
}

func isZeroMessage(message unified.Message) bool {
	return message.Role == "" &&
		message.ID == "" &&
		message.Name == "" &&
		len(message.Content) == 0 &&
		len(message.ToolCalls) == 0 &&
		len(message.ToolResults) == 0 &&
		len(message.Meta) == 0
}
