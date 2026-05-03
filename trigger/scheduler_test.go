package trigger

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMatcherComposition(t *testing.T) {
	event := Event{Type: EventTypeInterval, SourceID: "daily", Subject: "docs/readme.md"}

	ok, err := All{EventType(EventTypeInterval), SourceIs("daily"), SubjectGlob("docs/*.md")}.Match(event)
	require.NoError(t, err)
	require.True(t, ok)

	ok, err = Any{SourceIs("other"), SubjectGlob("*.go")}.Match(event)
	require.NoError(t, err)
	require.False(t, ok)
}

func TestSchedulerRunsIntervalRuleAndSkipsOverlap(t *testing.T) {
	executor := &blockingExecutor{started: make(chan struct{}), release: make(chan struct{})}
	scheduler := NewScheduler(executor)
	source := manualSource{id: "manual", events: make(chan Event, 2)}
	require.NoError(t, scheduler.AddRule(context.Background(), Rule{
		ID:      "rule",
		Source:  source,
		Matcher: EventType(EventTypeInterval),
		Target:  Target{Kind: TargetWorkflow, WorkflowName: "daily"},
	}))

	source.events <- Event{ID: "evt_1", Type: EventTypeInterval, SourceID: source.id, At: time.Now()}
	<-executor.started
	source.events <- Event{ID: "evt_2", Type: EventTypeInterval, SourceID: source.id, At: time.Now()}
	require.Eventually(t, func() bool {
		jobs := scheduler.Jobs()
		return len(jobs) == 1 && jobs[0].Skipped == 1
	}, time.Second, 10*time.Millisecond)
	close(executor.release)
	require.Eventually(t, func() bool {
		jobs := scheduler.Jobs()
		return len(jobs) == 1 && jobs[0].Matched == 1 && !jobs[0].Running
	}, time.Second, 10*time.Millisecond)
	require.NoError(t, scheduler.StopJob("rule"))
}

type manualSource struct {
	id     SourceID
	events chan Event
}

func (s manualSource) ID() SourceID      { return s.id }
func (s manualSource) EventType() string { return EventTypeInterval }
func (s manualSource) Start(ctx context.Context, emit EmitFunc) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case event := <-s.events:
			emit(event)
		}
	}
}

type blockingExecutor struct {
	started chan struct{}
	release chan struct{}
}

func (e *blockingExecutor) ExecuteTrigger(ctx context.Context, execution Execution) (ExecutionResult, error) {
	select {
	case e.started <- struct{}{}:
	default:
	}
	select {
	case <-ctx.Done():
		return ExecutionResult{}, ctx.Err()
	case <-e.release:
		return ExecutionResult{TargetKind: execution.Rule.Target.Kind, TargetName: execution.Rule.Target.Name()}, nil
	}
}
