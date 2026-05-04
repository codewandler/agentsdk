# Persistence, events, output, context, capabilities, and skills

## Output event model

This section defines the target model for replacing remaining
writer fields. The implementation rule is intentionally conservative: new code
should target these shapes, while existing writer paths are migrated only in
small follow-up slices that keep terminal dogfood stable.

## Goals

- Keep execution code structured: agent, harness, command, workflow, and safety
  code should publish typed data, not terminal strings.
- Keep rendering at channel boundaries: terminal, TUI, HTTP/SSE, JSON clients,
  and LLM-facing tool summaries choose their own representation.
- Preserve current dogfood behavior while removing writer-only seams gradually.
- Avoid one renderer interface that tries to fit every channel equally poorly.

## Non-goals

- This is not the full risk/safety policy design.
- This is not asynchronous workflow lifecycle.
- This is not a terminal UI rewrite.
- This does not remove every `fmt.Fprintf` that formats a payload into a
  `strings.Builder`; payload-local formatting is acceptable until a renderer
  registry is justified.

## Core event envelope

The target publication unit is an output event:

```go
type Event struct {
    ID        string
    Time      time.Time
    Scope     Scope
    Kind      Kind
    Severity  Severity
    Payload   any
    Source    Source
    Trace     Trace
}
```

Recommended fields:

- `ID`: optional stable event ID for streams and persisted events.
- `Time`: producer timestamp.
- `Scope`: app/session/turn/step/workflow identifiers.
- `Kind`: semantic event kind, such as `command.result` or `usage.recorded`.
- `Severity`: debug/info/warn/error.
- `Payload`: typed payload; never pre-rendered terminal text unless the payload
  is explicitly a legacy text payload.
- `Source`: component and package that emitted the event.
- `Trace`: correlation IDs for request, run, command path, or tool call.

## Scope shape

```go
type Scope struct {
    AppName      string
    SessionName  string
    SessionID    string
    AgentName    string
    TurnID       string
    WorkflowName string
    WorkflowRun  string
    StepID       string
}
```

This keeps channel consumers from parsing IDs out of display strings.

## Event kinds and payloads

### Displayable payloads

Displayable payloads represent user-facing information. The payload should be
serializable and should carry enough structure for non-terminal renderers.

```go
type DisplayPayload struct {
    Title       string
    Summary     string
    Body        any
    Fields      []Field
    Items       []Item
    Tables      []Table
    Attachments []AttachmentRef
}
```

Rules:

- Prefer typed domain payloads (`SessionInfoPayload`, `WorkflowRunPayload`) when
  a command already has one.
- Use `DisplayPayload` for generic notices, short summaries, and simple lists
  only after repeated shapes justify it.
- Large values should be references, not embedded strings.

### Notices

Notices are structured status messages that are not command results by
themselves.

```go
type NoticePayload struct {
    Code    string
    Level   Severity
    Message string
    Details []Field
}
```

Examples:

- fallback plugin selected
- no workflows configured
- compatibility evidence unavailable
- session resumed from path

### Command results

Command execution should publish both the raw `command.Result` and command
metadata:

```go
type CommandResultPayload struct {
    Path       []string
    Descriptor command.Descriptor
    Result     command.Result
    Error      string
}
```

`command.Result` stays the trusted in-process result type. The output event adds
channel-independent context around it.

### Workflow events

Workflow output publication mirrors the workflow event stream:

```go
type WorkflowEventPayload struct {
    RunID        workflow.RunID
    WorkflowName string
    Status       workflow.RunStatus
    StepID       string
    ActionName   string
    InputRef     string
    OutputRef    string
    Error        string
}
```

Rules:

- Synchronous `Session.ExecuteWorkflow` publishes a summary event today.
- Future async workflow lifecycle should publish started/running/queued/canceled
  events using the same envelope.
- Full workflow event history remains in workflow/thread storage; channel events
  are the live stream/read-model projection.

