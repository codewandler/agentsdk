# PLAN: Event-Sourced Thread, Context, and Prompt Architecture

Status: active implementation
Created: 2026-04-27
Last updated: 2026-04-27

## Goal

Design the next conversation/runtime architecture around an append-only event
log, typed thread lifecycle APIs, normalized internal conversation items, and
state-driven context projection.

This plan is intentionally greenfield-leaning. It should guide a future
refactor without requiring every part to land at once. The central principle is
that the durable source of truth is an event log, while model requests are late
projections from that log plus current harness state.

## Implementation Status

The first cleanup pass has landed and intentionally breaks compatibility with
the previous session API:

- `conversation.Session`, `conversation.EventStore`,
  `conversation.ThreadEventStore`, and `conversation/jsonlstore` are removed.
- `conversation` is now the small core package for tree lineage, payload
  events, turn fragments, internal items, projection, request defaults, and
  provider continuation metadata.
- `runtime.History` owns runtime conversation state, request defaults, thread
  event encoding for conversation payloads, and provider request projection.
- `thread.Store` owns create/resume/read/list/archive plus live append.
- `agentcontext.Manager` owns provider polling, manager-owned key/fingerprint
  diffs, render records, rollback, and the optional provider fingerprint fast
  path.
- `capability` and `capabilities/planner` implement the first stateful
  capability slice with `capability.attached` and
  `capability.state_event_dispatched`.

The second cleanup pass has also landed:

- Runtime history appends durable thread events before mutating the in-memory
  conversation tree.
- Context renders persist typed fragment add/update events, removal tombstones,
  render records, and a hybrid `harness.context_snapshot_recorded` event with
  provider fingerprints and inline provider snapshots.
- Successful thread-backed turns persist llmadapter route metadata and provider
  execution metadata at the same commit boundary as the assistant fragment.
- Thread-backed histories use durable thread/branch data for Codex continuation
  hints and expose whether durable thread events are available.
- `thread/jsonlstore` live appends now use append-only writes with file sync,
  closed-live protection, and discard cleanup instead of rewriting the whole
  store on every append.
- `capability` has low-boilerplate typed inner state-event definitions for
  replay validation; the planner registers all current inner state events.
- `agentcontext/contextproviders` includes reusable providers for environment,
  time, model, permissions, project instructions, loaded skills, and active
  tools.
- The default terminal `/context` command prints the last committed context
  render state for inspection.
- Provider projection now uses normalized `conversation.Item` inputs only;
  raw pending request messages are converted to items at the runtime boundary
  rather than being accepted as a second projection path.
- Tool call/result recovery now drops duplicate tool calls/results during
  projection and persists a recovered, paired tool transcript when a turn aborts
  after tool execution because of cancellation or max-step exhaustion.
- Compaction is now real projection behavior: summaries are projected at the
  compacted window position, replaced nodes stay in the tree/thread log,
  compaction is branch-local, pending context fragments remain outside the
  compacted window, and native continuation is not reused after compaction
  becomes the branch head.
- Thread-backed provider stream failures now append `provider.stream_failed`
  diagnostics with step/model/provider identity, recoverability, error text,
  last stream event, text/reasoning byte counts, and partial tool-call facts.
  These diagnostics are durable thread events only; they are not projected into
  model-visible conversation messages.

Remaining larger architecture work is intentionally outside this first
implementation slice:

1. Add indexing/repair for JSONL thread metadata if/when listing/search needs
   outgrow replaying JSONL.
2. Decide whether all durable non-capability event kinds need a shared typed
   registry, or whether schema validation should remain owned by the projections
   that consume those events.

## Executive Summary

Agentsdk should evolve toward three layers:

```text
Event log          append-only durable facts
Materialized views conversation tree, harness state, metadata, prompt state
ThreadStore        product-level thread lifecycle and live writer API
```

The model should never receive the event log directly. It receives a
provider-specific prompt projection built from:

```text
selected branch conversation items
+ active context fragments from provider render records
+ pending user input
+ request/provider options
```

This keeps arbitrary capability/plugin events useful for replay and state, while
preventing them from polluting model-visible history unless a context provider
explicitly projects them.

## Design Principles

- Event sourcing is the foundation; `ThreadStore` is the ergonomic lifecycle
  boundary above it.
- Any state that must survive resume, branch replay, or crash recovery is
  represented by durable events. Ephemeral render-only context may be queued by
  a provider without durability, but it is intentionally not resume-safe.
- Prefer the target architecture over backwards compatibility. Replace weak
  public APIs deliberately instead of wrapping them indefinitely.
- Conversation history is tree-shaped and branch-aware, but model prompts are
  linear projections.
- Harness/runtime state is separate from conversation messages.
- Context injection is typed, tagged, authority-aware, and diffable.
- Provider-specific request shapes are produced late, after internal history
  normalization.
- Arbitrary plugin/capability state is allowed, but only typed context providers
  make it model-visible.
- Original events remain durable even when compaction changes the active prompt
  window.

## Core Concepts

### Event Log

Append-only durable record of facts.

Initial event envelope:

```go
type Event struct {
    ID            EventID
    ThreadID      ThreadID
    BranchID      BranchID
    NodeID        NodeID
    ParentNodeID  NodeID
    Seq           int64
    Kind          EventKind
    Payload       json.RawMessage
    At            time.Time
    Source        EventSource
    CausationID   EventID
    CorrelationID string
}
```

Every durable event belongs to a `ThreadID` and `BranchID`. Avoid `SessionID` as
the durable identity; a session is a live runtime attachment to a persisted
thread. If needed, record the live session/run ID in `EventSource` metadata.

`Seq` is per-thread and monotonic. `ParentNodeID` is used for tree lineage.
`CausationID` ties derived events to the command/tool/user event that caused
them. `CorrelationID` ties a turn, tool call, or workflow together.

### Event Categories

Use explicit kind namespaces:

```text
thread.created
thread.metadata_updated
thread.archived

branch.created
branch.head_moved

conversation.user_message
conversation.assistant_message
conversation.reasoning
conversation.tool_call
conversation.tool_result
conversation.context_fragment
conversation.context_fragment_removed
conversation.compaction

harness.model_changed
harness.environment_changed
harness.skill_loaded
harness.skill_unloaded
harness.tool_activated
harness.tool_deactivated
harness.context_snapshot_recorded

capability.attached
capability.detached
capability.state_event_dispatched

provider.route_selected
provider.execution_metadata_recorded
plugin.<event>
ops.<event>
```

