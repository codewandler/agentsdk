package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/tool"
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
	toolMap := make(map[string]tool.Tool, len(options.Tools))
	for _, t := range options.Tools {
		toolMap[t.Name()] = t
	}
	if options.ToolCtx == nil {
		options.ToolCtx = &basicToolCtx{
			Context:   ctx,
			sessionID: string(session.SessionID()),
			extra:     map[string]any{},
		}
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
		events, err := client.Request(ctx, wireReq)
		if err != nil {
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}

		assistant, finishReason, usage, toolCalls, messageID, err := consumeEvents(ctx, events, emit)
		if err != nil {
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}
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
			emit(CompletedEvent{FinishReason: finishReason})
			return result, nil
		}

		transcript = append(transcript, assistant)
		results := make([]unified.ToolResult, 0, len(toolCalls))
		for _, call := range toolCalls {
			toolResult := executeTool(ctx, options.ToolCtx, toolMap, call)
			results = append(results, toolResult)
			output := textFromParts(toolResult.Content)
			emit(ToolResultEvent{CallID: toolResult.ToolCallID, Name: toolResult.Name, Output: output, IsError: toolResult.IsError})
		}
		transcript = append(transcript, unified.Message{Role: unified.RoleTool, ToolResults: results})
	}

	err := ErrMaxStepsReached
	fragment.Fail(err)
	emit(ErrorEvent{Err: err})
	return result, err
}

func consumeEvents(ctx context.Context, events <-chan unified.Event, emit func(Event)) (unified.Message, unified.FinishReason, unified.Usage, []unified.ToolCall, string, error) {
	var text strings.Builder
	var reasoning strings.Builder
	var usage unified.Usage
	var toolCalls []unified.ToolCall
	finishReason := unified.FinishReasonUnknown
	var messageID string

	for {
		select {
		case <-ctx.Done():
			return unified.Message{}, "", unified.Usage{}, nil, "", ctx.Err()
		case event, ok := <-events:
			if !ok {
				return assistantMessage(messageID, text.String(), reasoning.String(), toolCalls), finishReason, usage, toolCalls, messageID, nil
			}
			switch ev := event.(type) {
			case unified.MessageStartEvent:
				if ev.ID != "" {
					messageID = ev.ID
				}
			case unified.TextDeltaEvent:
				text.WriteString(ev.Text)
				emit(TextDeltaEvent{Text: ev.Text})
			case unified.ReasoningDeltaEvent:
				reasoning.WriteString(ev.Text)
				emit(ReasoningDeltaEvent{Text: ev.Text})
			case unified.ToolCallStartEvent:
				call := unified.ToolCall{ID: ev.ID, Name: ev.Name, Index: ev.Index}
				toolCalls = append(toolCalls, call)
				emit(ToolCallEvent{Call: call})
			case unified.ToolCallDoneEvent:
				call := unified.ToolCall{ID: ev.ID, Name: ev.Name, Arguments: ev.Args, Index: ev.Index}
				upsertToolCall(&toolCalls, call)
				emit(ToolCallEvent{Call: call})
			case unified.UsageEvent:
				usage = mergeUsage(usage, ev.Usage())
				emit(UsageEvent{Usage: ev.Usage()})
			case unified.CompletedEvent:
				finishReason = ev.FinishReason
				if ev.MessageID != "" {
					messageID = ev.MessageID
				}
			case unified.ErrorEvent:
				if ev.Err != nil {
					return unified.Message{}, "", unified.Usage{}, nil, "", ev.Err
				}
				return unified.Message{}, "", unified.Usage{}, nil, "", fmt.Errorf("runner: provider stream error")
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

func executeTool(ctx context.Context, toolCtx tool.Ctx, tools map[string]tool.Tool, call unified.ToolCall) unified.ToolResult {
	t, ok := tools[call.Name]
	if !ok {
		return toolResult(call, fmt.Sprintf("tool %q not found", call.Name), true)
	}
	if err := ctx.Err(); err != nil {
		return toolResult(call, err.Error(), true)
	}
	res, err := t.Execute(toolCtx, call.Arguments)
	if err != nil {
		return toolResult(call, err.Error(), true)
	}
	if res == nil {
		return toolResult(call, "", false)
	}
	return toolResult(call, res.String(), res.IsError())
}

func toolResult(call unified.ToolCall, output string, isError bool) unified.ToolResult {
	return unified.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    []unified.ContentPart{unified.TextPart{Text: output}},
		IsError:    isError,
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

func mergeUsage(a, b unified.Usage) unified.Usage {
	a.Tokens = append(a.Tokens, b.Tokens...)
	a.Costs = append(a.Costs, b.Costs...)
	if len(b.ProviderRaw) > 0 && json.Valid(b.ProviderRaw) {
		a.ProviderRaw = append([]byte(nil), b.ProviderRaw...)
	}
	return a
}

func textFromParts(parts []unified.ContentPart) string {
	var out []string
	for _, part := range parts {
		if text, ok := part.(unified.TextPart); ok {
			out = append(out, text.Text)
		}
	}
	return strings.Join(out, "\n")
}