### Usage records

Usage is already structured in `usage.Record`; output publication should wrap it:

```go
type UsageRecordPayload struct {
    Record usage.Record
    Totals usage.Aggregate
}
```

Rules:

- Usage persistence errors are diagnostics, not usage records.
- Terminal can continue printing per-step and per-session usage summaries.
- JSON/SSE clients should receive raw usage dimensions and totals.

### Diagnostics

Diagnostics are for operational or configuration issues:

```go
type DiagnosticPayload struct {
    Code      string
    Component string
    Message   string
    Error     string
    Details   []Field
}
```

Examples:

- invalid resources
- plugin resolution failures
- usage persistence append failure
- renderer failure
- compatibility policy diagnostic

### Debug events

Debug events are explicit and opt-in:

```go
type DebugPayload struct {
    Label string
    Data  any
}
```

Rules:

- Debug-message output currently printed by terminal should become
  `KindDebug`/`DebugPayload`.
- Debug payloads may be redacted by channel policy.
- Debug events are not model-visible unless an LLM summary renderer explicitly
  includes them.

### Risk/safety events

Risk and safety events need a separate policy model. The minimum publication
shape is:

```go
type RiskPayload struct {
    Operation string
    Risk      string
    Decision  string
    Details   []Field
}
```

Do not migrate risk logging opportunistically. Keep current terminal risk logging
until the safety/risk policy section defines approval gates, audit trails, and
channel-specific UX.

## Renderer contracts

### Terminal renderer

Contract:

```go
type TerminalRenderer interface {
    RenderTerminal(Event, io.Writer) error
}
```

Requirements:

- May use ANSI color and progressive streaming.
- May render markdown live.
- Should never be called from core agent/runtime code.
- Owns terminal-specific wording, spacing, and status lines.

Current terminal writer paths map here:

- runner event display in `terminal/ui`
- one-shot command rendering in `terminal/cli/run.go`
- REPL command rendering in `terminal/repl`
- usage summaries in `terminal/ui/usage.go`

### TUI renderer

Contract:

```go
type TUIRenderer interface {
    Reduce(Event) TUIState
}
```

Requirements:

- Treats events as state updates, not line output.
- Keeps partial reasoning/text/tool state separately from final summaries.
- Needs stable event IDs and correlation scope.

### HTTP/SSE renderer

Contract:

```go
type StreamRenderer interface {
    EncodeEvent(Event) (name string, data []byte, err error)
}
```

Requirements:

- Uses event kind as SSE event name.
- Encodes JSON payloads without terminal formatting.
- Supports replay by event ID when persistence exists.

### JSON / machine-readable renderer

Contract:

```go
type JSONRenderer interface {
    MarshalEvent(Event) ([]byte, error)
}
```

Requirements:

- Stable field names.
- Typed payload discriminator.
- No ANSI, markdown-only formatting, or human-only labels as canonical data.

### LLM-facing summary renderer

Contract:

```go
type LLMSummaryRenderer interface {
    Summarize(Event) (string, error)
}
```

Requirements:

- Compact, safe, and explicit about failures.
- Uses command descriptors and payload schemas where available.
- Redacts debug/risk details unless the policy allows them.

## `Display(mode)` decision

`payload.Display(mode)` remains sufficient for current command payloads because
commands are the only mature structured-result boundary today. Keep it for:

- `command.DisplayTerminal`
- `command.DisplayJSON`
- `command.DisplayLLM`
- small payload-local renderers that already have focused tests

Do not expand `Display(mode)` into the general event rendering system. Event
rendering needs scope, severity, timing, stream/replay IDs, and channel policy;
those do not belong on every command payload.

## Renderer registry decision

Do not add a global renderer registry yet.

Trade-off:

- A registry would make pluggable channels possible.
- It would also add indirection before there are enough independent channel
  implementations to prove the abstraction.