Only `conversation.*` events are direct candidates for prompt projection.
`harness.*`, `capability.*`, `provider.*`, `plugin.*`, and `ops.*` feed
materialized views and context providers.

Event kinds describe schemas, not instances. Do not encode the capability or
plugin name in the event kind. Put identity in the envelope or payload instead:

```go
type EventSource struct {
    Type      string // user, assistant, harness, capability, plugin, tool, runtime
    ID        string // capability instance id, plugin id, tool name, etc.
    SessionID string // optional live runtime/session id, not durable identity
}
```

For stateful capabilities, use a generic durable event kind such as
`capability.state_event_dispatched` and put the capability identity plus inner
event name in the payload. This keeps the global event registry small and stable
while still letting each capability define a typed inner event schema.

Example:

```go
type CapabilityStateEventDispatched struct {
    CapabilityName string          `json:"capability_name"` // planner
    InstanceID     string          `json:"instance_id"`     // planner_abc
    EventName      string          `json:"event_name"`      // step_added
    Body           json.RawMessage `json:"body"`
}
```

`Source{Type:"capability", ID:"planner_abc"}` identifies the concrete instance
that emitted the event. `CapabilityName` identifies the implementation/type used
to route replay to the correct loader.

### ThreadStore

`ThreadStore` is the product-level API over event storage. It owns thread
lifecycle, listing, archiving, metadata, and live writer semantics.

Sketch:

```go
type ThreadStore interface {
    CreateThread(context.Context, CreateThreadParams) (*LiveThread, error)
    ResumeThread(context.Context, ResumeThreadParams) (*LiveThread, error)
    ReadThread(context.Context, ReadThreadParams) (StoredThread, error)
    ListThreads(context.Context, ListThreadsParams) (ThreadPage, error)
    UpdateThreadMetadata(context.Context, UpdateThreadMetadataParams) (StoredThread, error)
    ArchiveThread(context.Context, ThreadID) error
    UnarchiveThread(context.Context, ThreadID) error
}

type LiveThread interface {
    // Append writes the provided events as one atomic batch.
    Append(context.Context, ...Event) error
    Flush(context.Context) error
    Shutdown(context.Context) error
    Discard(context.Context) error
}
```

`EventStore` may exist as an internal append/load primitive behind local or
remote `ThreadStore` implementations, but it should not constrain the public
architecture.

### Materialized Views

Views are deterministic projections from events:

- `ConversationTreeProjection`: branch/node lineage and selected branch path.
- `InternalItemProjection`: normalized provider-independent conversation items.
- `HarnessStateProjection`: current model, permissions, skills, tools,
  environment, capability attachments, and plugin state.
- `CapabilityProjection`: attached capability instances and their materialized
  state for the selected thread branch.
- `ThreadMetadataProjection`: title, preview, cwd, model, updated time, tags,
  archive state.
- `PromptProjection`: final provider request input.

Views may be rebuilt from the log or cached/indexed for performance.

### Internal Conversation Items

Add an internal item layer before provider projection.

Initial item types:

```text
UserMessageItem
AssistantMessageItem
ReasoningItem
ToolCallItem
ToolResultItem
ContextFragmentItem
CompactionItem
AnnotationItem
GhostSnapshotItem
```

Provider messages are a projection target, not the durable model.

## Context Architecture

### Context Snapshot

Each real user turn records a harness context snapshot. It is not a model
message.

Snapshot domains:

- model: name, provider, context window, reasoning effort, output params
- environment: cwd, shell, OS/kernel, current date/timezone, git state digest
- instructions: AGENTS.md paths, contents hash, effective scope hash
- skills: available catalog digest, loaded skill names, loaded body digests
- tools: available catalog digest, active tool names, protected tools
- permissions: sandbox, approval policy, writable roots, network policy
- plugins/capabilities: provider-specific state snapshots

Persist as `harness.context_snapshot_recorded`.

### Context Provider

Context providers own domain semantics. The context manager orchestrates them;
it does not know how to render planner steps, skills, permissions, or
environment facts.

From the context manager's point of view, every provider is pull-based. On each
render boundary, the context manager iterates over registered providers and asks
for its complete current provider context. The provider may compute that context
from current state, drain an internal queue into that state, apply a
timer/rate-limit, or return an empty current state. The context manager owns the
mechanical diff against the previous rendered provider context.

Sketch:

```go
type ContextProvider interface {
    Key() ProviderKey
    GetContext(context.Context, ContextRequest) (ProviderContext, error)
}

type ProviderKey string
type FragmentKey string

type ProviderContext struct {
    // Fragments is the full current model-visible fragment set for this
    // provider. A missing previously-rendered key means the fragment was
    // removed.
    Fragments []ContextFragment

    // Optional provider-level snapshot for future comparisons, diagnostics, or
    // replay optimization. It is not sent to the model directly.
    Snapshot *ProviderSnapshot

    // Optional provider-level fingerprint. If empty, the context manager can
    // compute one from the ordered fragment keys and fingerprints.
    Fingerprint string
}

type ContextRequest struct {
    ThreadID      ThreadID
    BranchID      BranchID
    TurnID        string
    HarnessState  HarnessState
    Preference    ContextPreference // hint only
    Previous      *ProviderRenderRecord
    TokenBudget   int
    Reason        ContextRenderReason
}

type ContextPreference string

const (
    ContextPreferChanges ContextPreference = "changes"
    ContextPreferFull    ContextPreference = "full"
)
```

`Preference` is a rendering hint, not a diff protocol. The base contract remains
that the provider returns its complete current fragment set. For example, after
resume or compaction the context manager may ask for `ContextPreferFull` so a
provider chooses more verbose fragments; a normal turn may ask for
`ContextPreferChanges` so the provider chooses concise fragments. In both cases,
the returned fragments represent the current provider state at that render
boundary.

Providers that want push-like behavior implement it internally. They expose the
same `GetContext` method, but their domain events, tool callbacks, background
timers, or commands update provider state or enqueue one-shot fragments. The
next `GetContext` call folds that queue into the returned complete fragment set.

Examples:

- A command enqueues arbitrary text as a one-shot `ContextFragment`; the next
  poll includes it in the provider's full returned set and then removes it from
  the provider queue. On the following poll it is absent, so the context manager
  records a removal tombstone.
