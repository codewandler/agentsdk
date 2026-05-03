package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/thread"
	"github.com/stretchr/testify/require"
)

func TestExecutorRunsWorkflowOverActionRefs(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(
		action.New(action.Spec{Name: "upper"}, func(_ action.Ctx, input any) action.Result {
			require.Equal(t, "hello", input)
			return action.Result{Data: "HELLO"}
		}),
		action.New(action.Spec{Name: "suffix"}, func(_ action.Ctx, input any) action.Result {
			require.Equal(t, "HELLO", input)
			return action.Result{Data: input.(string) + "!"}
		}),
	))

	def := Definition{Name: "shout", Steps: []Step{
		{ID: "upper", Action: ActionRef{Name: "upper"}},
		{ID: "suffix", Action: ActionRef{Name: "suffix"}, DependsOn: []string{"upper"}},
	}}

	result := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), def, "hello")
	require.NoError(t, result.Error)
	wfResult := result.Data.(Result)
	require.Equal(t, "HELLO!", wfResult.Data)
	require.Equal(t, "HELLO", wfResult.StepResults["upper"].Data)
}

func TestExecutorPassesMultipleDependencyOutputs(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(
		action.New(action.Spec{Name: "a"}, func(action.Ctx, any) action.Result { return action.Result{Data: "A"} }),
		action.New(action.Spec{Name: "b"}, func(action.Ctx, any) action.Result { return action.Result{Data: "B"} }),
		action.New(action.Spec{Name: "join"}, func(_ action.Ctx, input any) action.Result {
			deps := input.(map[string]any)
			return action.Result{Data: deps["a"].(string) + deps["b"].(string)}
		}),
	))
	def := Definition{Name: "join", Steps: []Step{
		{ID: "a", Action: ActionRef{Name: "a"}},
		{ID: "b", Action: ActionRef{Name: "b"}},
		{ID: "join", Action: ActionRef{Name: "join"}, DependsOn: []string{"a", "b"}},
	}}

	result := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), def, nil)
	require.NoError(t, result.Error)
	require.Equal(t, "AB", result.Data.(Result).Data)
}

func TestExecutorStopsOnStepError(t *testing.T) {
	boom := errors.New("boom")
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "fail"}, func(action.Ctx, any) action.Result {
		return action.Result{Error: boom}
	})))

	def := Definition{Name: "failflow", Steps: []Step{{ID: "fail", Action: ActionRef{Name: "fail"}}}}
	result := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), def, nil)
	require.ErrorIs(t, result.Error, boom)
	wfResult := result.Data.(Result)
	require.Contains(t, wfResult.StepResults, "fail")
}

func TestExecutorRejectsCyclesAndUnknownDeps(t *testing.T) {
	exec := Executor{Resolver: ResolverFunc(func(action.Ctx, ActionRef) (action.Action, bool) { return nil, false })}

	cycle := Definition{Name: "cycle", Steps: []Step{
		{ID: "a", Action: ActionRef{Name: "a"}, DependsOn: []string{"b"}},
		{ID: "b", Action: ActionRef{Name: "b"}, DependsOn: []string{"a"}},
	}}
	require.ErrorContains(t, exec.Execute(context.Background(), cycle, nil).Error, "cycle")

	unknown := Definition{Name: "unknown", Steps: []Step{{ID: "a", Action: ActionRef{Name: "a"}, DependsOn: []string{"missing"}}}}
	require.ErrorContains(t, exec.Execute(context.Background(), unknown, nil).Error, "unknown step")
}

func TestWorkflowActionExposesDefinition(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
		return action.Result{Data: input}
	})))
	wa := WorkflowAction{
		Definition: Definition{Name: "echo_flow", Description: "echo workflow", Steps: []Step{{ID: "echo", Action: ActionRef{Name: "echo"}}}},
		Executor:   Executor{Resolver: RegistryResolver{Registry: reg}},
	}

	require.Equal(t, "echo_flow", wa.Spec().Name)
	result := wa.Execute(context.Background(), "hi")
	require.NoError(t, result.Error)
	require.Equal(t, "hi", result.Data.(Result).Data)
}

