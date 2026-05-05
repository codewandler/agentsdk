package planner

import (
	"context"

	"github.com/codewandler/agentsdk/action"
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

func TestPlannerCreatePlanAfterCreatedReturnsHelpfulError(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	_, err := p.ApplyActions(context.Background(), []Action{{
		Action: ActionCreatePlan,
		Plan:   &PlanPatch{ID: "plan_1", Title: "Existing plan"},
	}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = p.ApplyActions(context.Background(), []Action{{
		Action: ActionCreatePlan,
		Plan:   &PlanPatch{ID: "plan_2", Title: "New plan"},
	}})
	if err == nil {
		t.Fatal("expected create_plan to fail after plan exists")
	}
	for _, want := range []string{"plan already created", "plan_1", "Existing plan", "update the existing plan"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error = %q, want substring %q", err.Error(), want)
		}
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
		mustStateEvent(t, EventStepAdded, StepAdded{Step: Step{ID: "step_1", Title: "First", Status: StepPending}}),
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
	result, err := tool.Execute(fakeToolCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`{"actions":[{"action":"create_plan","plan":{"id":"plan_1","title":"Tool"}}]}`))
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

func TestPlannerAddStepWithDependsOn(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_a", Title: "A"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_b", Title: "B", DependsOn: []string{"step_a"}}},
	}); err != nil {
		t.Fatal(err)
	}
	plan, _ := p.State(context.Background())
	if got := stepOrder(plan, "step_a"); got != 0 {
		t.Fatalf("step_a order = %d, want 0", got)
	}
	if got := stepOrder(plan, "step_b"); got != 1 {
		t.Fatalf("step_b order = %d, want 1", got)
	}
}

func TestPlannerDependsOnRejectsCycles(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_a", Title: "A"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_b", Title: "B"}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionSetStepDependsOn, StepID: "step_a", DependsOn: []string{"step_b"}},
		{Action: ActionSetStepDependsOn, StepID: "step_b", DependsOn: []string{"step_a"}},
	})
	if err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got: %v", err)
	}
}

func TestPlannerParentRejectsSelf(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_a", Title: "A", ParentID: "step_a"}},
	}); err == nil || !strings.Contains(err.Error(), "own parent") {
		t.Fatalf("expected self-parent error, got: %v", err)
	}
}

func TestPlannerRemoveStepRejectsIfChildrenExist(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_parent", Title: "Parent"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_child", Title: "Child", ParentID: "step_parent"}},
	}); err != nil {
		t.Fatal(err)
	}
	_, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionRemoveStep, StepID: "step_parent"},
	})
	if err == nil || !strings.Contains(err.Error(), "sub-tasks") {
		t.Fatalf("expected sub-tasks error, got: %v", err)
	}
}

func TestPlannerRemoveStepCleansUpDependsOn(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_a", Title: "A"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_b", Title: "B", DependsOn: []string{"step_a"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionRemoveStep, StepID: "step_a"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, _ := p.State(context.Background())
	b, ok := findStep(plan, "step_b")
	if !ok {
		t.Fatal("step_b missing")
	}
	if len(plan.Steps[b].DependsOn) != 0 {
		t.Fatalf("step_b depends_on = %v, want empty", plan.Steps[b].DependsOn)
	}
}

func TestPlannerSetStepDependsOnTopoSorts(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_c", Title: "C"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_a", Title: "A"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_b", Title: "B"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionSetStepDependsOn, StepID: "step_b", DependsOn: []string{"step_a"}},
	}); err != nil {
		t.Fatal(err)
	}
	plan, _ := p.State(context.Background())
	ids := make([]string, len(plan.Steps))
	for i, s := range plan.Steps {
		ids[i] = s.ID
	}
	if ids[0] != "step_a" || ids[1] != "step_b" {
		t.Fatalf("unexpected order: %v", ids)
	}
}

func TestPlannerSetStepParent(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_parent", Title: "Parent"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_child", Title: "Child"}},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionSetStepParent, StepID: "step_child", ParentID: "step_parent"},
	}); err != nil {
		t.Fatal(err)
	}
	plan, _ := p.State(context.Background())
	idx, ok := findStep(plan, "step_child")
	if !ok {
		t.Fatal("step_child missing")
	}
	if plan.Steps[idx].ParentID != "step_parent" {
		t.Fatalf("parent = %q, want step_parent", plan.Steps[idx].ParentID)
	}
}

func TestPlannerReplayDependsOnChanged(t *testing.T) {
	p := New(capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}, &recordingRuntime{})
	if _, err := p.ApplyActions(context.Background(), []Action{
		{Action: ActionCreatePlan, Plan: &PlanPatch{ID: "plan_1"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_a", Title: "A"}},
		{Action: ActionAddStep, Step: &StepPatch{ID: "step_b", Title: "B"}},
		{Action: ActionSetStepDependsOn, StepID: "step_b", DependsOn: []string{"step_a"}},
	}); err != nil {
		t.Fatal(err)
	}
	plan, _ := p.State(context.Background())
	idx, _ := findStep(plan, "step_b")
	if plan.Steps[idx].DependsOn[0] != "step_a" {
		t.Fatalf("unexpected depends_on: %v", plan.Steps[idx].DependsOn)
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
	action.BaseCtx
}

func (c fakeToolCtx) WorkDir() string       { return "" }
func (c fakeToolCtx) AgentID() string       { return "" }
func (c fakeToolCtx) SessionID() string     { return "" }
func (c fakeToolCtx) Extra() map[string]any { return nil }

func agentcontextRequest() agentcontext.Request {
	return agentcontext.Request{}
}

func stepOrder(plan Plan, id string) int {
	for i, s := range plan.Steps {
		if s.ID == id {
			return i
		}
	}
	return -1
}