- A stateful capability command appends a durable event, updates materialized
  capability state, and `GetContext` returns the capability's current context
  fragments.
- A planner returns stable fragments such as `planner/meta` and
  `planner/step/<id>` so changing one step only changes one fragment.
- A `TimeContextProvider` returns a `time/current` fragment whose content is
  rounded to the minute. The context manager sees the same fingerprint within
  the minute and a changed fingerprint after the minute changes.

The context manager does not know whether returned fragments came from state,
a drained queue, a timer, or a full snapshot. It only records the returned
provider snapshot/fingerprint and appends model-visible fragment changes.

A provider-local queue is not durable state by itself. If a queued item matters
after resume, it must either be backed by a durable event or have already been
rendered and persisted as a model-visible context fragment. Queued content that
has not yet been rendered may be lost across process restart by design.

Provider examples:

- `EnvironmentProvider`
- `ProjectInstructionsProvider`
- `SkillsProvider`
- `ToolsProvider`
- `ModelProvider`
- `PermissionsProvider`
- `PlannerCapability`
- plugin-provided providers

### Thread Capabilities

Capabilities are thread-attached runtime extensions. They can expose tools,
own durable state, apply replayed state events, and contribute context
fragments. A capability is not the model-running agent. The same capability can
be used by a CLI coding agent, a research agent, a review agent, or a
human-driven session.

Ownership split:

```text
App
  owns capability registry/factories
  knows which capability types are available

Thread/branch
  owns attached capability instances
  owns durable capability state events

Agent/runtime
  consumes capability tools
  sees capability context
  may trigger capability mutations
```

The capability's durable state lives in the thread event log. The in-memory
state object is a materialized view rebuilt by replaying branch events.

Sketch:

```go
type Capability interface {
    Name() string
    InstanceID() string
    Tools() []tool.Tool
    ContextProvider() ContextProvider
}

type StatefulCapability[T any] interface {
    Capability
    State(context.Context) (T, error)
    ApplyEvent(context.Context, CapabilityStateEvent) error
}

type CapabilityStateEvent struct {
    Name string          // step_added, step_removed, etc.
    Body json.RawMessage // decoded by the capability or a typed helper
}

type CapabilityRuntime interface {
    ThreadID() ThreadID
    BranchID() BranchID
    Source() EventSource
    // AppendEvents writes the provided events as one atomic batch.
    AppendEvents(context.Context, ...Event) error
}
```

`context.Context` carries cancellation, deadline, and request scope only. It
must not be used as a service locator for event-log access. Capabilities that
need to mutate durable state receive an explicit `CapabilityRuntime` when they
are attached or constructed.

Example:

```go
type Plan struct {
    Steps []PlanStep
}

type PlannerCapability struct {
    StatefulCapability[Plan]
}
```

The planner action tool supports actions such as:

```text
create_plan
add_step
remove_step
set_step_title
set_step_status
reorder_step
set_current_step
```

Those commands append events such as:

```text
capability.state_event_dispatched {
  capability_name: "planner",
  instance_id: "planner_abc",
  event_name: "step_added",
  body: {...}
}
```

The event source also identifies the planner capability instance:

```text
Source.Type = "capability"
Source.ID   = "planner_abc"
```

`capability.attached` records enough information to load the right stateful
capability implementation on resume:

```go
type CapabilityAttached struct {
    CapabilityName string          `json:"capability_name"`
    InstanceID     string          `json:"instance_id"`
    Config         json.RawMessage `json:"config,omitempty"`
}
```

On resume:

1. Replay `capability.attached` events and instantiate each capability through
   the app's capability registry.
2. Replay matching `capability.state_event_dispatched` events to the instance's
   `ApplyEvent` method.
3. Rebuild materialized state in memory.
4. Let the capability's context provider return the current granular fragment
   set.

The planner's materialized state is rebuilt by replaying its dispatched inner
events. Its context provider decides how that state becomes model-visible.

Stateful capabilities should support configurable render modes:

- `single_snapshot`: return one full-state fragment, useful for small state.
- `granular_snapshot`: return many stable fragments, useful for larger state.
- `custom`: capability decides how to split current state into stable fragments.

A capability can be registered directly as a context provider, or a
`CapabilityManagerProvider` can be the single context provider for all attached
capabilities. In both cases, the context manager only calls `GetContext`. The
capability or manager owns any internal queues, timers, and fragment granularity
policy. The context manager owns the mechanical diff across returned fragments.

Planner example:

```go
func (p *PlannerProvider) GetContext(ctx context.Context, req ContextRequest) (ProviderContext, error) {
    plan, err := p.capability.State(ctx)
    if err != nil {
        return ProviderContext{}, err
    }

    fragments := []ContextFragment{{
        Key:     "planner/meta",
        Content: fmt.Sprintf("Plan has %d steps; current step: %s", len(plan.Steps), plan.CurrentStepID),
        Role:    unified.RoleUser,
    }}

    for _, step := range plan.Steps {
        fragments = append(fragments, ContextFragment{
            Key:     FragmentKey("planner/step/" + step.ID),
            Content: renderStep(step),
            Role:    unified.RoleUser,
        })
    }

    return ProviderContext{Fragments: fragments}, nil
}
```

If a step is deleted, the next provider response simply omits
`planner/step/<id>`. The context manager detects the missing key and records a
`conversation.context_fragment_removed` event.

### Capability Manager

The capability manager is the runtime projection for thread-attached
capabilities. It is responsible for turning durable attachment/state events into
live capability instances.

Sketch:

```go
type CapabilityRegistry interface {
    Register(CapabilityFactory)
    Create(context.Context, CapabilityAttachSpec, CapabilityRuntime) (Capability, error)
}

type CapabilityFactory interface {
    Name() string
    New(context.Context, CapabilityAttachSpec, CapabilityRuntime) (Capability, error)
}

type CapabilityAttachSpec struct {
    ThreadID       ThreadID
    BranchID       BranchID
    CapabilityName string
    InstanceID     string
    Config         json.RawMessage
}

type CapabilityManager interface {
    Attach(context.Context, CapabilityAttachSpec) (Capability, error)
    Replay(context.Context, []Event) error
    Capability(instanceID string) (Capability, bool)
    Tools() []tool.Tool
    ContextProvider() ContextProvider
}
```

