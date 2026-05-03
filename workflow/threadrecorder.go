package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/thread"
)

// ThreadRecorder records live workflow events into a thread log. It is intended
// to be wired through Executor.OnEvent by harness/session code; Executor itself
// remains persistence-agnostic.
type ThreadRecorder struct {
	Live thread.Live

	mu  sync.Mutex
	err error
}

// OnEvent adapts ThreadRecorder to Executor.OnEvent. Recording errors are
// retained and can be inspected with Err after execution.
func (r *ThreadRecorder) OnEvent(ctx action.Ctx, event action.Event) {
	if err := r.Record(ctx, event); err != nil {
		r.mu.Lock()
		r.err = errors.Join(r.err, err)
		r.mu.Unlock()
	}
}

// Err returns any errors collected by OnEvent.
func (r *ThreadRecorder) Err() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

// Record appends event to the configured live thread when event is a concrete
// workflow execution event. Non-workflow events are ignored.
func (r *ThreadRecorder) Record(ctx context.Context, event any) error {
	threadEvent, ok, err := ThreadEventForWorkflowEvent(event)
	if err != nil || !ok {
		return err
	}
	if r == nil || r.Live == nil {
		return fmt.Errorf("workflow: thread recorder live thread is nil")
	}
	return r.Live.Append(ctx, threadEvent)
}

// ThreadEventForWorkflowEvent maps a concrete workflow event payload to the
// persistent thread event shape. Unsupported non-workflow events return ok=false.
func ThreadEventForWorkflowEvent(event any) (thread.Event, bool, error) {
	kind, payload, ok := workflowThreadEventPayload(event)
	if !ok {
		return thread.Event{}, false, nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return thread.Event{}, false, fmt.Errorf("workflow: marshal thread event %q: %w", kind, err)
	}
	return thread.Event{Kind: kind, Payload: raw}, true, nil
}

func workflowThreadEventPayload(event any) (thread.EventKind, any, bool) {
	switch e := event.(type) {
	case Queued:
		return EventQueued, e, true
	case Started:
		return EventStarted, e, true
	case StepStarted:
		return EventStepStarted, e, true
	case StepCompleted:
		return EventStepCompleted, e, true
	case StepFailed:
		return EventStepFailed, e, true
	case Completed:
		return EventCompleted, e, true
	case Failed:
		return EventFailed, e, true
	case Canceled:
		return EventCanceled, e, true
	default:
		return "", nil, false
	}
}