Decision: keep direct renderer functions/interfaces per channel. Add a registry
only after at least terminal plus one non-terminal channel need the same dynamic
lookup behavior.

## Migration plan for existing writer paths

### `harness.SessionLoadConfig.App.Output`

~~Current role: passes an `io.Writer` to `agent.WithOutput` for verbose/diagnostic
agent output.~~

Done: `AppLoadConfig.Output` now flows to `SessionOpenRequest.Out` →
`Session.out`. The session owns its terminal writer directly; it no longer
passes a writer to the agent.

### `agent.WithOutput`

~~Current role: writer for usage persistence diagnostics and auto-compaction
messages.~~

Done: `WithOutput` deleted. `out io.Writer` removed from `agent.Instance`.
Usage persistence errors route through `DiagnosticHandler` →
`SessionEventDiagnostic`. Compaction rendering deleted — events already flow
through `CompactionEventHandler` → `SessionEventCompaction`.

### Terminal event handler writer paths

Current role: `terminal/ui.AgentEventHandlerFactory(out)` renders runner events
directly to `io.Writer`. The factory accepts `runner.EventHandlerContext` (not
`*agent.Instance`) so `terminal/ui` does not import the `agent` package.

Target: terminal should subscribe to session/agent output events and render them
through a terminal renderer. Keep current handler until the event stream covers
runner events completely.

### Debug-message output

Current role: `terminal/cli/load.go` writes debug-message payloads directly.

Target: publish `DebugPayload` events and let terminal/JSON renderers decide
whether to display or redact them.

### Auto-compaction output

~~Current role: `agent.maybeAutoCompact` writes success/failure text to
`agent.Out()`.~~

Done: `compact_render.go` deleted. Compaction lifecycle events already flow
through `CompactionEventHandler` → `SessionEventCompaction` with full metadata
(`replaced_count`, `tokens_before`, `tokens_after`, `tokens_saved`). Terminal
renderers subscribe to session events.

### Usage persistence error output

~~Current role: verbose `fmt.Fprintf(a.Out(), ...)` for marshal/append failures.~~

Done: usage persistence errors emit `agent.Diagnostic{Component:
"usage_persistence", ...}` through `DiagnosticHandler` → harness publishes
`SessionEventDiagnostic`.

## Risk logging decision

Keep risk logging out of this slice. The current terminal risk log path remains
until the safety/risk policy section defines approvals, audit trail persistence,
and channel-specific UX.

## Acceptance checklist

This design intentionally completes section 5 by defining the model and migration
contracts. It does not mechanically replace every writer call in the same slice;
that would mix design with broad behavior changes and risk destabilizing the
terminal dogfood path.

## Agent/runtime boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) identifies writer/output and usage side paths in `agent.Instance` as cleanup candidates. The target remains unchanged: execution code should publish typed data, while terminal, HTTP/SSE, and future UI channels render structured events/results at the boundary.

## Persistence and thread model

The `thread` package remains the durable event foundation with tightened
inspection/replay contracts.

## Foundation

The durable model remains an append-only thread log:

- `thread.Store` owns create/open/resume/fork/list/archive semantics.
- `thread.Live` is the append handle for one thread branch.
- JSONL remains the first concrete durable store.
- `thread.MemoryStore` remains the fast in-process/test implementation.

No additional store backend was added in this slice. The existing `thread.Store`
interface is still the abstraction boundary; adding a database-backed store later
should not require changing harness, runtime, workflow, skill, or usage replay
contracts.

## Event schema versioning

`thread.Event` now carries `SchemaVersion`. Stores default omitted/zero versions
to `thread.CurrentEventSchemaVersion` while preserving older JSONL files that do
not include the field. JSONL records still keep their envelope `version`; event
`schema_version` is the payload/event-contract version used by replay code.

Current policy:

