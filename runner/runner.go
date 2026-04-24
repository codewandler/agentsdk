package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/llmadapter/unified"
)

var ErrMaxStepsReached = errors.New("runner: max steps reached")

type Result struct {
	Events []Event
	Steps  int
}

func RunTurn(ctx context.Context, session *conversation.Session, client unified.Client, req conversation.Request, opts ...Option) (Result, error) {
	if session == nil {
		return Result{}, fmt.Errorf("runner: session is required")
	}
	if client == nil {
		return Result{}, fmt.Errorf("runner: client is required")
	}
	options := applyOptions(opts)
	if options.ToolCtx == nil {
		options.ToolCtx = &basicToolCtx{
			Context:   ctx,
			sessionID: string(session.SessionID()),
			extra:     map[string]any{},
		}
	}
	executor := options.ToolExecutor
	if executor == nil {
		executor = newDefaultToolExecutor(options.Tools, options.ToolCtx, options.ToolTimeout)
	}

	var result Result
	emit := func(event Event) {
		result.Events = append(result.Events, event)
		if options.OnEvent != nil {
			options.OnEvent(event)
		}
	}

	fragment := conversation.NewTurnFragment()
	transcript := append([]unified.Message(nil), req.Messages...)

	for step := 0; step < options.MaxSteps; step++ {
		result.Steps++
		stepReq := req
		stepReq.Messages = append([]unified.Message(nil), transcript...)
		wireReq, err := session.BuildRequest(stepReq)
		if err != nil {
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}
		emit(StepStartEvent{Step: result.Steps, MaxSteps: options.MaxSteps, Model: wireReq.Model})
		events, err := client.Request(ctx, wireReq)
		if err != nil {
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}

		assistant, finishReason, usage, toolCalls, messageID, err := consumeEvents(ctx, events, emit, eventContext{
			step:             result.Steps,
			model:            wireReq.Model,
			providerIdentity: options.ProviderIdentity,
		})
		if err != nil {
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}
		emit(StepDoneEvent{
			Step:             result.Steps,
			MaxSteps:         options.MaxSteps,
			Model:            wireReq.Model,
			ProviderIdentity: options.ProviderIdentity,
			Usage:            usage,
			FinishReason:     finishReason,
		})
		if messageID != "" {
			assistant.ID = messageID
			fragment.AddContinuation(conversation.NewProviderContinuation(options.ProviderIdentity, messageID, unified.Extensions{}))
		}

		if len(toolCalls) == 0 {
			fragment.AddRequestMessages(transcript...)
			fragment.SetAssistantMessage(assistant)
			fragment.SetUsage(usage)
			fragment.Complete(finishReason)
			if _, err := session.CommitFragment(fragment); err != nil {
				emit(ErrorEvent{Err: err})
				return result, err
			}
			emit(CompletedEvent{Step: result.Steps, FinishReason: finishReason})
			return result, nil
		}

		transcript = append(transcript, assistant)
		results := make([]unified.ToolResult, 0, len(toolCalls))
		canceled := false
		for _, call := range toolCalls {
			var toolResult unified.ToolResult
			if canceled || ctx.Err() != nil {
				canceled = true
				toolResult = toolResultFromContext(call, context.Canceled)
			} else {
				toolResult = executor.ExecuteTool(ctx, call)
				if ctx.Err() != nil || toolResultOutput(toolResult) == canceledToolOutput || toolResultOutput(toolResult) == timedOutToolOutput {
					canceled = true
				}
			}
			results = append(results, toolResult)
			output := textFromParts(toolResult.Content)
			emit(ToolResultEvent{CallID: toolResult.ToolCallID, Name: toolResult.Name, Output: output, IsError: toolResult.IsError})
		}
		transcript = append(transcript, unified.Message{Role: unified.RoleTool, ToolResults: results})
		if err := ctx.Err(); err != nil {
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}
	}

	err := ErrMaxStepsReached
	fragment.Fail(err)
	emit(ErrorEvent{Err: err})
	return result, err
}

type eventContext struct {
	step             int
	model            string
	providerIdentity conversation.ProviderIdentity
}