`Attach` appends `capability.attached` through its runtime/event appender.
Commands exposed by capability tools call into the capability; the capability
validates actions, appends `capability.state_event_dispatched` through its
runtime, and only then applies state in memory. The manager routes replayed
state events to the correct instance by `InstanceID`.

Branch behavior:

- A branch inherits capability attachments and state events from its ancestry.
- New state events on a branch mutate only that branch's materialized
  capability state.
- Detaching a capability appends `capability.detached` for that branch and
  removes its tools/context from later renders.

The app owns the registry of available capability types. The thread owns which
instances are attached. The model-running agent only sees the resulting tools
and context.

### Context Fragment

Fragments are model-visible context items with explicit role and markers.
Fragment keys are stable within a provider. The canonical identity is
`ProviderKey + FragmentKey`.

Sketch:

```go
type ContextFragment struct {
    Key         FragmentKey
    Role        unified.Role
    StartMarker string
    EndMarker   string
    Content     string
    Fingerprint string
    Authority   FragmentAuthority
    CachePolicy ContextCachePolicy
}

type ContextCachePolicy struct {
    Stable      bool
    MaxAge      time.Duration
    CacheScope  ContextCacheScope // thread, branch, turn, none
}

type ContextCacheScope string

const (
    ContextCacheNone   ContextCacheScope = "none"
    ContextCacheTurn   ContextCacheScope = "turn"
    ContextCacheBranch ContextCacheScope = "branch"
    ContextCacheThread ContextCacheScope = "thread"
)
```

If `Fingerprint` is empty, the context manager computes it from the renderable
fragment fields. Providers should set fingerprints only when they can do it
cheaper or more semantically than hashing the rendered content.

Granularity matters. Large state should be split into smaller stable fragments
instead of one large blob. A planner should prefer:

```text
planner/meta
planner/step/<step_id>
planner/open_question/<question_id>
```

over a single `planner/full` fragment when steps or questions can change
independently.

Authority guidance:

- `developer`: harness-owned policy and capability instructions.
- `user`: project/user/repo-provided content and environment facts.
- `tool`: only actual tool outputs, not persistent context.

Examples:

- permissions and sandbox rules: `developer`
- available skill catalog: `developer`
- loaded skill body: `user`
- AGENTS.md: `user`
- environment facts: `user`
- planner state: usually `user`, unless the planner is a harness policy
  capability

### Diff Semantics

Use manager-owned mechanical diffs over provider-returned full current fragments.
The context manager does not compute semantic changes such as "step title
changed"; it computes key/fingerprint changes.

Render record:

```go
type ProviderRenderRecord struct {
    ProviderKey ProviderKey
    Fingerprint string
    SnapshotRef *SnapshotRef
    Fragments   map[FragmentKey]RenderedFragmentRecord
    RenderedAt  EventID
}

type RenderedFragmentRecord struct {
    Key         FragmentKey
    Fingerprint string
    EventID      EventID
    Removed      bool
}

type ContextRenderDiff struct {
    Added     []ContextFragment
    Updated   []ContextFragment
    Removed   []ContextFragmentRemoved
    Unchanged []RenderedFragmentRecord
}

type ContextFragmentRemoved struct {
    ProviderKey         ProviderKey
    FragmentKey         FragmentKey
    PreviousFingerprint string
}
```

Diff algorithm per provider:

1. Call `GetContext`.
2. Normalize and sort returned fragments by `FragmentKey`.
3. Compute missing fingerprints.
4. Compare returned keys and fingerprints to the previous
   `ProviderRenderRecord`.
5. Append `conversation.context_fragment` for added or updated fragments.
6. Append `conversation.context_fragment_removed` for previously-rendered keys
   missing from the current provider context.
7. Persist the new `ProviderRenderRecord`.

Deletion example:

```text
previous: planner/meta, planner/step/a, planner/step/b
current:  planner/meta, planner/step/a
diff:     remove planner/step/b
```

The tombstone is an internal projection event. It does not need to be shown to
the model as prose; it tells prompt projection that the old fragment is no
longer active.

Recommended defaults:

- Permissions: full fragment on any change.
- Available tools: full concise catalog on catalog/activation change.
- Loaded skills: full loaded-skill block when loaded set/body digest changes.
- AGENTS.md: full scoped instructions when effective digest changes.
- Environment: separate current fragments for cwd, git, network, and date/time
  facts.
- Planner state: multiple stable fragments, usually one metadata fragment plus
  one fragment per step or open question.
- Time/state providers: stable rounded fragments, such as current minute, so
  fingerprints naturally change at the desired rate.

Later fragments override earlier fragments by recency. The event log is not
rewritten when context changes.

### Context Manager

The context manager owns provider polling, mechanical diffs, render records,
context event emission, and cache lookup.

Sketch:

```go
type ContextManager interface {
    RegisterProvider(ContextProvider)
    BuildContext(context.Context, ContextBuildRequest) (ContextBuildResult, error)
}

type ContextBuildRequest struct {
    ThreadID     ThreadID
    BranchID     BranchID
    TurnID       string
    HarnessState HarnessState
    Reason       ContextRenderReason
    Preference   ContextPreference
    TokenBudget  int
}

type ContextBuildResult struct {
    Events        []Event
    ActiveRecords []ProviderRenderRecord
    Fragments     []ContextFragment
}
```

`BuildContext` polls providers in stable order. For each provider, it loads the
previous `ProviderRenderRecord`, calls `GetContext`, computes the mechanical
diff, emits context events for changed fragments/removals, and stores the next
render record. Prompt projection uses active render records, not the raw event
log scan, as its fast path.

### Context Events

Rendered context changes are durable events.

```go
type ContextFragmentRecorded struct {
    ProviderKey ProviderKey       `json:"provider_key"`
    FragmentKey FragmentKey       `json:"fragment_key"`
    Role        unified.Role      `json:"role"`
    Authority   FragmentAuthority `json:"authority"`
    StartMarker string            `json:"start_marker,omitempty"`
    EndMarker   string            `json:"end_marker,omitempty"`
    Content     string            `json:"content"`
    Fingerprint string            `json:"fingerprint"`
}

type ContextFragmentRemovedRecorded struct {
    ProviderKey         ProviderKey `json:"provider_key"`
    FragmentKey         FragmentKey `json:"fragment_key"`
    PreviousFingerprint string      `json:"previous_fingerprint"`
}
```