func TestExecutorEmitsWorkflowEvents(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(
		action.New(action.Spec{Name: "upper"}, func(_ action.Ctx, input any) action.Result {
			return action.Result{Data: "HELLO", Events: []action.Event{"action-event"}}
		}),
		action.New(action.Spec{Name: "suffix"}, func(_ action.Ctx, input any) action.Result {
			return action.Result{Data: input.(string) + "!"}
		}),
	))
	def := Definition{Name: "shout", Steps: []Step{
		{ID: "upper", Action: ActionRef{Name: "upper"}},
		{ID: "suffix", Action: ActionRef{Name: "suffix"}, DependsOn: []string{"upper"}},
	}}
	var live []action.Event
	exec := Executor{Resolver: RegistryResolver{Registry: reg}, OnEvent: func(_ action.Ctx, event action.Event) {
		live = append(live, event)
	}}

	result := exec.Execute(context.Background(), def, "hello")
	require.NoError(t, result.Error)
	require.Equal(t, "HELLO!", result.Data.(Result).Data)
	require.Len(t, live, 6)
	require.Equal(t, []thread.EventKind{EventStarted, EventStepStarted, EventStepCompleted, EventStepStarted, EventStepCompleted, EventCompleted}, eventKinds(live))
	require.Equal(t, "upper", live[1].(StepStarted).StepID)
	require.Equal(t, InlineValue("HELLO!"), live[5].(Completed).Output)
	require.Contains(t, result.Events, action.Event("action-event"))
	completed := live[5].(Completed)
	require.False(t, completed.At.IsZero())
	require.Contains(t, result.Events, action.Event(Completed{RunID: live[0].(Started).RunID, WorkflowName: "shout", Output: InlineValue("HELLO!"), At: completed.At}))
}

func TestExecutorEmitsFailureEvents(t *testing.T) {
	boom := errors.New("boom")
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "fail"}, func(action.Ctx, any) action.Result {
		return action.Result{Error: boom}
	})))
	def := Definition{Name: "failflow", Steps: []Step{{ID: "fail", Action: ActionRef{Name: "fail"}}}}
	var live []action.Event
	exec := Executor{Resolver: RegistryResolver{Registry: reg}, OnEvent: func(_ action.Ctx, event action.Event) {
		live = append(live, event)
	}}

	result := exec.Execute(context.Background(), def, nil)
	require.ErrorIs(t, result.Error, boom)
	require.Equal(t, []thread.EventKind{EventStarted, EventStepStarted, EventStepFailed, EventFailed}, eventKinds(live))
	require.Equal(t, boom.Error(), live[2].(StepFailed).Error)
	failed := live[2].(StepFailed)
	require.False(t, failed.At.IsZero())
	require.Contains(t, result.Events, action.Event(StepFailed{RunID: live[0].(Started).RunID, WorkflowName: "failflow", StepID: "fail", ActionName: "fail", Attempt: 1, Error: boom.Error(), At: failed.At}))
}

func TestExecutorValidatesActionInputAndOutput(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(
		action.New(action.Spec{Name: "needs_string", Input: action.TypeOf[string](), Output: action.TypeOf[string]()}, func(action.Ctx, any) action.Result {
			return action.Result{Data: "ok"}
		}),
		action.New(action.Spec{Name: "returns_wrong", Output: action.TypeOf[string]()}, func(action.Ctx, any) action.Result {
			return action.Result{Data: 123}
		}),
	))

	inputResult := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), Definition{Name: "input", Steps: []Step{{ID: "needs_string", Action: ActionRef{Name: "needs_string"}}}}, 123)
	require.ErrorContains(t, inputResult.Error, "invalid input")

	outputResult := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), Definition{Name: "output", Steps: []Step{{ID: "returns_wrong", Action: ActionRef{Name: "returns_wrong"}}}}, nil)
	require.ErrorContains(t, outputResult.Error, "invalid output")
}

func TestExecutorRunsIndependentStepsInParallelWithFanIn(t *testing.T) {
	reg := action.NewRegistry()
	started := make(chan string, 2)
	release := make(chan struct{})
	require.NoError(t, reg.Register(
		action.New(action.Spec{Name: "a"}, func(action.Ctx, any) action.Result {
			started <- "a"
			<-release
			return action.Result{Data: "A"}
		}),
		action.New(action.Spec{Name: "b"}, func(action.Ctx, any) action.Result {
			started <- "b"
			<-release
			return action.Result{Data: "B"}
		}),
		action.New(action.Spec{Name: "join"}, func(_ action.Ctx, input any) action.Result {
			deps := input.(map[string]any)
			return action.Result{Data: deps["a"].(string) + deps["b"].(string)}
		}),
	))
	def := Definition{Name: "parallel", Steps: []Step{
		{ID: "a", Action: ActionRef{Name: "a"}},
		{ID: "b", Action: ActionRef{Name: "b"}},
		{ID: "join", Action: ActionRef{Name: "join"}, DependsOn: []string{"a", "b"}},
	}}
	done := make(chan action.Result, 1)
	go func() {
		done <- Executor{Resolver: RegistryResolver{Registry: reg}, MaxConcurrency: 2}.Execute(context.Background(), def, nil)
	}()

	require.ElementsMatch(t, []string{"a", "b"}, []string{<-started, <-started})
	close(release)
	result := <-done
	require.NoError(t, result.Error)
	require.Equal(t, "AB", result.Data.(Result).Data)
}

