package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

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