`conversation.context_fragment` stores the full replacement content for one
fragment. `conversation.context_fragment_removed` stores a tombstone for one
previously active fragment. Prompt projection reconstructs active context by
applying replacement and removal events in branch order.

### Context Caching

The manager-owned diff model gives straightforward cache keys:

```text
thread_id + branch_id + provider_key + fragment_key + fingerprint
```

Useful caches:

- rendered fragment text: reuse byte-identical content
- token count: avoid recounting unchanged fragments
- provider-level fingerprint: quickly skip prompt assembly work when a whole
  provider is unchanged
- provider snapshot refs: avoid storing large provider snapshots inline
- prompt prefix segments: keep stable high-authority fragments byte-identical
  for provider-side prompt caching

Caching rules:

- Cache entries are derived data. The event log and render records remain the
  source of truth.
- Stable ordering matters. Providers should return deterministic fragment keys,
  and the context manager should sort by provider order then fragment key.
- Cache invalidation is fingerprint-based. If content or render metadata changes
  in a model-visible way, the fingerprint changes.
- Large state should be split into fragments whose update frequency matches the
  domain. This is how small updates avoid invalidating a large cached blob.
- A provider may expose a cheap provider-level fingerprint later, but this is an
  optimization. The base contract still allows the context manager to hash
  returned fragments.

Optional fast path:

```go
type FingerprintingContextProvider interface {
    ContextProvider
    StateFingerprint(context.Context, ContextRequest) (fingerprint string, ok bool, err error)
}
```

If `ok` is true and the fingerprint matches the previous provider render record,
the context manager can skip `GetContext` for that provider and reuse the active
fragment records. Providers that cannot compute this cheaply should not
implement the fast path.

## Provider Projection And Continuation

Agentsdk owns durable conversation trees, branch selection, turn commit
semantics, and full canonical request projection. Provider transports and
provider-internal continuation are llmadapter concerns.

Important rule:

```text
agentsdk -> full canonical request projection
llmadapter -> may internally optimize transport/continuation
```

For Codex, this means agentsdk still sends a replay-capable full request even
when llmadapter uses Codex WebSocket and internal `previous_response_id` behind
the scenes. Agentsdk should not expose a Codex-specific delta-only request path.

### Projection Strategy

Projection strategy is driven by llmadapter route/provider metadata, not by
provider-name heuristics.

```go
type ProviderTurnMetadata struct {
    ProviderName         string
    APIKind              string
    APIFamily            string
    NativeModel          string
    ResponseID           string
    ConsumerContinuation unified.ContinuationMode
    InternalContinuation unified.ContinuationMode
    Transport            unified.TransportKind
    Extensions           unified.Extensions
    Usage                unified.Usage
    RequestFingerprint   string
}
```

Rules:

- Use public `previous_response_id` only when the selected route explicitly
  reports `consumer_continuation=previous_response_id`.
- Treat `consumer_continuation=replay` as full replay, even if
  `internal_continuation=previous_response_id`.
- Store provider metadata only for committed turns.
- Failed, canceled, or incomplete turns do not advance provider continuation
  state.
- Branch switches must never reuse a previous provider response from another
  branch.
- Model/provider/API/instruction/context rewrites invalidate public native
  continuation unless the selected route explicitly proves it is safe.

### Codex Session Hints

Codex WebSocket continuation is internal to llmadapter, but it needs stable
caller hints to key branch-safe provider state.

When the selected route is Codex Responses, add extensions:

```go
unified.SetCodexExtensions(&req.Extensions, unified.CodexExtensions{
    InteractionMode: unified.InteractionSession,
    SessionID:       string(threadID),
    BranchID:        string(branchID),
    BranchHeadID:    string(branchHeadID),
    InputBaseHash:   inputBaseHash,
    ParentResponseID: parentResponseID,
})
```

Naming note: llmadapter calls this `codex.session_id`, but in the new agentsdk
architecture the stable value should be the durable `ThreadID`. A live runtime
session ID may be recorded separately for diagnostics, but it should not be the
provider session key for persisted thread replay.

`InputBaseHash` should be derived from the normalized full provider request
projection. The caller-provided hash is a hint for llmadapter diagnostics and
keying; llmadapter must still verify lineage from the actual outgoing request.

### Metadata Commit Boundary

During a model turn:

1. `provider.route_selected` records route identity and declared continuation
   capability from llmadapter `RouteEvent`.
2. The runner streams the response.
3. Provider execution metadata, such as actual transport and internal
   continuation, is collected from llmadapter events.
4. On successful turn completion, append assistant/tool events plus
   `provider.execution_metadata_recorded` atomically.
5. On failure, append failure/ops events if useful, but do not update committed
   continuation metadata.

This preserves the event-sourced rule: provider continuation is a property of a
completed, committed branch head, not a speculative stream.

## Planner Capability First Slice

Implement the planner as the first concrete stateful capability. Unlike the
earlier self-contained-only option, introduce small shared `capability` and
`agentcontext` packages now because other providers/capabilities are expected
soon.

Decisions:

- Shared package shape: `capability`, `agentcontext`, and
  `capabilities/planner`.
- Planner command API: validate actions, build events, append them through the
  capability runtime, then apply them to in-memory state.
- Outer durable capability payloads live in shared `capability`.
- Plan creation requires explicit `plan_created`.
- One planner capability instance owns one plan.
- Step removal hard-removes from materialized state.
- Tool results are model-facing only; they do not return emitted events or
  persistence metadata.
- IDs use `gonanoid`, while caller-provided IDs remain supported.
- Context provider types live in shared `agentcontext`.
- Tool count stays low through one action-batch planner tool.
- Batch actions are all-or-nothing.

Target packages:

```text
capability
agentcontext
capabilities/planner
```

Initial files:

```text
capability/
  capability.go    Capability, StatefulCapability, registry/factory contracts
  events.go        capability.attached and capability.state_event_dispatched payloads
  runtime.go       CapabilityRuntime and event append contract

agentcontext/
  provider.go      ContextProvider, ProviderContext, ContextRequest
  fragment.go      ContextFragment, ProviderKey, FragmentKey

capabilities/planner/
planner.go        Plan, Step, Status, Planner
actions.go        planner action types and batch command handling
events.go         typed inner state events and event registry declarations
replay.go         ApplyEvent and replay helpers
context.go        granular context fragment rendering
tools.go          single low-count planner tool wrapper
ids.go            gonanoid-backed ID generation
planner_test.go   replay, action, rendering, deletion tests
```