- new events are written with `schema_version: 1`;
- older files without the field are treated as version 1;
- future migrations should branch on `Event.Kind` + `SchemaVersion` and keep old
  decoders until old session files are intentionally unsupported.

## Harness lifecycle inspection

`harness.Session.ThreadEvents(ctx)` exposes persisted events for the current
session thread/branch when the session is thread-backed. It returns `(events,
false, nil)` for non-thread-backed sessions. This gives terminal, HTTP/SSE, tests,
and debugging surfaces a stable harness-level way to inspect durable replay input
without reaching into `agent.Instance` or JSONL store paths directly.

Existing lifecycle APIs remain the primary session controls:

- `Service.OpenSession`
- `Service.ResumeSession`
- `Service.Sessions`
- `Session.Close`
- `Service.Close`

## Replay coverage

This slice makes replay expectations explicit in tests:

- workflow runs are projected from thread-backed workflow events;
- context renders are replayed by `runtime.ResumeThreadRuntime`;
- capability state is replayed by the capability manager/runtime path;
- skill activations and exact reference activations are replayed by `agent` from
  skill thread events;
- usage records are replayed into `usage.Tracker` from `harness.usage_recorded`;
- harness session thread events are inspectable and carry schema versions.

Several of those areas already had coverage before this slice; section 21 keeps
that coverage in place and adds targeted gaps around event versioning, harness
thread event inspection, and usage replay.

## Compaction and indexing

Compaction already records durable conversation/context events through the thread
runtime. This slice did not add a new compaction read model or workflow event
index because current projection performance is sufficient for dogfood-sized JSONL
threads. Add indexing only after real sessions show lookup or replay costs that
need it.

## Non-goals

- No database store backend yet.
- No global event bus separate from thread.
- No migration CLI yet.
- No workflow-run side index until JSONL projection becomes insufficient.

## Context system

`agentcontext.Manager` remains the render/replay owner with tightened
inspection/lifecycle seams. The goal is not to move context rendering
into channels or plugins; channels inspect context state and plugins contribute
providers.

## Ownership model

| Layer | Responsibility |
|-------|----------------|
| `agentcontext` | Provider interfaces, render records, diffs, replayable snapshots, cache policy metadata, and descriptors. |
| `agentcontext/contextproviders` | Built-in provider implementations for environment, time, model, tools, skills, files, git, commands, and project inventory. |
| `agent` | Builds the baseline agent-local provider set and registers providers on the runtime context manager. |
| `app` | Collects app/plugin context contributions and forwards them when an agent is instantiated. |
| `harness` | Projects session-scoped providers, such as the agent-callable command catalog, and exposes context inspection APIs. |
| `channel/*` | Presents context state over protocol-specific surfaces without rendering or mutating providers. |

`agentcontext.Manager` remains the single render/replay model. It owns provider
ordering, duplicate-key validation, diff calculation, render records, and the
machine-readable `StateSnapshot` used by harness/channel inspection.

## Provider lifecycle

Context providers now fall into explicit lifecycle categories:

- **app-level providers** come from `app.WithAgentOptions(...)` or app/plugin
  configuration and are registered for every instantiated agent;
- **plugin app-scoped providers** implement `app.ContextProvidersPlugin` and must
  be stateless/config-only because the plugin instance is registered once;
- **plugin agent-local providers** implement `app.AgentContextPlugin` and are
  created during agent instantiation after skill/tool state is available;
- **session projection providers** are attached by `harness.Session`, for example
  the `agent_command_catalog` provider used by the `session_command` tool;
- **agent-local baseline providers** are built by `agent.Instance` for current
  environment, time, model, tools, skills, and instruction files.

This keeps app/plugin contribution separate from session projection. A plugin can
contribute context, but a harness session decides which session-scoped providers
are projected into an agent.

## Agent façade responsibility