func consumeEvents(ctx context.Context, events <-chan unified.Event, emit func(Event), meta eventContext) (unified.Message, unified.FinishReason, unified.Usage, []unified.ToolCall, string, error) {
	var text strings.Builder
	var reasoning strings.Builder
	var usage unified.Usage
	var toolCalls []unified.ToolCall
	toolBuilders := map[int]*toolCallBuilder{}
	finishReason := unified.FinishReasonUnknown
	var messageID string
	var sawCompleted bool

	for {
		select {
		case <-ctx.Done():
			return unified.Message{}, "", unified.Usage{}, nil, "", ctx.Err()
		case event, ok := <-events:
			if !ok {
				if !sawCompleted {
					return unified.Message{}, "", unified.Usage{}, nil, "", fmt.Errorf("runner: stream ended without completed event")
				}
				return assistantMessage(messageID, text.String(), reasoning.String(), toolCalls), finishReason, usage, toolCalls, messageID, nil
			}
			switch ev := event.(type) {
			case unified.MessageStartEvent:
				if ev.ID != "" {
					messageID = ev.ID
				}
			case unified.TextDeltaEvent:
				text.WriteString(ev.Text)
				emit(TextDeltaEvent{Step: meta.step, Text: ev.Text})
			case unified.ReasoningDeltaEvent:
				reasoning.WriteString(ev.Text)
				emit(ReasoningDeltaEvent{Step: meta.step, Text: ev.Text})
			case unified.ToolCallStartEvent:
				call := unified.ToolCall{ID: ev.ID, Name: ev.Name, Index: ev.Index}
				builder := ensureToolCallBuilder(toolBuilders, ev.Index)
				builder.id = firstNonEmpty(ev.ID, builder.id)
				builder.name = firstNonEmpty(ev.Name, builder.name)
				toolCalls = append(toolCalls, call)
				emit(ToolCallEvent{Step: meta.step, Call: call})
			case unified.ToolCallArgsDeltaEvent:
				builder := ensureToolCallBuilder(toolBuilders, ev.Index)
				builder.id = firstNonEmpty(ev.ID, builder.id)
				builder.args.WriteString(ev.Delta)
				if ev.ID != "" || ev.Delta != "" {
					updateToolCallArgs(&toolCalls, ev.Index, ev.ID, builder.name, builder.args.Bytes())
				}
				emit(ToolCallArgsDeltaEvent{Step: meta.step, Index: ev.Index, ID: ev.ID, Name: builder.name, Delta: ev.Delta})
			case unified.ToolCallDoneEvent:
				builder := ensureToolCallBuilder(toolBuilders, ev.Index)
				builder.id = firstNonEmpty(ev.ID, builder.id)
				builder.name = firstNonEmpty(ev.Name, builder.name)
				args := ev.Args
				if len(args) == 0 {
					args = builder.args.Bytes()
				}
				call := unified.ToolCall{ID: ev.ID, Name: ev.Name, Arguments: ev.Args, Index: ev.Index}
				call.ID = firstNonEmpty(call.ID, builder.id)
				call.Name = firstNonEmpty(call.Name, builder.name)
				call.Arguments = append(json.RawMessage(nil), args...)
				upsertToolCall(&toolCalls, call)
				emit(ToolCallEvent{Step: meta.step, Call: call})
			case unified.UsageEvent:
				usage = mergeUsage(usage, ev.Usage())
				emit(UsageEvent{Step: meta.step, Model: meta.model, ProviderIdentity: meta.providerIdentity, Usage: ev.Usage()})
			case unified.CompletedEvent:
				sawCompleted = true
				finishReason = ev.FinishReason
				if ev.MessageID != "" {
					messageID = ev.MessageID
				}
			case unified.ErrorEvent:
				if ev.Err != nil {
					return unified.Message{}, "", unified.Usage{}, nil, "", ev.Err
				}
				return unified.Message{}, "", unified.Usage{}, nil, "", fmt.Errorf("runner: provider stream error")
			case unified.WarningEvent:
				emit(WarningEvent{Step: meta.step, Warning: ev})
			case unified.RawEvent:
				emit(RawEvent{Step: meta.step, Raw: ev})
			}
		}
	}
}

func assistantMessage(id, text, reasoning string, toolCalls []unified.ToolCall) unified.Message {
	var content []unified.ContentPart
	if reasoning != "" {
		content = append(content, unified.ReasoningPart{Text: reasoning})
	}
	if text != "" {
		content = append(content, unified.TextPart{Text: text})
	}
	return unified.Message{
		Role:      unified.RoleAssistant,
		ID:        id,
		Content:   content,
		ToolCalls: append([]unified.ToolCall(nil), toolCalls...),
	}
}

func upsertToolCall(calls *[]unified.ToolCall, call unified.ToolCall) {
	for i, existing := range *calls {
		if existing.Index == call.Index || (call.ID != "" && existing.ID == call.ID) {
			(*calls)[i] = call
			return
		}
	}
	*calls = append(*calls, call)
}

func updateToolCallArgs(calls *[]unified.ToolCall, index int, id string, name string, args []byte) {
	for i, existing := range *calls {
		if existing.Index == index || (id != "" && existing.ID == id) {
			if id != "" {
				existing.ID = id
			}
			if name != "" {
				existing.Name = name
			}
			existing.Arguments = append(json.RawMessage(nil), args...)
			(*calls)[i] = existing
			return
		}
	}
	*calls = append(*calls, unified.ToolCall{ID: id, Name: name, Index: index, Arguments: append(json.RawMessage(nil), args...)})
}

type toolCallBuilder struct {
	id   string
	name string
	args bytes.Buffer
}

func ensureToolCallBuilder(builders map[int]*toolCallBuilder, index int) *toolCallBuilder {
	builder, ok := builders[index]
	if !ok {
		builder = &toolCallBuilder{}
		builders[index] = builder
	}
	return builder
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func mergeUsage(a, b unified.Usage) unified.Usage {
	a.Tokens = append(a.Tokens, b.Tokens...)
	a.Costs = append(a.Costs, b.Costs...)
	if len(b.ProviderRaw) > 0 && json.Valid(b.ProviderRaw) {
		a.ProviderRaw = append([]byte(nil), b.ProviderRaw...)
	}
	return a
}

func toolResultOutput(result unified.ToolResult) string {
	return textFromParts(result.Content)
}