Initial planner state:

```go
type Plan struct {
    ID            string
    Title         string
    CurrentStepID string
    Steps         []Step
}

type Step struct {
    ID     string
    Order  int
    Title  string
    Status StepStatus
}

type StepStatus string

const (
    StepPending    StepStatus = "pending"
    StepInProgress StepStatus = "in_progress"
    StepCompleted  StepStatus = "completed"
)
```

Initial inner events:

```text
plan_created
step_added
step_removed
step_title_changed
step_status_changed
step_reordered
current_step_changed
```

Plan creation is explicit. A planner capability instance starts without a plan
until a `plan_created` event is emitted and applied. One planner capability
instance owns exactly one plan; multiple plans can be represented later by
attaching multiple planner instances.

Command/action flow:

```go
type PlannerAction struct {
    Action string     `json:"action"`
    Plan   *PlanPatch `json:"plan,omitempty"`
    Step   *StepPatch `json:"step,omitempty"`
    StepID string     `json:"step_id,omitempty"`
    Status StepStatus `json:"status,omitempty"`
    Title  string     `json:"title,omitempty"`
    Order  *int       `json:"order,omitempty"`
}

type ApplyActionsResult struct {
    Message string
    Plan    Plan
}
```

Planner commands validate all actions, build all state events, append those
events through `CapabilityRuntime`, and only then apply them to in-memory state.
If validation or append fails, no in-memory state is changed. This gives tools an
ergonomic mutation API while keeping replay deterministic and persistence
separate from model-facing tool output.

The single model-facing tool should accept a batch of actions:

```json
{
  "actions": [
    {"action": "create_plan", "plan": {"title": "Thread/context rewrite"}},
    {"action": "add_step", "step": {"title": "Implement planner capability"}},
    {"action": "set_step_status", "step_id": "step_abc", "status": "in_progress"}
  ]
}
```

Each action maps to one or more typed inner state events. The tool response
returns concise human/model-facing text plus the current plan if useful. It does
not return emitted events. The capability itself persists those events by
appending `capability.state_event_dispatched` through its runtime handle.

All-or-nothing action flow:

1. Validate every action against the current plan plus prior actions in the same
   batch.
2. Build all inner state events.
3. Wrap them as durable `capability.state_event_dispatched` events.
4. Append the full event batch through `CapabilityRuntime.AppendEvents`.
5. Apply events to in-memory planner state only after append succeeds.
6. Return a normal tool result.

IDs and ordering:

- Caller-provided IDs are accepted for deterministic tests and replay.
- Empty IDs are generated with `gonanoid`.
- Step ordering is explicit through `Order`, not inferred only from slice
  position.
- Step removal is a hard remove from materialized state. The event log retains
  historical evidence, and context fragment tombstones remove old prompt state.

Initial context fragments:

```text
planner/meta
planner/step/<step_id>
```

Implementation stance:

- Shared `capability` owns outer durable payload types.
- Shared `agentcontext` owns provider/fragment interfaces and types.
- `capabilities/planner` owns planner state, actions, inner event definitions,
  replay, context rendering, and tools.
- Core planner methods create events, append them via `CapabilityRuntime`, and
  apply them only after append success.
- Tool wrappers return normal model-facing results. They do not expose emitted
  events or persistence metadata.

## Prompt Assembly Flow

The context manager polls providers at every prompt render boundary: initial
thread creation, normal user turns, and follow-up model calls after tool-induced
state changes.

### New Thread

1. `ThreadStore.CreateThread` opens a live thread.
2. Discover project resources: AGENTS.md, skills, commands, plugins.
3. Build initial `HarnessState`.
4. Ask each context provider for `GetContext` with `ContextPreferFull`.
5. Compute initial render records and append `conversation.context_fragment`
   events for returned fragments.
6. Persist `harness.context_snapshot_recorded` with provider fingerprints and
   snapshot refs.
7. Append the first `conversation.user_message`.
8. Build provider request from normalized internal items.

### Normal Turn

1. Load branch head and current materialized harness state.
2. Ask each context provider for `GetContext` with `ContextPreferChanges`.
3. The context manager computes key/fingerprint diffs against the previous
   `ProviderRenderRecord`.
4. Append added/updated context fragments and removal tombstones.
5. Persist the new context snapshot/render records even if no fragments changed.
6. Append pending user message.
7. Normalize internal items.
8. Project to a full provider request.
9. Add provider-specific safe hints, such as Codex thread/branch extensions.
10. Run model/tool loop.
11. Append assistant/tool/result events and committed provider metadata
    atomically per completed step.

### Tool-Induced State Change

Example: `skill_load`.

1. Assistant emits `conversation.tool_call`.
2. Tool executes and appends `conversation.tool_result`.
3. Tool also appends `harness.skill_loaded`.
4. Before the follow-up model request, the context manager calls `GetContext`
   on providers with a changes preference.
5. `SkillsProvider` returns its full current skill fragments.
6. The context manager detects the loaded skill fragment as added/updated.
7. The model sees both the causal tool result and the durable context fragment.

The tool result says what happened. The fragment says what persistent context is
now active.

## Normalization Rules

Before every provider request, normalize internal items:

- every tool call must have a matching tool result
- every tool result must have a preceding matching tool call
- canceled/interrupted calls receive synthetic aborted results
- orphaned tool results are dropped from provider projection
- duplicate call IDs are rejected or disambiguated before projection
- unsupported media is stripped or replaced with explanatory placeholders
- provider continuation IDs are metadata, not the primary history model

An orphaned tool result means a result exists without a matching prior call in
the projected branch. A missing result means a call exists without a matching
result. Both can poison provider replay.

## Compaction Model

Compaction is an event and projection choice, not deletion.

1. Run a compaction task over selected internal items.
2. Append `conversation.compaction` with summary, replacement window, source
   node IDs, and strategy metadata.
3. Keep original events intact.
4. `PromptProjection` may choose the compacted replacement window plus latest
   context fragments.
5. Context snapshots remain separate from compaction and can be reinjected after
   compaction when needed.

## Persistence Strategy

Start local and simple:

```text
.agentsdk/threads/YYYY/MM/DD/thread-<timestamp>-<thread_id>.jsonl
.agentsdk/index.sqlite   optional metadata/search/listing index
```

