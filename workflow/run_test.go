package workflow

import (
	"context"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/stretchr/testify/require"
)

func TestValueRefHelpers(t *testing.T) {
	require.Equal(t, ValueRef{Inline: "ok"}, InlineValue("ok"))
	require.Equal(t, ValueRef{}, InlineValue(nil))
	require.Equal(t, ValueRef{ExternalURI: "s3://bucket/value.json", MediaType: "application/json"}, ExternalValue("s3://bucket/value.json", "application/json"))
	require.Equal(t, ValueRef{ID: "secret-output", Redacted: true}, RedactedValue("secret-output"))
}

func TestProjectorMaterializesQueuedAndCanceledRunState(t *testing.T) {
	queuedAt := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	startedAt := queuedAt.Add(time.Second)
	canceledAt := startedAt.Add(2 * time.Second)
	metadata := RunMetadata{SessionID: "session_1", AgentName: "coder", ThreadID: "thread_1", BranchID: "main", Trigger: "command", CommandPath: []string{"workflow", "start"}}
	events := []any{
		Queued{RunID: "run_1", WorkflowName: "slow", Metadata: metadata, Input: InlineValue("hello"), DefinitionHash: "hash", DefinitionVersion: "v1", At: queuedAt},
		Started{RunID: "run_1", WorkflowName: "slow", At: startedAt},
		Canceled{RunID: "run_1", WorkflowName: "slow", Reason: "stop", At: canceledAt},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunCanceled, state.Status)
	require.Equal(t, startedAt, state.StartedAt)
	require.Equal(t, canceledAt, state.CompletedAt)
	require.Equal(t, 2*time.Second, state.Duration)
	require.Equal(t, "stop", state.Error)
	require.Equal(t, metadata, state.Metadata)
	require.Equal(t, InlineValue("hello"), state.Input)
	require.Equal(t, "hash", state.DefinitionHash)
	require.Equal(t, "v1", state.DefinitionVersion)
}

func TestProjectorMaterializesSuccessfulRunState(t *testing.T) {
	startedAt := time.Date(2026, 5, 2, 13, 0, 0, 0, time.UTC)
	completedAt := startedAt.Add(1500 * time.Millisecond)
	events := []any{
		Started{RunID: "run_1", WorkflowName: "shout", At: startedAt},
		StepStarted{RunID: "run_1", WorkflowName: "shout", StepID: "upper", ActionName: "upper", Attempt: 1, At: startedAt.Add(100 * time.Millisecond)},
		"ignored action event",
		StepCompleted{RunID: "run_1", WorkflowName: "shout", StepID: "upper", ActionName: "upper", Attempt: 1, Output: InlineValue("HELLO"), At: completedAt.Add(-100 * time.Millisecond)},
		Completed{RunID: "run_1", WorkflowName: "shout", Output: InlineValue("HELLO"), At: completedAt},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunID("run_1"), state.ID)
	require.Equal(t, "shout", state.WorkflowName)
	require.Equal(t, RunSucceeded, state.Status)
	require.Equal(t, startedAt, state.StartedAt)
	require.Equal(t, completedAt, state.CompletedAt)
	require.Equal(t, 1500*time.Millisecond, state.Duration)
	require.Equal(t, InlineValue("HELLO"), state.Output)
	require.Equal(t, StepSucceeded, state.Steps["upper"].Status)
	require.Equal(t, 1, state.Steps["upper"].Attempt)
	require.Equal(t, InlineValue("HELLO"), state.Steps["upper"].Output)
	require.Equal(t, []AttemptState{{Attempt: 1, Status: StepSucceeded, Output: InlineValue("HELLO")}}, state.Steps["upper"].Attempts)
}

func TestProjectorMaterializesFailedRunState(t *testing.T) {
	startedAt := time.Date(2026, 5, 2, 13, 0, 0, 0, time.UTC)
	failedAt := startedAt.Add(2 * time.Second)
	events := []any{
		Started{RunID: "run_1", WorkflowName: "failflow", At: startedAt},
		StepStarted{RunID: "run_1", WorkflowName: "failflow", StepID: "fail", ActionName: "fail", Attempt: 1, At: startedAt.Add(100 * time.Millisecond)},
		StepFailed{RunID: "run_1", WorkflowName: "failflow", StepID: "fail", ActionName: "fail", Attempt: 1, Error: "boom", At: failedAt.Add(-100 * time.Millisecond)},
		Failed{RunID: "run_1", WorkflowName: "failflow", Error: "workflow failed: boom", At: failedAt},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunFailed, state.Status)
	require.Equal(t, startedAt, state.StartedAt)
	require.Equal(t, failedAt, state.CompletedAt)
	require.Equal(t, 2*time.Second, state.Duration)
	require.Equal(t, "workflow failed: boom", state.Error)
	require.Equal(t, StepFailedStatus, state.Steps["fail"].Status)
	require.Equal(t, 1, state.Steps["fail"].Attempt)
	require.Equal(t, "boom", state.Steps["fail"].Error)
	require.Equal(t, []AttemptState{{Attempt: 1, Status: StepFailedStatus, Error: "boom"}}, state.Steps["fail"].Attempts)
}

