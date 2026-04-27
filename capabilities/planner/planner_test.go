package planner

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/thread"
)

func TestPlannerRequiresExplicitPlanCreation(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	_, err := p.ApplyActions(context.Background(), []Action{{
		Action: ActionAddStep,
		Step:   &StepPatch{ID: "step_1", Title: "Implement"},
	}})
	if err == nil || !strings.Contains(err.Error(), "plan has not been created") {
		t.Fatalf("error = %v, want plan has not been created", err)
	}
}

func TestPlannerApplyActionsAppendsThenApplies(t *testing.T) {
	runtime := &recordingRuntime{threadID: "thread_1", branchID: thread.MainBranch}
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, runtime)

	result, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1", Title: "Planner tests"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_1", Title: "Add planner", Status: StepInProgress}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_2", Title: "Write tests"}},
		{Action: ActionSetCurrentStep, StepID: "step_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Plan.ID != "plan_1" || len(result.Plan.Steps) != 2 || result.Plan.CurrentStepID != "step_1" {
		t.Fatalf("unexpected plan: %#v", result.Plan)
	}
	if got, want := len(runtime.events), 4; got != want {
		t.Fatalf("events = %d, want %d", got, want)
	}
	if runtime.events[0].Kind != capability.EventStateEventDispatched {
		t.Fatalf("event kind = %q", runtime.events[0].Kind)
	}
	if runtime.events[0].ThreadID != "thread_1" || runtime.events[0].BranchID != thread.MainBranch {
		t.Fatalf("event ids not filled: %#v", runtime.events[0])
	}
}

func TestPlannerApplyActionsIsAllOrNothing(t *testing.T) {
	runtime := &recordingRuntime{threadID: "thread_1", branchID: thread.MainBranch}
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, runtime)
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_1", Title: "valid"}},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionSetStepTitle, StepID: "step_1", Title: "changed"},
		{Action: ActionSetStepStatus, StepID: "missing", Status: StepCompleted},
	})
	if err == nil {
		t.Fatal("expected batch failure")
	}
	plan, _ := p.State(context.Background())
	if plan.Steps[0].Title != "valid" {
		t.Fatalf("state changed despite failed batch: %#v", plan.Steps[0])
	}
	if got, want := len(runtime.events), 2; got != want {
		t.Fatalf("events = %d, want %d; failed batch should append none", got, want)
	}
}

func TestPlannerDoesNotMutateWhenAppendFails(t *testing.T) {
	runtime := &recordingRuntime{err: errAppend}
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, runtime)
	_, err := p.ApplyActions(context.Background(), []Action{{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}}})
	if err == nil {
		t.Fatal("expected append error")
	}
	plan, _ := p.State(context.Background())
	if plan.ID != "" {
		t.Fatalf("plan mutated despite append failure: %#v", plan)
	}
}

func TestPlannerReplayReconstructsState(t *testing.T) {
	stateEvents := []capability.StateEvent{
		mustStateEvent(t, EventPlanCreated, PlanCreated{PlanID: "plan_1", Title: "Replay"}),
		mustStateEvent(t, EventStepAdded, StepAdded{Step: Step{ID: "step_1", Order: 1, Title: "First", Status: StepPending}}),
		mustStateEvent(t, EventStepStatusChanged, StepStatusChanged{StepID: "step_1", Status: StepCompleted}),
	}
	replayed := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, nil)
	for _, event := range stateEvents {
		if err := replayed.ApplyEvent(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	plan, _ := replayed.State(context.Background())
	if len(plan.Steps) != 1 || plan.Steps[0].Status != StepCompleted {
		t.Fatalf("unexpected replayed plan: %#v", plan)
	}
}

func TestPlannerContextOmitsDeletedStep(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1", Title: "Context"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_1", Title: "Keep"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_2", Title: "Delete"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ApplyActions(context.Background(), []Action{{Action: ActionRemoveStep, StepID: "step_2"}}); err != nil {
		t.Fatal(err)
	}
	context, err := p.ContextProvider().GetContext(context.Background(), agentcontextRequest())
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range context.Fragments {
		if fragment.Key == "planner/step/step_2" {
			t.Fatalf("deleted step fragment still present: %#v", context.Fragments)
		}
	}
}

func TestPlannerToolReturnsModelFacingResultOnly(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	tool := p.Tools()[0]
	result, err := tool.Execute(fakeToolCtx{Context: context.Background()}, json.RawMessage(`{"actions":[{"action":"create_plan","plan":{"id":"plan_1","title":"Tool"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	var decoded ApplyActionsResult
	if err := json.Unmarshal([]byte(result.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Plan.ID != "plan_1" {
		t.Fatalf("tool result plan = %#v", decoded.Plan)
	}
	if strings.Contains(result.String(), "capability.state_event_dispatched") || strings.Contains(result.String(), "event_name") {
		t.Fatalf("tool result leaked persistence details: %s", result.String())
	}
}

func mustStateEvent(t *testing.T, name string, body any) capability.StateEvent {
	t.Helper()
	event, err := stateEvent(name, body)
	if err != nil {
		t.Fatal(err)
	}
	return event
}

var errAppend = &appendError{}

type appendError struct{}

func (*appendError) Error() string { return "append failed" }

type recordingRuntime struct {
	threadID thread.ID
	branchID thread.BranchID
	events   []thread.Event
	err      error
}

func (r *recordingRuntime) ThreadID() thread.ID {
	if r.threadID == "" {
		return "thread_test"
	}
	return r.threadID
}
func (r *recordingRuntime) BranchID() thread.BranchID {
	if r.branchID == "" {
		return thread.MainBranch
	}
	return r.branchID
}
func (r *recordingRuntime) Source() thread.EventSource {
	return thread.EventSource{Type: "capability", ID: "planner_1"}
}
func (r *recordingRuntime) AppendEvents(_ context.Context, events ...thread.Event) error {
	if r.err != nil {
		return r.err
	}
	for _, event := range events {
		if event.ThreadID == "" {
			event.ThreadID = r.ThreadID()
		}
		if event.BranchID == "" {
			event.BranchID = r.BranchID()
		}
		r.events = append(r.events, event)
	}
	return nil
}

type fakeToolCtx struct {
	context.Context
}

func (c fakeToolCtx) WorkDir() string       { return "" }
func (c fakeToolCtx) AgentID() string       { return "" }
func (c fakeToolCtx) SessionID() string     { return "" }
func (c fakeToolCtx) Extra() map[string]any { return nil }

func agentcontextRequest() agentcontext.Request {
	return agentcontext.Request{}
}