JSONL remains the authoritative event log. SQLite is an index/cache that can be
repaired by replaying JSONL.

`LiveThread` should buffer writes and support:

- deferred file creation until first persisted item
- explicit flush barriers
- shutdown with final flush
- discard on failed session startup
- retry of buffered writes after transient I/O errors

## Migration Plan

### Phase 1: Type Definitions

- Add event envelope and typed event registry.
- Add internal item types.
- Add context snapshot and fragment types.
- Add provider interfaces without changing runner behavior.

### Phase 2: ThreadStore Layer

- Introduce a top-level `thread` package. `ThreadStore` and `LiveThread` live
  there, not in `conversation`.
- Implement `LocalThreadStore` using current JSONL store concepts.
- Add list/read/archive metadata in memory first, then SQLite index.
- Replace existing `conversation.EventStore` usage with the new `thread.Store`
  flow as soon as the first local implementation is usable.

### Phase 3: Projection Layer

- Build event-to-tree and event-to-internal-item projections.
- Add normalization before provider projection.
- Replace direct `conversation.Session.Messages()`-style projection with
  explicit prompt/internal-item projections.
- Drive public continuation strategy from llmadapter metadata, not provider/API
  heuristics.
- Add Codex thread/branch request hints while keeping Codex projection as full
  replay.
- Persist route/provider execution metadata with committed turns.

### Phase 4: Context Manager

- Add core `ContextManager`.
- Implement environment, project instructions, skills, tools, model, and
  permissions providers.
- Persist context snapshots per real user turn.
- Add provider render records, manager-owned key/fingerprint diffing, and
  context fragment removal tombstones.
- Add tests for no-change, full-initial, changed-fragment, removed-fragment,
  and cache-key stability paths.

### Phase 5: Runner Integration

- Change runner to append typed events during model/tool loops.
- Ensure tool-induced harness events are visible to context providers before
  follow-up model requests.
- Add synthetic aborted tool results for cancellation and timeout.
- Add retry/ops events without making them prompt-visible by default.

### Phase 6: Compaction

- Add compaction event type and prompt projection support.
- Add model-driven summary generation behind an option.
- Add replay tests showing original events remain intact.

### Phase 7: API Cleanup

- Replace current `conversation.Session`, `runner.RunTurn`, and
  `runtime.Engine` APIs where they conflict with the new architecture.
- Migrate examples and tests in the same sweep as API changes.
- Remove direct `[]unified.Message` history projection once internal item
  projection is available.

## Testing Strategy

- Event replay reconstructs identical tree and harness state.
- Branch projections are deterministic.
- Context providers emit full initial fragments and no-op unchanged diffs.
- Context manager emits added/updated fragments and removal tombstones from
  provider key/fingerprint diffs.
- Skill loading emits both tool result and loaded-skill context update.
- Tool call/output normalization handles missing and orphaned pairs.
- Codex projection remains full replay while adding thread/branch session hints.
- Public native continuation is used only when llmadapter route metadata reports
  `consumer_continuation=previous_response_id`.
- Provider execution metadata is committed only after successful completed
  turns.
- Branch switch does not reuse provider continuation from another branch.
- Compaction changes prompt projection without deleting source events.
- JSONL index repair reconstructs thread list metadata.
- Context cache keys remain stable for unchanged fragments and change when
  model-visible content changes.

## Design Decisions and Open Options

### ThreadStore Package

Decision: `ThreadStore` lives in a top-level `thread` package.

Rationale:

- `thread` owns product-level lifecycle: create, resume, list, read, archive,
  metadata, live writer, and storage backends.
- `conversation` can remain focused on tree lineage, internal items, projection,
  and prompt semantics.
- Future thread stores may be local, remote, or hybrid without making
  `conversation` depend on storage/indexing concerns.

Sketch:

```go
package thread

type Store interface {
    Create(context.Context, CreateParams) (*Live, error)
    Resume(context.Context, ResumeParams) (*Live, error)
    Read(context.Context, ReadParams) (Stored, error)
    List(context.Context, ListParams) (Page, error)
    Archive(context.Context, ID) error
}
```

### Context Snapshot Granularity

Question: should context snapshots be one combined event or provider-scoped
events?

Option A: single combined snapshot event.

```text
harness.context_snapshot_recorded
payload: {
  "providers": {
    "environment": {...},
    "skills": {...},
    "tools": {...}
  }
}
```

Advantages:

- One durable baseline per turn.
- Simple replay: load the latest snapshot event.
- Easy to reason about all context as one coherent turn boundary.
- Easier to store a single fingerprint for "context changed".

Disadvantages:

- Large payload can churn when one provider changes.
- Plugin/provider schema evolution is concentrated in one event payload.
- Harder to query "latest skills snapshot" without reading the whole snapshot.

Option B: provider-scoped snapshot events.

```text
harness.context_snapshot_recorded
payload: { "provider": "skills", "snapshot": {...} }

harness.context_snapshot_recorded
payload: { "provider": "environment", "snapshot": {...} }
```

Advantages:

- Providers evolve independently.
- Smaller events when only one provider changes.
- Easy to replay/query one provider domain.
- Better fit for plugin-provided context providers.

Disadvantages:

- A turn baseline is a set of events, not one event.
- Requires grouping by turn/correlation ID.
- More append noise for many providers.

Option C: hybrid turn snapshot with provider hashes and optional external
provider payloads.

```go
type ContextSnapshotRecorded struct {
    TurnID    string
    Providers map[string]ProviderSnapshotRef
}

type ProviderSnapshotRef struct {
    Fingerprint string
    Inline      json.RawMessage
    EventID     EventID
}
```

Advantages:

- Keeps a coherent per-turn baseline.
- Allows large/plugin-specific provider snapshots to live separately.
- Supports efficient "did anything change?" checks.

Disadvantages:

- More complex to implement.
- Requires reference integrity checks.

Decision: Option C. Use a hybrid turn snapshot with provider hashes and
optional external provider payloads.

### Plugin Fragment Authority

Question: should plugin-provided context providers be allowed to emit
`developer` fragments?

Option A: plugins may emit only `user` fragments by default.

Advantages:

- Safe default: third-party/plugin content cannot override harness policy.
- Matches the trust split used for loaded skill bodies and project docs.
- Keeps `developer` authority reserved for core runtime/config.

