# Capability system follow-ups

Section 18 keeps capabilities as explicit, replayable session/runtime extensions rather than hidden agent defaults.

## Ownership

- `capability` owns the capability interface, registry, manager, attach/detach events, state-event dispatch, descriptors, and optional projection facets.
- `capabilities/*` packages own concrete dogfood capabilities. The planner remains the first built-in capability.
- `app` owns plugin-contributed capability factories through `app.CapabilityFactoriesPlugin`.
- `agent.Spec.Capabilities` and `agent.WithCapabilities(...)` select instances explicitly for an agent/session.
- `runtime.ThreadRuntime` owns live attachment, replay, context projection, and tool/action projection for one thread branch.
- `harness.Session` exposes inspection and command surfaces, but does not create hidden capability defaults.

The registry is intentionally explicit. If an agent spec requests a capability and no matching factory is contributed by host/plugin configuration, construction fails instead of silently installing the planner.

## Projection facets

Every capability still exposes LLM-facing tools and a context provider through the base interface:

```go
type Capability interface {
    Name() string
    InstanceID() string
    Tools() []tool.Tool
    ContextProvider() agentcontext.Provider
}
```

Capabilities may also implement `capability.ActionsProvider` when a Go-native action projection is useful for workflows, triggers, or host code:

```go
type ActionsProvider interface {
    Actions() []action.Action
}
```

This keeps tool projection and action projection separate. Tools remain model-facing. Actions remain typed execution primitives for non-LLM surfaces.

## Inspection

`capability.Descriptor` is side-effect-free metadata for debug and channel surfaces. It reports:

- capability name and instance ID;
- projected tool names;
- projected action names;
- context provider descriptor;
- stateful/replayable flags.

Inspection APIs:

- `capability.Manager.Descriptors()`
- `capability.Manager.Actions()`
- `runtime.ThreadRuntime.CapabilityDescriptors()`
- `runtime.ThreadRuntime.CapabilityActions()`
- `runtime.Engine.CapabilityDescriptors()`
- `runtime.Engine.CapabilityActions()`
- `agent.Instance.CapabilityDescriptors()`
- `agent.Instance.CapabilityActions()`
- `harness.Session.CapabilityState()`
- `/capabilities`

## Planner dogfood capability

The planner remains a dogfood capability and now advertises all three projections:

- tool: `plan`
- action: `planner.apply_actions`
- context provider: active plan metadata and steps

Planner state remains event-sourced through `capability.state_event_dispatched`, so resumed thread runtimes replay plan state before future turns.

## Attachment lifecycle

Capabilities attach through explicit `capability.AttachSpec` values on an agent spec or agent option. Runtime attachment is idempotent per instance ID: `ThreadRuntime.EnsureCapabilities(...)` attaches missing configured capabilities before a turn and skips already-attached instances. Resume replays `capability.attached`, `capability.detached`, and validated state events from the selected thread branch.