`agent.Instance` still assembles the baseline provider list because it already
owns the active tool set, skill state, model identity, workspace, and instruction
paths. This section does not introduce a new context composition service because
that would add indirection without deleting duplicated code. The narrower seam is
inspection: `agent.Instance` exposes descriptors and snapshots from the runtime
instead of forcing callers to parse human text.

## Inspection APIs

New inspection surfaces:

- `agentcontext.ProviderDescriptor` and `agentcontext.DescribedProvider` publish
  side-effect-free provider metadata;
- `agentcontext.Manager.Descriptors()` lists registered providers without
  rendering them;
- `agentcontext.Manager.StateSnapshot()` returns provider descriptors plus the
  last committed render records in a JSON-friendly shape;
- runtime, agent, and harness wrappers expose those snapshots without moving
  ownership out of the manager;
- `/context` now returns a structured display payload; terminal mode remains
  human-readable, JSON/machine channels can consume the payload;
- HTTP exposes `GET /api/agentsdk/v1/sessions/{session}/context`.

The snapshot includes rendered fragment content because that content is already
model-visible. Channel adapters should redact or authorize the endpoint before
exposing it to untrusted clients.

## Cache policy conventions

Provider descriptors carry the default cache policy when it is known. Built-in
providers use explicit cache scopes:

- environment and instruction-file context are stable thread-scoped context;
- time and git/project inventory are turn-scoped or time-bucketed context;
- model/tool context is turn-scoped because active model/tool state can change;
- skill inventory is thread-scoped and changes through activation state.

The tests cover descriptor export, state snapshots, HTTP context inspection, and
existing manager replay/diff behavior.

## Capability system

Capabilities are explicit, replayable session/runtime extensions rather than hidden agent defaults.

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

## Skill system

Skill ownership is deliberately boring while the runtime and CLI surfaces are tightened around it.

## Ownership boundary

Skill repository construction stays in `agent.Instance` for now:

- `app` and plugins contribute `skill.Source` values.
- `agent.New` scans those sources into a `skill.Repository` and creates session-scoped `skill.ActivationState`.
- `harness` exposes inspection and activation commands over the current session agent.
- Model tools receive both the mutable state and a session-aware activator in `tool.Ctx.Extra()`.

This avoids introducing a separate skill service before the daemon/session lifecycle has more production pressure. Revisit moving skill state outward only if multiple live agents need to share one mutable skill activation projection.

## Persisted activation

Thread-backed agents persist dynamic skill activation as thread events:

- `skill.EventSkillActivated`
- `skill.EventSkillReferenceActivated`

The model-facing `skill` tool now prefers `skill.ActivatorContextKey` when present. `agent.Instance` installs itself as that activator in the default tool context, so tool-driven skill/reference activations use `agent.ActivateSkill(...)` and `agent.ActivateSkillReferences(...)` instead of mutating `skill.ActivationState` directly. That keeps resumed sessions consistent with user-driven harness commands.

Fallback behavior remains: direct `ActivationState` mutation still works for tests or custom hosts that only provide `skill.ContextKey`.

## Harness commands

Skill commands are command-tree based:

```text
/skills
/skill activate <name>
/skill refs <name>
/skill ref <name> <path>
```

`/skills` lists discovered skills, status, source identity, active references, and replay diagnostics. `/skill refs` lists exact reference paths and trigger metadata. `/skill ref` activates one exact reference and persists it through the agent activator.

## Discover output

`agentsdk discover` now reports discovered skill references under the `Skills:` section when skill sources are available:

```text
Skills:
  go  Go skill  .agents/skills/go
  References:
    go/references/testing.md  triggers=tests
```

This is intentionally an inventory surface, not an activation surface. Runtime activation still belongs to harness/session commands and the model tool.

## Context metadata

The skill inventory context provider now includes richer metadata for each catalog entry:

- source label and source ID
- skill directory
- activation status
- domain, role, risk, compatibility
- allowed tools
- discovered reference count and exact reference paths

Active skill bodies and active reference bodies continue to be materialized only when activated.
