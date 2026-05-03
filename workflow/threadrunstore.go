package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/codewandler/agentsdk/thread"
)

// ThreadRunStore stores and reads workflow run events from a thread log. A
// ThreadRunStore is scoped to one thread and, optionally, one branch.
type ThreadRunStore struct {
	Store    thread.Store
	Live     thread.Live
	ThreadID thread.ID
	BranchID thread.BranchID
}

// Append records workflow events for runID into the configured live thread.
// Non-workflow events are ignored.
func (s ThreadRunStore) Append(ctx context.Context, runID RunID, events ...any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	if s.Live == nil {
		return fmt.Errorf("workflow: thread run store live thread is nil")
	}
	threadEvents := make([]thread.Event, 0, len(events))
	for _, event := range events {
		if workflowEventRunID(event) != "" && workflowEventRunID(event) != runID {
			return fmt.Errorf("workflow: event run id %q does not match append run id %q", workflowEventRunID(event), runID)
		}
		threadEvent, ok, err := ThreadEventForWorkflowEvent(event)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		threadEvents = append(threadEvents, threadEvent)
	}
	if len(threadEvents) == 0 {
		return nil
	}
	return s.Live.Append(ctx, threadEvents...)
}

// Events returns concrete workflow events for runID from the configured thread
// and branch.
func (s ThreadRunStore) Events(ctx context.Context, runID RunID) ([]any, bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if s.Store == nil {
		return nil, false, fmt.Errorf("workflow: thread run store thread store is nil")
	}
	threadID := s.ThreadID
	if threadID == "" && s.Live != nil {
		threadID = s.Live.ID()
	}
	if threadID == "" {
		return nil, false, fmt.Errorf("workflow: thread run store thread id is required")
	}
	stored, err := s.Store.Read(ctx, thread.ReadParams{ID: threadID})
	if err != nil {
		return nil, false, err
	}
	branchEvents, err := stored.EventsForBranch(s.BranchID)
	if err != nil {
		return nil, false, err
	}
	out := make([]any, 0)
	for _, event := range branchEvents {
		payload, ok, err := WorkflowEventForThreadEvent(event)
		if err != nil {
			return nil, false, err
		}
		if !ok || workflowEventRunID(payload) != runID {
			continue
		}
		out = append(out, payload)
	}
	if len(out) == 0 {
		return nil, false, nil
	}
	return out, true, nil
}

// Runs returns projected summaries for all workflow runs recorded in the
// configured thread and branch.
func (s ThreadRunStore) Runs(ctx context.Context) ([]RunSummary, error) {
	events, err := s.allEvents(ctx)
	if err != nil {
		return nil, err
	}
	states, err := Projector{}.Project(events)
	if err != nil {
		return nil, err
	}
	summaries := make([]RunSummary, 0, len(states))
	for _, state := range states {
		summaries = append(summaries, RunSummary{
			ID:                state.ID,
			WorkflowName:      state.WorkflowName,
			Status:            state.Status,
			StartedAt:         state.StartedAt,
			CompletedAt:       state.CompletedAt,
			Duration:          state.Duration,
			Error:             state.Error,
			Metadata:          state.Metadata,
			Input:             state.Input,
			DefinitionHash:    state.DefinitionHash,
			DefinitionVersion: state.DefinitionVersion,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		left, right := summaries[i], summaries[j]
		if !left.StartedAt.Equal(right.StartedAt) {
			if left.StartedAt.IsZero() {
				return false
			}
			if right.StartedAt.IsZero() {
				return true
			}
			return left.StartedAt.Before(right.StartedAt)
		}
		return left.ID < right.ID
	})
	return summaries, nil
}

// State projects the current state for runID from thread-backed workflow events.
func (s ThreadRunStore) State(ctx context.Context, runID RunID) (RunState, bool, error) {
	events, ok, err := s.Events(ctx, runID)
	if err != nil || !ok {
		return RunState{}, ok, err
	}
	return Projector{}.ProjectRun(events, runID)
}

func (s ThreadRunStore) allEvents(ctx context.Context) ([]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.Store == nil {
		return nil, fmt.Errorf("workflow: thread run store thread store is nil")
	}
	threadID := s.ThreadID
	if threadID == "" && s.Live != nil {
		threadID = s.Live.ID()
	}
	if threadID == "" {
		return nil, fmt.Errorf("workflow: thread run store thread id is required")
	}
	stored, err := s.Store.Read(ctx, thread.ReadParams{ID: threadID})
	if err != nil {
		return nil, err
	}
	branchEvents, err := stored.EventsForBranch(s.BranchID)
	if err != nil {
		return nil, err
	}
	out := make([]any, 0)
	for _, event := range branchEvents {
		payload, ok, err := WorkflowEventForThreadEvent(event)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		out = append(out, payload)
	}
	return out, nil
}

// WorkflowEventForThreadEvent decodes a persistent thread event into its
// concrete workflow payload. Unsupported non-workflow event kinds return
// ok=false.
func WorkflowEventForThreadEvent(event thread.Event) (any, bool, error) {
	switch event.Kind {
	case EventQueued:
		var payload Queued
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	case EventStarted:
		var payload Started
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	case EventStepStarted:
		var payload StepStarted
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	case EventStepCompleted:
		var payload StepCompleted
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	case EventStepFailed:
		var payload StepFailed
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	case EventStepSkipped:
		var payload StepSkipped
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	case EventCompleted:
		var payload Completed
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	case EventFailed:
		var payload Failed
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	case EventCanceled:
		var payload Canceled
		return payload, true, decodeWorkflowThreadEvent(event, &payload)
	default:
		return nil, false, nil
	}
}

func decodeWorkflowThreadEvent(event thread.Event, target any) error {
	if len(event.Payload) == 0 {
		return fmt.Errorf("workflow: thread event %q payload is required", event.Kind)
	}
	if err := json.Unmarshal(event.Payload, target); err != nil {
		return fmt.Errorf("workflow: decode thread event %q: %w", event.Kind, err)
	}
	return nil
}

func workflowEventRunID(event any) RunID {
	switch e := event.(type) {
	case Queued:
		return e.RunID
	case Started:
		return e.RunID
	case StepStarted:
		return e.RunID
	case StepCompleted:
		return e.RunID
	case StepFailed:
		return e.RunID
	case StepSkipped:
		return e.RunID
	case Completed:
		return e.RunID
	case Failed:
		return e.RunID
	case Canceled:
		return e.RunID
	default:
		return ""
	}
}
