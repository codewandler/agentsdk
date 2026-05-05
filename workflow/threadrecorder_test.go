package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/thread"
	"github.com/stretchr/testify/require"
)

func TestThreadEventForWorkflowEventMapsConcreteEvents(t *testing.T) {
	tests := []struct {
		name  string
		event any
		kind  thread.EventKind
	}{
		{name: "started", event: Started{RunID: "run_1", WorkflowName: "echo"}, kind: EventStarted},
		{name: "step started", event: StepStarted{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1}, kind: EventStepStarted},
		{name: "step completed", event: StepCompleted{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1, Output: InlineValue("ok")}, kind: EventStepCompleted},
		{name: "step failed", event: StepFailed{RunID: "run_1", WorkflowName: "echo", StepID: "echo", ActionName: "echo", Attempt: 1, Error: "boom"}, kind: EventStepFailed},
		{name: "completed", event: Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("ok")}, kind: EventCompleted},
		{name: "failed", event: Failed{RunID: "run_1", WorkflowName: "echo", Error: "boom"}, kind: EventFailed},
	}

	registry, err := thread.NewEventRegistry(EventDefinitions()...)
	require.NoError(t, err)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, ok, err := ThreadEventForWorkflowEvent(tt.event)
			require.NoError(t, err)
			require.True(t, ok)
			require.Equal(t, tt.kind, event.Kind)
			require.NotEmpty(t, event.Payload)
			require.NoError(t, registry.Validate(event))
		})
	}
}

func TestThreadEventForWorkflowEventIgnoresNonWorkflowEvents(t *testing.T) {
	event, ok, err := ThreadEventForWorkflowEvent("ordinary action event")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, thread.Event{}, event)
}

func TestThreadRecorderRecordsWorkflowEvents(t *testing.T) {
	ctx := action.NewCtx(context.Background())
	registry, err := thread.NewEventRegistry(append(thread.CoreEventDefinitions(), EventDefinitions()...)...)
	require.NoError(t, err)
	store := thread.NewMemoryStore(thread.WithEventRegistry(registry))
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_1"})
	require.NoError(t, err)
	recorder := &ThreadRecorder{Live: live}

	require.NoError(t, recorder.Record(ctx, Started{RunID: "run_1", WorkflowName: "echo"}))
	require.NoError(t, recorder.Record(ctx, "ordinary action event"))
	require.NoError(t, recorder.Record(ctx, Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("ok")}))

	stored, err := store.Read(ctx, thread.ReadParams{ID: "thread_1"})
	require.NoError(t, err)
	events, err := stored.EventsForBranch(thread.MainBranch)
	require.NoError(t, err)
	require.Len(t, events, 3)
	require.Equal(t, thread.EventThreadCreated, events[0].Kind)
	require.Equal(t, EventStarted, events[1].Kind)
	require.Equal(t, EventCompleted, events[2].Kind)

	var started Started
	require.NoError(t, json.Unmarshal(events[1].Payload, &started))
	require.Equal(t, Started{RunID: "run_1", WorkflowName: "echo"}, started)
}

func TestThreadRecorderOnEventCollectsErrors(t *testing.T) {
	recorder := &ThreadRecorder{}
	recorder.OnEvent(action.NewCtx(context.Background()), Started{RunID: "run_1", WorkflowName: "echo"})

	require.Error(t, recorder.Err())
}

func TestThreadRecorderRecordsExecutorEvents(t *testing.T) {
	ctx := action.NewCtx(context.Background())
	registry, err := thread.NewEventRegistry(append(thread.CoreEventDefinitions(), EventDefinitions()...)...)
	require.NoError(t, err)
	store := thread.NewMemoryStore(thread.WithEventRegistry(registry))
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_1"})
	require.NoError(t, err)
	recorder := &ThreadRecorder{Live: live}

	actions := action.NewRegistry()
	require.NoError(t, actions.Register(action.New(action.Spec{Name: "echo"}, func(action.Ctx, any) action.Result {
		return action.Result{Data: "ok"}
	})))
	result := Executor{Resolver: RegistryResolver{Registry: actions}, RunID: "run_1", OnEvent: recorder.OnEvent}.Execute(ctx, Definition{Name: "echo", Steps: []Step{{ID: "echo", Action: ActionRef{Name: "echo"}}}}, nil)
	require.NoError(t, result.Error)
	require.NoError(t, recorder.Err())

	stored, err := store.Read(ctx, thread.ReadParams{ID: "thread_1"})
	require.NoError(t, err)
	events, err := stored.EventsForBranch(thread.MainBranch)
	require.NoError(t, err)
	require.Len(t, events, 5)
	require.Equal(t, []thread.EventKind{
		thread.EventThreadCreated,
		EventStarted,
		EventStepStarted,
		EventStepCompleted,
		EventCompleted,
	}, []thread.EventKind{events[0].Kind, events[1].Kind, events[2].Kind, events[3].Kind, events[4].Kind})
}
