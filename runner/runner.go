package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
)

var ErrMaxStepsReached = errors.New("runner: max steps reached")

type Result struct {
	Events []Event
	Steps  int
}

type History interface {
	SessionID() string
	Tree() *conversation.Tree
	Branch() conversation.BranchID
	BuildRequestForProvider(conversation.Request, conversation.ProviderIdentity) (unified.Request, error)
	CommitFragment(*conversation.TurnFragment) ([]conversation.NodeID, error)
}

type threadEventHistory interface {
	AppendThreadEvents(context.Context, ...thread.Event) error
	CommitFragmentWithThreadEvents(context.Context, *conversation.TurnFragment, ...thread.Event) ([]conversation.NodeID, error)
}

func RunTurn(ctx context.Context, history History, client unified.Client, req conversation.Request, opts ...Option) (Result, error) {
	if history == nil {
		return Result{}, fmt.Errorf("runner: history is required")
	}
	if client == nil {
		return Result{}, fmt.Errorf("runner: client is required")
	}
	options := applyOptions(opts)
	if options.ToolCtx == nil {
		options.ToolCtx = &basicToolCtx{
			Context:   ctx,
			sessionID: history.SessionID(),
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
	currentProviderIdentity := options.ProviderIdentity

	for step := 0; step < options.MaxSteps; step++ {
		result.Steps++
		stepReq := req
		stepReq.Messages = append([]unified.Message(nil), transcript...)
		var prepared PreparedRequest
		if options.RequestPreparer != nil {
			nativeContinuation, err := nativeContinuationAvailable(history, currentProviderIdentity)
			if err != nil {
				fragment.Fail(err)
				emit(ErrorEvent{Err: err})
				return result, err
			}
			prepared, err = options.RequestPreparer(ctx, RequestPrepareMeta{
				Step:               result.Steps,
				ProviderIdentity:   currentProviderIdentity,
				NativeContinuation: nativeContinuation,
			}, stepReq)
			if err != nil {
				fragment.Fail(err)
				emit(ErrorEvent{Err: err})
				return result, err
			}
			stepReq = prepared.Request
		}
		wireReq, err := history.BuildRequestForProvider(stepReq, currentProviderIdentity)
		if err != nil {
			rollbackPreparedRequest(ctx, prepared)
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}
		emit(StepStartEvent{Step: result.Steps, MaxSteps: options.MaxSteps, Model: wireReq.Model})
		events, err := client.Request(ctx, wireReq)
		if err != nil {
			rollbackPreparedRequest(ctx, prepared)
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}

		assistant, finishReason, usage, toolCalls, messageID, providerIdentity, routeEvent, executionEvent, err := consumeEvents(ctx, events, emit, eventContext{
			step:             result.Steps,
			model:            wireReq.Model,
			providerIdentity: currentProviderIdentity,
		})
		if err != nil {
			rollbackPreparedRequest(ctx, prepared)
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
		}
		currentProviderIdentity = providerIdentity
		emit(StepDoneEvent{
			Step:             result.Steps,
			MaxSteps:         options.MaxSteps,
			Model:            wireReq.Model,
			ProviderIdentity: providerIdentity,
			Usage:            usage,
			FinishReason:     finishReason,
		})
		if messageID != "" {
			if reusableMessageID(messageID) {
				assistant.ID = messageID
			} else {
				assistant.ID = ""
			}
			fragment.AddContinuation(conversation.NewProviderContinuationFromRoute(providerIdentity, messageID, routeEvent, executionEvent, unified.Extensions{}))
		}

		if len(toolCalls) == 0 {
			fragment.AddRequestMessages(transcript...)
			fragment.SetAssistantMessage(assistant)
			fragment.SetUsage(usage)
			fragment.Complete(finishReason)
			if _, err := commitFinalFragment(ctx, history, prepared, fragment); err != nil {
				emit(ErrorEvent{Err: err})
				return result, err
			}
			emit(CompletedEvent{Step: result.Steps, FinishReason: finishReason})
			return result, nil
		}

		if err := commitPreparedRequest(ctx, history, prepared); err != nil {
			fragment.Fail(err)
			emit(ErrorEvent{Err: err})
			return result, err
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

func nativeContinuationAvailable(history History, identity conversation.ProviderIdentity) (bool, error) {
	if history == nil {
		return false, nil
	}
	continuation, ok, err := conversation.ContinuationAtBranchHead(history.Tree(), history.Branch(), identity)
	if err != nil {
		return false, err
	}
	return ok && continuation.SupportsPublicPreviousResponseID(), nil
}

func commitPreparedRequest(ctx context.Context, history History, prepared PreparedRequest) error {
	if len(prepared.ThreadEvents) > 0 {
		threadHistory, ok := history.(threadEventHistory)
		if !ok {
			rollbackPreparedRequest(ctx, prepared)
			return fmt.Errorf("runner: prepared thread events require thread-backed history")
		}
		if err := threadHistory.AppendThreadEvents(ctx, prepared.ThreadEvents...); err != nil {
			rollbackPreparedRequest(ctx, prepared)
			return err
		}
	}
	if prepared.Commit == nil {
		return nil
	}
	return prepared.Commit(ctx)
}

func commitFinalFragment(ctx context.Context, history History, prepared PreparedRequest, fragment *conversation.TurnFragment) ([]conversation.NodeID, error) {
	if len(prepared.ThreadEvents) > 0 {
		threadHistory, ok := history.(threadEventHistory)
		if !ok {
			rollbackPreparedRequest(ctx, prepared)
			return nil, fmt.Errorf("runner: prepared thread events require thread-backed history")
		}
		ids, err := threadHistory.CommitFragmentWithThreadEvents(ctx, fragment, prepared.ThreadEvents...)
		if err != nil {
			rollbackPreparedRequest(ctx, prepared)
			return nil, err
		}
		if prepared.Commit != nil {
			if err := prepared.Commit(ctx); err != nil {
				return ids, err
			}
		}
		return ids, nil
	}
	ids, err := history.CommitFragment(fragment)
	if err != nil {
		rollbackPreparedRequest(ctx, prepared)
		return nil, err
	}
	if prepared.Commit != nil {
		if err := prepared.Commit(ctx); err != nil {
			return ids, err
		}
	}
	return ids, nil
}

func rollbackPreparedRequest(ctx context.Context, prepared PreparedRequest) {
	if prepared.Rollback != nil {
		prepared.Rollback(ctx)
	}
}

type eventContext struct {
	step             int
	model            string
	providerIdentity conversation.ProviderIdentity
}

func consumeEvents(ctx context.Context, events <-chan unified.Event, emit func(Event), meta eventContext) (unified.Message, unified.FinishReason, unified.Usage, []unified.ToolCall, string, conversation.ProviderIdentity, unified.RouteEvent, unified.ProviderExecutionEvent, error) {
	var text strings.Builder
	var reasoning strings.Builder
	var reasoningSignature strings.Builder
	var usage unified.Usage
	var toolCalls []unified.ToolCall
	toolBuilders := map[int]*toolCallBuilder{}
	finishReason := unified.FinishReasonUnknown
	providerIdentity := meta.providerIdentity
	var routeEvent unified.RouteEvent
	var executionEvent unified.ProviderExecutionEvent
	var messageID string
	var sawCompleted bool

	for {
		select {
		case <-ctx.Done():
			return unified.Message{}, "", unified.Usage{}, nil, "", providerIdentity, routeEvent, executionEvent, ctx.Err()
		case event, ok := <-events:
			if !ok {
				if !sawCompleted {
					return unified.Message{}, "", unified.Usage{}, nil, "", providerIdentity, routeEvent, executionEvent, fmt.Errorf("runner: stream ended without completed event")
				}
				return assistantMessage(messageID, text.String(), reasoning.String(), reasoningSignature.String(), toolCalls), finishReason, usage, toolCalls, messageID, providerIdentity, routeEvent, executionEvent, nil
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
				reasoningSignature.WriteString(ev.Signature)
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
				emit(UsageEvent{Step: meta.step, Model: meta.model, ProviderIdentity: providerIdentity, Usage: ev.Usage()})
			case unified.CompletedEvent:
				sawCompleted = true
				finishReason = ev.FinishReason
				if ev.MessageID != "" {
					messageID = ev.MessageID
				}
			case unified.ErrorEvent:
				if ev.Err != nil {
					return unified.Message{}, "", unified.Usage{}, nil, "", providerIdentity, routeEvent, executionEvent, ev.Err
				}
				return unified.Message{}, "", unified.Usage{}, nil, "", providerIdentity, routeEvent, executionEvent, fmt.Errorf("runner: provider stream error")
			case unified.WarningEvent:
				emit(WarningEvent{Step: meta.step, Warning: ev})
			case unified.RawEvent:
				emit(RawEvent{Step: meta.step, Raw: ev})
			case unified.RouteEvent:
				routeEvent = ev
				providerIdentity = providerIdentityFromRouteEvent(ev)
				emit(RouteEvent{Step: meta.step, Route: ev, ProviderIdentity: providerIdentity})
			case unified.ProviderExecutionEvent:
				executionEvent = ev
				emit(ProviderExecutionEvent{Step: meta.step, Execution: ev})
			}
		}
	}
}

func providerIdentityFromRouteEvent(ev unified.RouteEvent) conversation.ProviderIdentity {
	return conversation.ProviderIdentity{
		ProviderName: ev.ProviderName,
		APIKind:      ev.TargetAPI,
		APIFamily:    ev.TargetFamily,
		NativeModel:  ev.NativeModel,
	}
}

func assistantMessage(id, text, reasoning string, reasoningSignature string, toolCalls []unified.ToolCall) unified.Message {
	var content []unified.ContentPart
	if reasoning != "" || reasoningSignature != "" {
		content = append(content, unified.ReasoningPart{Text: reasoning, Signature: reasoningSignature})
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

func reusableMessageID(id string) bool {
	return !strings.HasPrefix(id, "resp_")
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