func TestProjectorKeepsRunsSeparate(t *testing.T) {
	events := []any{
		Started{RunID: "run_1", WorkflowName: "echo"},
		Completed{RunID: "run_1", WorkflowName: "echo", Output: InlineValue("one")},
		Started{RunID: "run_2", WorkflowName: "echo"},
		Completed{RunID: "run_2", WorkflowName: "echo", Output: InlineValue("two")},
	}

	states, err := Projector{}.Project(events)
	require.NoError(t, err)
	require.Equal(t, InlineValue("one"), states["run_1"].Output)
	require.Equal(t, InlineValue("two"), states["run_2"].Output)
}

func TestProjectorRejectsPointerWorkflowEvents(t *testing.T) {
	_, err := Projector{}.Project([]any{&Started{RunID: "run_1", WorkflowName: "echo"}})
	require.Error(t, err)
}

func TestProjectorTracksMultipleAttempts(t *testing.T) {
	events := []any{
		Started{RunID: "run_1", WorkflowName: "retryflow"},
		StepStarted{RunID: "run_1", WorkflowName: "retryflow", StepID: "fetch", ActionName: "fetch", Attempt: 1},
		StepFailed{RunID: "run_1", WorkflowName: "retryflow", StepID: "fetch", ActionName: "fetch", Attempt: 1, Error: "temporary"},
		StepStarted{RunID: "run_1", WorkflowName: "retryflow", StepID: "fetch", ActionName: "fetch", Attempt: 2},
		StepCompleted{RunID: "run_1", WorkflowName: "retryflow", StepID: "fetch", ActionName: "fetch", Attempt: 2, Output: InlineValue("ok")},
		Completed{RunID: "run_1", WorkflowName: "retryflow", Output: InlineValue("ok")},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	step := state.Steps["fetch"]
	require.Equal(t, StepSucceeded, step.Status)
	require.Equal(t, 2, step.Attempt)
	require.Equal(t, []AttemptState{
		{Attempt: 1, Status: StepFailedStatus, Error: "temporary"},
		{Attempt: 2, Status: StepSucceeded, Output: InlineValue("ok")},
	}, step.Attempts)
}
func TestExecutorResultCarriesRunIDForProjection(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "echo"}, func(action.Ctx, any) action.Result {
		return action.Result{Data: "ok"}
	})))
	result := Executor{Resolver: RegistryResolver{Registry: reg}, RunID: "run_1"}.Execute(context.Background(), Definition{Name: "echo", Steps: []Step{{ID: "echo", Action: ActionRef{Name: "echo"}}}}, nil)
	require.NoError(t, result.Error)
	wfResult := result.Data.(Result)
	require.Equal(t, RunID("run_1"), wfResult.RunID)
	state, ok, err := Projector{}.ProjectRun(result.Events, wfResult.RunID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, RunSucceeded, state.Status)
}

func TestProjectorPropagatesExternalAndRedactedValueRefs(t *testing.T) {
	external := ExternalValue("file:///tmp/output.json", "application/json")
	redacted := RedactedValue("final-output")
	events := []any{
		Started{RunID: "run_1", WorkflowName: "refs"},
		StepStarted{RunID: "run_1", WorkflowName: "refs", StepID: "fetch", ActionName: "fetch", Attempt: 1},
		StepCompleted{RunID: "run_1", WorkflowName: "refs", StepID: "fetch", ActionName: "fetch", Attempt: 1, Output: external},
		Completed{RunID: "run_1", WorkflowName: "refs", Output: redacted},
	}

	state, ok, err := Projector{}.ProjectRun(events, "run_1")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, external, state.Steps["fetch"].Output)
	require.Equal(t, []AttemptState{{Attempt: 1, Status: StepSucceeded, Output: external}}, state.Steps["fetch"].Attempts)
	require.Equal(t, redacted, state.Output)
}
