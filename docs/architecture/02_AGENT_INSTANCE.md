# Agent package and Instance boundary

This note started as the section-3 `agent.Instance` audit from `docs/04_TASKLIST.md`.
It now also records the current architecture problem after the harness/session,
workflow, trigger, channel, persistence, and compaction boundaries became more concrete.

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

## Current architecture problem after the docs split

The earlier audit was intentionally conservative because the harness/session boundary was not yet proven. That has changed: `harness.Service`, `harness.Session`, session subscriptions, workflow lifecycle, daemon/service mode, HTTP/SSE channel hosting, trigger scheduling, thread inspection, and compaction visibility now exist.

That means the remaining `agent.Instance` breadth is no longer just a temporary convenience. It is now the main architecture smell:

- `agent` still knows too much about session/thread opening and JSONL-backed persistence.
- `agent` still translates app/resource/plugin state into runtime construction directly.
- `agent` owns skill activation state, capability setup, context provider setup, usage persistence, compaction, and event/output plumbing in one type.
- `agent.Instance` is used both as a configured agent façade and as a live session/runtime holder.
- Channels and daemon hosts increasingly want harness/session events, not direct writer/output hooks from `agent`.

Because this is pre-1.0 and we are the only consumer, do not add compatibility wrappers to hide this. Prefer moving responsibilities to clearer homes and deleting stale accessors/options once the current dogfood paths compile and tests pass.

## Cleanup direction

The desired end state is not a second agent runtime. It is a smaller `agent` package that owns agent specification and per-agent runtime construction inputs, while live execution belongs to harness/session/runtime boundaries.

Concrete ownership targets:

| Responsibility currently near `agent.Instance` | Preferred owner |
| --- | --- |
| Agent spec normalization and model policy defaults | `agent` |
| Low-level model/tool turn loop | `runtime` / `runner` |
| Session open/resume/list/close and store selection | `harness.Service` / `harness.Session` |
| Thread event persistence and replay | `thread`, with harness inspection APIs |
| App/resource/plugin composition | `app`, `resource`, `agentdir`, named `plugins` |
| Command/workflow/trigger dispatch | `harness`, `command`, `workflow`, `trigger` |
| Skill activation lifecycle for live sessions | harness/session-owned state projected into agent/runtime context |
| Capability/context provider lifecycle | session/runtime composition helpers, not terminal or app code |
| Usage, compaction, diagnostics, output events | structured session/channel events |

## Next cleanup slices

Keep the cleanup incremental, but bias toward deletion over compatibility:

1. **Session/thread ownership:** move JSONL store selection and open/resume helpers behind harness/session APIs, then remove direct store path knowledge from `agent` where possible.
2. **Runtime construction seam:** split option/spec normalization from live runtime creation so harness can construct runtime state without treating `agent.Instance` as the session owner.
3. **Skill/capability/context lifecycle:** make session-owned activation and projection state explicit; keep app/plugin registration as metadata/definitions, not live mutable state.
4. **Output/event path:** route compaction, usage, diagnostics, command/workflow notices, and safety events through structured session events; remove writer-centric APIs once no dogfood path needs them.
5. **Accessor deletion pass:** after the above, delete `agent.Instance` accessors/options that only exist because terminal/harness used to reach through the façade.

## Non-goals

- Do not keep old paths for hypothetical external users.
- Do not create `agentv2` or a parallel runtime package.
- Do not move code just to make files smaller.
- Do not start datasource expansion until this core ownership cleanup is clearer.
