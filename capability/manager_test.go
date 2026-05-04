package capability

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
	"github.com/invopop/jsonschema"
)

func TestRegistryRejectsDuplicatesAndCreates(t *testing.T) {
	factory := fakeFactory{name: "fake"}
	registry, err := NewRegistry(factory)
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(factory); err == nil {
		t.Fatal("expected duplicate factory error")
	}
	if _, err := registry.Create(context.Background(), AttachSpec{CapabilityName: "missing"}, nil); err == nil {
		t.Fatal("expected missing factory error")
	}
}

func TestManagerAttachAppendsEventAndExposesToolsAndContext(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_cap"})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(live, thread.EventSource{Type: "runtime", ID: "test"})
	registry, err := NewRegistry(fakeFactory{name: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(registry, runtime)

	instance, err := manager.Attach(ctx, AttachSpec{CapabilityName: "fake", InstanceID: "fake_1"})
	if err != nil {
		t.Fatal(err)
	}
	if instance.Name() != "fake" || instance.InstanceID() != "fake_1" {
		t.Fatalf("unexpected instance: %s %s", instance.Name(), instance.InstanceID())
	}

	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := stored.Events[len(stored.Events)-1].Kind, EventAttached; got != want {
		t.Fatalf("last event kind = %q, want %q", got, want)
	}
	if got := len(manager.Tools()); got != 1 {
		t.Fatalf("tools = %d, want 1", got)
	}
	providerContext, err := manager.ContextProvider().GetContext(ctx, agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(providerContext.Fragments); got != 1 {
		t.Fatalf("fragments = %d, want 1", got)
	}
	descriptors := manager.Descriptors()
	if got, want := len(descriptors), 1; got != want {
		t.Fatalf("descriptors = %d, want %d", got, want)
	}
	if descriptors[0].Name != "fake" || descriptors[0].InstanceID != "fake_1" {
		t.Fatalf("unexpected descriptor: %#v", descriptors[0])
	}
	if len(descriptors[0].Tools) != 1 || descriptors[0].Tools[0] != "fake_tool" {
		t.Fatalf("unexpected descriptor tools: %#v", descriptors[0].Tools)
	}
	if len(descriptors[0].Actions) != 1 || descriptors[0].Actions[0] != "fake.action" {
		t.Fatalf("unexpected descriptor actions: %#v", descriptors[0].Actions)
	}
	if len(manager.Actions()) != 1 || manager.Actions()[0].Spec().Name != "fake.action" {
		t.Fatalf("unexpected manager actions: %#v", manager.Actions())
	}
}

func TestManagerReplayAppliesStateEventsAndDetach(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_replay"})
	if err != nil {
		t.Fatal(err)
	}
	spec := AttachSpec{
		ThreadID:       live.ID(),
		BranchID:       live.BranchID(),
		CapabilityName: "fake",
		InstanceID:     "fake_replay",
	}
	attach, err := AttachEvent(spec)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]string{"value": "applied"})
	dispatched, err := DispatchEvent(spec, StateEvent{Name: "set", Body: body})
	if err != nil {
		t.Fatal(err)
	}
	detached, err := DetachEvent(spec)
	if err != nil {
		t.Fatal(err)
	}

	if err := live.Append(ctx, attach, dispatched, detached); err != nil {
		t.Fatal(err)
	}
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}

	registry, err := NewRegistry(fakeFactory{name: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(registry, NewRuntime(live, thread.EventSource{}))
	if err := manager.Replay(ctx, stored.Events); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.Capability("fake_replay"); ok {
		t.Fatal("expected capability to be detached after replay")
	}

	manager = NewManager(registry, NewRuntime(live, thread.EventSource{}))
	if err := manager.Replay(ctx, stored.Events[:3]); err != nil {
		t.Fatal(err)
	}
	instance, ok := manager.Capability("fake_replay")
	if !ok {
		t.Fatal("expected replayed capability")
	}
	fake := instance.(*fakeCapability)
	if fake.applied != "set" {
		t.Fatalf("applied event = %q, want set", fake.applied)
	}
}

func TestManagerReplayRejectsUnregisteredStateEvent(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{ID: "thread_replay_invalid"})
	if err != nil {
		t.Fatal(err)
	}
	spec := AttachSpec{
		ThreadID:       live.ID(),
		BranchID:       live.BranchID(),
		CapabilityName: "fake",
		InstanceID:     "fake_invalid",
	}
	attach, err := AttachEvent(spec)
	if err != nil {
		t.Fatal(err)
	}
	dispatched, err := DispatchEvent(spec, StateEvent{Name: "unknown", Body: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := live.Append(ctx, attach, dispatched); err != nil {
		t.Fatal(err)
	}
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := NewRegistry(fakeFactory{name: "fake"})
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(registry, NewRuntime(live, thread.EventSource{}))
	if err := manager.Replay(ctx, stored.Events); err == nil {
		t.Fatal("expected replay to reject unregistered state event")
	}
}

type fakeFactory struct {
	name string
}

func (f fakeFactory) Name() string { return f.name }

func (f fakeFactory) StateEventDefinitions() []StateEventDefinition {
	return []StateEventDefinition{
		DefineStateEvent[map[string]string](f.name, "set"),
	}
}

func (f fakeFactory) New(_ context.Context, spec AttachSpec, runtime Runtime) (Capability, error) {
	return &fakeCapability{name: spec.CapabilityName, instanceID: spec.InstanceID, runtime: runtime}, nil
}

type fakeCapability struct {
	name       string
	instanceID string
	runtime    Runtime
	applied    string
}

func (c *fakeCapability) Name() string       { return c.name }
func (c *fakeCapability) InstanceID() string { return c.instanceID }
func (c *fakeCapability) Tools() []tool.Tool { return []tool.Tool{fakeTool{name: "fake_tool"}} }
func (c *fakeCapability) ContextProvider() agentcontext.Provider {
	return fakeContextProvider{key: agentcontext.ProviderKey(c.instanceID)}
}
func (c *fakeCapability) Actions() []action.Action {
	return []action.Action{action.New(action.Spec{Name: "fake.action"}, func(action.Ctx, any) action.Result {
		return action.OK("ok")
	})}
}
func (c *fakeCapability) ApplyEvent(_ context.Context, event StateEvent) error {
	c.applied = event.Name
	return nil
}

type fakeContextProvider struct {
	key agentcontext.ProviderKey
}

func (p fakeContextProvider) Key() agentcontext.ProviderKey { return p.key }
func (p fakeContextProvider) GetContext(context.Context, agentcontext.Request) (agentcontext.ProviderContext, error) {
	return agentcontext.ProviderContext{Fragments: []agentcontext.ContextFragment{{Key: agentcontext.FragmentKey(p.key + "/fragment"), Content: "context"}}}, nil
}

type fakeTool struct {
	name string
}

func (t fakeTool) Name() string               { return t.name }
func (t fakeTool) Description() string        { return "" }
func (t fakeTool) Schema() *jsonschema.Schema { return nil }
func (t fakeTool) Execute(tool.Ctx, json.RawMessage) (tool.Result, error) {
	return tool.Text("ok"), nil
}
func (t fakeTool) Guidance() string { return "" }
