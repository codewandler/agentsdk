# 06 — Agent Façade Audit

This note records the section-3 `agent.Instance` audit from `docs/04_TASKLIST.md`.
It is intentionally conservative: `agent.Instance` remains the public façade while
ownership seams stabilize in harness/session, runtime, context, capability, and
output design work.

## Current shape

`agent.Instance` is currently the SDK-facing façade for one configured agent. It
combines option normalization, model/client routing, runtime construction,
session/thread setup, skill state, context provider composition, capability
attachment, usage recording, writer diagnostics, and runner event handling.

That breadth is acceptable pre-1.0 only because the type is still the main
composition point between app resources, harness sessions, and terminal CLI
policy. The cleanup rule for now is:

> Extract only when the new seam deletes duplication or gives a clearer owner.
> Do not split `agent.Instance` into new public types just to make the file
> smaller.

## Responsibility classification

| Responsibility | Current owner | Classification | Notes / next seam |
| --- | --- | --- | --- |
| Model routing and policy | `agent.Instance` plus `agent/model_policy*.go` | Keep in `agent` for now | The policy depends on `InferenceOptions`, auto-mux result, source API, and compatibility evidence. It is already partially separated into model policy files. Extract further only if non-agent hosts need the same policy path directly. |
| Runtime construction | `agent.Instance.initRuntime`, `runtimeOptions`, `baseRuntimeOptions` | Keep façade, runtime remains execution owner | `agentruntime.Engine` owns turn execution. `agent.Instance` should continue translating agent options/spec state into runtime options until harness/session owns more lifecycle. |
| Session/thread setup | `agent.Instance.initSession`, `startPersistentSession`, `startEphemeralCapabilitySession` | Candidate for harness/session after lifecycle API lands | JSONL-backed session open/resume is real lifecycle work, but today it is not duplicated enough to justify a new abstraction. Move it only alongside stable harness open/list/resume/close APIs. |
| Skill repository/state | `agent.Instance.initSkills`, activation methods, replay helpers | Keep in `agent` for now | Skill inventory affects materialized system prompt and context providers. It can move outward only if session lifecycle starts owning skill activation persistence. |
| Context provider setup | `agent.Instance.runContextProviderFactories`, `contextProviders`, `initThreadRuntime` registration | Keep helper-level seam in `agent` | Context providers need per-agent state and thread-runtime registration. Current helper keeps duplicate registration out of runtime options. Revisit when app/plugin/session provider lifecycle is clarified. |
| Capability registry/session setup | `agent.Instance.ensureCapabilityRegistry`, `initThreadRuntime`, capability specs in runtime options | Keep explicit and host-owned | Capabilities intentionally have no hidden default registry. Setup belongs near thread runtime because capability events are thread-backed. A new seam is useful only if harness/session owns thread runtime lifecycle. |
| Usage tracking | `usage.Tracker` in `agent.Instance`, `recordEvent`, `persistUsageEvent`, `replayUsageEvents` | Keep until event publication model exists | Usage is sourced from runner events and persisted to thread events when available. Extract when structured event/displayable publication replaces writer/event side paths. |
| Writer output | `WithOutput`, `Out`, verbose diagnostics, compaction/debug paths | Mark unstable; do not expand | Writer output is a known pre-1.0 seam. Do not move opportunistically. Replace after the structured output/event model is designed. |
| Event handling | `newEventHandler`, `recordEvent`, optional `WithEventHandlerFactory` | Keep façade hook | The façade needs to update route/usage state before host handlers see events. Preserve this ordering. Broader event subscription belongs in harness/session lifecycle work. |

## Decisions from this audit

### Session/thread opening

No extraction in this round.

`initSession` still contains JSONL store knowledge (`thread/jsonlstore`) and both
persistent and in-memory capability thread paths. That is not ideal, but moving it
now would create an abstraction before the harness API has stable open/list/resume
semantics. The better next owner is likely a harness/session lifecycle component
that can also serve CLI resume and non-terminal channels.

### Context provider lifecycle

No extraction in this round.

The current helpers already prevent one important ownership bug: context providers
are registered either through engine options or directly on `ThreadRuntime`, not
both. A new helper package would not delete duplication today.

### Capability session setup

No extraction in this round.

Capability setup is explicitly tied to thread runtime creation and persisted
capability events. It should move only if thread runtime lifecycle moves with it.

### JSONL store knowledge

Keep in `agent` temporarily.

Direct `threadjsonlstore.Open(...)` calls are limited to agent session open/resume
paths. Reducing that knowledge is desirable, but the cleaner owner is the future
harness/session lifecycle API. Introducing a separate store resolver now would add
indirection without changing behavior.

### Public façade/accessors

Keep current public surfaces.

Reviewed public accessors/options in `agent/agent.go`, `agent/options.go`,
`agent/action.go`, and `agent/compact.go`. None are clearly stale enough to delete
in this batch:

- `SessionID`, `SessionStorePath`, and `LiveThread` are still used by harness/CLI
  persistence and workflow run lookup paths.
- `Tracker`, `Out`, `ParamsSummary`, `Spec`, `MaterializedSystem`, and
  `ContextState` are still presentation/debug/introspection surfaces.
- `RunTurn`, action-backed turn helpers, and compaction methods remain active
  execution APIs.
- `WithOutput` remains documented as unstable rather than removed because verbose
  diagnostics and compaction still use it.

## Follow-up triggers

Extract from `agent.Instance` only when one of these concrete triggers happens:

1. Harness/session gains stable named-session open/list/resume/close APIs and can
   own thread store selection.
2. A non-terminal channel needs the same session lifecycle without going through
   `agent.New` directly.
3. Structured output/event publication replaces writer diagnostics and needs one
   event sink shared by agent, harness, and terminal.
4. App/plugin/session context provider lifecycle is clarified enough to make a
   context provider composer reusable outside `agent`.
5. Capability registry attachment becomes a session projection rather than an
   agent construction option.

Until then, `agent.Instance` should stay a façade over clearer internal helpers,
not be replaced abruptly.
