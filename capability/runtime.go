package capability

import (
	"context"
	"fmt"

	"github.com/codewandler/agentsdk/thread"
)

type Runtime interface {
	ThreadID() thread.ID
	BranchID() thread.BranchID
	Source() thread.EventSource
	AppendEvents(context.Context, ...thread.Event) error
}

type LiveRuntime struct {
	Live        thread.Live
	SourceValue thread.EventSource
}

func NewRuntime(live thread.Live, source thread.EventSource) LiveRuntime {
	return LiveRuntime{Live: live, SourceValue: source}
}

func (r LiveRuntime) ThreadID() thread.ID {
	if r.Live == nil {
		return ""
	}
	return r.Live.ID()
}

func (r LiveRuntime) BranchID() thread.BranchID {
	if r.Live == nil {
		return ""
	}
	return r.Live.BranchID()
}

func (r LiveRuntime) Source() thread.EventSource {
	return r.SourceValue
}

func (r LiveRuntime) AppendEvents(ctx context.Context, events ...thread.Event) error {
	if r.Live == nil {
		return fmt.Errorf("capability: live thread is required")
	}
	for i := range events {
		if events[i].ThreadID == "" {
			events[i].ThreadID = r.ThreadID()
		}
		if events[i].BranchID == "" {
			events[i].BranchID = r.BranchID()
		}
		if events[i].Source.Type == "" && events[i].Source.ID == "" && events[i].Source.SessionID == "" {
			events[i].Source = r.SourceValue
		}
	}
	return r.Live.Append(ctx, events...)
}

// ThreadEventObserver is called after thread events are successfully persisted.
// It receives the same events that were appended to the thread log.
type ThreadEventObserver func(ctx context.Context, events []thread.Event)

// ObservableRuntime wraps a Runtime and calls an observer after every
// successful AppendEvents. This allows presentation layers (runner event
// system, UI) to observe all persisted thread events — capability state,
// compaction, context fragments — without polling.
type ObservableRuntime struct {
	Inner    Runtime
	Observer ThreadEventObserver
}

func (r ObservableRuntime) ThreadID() thread.ID          { return r.Inner.ThreadID() }
func (r ObservableRuntime) BranchID() thread.BranchID    { return r.Inner.BranchID() }
func (r ObservableRuntime) Source() thread.EventSource   { return r.Inner.Source() }

func (r ObservableRuntime) AppendEvents(ctx context.Context, events ...thread.Event) error {
	if err := r.Inner.AppendEvents(ctx, events...); err != nil {
		return err
	}
	if r.Observer != nil {
		r.Observer(ctx, events)
	}
	return nil
}
