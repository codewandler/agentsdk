package planner

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
)

func TestPlannerCapabilityPersistsResumesAndReplaysThroughThreadStore(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{
		ID:     "thread_planner",
		Source: thread.EventSource{Type: "session", SessionID: "session_1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := capability.NewRegistry(Factory{})
	if err != nil {
		t.Fatal(err)
	}
	manager := capability.NewManager(registry, capability.NewRuntime(live, thread.EventSource{Type: "session", SessionID: "session_1"}))

	if _, err := manager.Attach(ctx, capability.AttachSpec{CapabilityName: CapabilityName, InstanceID: "planner_1"}); err != nil {
		t.Fatal(err)
	}
	planTool := requirePlanTool(t, manager)
	result, err := planTool.Execute(fakeToolCtx{Context: ctx}, json.RawMessage(`{"actions":[
		{"action":"create_plan","plan":{"id":"plan_1","title":"Thread replay"}},
		{"action":"add_step","step":{"id":"step_1","title":"Attach planner","status":"completed"}},
		{"action":"add_step","step":{"id":"step_2","title":"Replay planner","status":"in_progress"}},
		{"action":"set_current_step","step_id":"step_2"}
	]}`))
	if err != nil {
		t.Fatal(err)
	}
	var decoded ApplyActionsResult
	if err := json.Unmarshal([]byte(result.String()), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Plan.CurrentStepID != "step_2" || len(decoded.Plan.Steps) != 2 {
		t.Fatalf("unexpected tool result: %#v", decoded.Plan)
	}

	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	requireEventCount(t, stored.Events, capability.EventAttached, 1)
	requireEventCount(t, stored.Events, capability.EventStateEventDispatched, 4)

	resumedLive, err := store.Resume(ctx, thread.ResumeParams{
		ID:     live.ID(),
		Source: thread.EventSource{Type: "session", SessionID: "session_2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resumed := capability.NewManager(registry, capability.NewRuntime(resumedLive, thread.EventSource{Type: "session", SessionID: "session_2"}))
	if err := resumed.Replay(ctx, stored.Events); err != nil {
		t.Fatal(err)
	}
	replayed := requirePlanner(t, resumed, "planner_1")
	plan, err := replayed.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ID != "plan_1" || plan.CurrentStepID != "step_2" || len(plan.Steps) != 2 {
		t.Fatalf("unexpected replayed plan: %#v", plan)
	}

	providerContext, err := resumed.ContextProvider().GetContext(ctx, agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	requireFragment(t, providerContext, "planner_1/planner/meta")
	requireFragment(t, providerContext, "planner_1/planner/step/step_1")
	requireFragment(t, providerContext, "planner_1/planner/step/step_2")

	planTool = requirePlanTool(t, resumed)
	if _, err := planTool.Execute(fakeToolCtx{Context: ctx}, json.RawMessage(`{"actions":[{"action":"remove_step","step_id":"step_2"}]}`)); err != nil {
		t.Fatal(err)
	}
	stored, err = store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	requireEventCount(t, stored.Events, capability.EventStateEventDispatched, 5)

	finalLive, err := store.Resume(ctx, thread.ResumeParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	final := capability.NewManager(registry, capability.NewRuntime(finalLive, thread.EventSource{}))
	if err := final.Replay(ctx, stored.Events); err != nil {
		t.Fatal(err)
	}
	finalContext, err := final.ContextProvider().GetContext(ctx, agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	requireFragment(t, finalContext, "planner_1/planner/step/step_1")
	requireNoFragment(t, finalContext, "planner_1/planner/step/step_2")
}

func requirePlanTool(t *testing.T, manager *capability.Manager) tool.Tool {
	t.Helper()
	for _, tool := range manager.Tools() {
		if tool.Name() == "plan" {
			return tool
		}
	}
	t.Fatal("missing plan tool")
	return nil
}

func requirePlanner(t *testing.T, manager *capability.Manager, instanceID string) *Planner {
	t.Helper()
	instance, ok := manager.Capability(instanceID)
	if !ok {
		t.Fatalf("missing capability %q", instanceID)
	}
	planner, ok := instance.(*Planner)
	if !ok {
		t.Fatalf("capability %q type = %T, want *Planner", instanceID, instance)
	}
	return planner
}

func requireEventCount(t *testing.T, events []thread.Event, kind thread.EventKind, want int) {
	t.Helper()
	var got int
	for _, event := range events {
		if event.Kind == kind {
			got++
		}
	}
	if got != want {
		t.Fatalf("event count for %q = %d, want %d", kind, got, want)
	}
}

func requireFragment(t *testing.T, context agentcontext.ProviderContext, key agentcontext.FragmentKey) {
	t.Helper()
	for _, fragment := range context.Fragments {
		if fragment.Key == key {
			return
		}
	}
	t.Fatalf("missing fragment %q in %#v", key, context.Fragments)
}

func requireNoFragment(t *testing.T, context agentcontext.ProviderContext, key agentcontext.FragmentKey) {
	t.Helper()
	for _, fragment := range context.Fragments {
		if fragment.Key == key {
			t.Fatalf("unexpected fragment %q in %#v", key, context.Fragments)
		}
	}
}