Disadvantages:

- Trusted in-house plugins cannot naturally provide high-authority tool rules.
- Plugin authors may need extra configuration for legitimate policy fragments.

Option B: plugins may request `developer` fragments, gated by trust policy.

```go
type FragmentAuthorityPolicy interface {
    AllowDeveloperFragment(providerID string, fragment ContextFragment) bool
}
```

Advantages:

- Supports trusted enterprise/internal plugins.
- Keeps default safe while allowing explicit escalation.
- Auditable: policy controls which plugin emits high-authority context.

Disadvantages:

- Requires trust configuration and clear UX.
- Misconfiguration can give untrusted plugin content too much authority.

Option C: plugins can emit `developer` only through signed/installed plugin
metadata.

Advantages:

- Stronger supply-chain story.
- Good future fit for plugin marketplaces.

Disadvantages:

- Too heavy for early local plugins.
- Requires signing/trust infrastructure.

Decision: Option B. Default to `user`, allow `developer` only when the app
explicitly grants that plugin/provider authority.

### Git State In Environment Context

Question: how much git state should be included by default?

Option A: minimal git identity only.

```text
branch: main
head: abc1234
dirty: true
```

Advantages:

- Low token cost.
- Stable across large repos.
- Avoids leaking filenames unnecessarily.

Disadvantages:

- Model cannot infer which files changed.
- Less helpful for review/commit/release tasks.

Option B: include changed file summary.

```text
branch: main
head: abc1234
changed_files:
  M conversation/session.go
  A thread/store.go
  D old/file.go
```

Advantages:

- Useful for coding agents and review flows.
- Still compact compared to full diffs.
- Helps model avoid asking for obvious repo status.

Disadvantages:

- Can be large in big worktrees.
- File names can be sensitive.
- Needs truncation and ignore rules.

Option C: provider-controlled git context.

```go
type GitContextPolicy struct {
    Mode       GitContextMode // off, minimal, changed_files, summary
    MaxFiles   int
    MaxBytes   int
    Redactions []string
}
```

Advantages:

- App/agent can tune context by domain.
- Good default for CLI coding agents; stricter for sensitive apps.
- Supports truncation and redaction explicitly.

Disadvantages:

- More configuration surface.
- Requires careful defaults.

Decision: Option C. Git context is provider-controlled and configurable. Use
default `minimal` for SDK and `changed_files` for coding-agent apps, capped by
`MaxFiles` and `MaxBytes`.

### Stateful Capability Context Granularity

Question: should stateful-capability context be one full blob, semantic deltas, or
granular full-state fragments?

Option A: one full-state fragment.

```text
planner/full:
  1. completed: inspect repo
  2. in_progress: add ThreadStore
  3. pending: add tests
```

Advantages:

- Very simple.
- Robust after compaction/resume.
- No deleted-fragment handling beyond replacing one blob.

Disadvantages:

- Any small change replaces the whole state.
- Large capability state wastes tokens and cache space.

Option B: semantic delta fragments.

```text
planner/update:
  Step 2 title changed from "storage" to "ThreadStore lifecycle".
```

Advantages:

- Token efficient for tiny changes.
- Expresses domain intent.

Disadvantages:

- Fragile after compaction, branch switch, or missed context.
- Requires the model to reconstruct state from prior prompt history.
- Makes replay and prompt projection harder.

Option C: granular full-state fragments.

```text
planner/meta
planner/step/<step_id>
planner/open_question/<question_id>
```

Advantages:

- Providers stay simple: return current state fragments, not diffs.
- Context manager can mechanically diff by key/fingerprint.
- Small changes replace only one small fragment.
- Deletions are explicit tombstones for missing keys.
- Good fit for prompt caching because unchanged fragments remain byte-identical.

Disadvantages:

- Providers must choose good stable fragment keys.
- Very fragmented state needs ordering and token-budget policy.

Decision: Option C. Stateful capabilities return granular full-state fragments.
Semantic deltas are out of scope for the first version. If a domain really needs
delta prose, model it as an additional current fragment with clear expiry or
durability rules, not as the primary state representation.

### Plugin Event Schema Strictness

Question: how strict should event schema registration be for arbitrary plugin
events and stateful-capability inner events?

Option A: open JSON events.

```go
Append(Event{
    Kind: "plugin.event",
    Payload: json.RawMessage(`{...}`),
})
```

Advantages:

- Maximum flexibility.
- Easy for experimental plugins.
- No registry needed.

Disadvantages:

- Replay errors move to runtime.
- Hard to index/query safely.
- Hard to validate or migrate schemas.

Option B: strict registry for all event kinds.

```go
type EventRegistry interface {
    Register(kind EventKind, schema EventSchema, decoder Decoder)
    Decode(event Event) (any, error)
}
```

Advantages:

- Strong validation.
- Better migrations and testability.
- Safer replay and projections.

Disadvantages:

- Higher burden for simple plugins.
- Slower experimentation.

Low-boilerplate typed registration should make this tolerable:

```go
var StepAddedEvent = eventkind.Define[StepAdded]("step_added")

type StepAdded struct {
    StepID string `json:"step_id"`
    Title  string `json:"title"`
}

registry.RegisterCapabilityEvent("planner", StepAddedEvent)
```

The global durable event remains `capability.state_event_dispatched`; the
registry validates and decodes the inner `event_name` and `body` for that
capability type.

Option C: tiered strictness.

```go
type EventKindPolicy int

const (
    EventKindExperimental EventKindPolicy = iota
    EventKindRegistered
    EventKindIndexed
)
```

Advantages:

- Allows experiments without blocking.
- Requires registration for events used by projections/indexes.
- Lets product apps enforce stricter policy.

Disadvantages:

- Requires clear rules for promotion from experimental to registered.
- Some complexity remains in replay.

Decision: Option B, with low-boilerplate typed helpers. All durable events that
affect replay, materialized state, indexes, or prompt context must be
registered. Experimental content can still be queued as ephemeral context by a
provider, but it is not durable thread state unless it has a registered schema.

## Near-Term Recommendation

Start by adding types and tests for internal items, context fragments, and
normalization. This is the smallest foundation that improves current behavior
and does not force an immediate storage migration.

Then add `ThreadStore` as a wrapper over the existing JSONL event store. Once
the lifecycle boundary exists, context snapshots, metadata indexing, archiving,
and compaction can land incrementally.