func TestExecutorHonorsRetryTimeoutContinueConditionAndInputMapping(t *testing.T) {
	var flakyAttempts int32
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(
		action.New(action.Spec{Name: "seed"}, func(action.Ctx, any) action.Result {
			return action.Result{Data: map[string]any{"value": "ok", "run": true}}
		}),
		action.New(action.Spec{Name: "flaky"}, func(action.Ctx, any) action.Result {
			attempt := atomic.AddInt32(&flakyAttempts, 1)
			if attempt == 1 {
				return action.Result{Error: errors.New("temporary")}
			}
			return action.Result{Data: "retried"}
		}),
		action.New(action.Spec{Name: "slow"}, func(ctx action.Ctx, _ any) action.Result {
			select {
			case <-ctx.Done():
				return action.Result{Error: ctx.Err()}
			case <-time.After(100 * time.Millisecond):
				return action.Result{Data: "too late"}
			}
		}),
		action.New(action.Spec{Name: "mapped"}, func(_ action.Ctx, input any) action.Result {
			return action.Result{Data: input}
		}),
	))
	def := Definition{Name: "semantics", Steps: []Step{
		{ID: "seed", Action: ActionRef{Name: "seed"}},
		{ID: "flaky", Action: ActionRef{Name: "flaky"}, DependsOn: []string{"seed"}, Retry: RetryPolicy{MaxAttempts: 2}, IdempotencyKey: "flaky-key"},
		{ID: "slow", Action: ActionRef{Name: "slow"}, DependsOn: []string{"flaky"}, Timeout: time.Millisecond, ErrorPolicy: StepErrorContinue},
		{ID: "skipped", Action: ActionRef{Name: "mapped"}, DependsOn: []string{"seed"}, When: Condition{StepID: "seed", Equals: "nope"}},
		{ID: "mapped", Action: ActionRef{Name: "mapped"}, DependsOn: []string{"seed", "flaky", "slow"}, InputMap: map[string]string{"seed": "steps.seed.output.value", "flaky": "steps.flaky.output", "initial": "input"}},
	}}

	result := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), def, "hello")
	require.NoError(t, result.Error)
	wfResult := result.Data.(Result)
	require.Equal(t, int32(2), atomic.LoadInt32(&flakyAttempts))
	require.Error(t, wfResult.StepResults["slow"].Error)
	require.Equal(t, map[string]any{"seed": "ok", "flaky": "retried", "initial": "hello"}, wfResult.StepResults["mapped"].Data)
	state, ok, err := Projector{}.ProjectRun(result.Events, wfResult.RunID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, StepSkippedStatus, state.Steps["skipped"].Status)
	require.Equal(t, "flaky-key", state.Steps["flaky"].IdempotencyKey)
	require.Equal(t, []AttemptState{{Attempt: 1, Status: StepFailedStatus, Error: "temporary"}, {Attempt: 2, Status: StepSucceeded, Output: InlineValue("retried")}}, state.Steps["flaky"].Attempts)
}

func TestExecutorPreservesExternalAndRedactedValueRefs(t *testing.T) {
	reg := action.NewRegistry()
	external := ExternalValue("file:///tmp/output.json", "application/json")
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "external"}, func(action.Ctx, any) action.Result {
		return action.Result{Data: external}
	})))
	def := Definition{Name: "refs", Steps: []Step{{ID: "external", Action: ActionRef{Name: "external"}}}}

	result := Executor{Resolver: RegistryResolver{Registry: reg}}.Execute(context.Background(), def, nil)
	require.NoError(t, result.Error)
	state, ok, err := Projector{}.ProjectRun(result.Events, result.Data.(Result).RunID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, external, state.Output)
	require.Equal(t, external, state.Steps["external"].Output)
}

func eventKinds(events []action.Event) []thread.EventKind {
	out := make([]thread.EventKind, len(events))
	for i, event := range events {
		switch event.(type) {
		case Started:
			out[i] = EventStarted
		case StepStarted:
			out[i] = EventStepStarted
		case StepCompleted:
			out[i] = EventStepCompleted
		case StepFailed:
			out[i] = EventStepFailed
		case Completed:
			out[i] = EventCompleted
		case Failed:
			out[i] = EventFailed
		}
	}
	return out
}

func TestEventDefinitionsValidateConcretePayloads(t *testing.T) {
	registry, err := thread.NewEventRegistry(EventDefinitions()...)
	require.NoError(t, err)
	payload, err := json.Marshal(StepStarted{RunID: "run_1", WorkflowName: "wf", StepID: "step", ActionName: "action", Attempt: 1})
	require.NoError(t, err)
	require.NoError(t, registry.Validate(thread.Event{Kind: EventStepStarted, Payload: payload}))
	require.Error(t, registry.Validate(thread.Event{Kind: EventStepStarted, Payload: []byte(`{`)}))
}
