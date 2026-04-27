# Architecture Review: Event-Sourced Thread Context Plan vs. Implementation

**Plan:** `.agents/plans/PLAN-event-sourced-thread-context-architecture.md`
**Reviewed:** 2026-04-27T09:42:00+02:00
**Scope:** Full plan-vs-implementation gap analysis and induced architecture review

**Resolution:** Subsequent commits completed the noted high-priority closeout:
provider stream diagnostics, typed outer thread event registry, projection
recovery cleanup, and the provider-controlled git context provider. The plan is
now marked implemented with only future hardening deferred.

---

## Executive Assessment

The implementation has landed the plan's core structural decisions with
remarkable fidelity. The three-layer model (event log → materialized views →
thread store), the context provider/manager architecture, the capability
system, and the projection pipeline are all present and working. The plan's
migration phases 1–6 are substantially complete. Phase 7 (API cleanup) is
partially done — the dual `Messages`/`PendingMessages` projection path was
just removed.

The architecture is sound. The remaining gaps are mostly in hardening,
observability, and features the plan explicitly deferred. There are a few
structural divergences worth tracking.

---

## Layer-by-Layer Conformance

### 1. Event Log / Thread Store

| Plan Element | Status | Notes |
|---|---|---|
| `thread.Event` envelope with ID, ThreadID, BranchID, NodeID, Seq, Kind, Payload, At, Source, CausationID, CorrelationID | ✅ Implemented | Exact match in `thread/event.go` |
| `thread.Store` with Create/Resume/Read/List/Archive/Unarchive | ✅ Implemented | `thread/store.go` — also has `Fork` which the plan didn't mention but is a natural extension |
| `thread.Live` with Append/Flush/Shutdown/Discard | ✅ Implemented | Exact match |
| `thread/jsonlstore` with append-only writes, file sync, discard cleanup | ✅ Implemented | `thread/jsonlstore/store.go` |
| `thread.MemoryStore` for testing | ✅ Implemented | `thread/memorystore.go` |
| Event kind namespaces (`thread.*`, `branch.*`, `conversation.*`, `harness.*`, `capability.*`, `provider.*`) | ✅ Implemented | Spread across `thread/event.go`, `capability/events.go`, `runtime/thread_runtime.go`, `runtime/history_events.go`, `runner/runner.go` |
| `EventSource` with Type/ID/SessionID | ✅ Implemented | Exact match |
| Branch-aware event windowing for replay | ✅ Implemented | `Stored.EventsForBranch` with `branchWindows` |
| Deferred file creation until first persisted item | ⚠️ Partial | JSONL store creates on `Create`, not on first append |
| Retry of buffered writes after transient I/O errors | ❌ Not implemented | Plan listed this as a `LiveThread` feature |
| SQLite metadata index | ❌ Deferred | Plan item #2 in remaining work |

### 2. Conversation Tree / Internal Items / Projection

| Plan Element | Status | Notes |
|---|---|---|
| `conversation.Tree` with branch-aware append, fork, path | ✅ Implemented | `conversation/tree.go` |
| Internal item types (UserMessage, AssistantMessage, ToolCall, ToolResult, ContextFragment, Compaction, Annotation) | ✅ Implemented | `conversation/item.go` — uses `ItemKind` enum |
| `NormalizeItems` with orphan/duplicate/missing tool call handling | ✅ Implemented | Just enhanced in latest commit |
| `MessagesFromItems` as late projection | ✅ Implemented | |
| `ProjectionPolicy` interface with `defaultProject` | ✅ Implemented | `conversation/projection_policy.go` |
| Dual `Messages`/`Items` path removed | ✅ Just landed | Latest commit removes `ProjectionInput.Messages`/`PendingMessages` |
| Compaction as event + projection, not deletion | ✅ Implemented | `CompactionEvent` with `Replaces` node IDs |
| Provider continuation from route metadata, not heuristics | ✅ Implemented | `ProviderIdentity` from `RouteEvent` |
| Codex session hints with thread/branch extensions | ✅ Implemented | `AddCodexSessionHints` in `conversation/codex_extensions.go` |
| `GhostSnapshotItem` | ❌ Not implemented | Plan mentioned it; no evidence in code |

### 3. Context Architecture

| Plan Element | Status | Notes |
|---|---|---|
| `agentcontext.Provider` with `Key()` + `GetContext()` | ✅ Implemented | Exact match |
| `agentcontext.FingerprintingProvider` fast path | ✅ Implemented | |
| `agentcontext.Manager` with register, build, prepare/commit | ✅ Implemented | Full optimistic concurrency with version check |
| `ContextFragment` with Key, Role, StartMarker, EndMarker, Content, Fingerprint, Authority, CachePolicy | ✅ Implemented | Exact match |
| `ProviderRenderRecord` with per-fragment fingerprints | ✅ Implemented | |
| `RenderDiff` with Added/Updated/Removed/Unchanged | ✅ Implemented | |
| `FragmentAuthority` (developer/user/tool) | ✅ Implemented | |
| `CachePolicy` with Stable, MaxAge, Scope | ✅ Implemented | |
| `Preference` (changes/full) | ✅ Implemented | |
| `RenderReason` enum | ✅ Implemented | Has initial, turn, tool_followup, resume, compaction, branch_switch, forced_full_refresh |
| Manager-owned mechanical diff (not provider-returned diffs) | ✅ Implemented | `buildProviderRecord` in manager.go |
| Removal tombstones for missing keys | ✅ Implemented | |
| Hybrid context snapshot (Option C) | ✅ Implemented | `contextSnapshotRecorded` with `providerSnapshotRef` per provider |
| Context render committed event for resume | ✅ Implemented | `EventContextRenderCommitted` |
| Context replay from thread events | ✅ Implemented | `ReplayContextRenders` |

**Built-in providers:**

| Provider | Plan | Status |
|---|---|---|
| Environment | ✅ | `contextproviders.Environment` — cwd, os, arch, kernel, hostname |
| Time | ✅ | `contextproviders.Time` — bucketed time with configurable interval |
| Model | ✅ | `contextproviders.Model` — static model info |
| Permissions | ✅ | `contextproviders.Permissions` — static permissions content |
| Project Instructions | ✅ | `contextproviders.ProjectInstructions` — multi-file AGENTS.md |
| Skills | ✅ | `contextproviders.Skills` — loaded skill bodies |
| Tools | ✅ | `contextproviders.Tools` — active tool catalog |
| Git state | ❌ | Plan described `GitContextPolicy` with configurable modes; not implemented |

### 4. Capability System

| Plan Element | Status | Notes |
|---|---|---|
| `capability.Capability` interface (Name, InstanceID, Tools, ContextProvider) | ✅ Implemented | |
| `capability.StatefulCapability[T]` with State + ApplyEvent | ✅ Implemented | |
| `capability.Factory` + `capability.Registry` | ✅ Implemented | |
| `capability.Manager` with Attach/Replay/Tools/ContextProvider | ✅ Implemented | |
| `capability.Runtime` with ThreadID/BranchID/Source/AppendEvents | ✅ Implemented | |
| `capability.LiveRuntime` concrete implementation | ✅ Implemented | |
| `capability.attached` / `capability.detached` / `capability.state_event_dispatched` events | ✅ Implemented | |
| `StateEventDefinition` with typed body validation | ✅ Implemented | `DefineStateEvent[T]` generic helper |
| `StateEventDefinitions` interface on factories | ✅ Implemented | |
| `stateEventRegistry` for replay validation | ✅ Implemented | |
| Manager as aggregate context provider | ✅ Implemented | `managerProvider` with instance-prefixed fragment keys |

### 5. Planner Capability (First Slice)

| Plan Element | Status | Notes |
|---|---|---|
| `Plan` / `Step` / `StepStatus` types | ✅ Implemented | |
| Inner events: plan_created, step_added, step_removed, step_title_changed, step_status_changed, step_reordered, current_step_changed | ✅ Implemented | All 7 events in `planner/events.go` |
| Batch action tool | ✅ Implemented | `planner/tools.go` |
| All-or-nothing action flow (validate → build events → append → apply) | ✅ Implemented | `planner/actions.go` |
| Granular context fragments (planner/meta + planner/step/<id>) | ✅ Implemented | `planner/context.go` |
| gonanoid IDs with caller-provided ID support | ✅ Implemented | `planner/ids.go` |
| Explicit plan creation required | ✅ Implemented | `requireCreated` guard |
| Step removal as hard remove from materialized state | ✅ Implemented | |
| Factory with state event definitions | ✅ Implemented | `planner/factory.go` |

### 6. Runner Integration

| Plan Element | Status | Notes |
|---|---|---|
| Runner appends typed events during model/tool loops | ✅ Implemented | Via `History.CommitFragment` → thread events |
| Tool-induced harness events visible before follow-up | ✅ Implemented | `PrepareRequest` runs context render before each step |
| Synthetic aborted tool results for cancellation/timeout | ✅ Implemented | `toolResultFromContext` in runner |
| Recovery transcript commit on abort | ✅ Just landed | `commitRecoveredTranscript` |
| Provider metadata committed with successful turns | ✅ Implemented | `commitFinalFragment` with thread events |
| Failed turns don't advance continuation | ✅ Implemented | Fragment is `Fail()`'d, no commit |

### 7. Prompt Assembly Flow

| Plan Element | Status | Notes |
|---|---|---|
| Context render at every prompt boundary | ✅ Implemented | `PrepareRequest` in `ThreadRuntime` |
| Initial thread: full context preference | ⚠️ Implicit | No explicit `PreferFull` on first turn; relies on empty previous records |
| Normal turn: changes preference | ⚠️ Implicit | Default preference is zero value, not explicitly `PreferChanges` |
| Context injection as Instructions (developer) or Items (user) | ✅ Implemented | `contextInjectionForRender` |
| Native continuation: only changed fragments | ✅ Implemented | Separate path in `contextInjectionForRender` |
| Full replay: all active fragments | ✅ Implemented | |

---

## Structural Gaps

### G1 — No `harness.*` state-change events beyond context snapshots

The plan describes `harness.model_changed`, `harness.environment_changed`,
`harness.skill_loaded`, `harness.skill_unloaded`, `harness.tool_activated`,
`harness.tool_deactivated` as durable events. The implementation only persists
`harness.context_snapshot_recorded` and `harness.context_render_committed`.
Individual harness state changes are not recorded as separate events.

**Impact:** Harness state changes are captured indirectly through context
fragment diffs, but there's no event-level audit trail of *what* changed (e.g.,
"model changed from X to Y"). This makes it harder to build materialized views
like `HarnessStateProjection` from the event log alone.

**Recommendation:** Low priority. The context snapshot captures the effect. Add
individual harness events when a consumer needs them (e.g., thread metadata
auto-titling from model changes).

### G2 — No `HarnessState` type passed through context requests

The plan describes `ContextRequest.HarnessState` as a typed struct carrying
model, permissions, skills, tools, environment, etc. The implementation uses
`HarnessState any` in `agentcontext.Request` — it's an untyped `any` field.

**Impact:** Context providers can't reliably inspect harness state without type
assertions. Currently, built-in providers are self-contained (they read their
own state), so this doesn't block anything. But plugin providers that need
cross-cutting harness state would need a typed contract.

**Recommendation:** Define a `HarnessState` struct when the first provider
needs it. The `any` placeholder is fine for now.

### G3 — No `GhostSnapshotItem` type

The plan lists `GhostSnapshotItem` as an internal item type. It doesn't exist
in the implementation. The purpose isn't fully specified in the plan either.

**Recommendation:** Clarify intent or remove from plan. Likely intended for
representing compacted/replaced content as a ghost in the tree.

### G4 — Git context provider not implemented

The plan describes a `GitContextPolicy` with configurable modes (off, minimal,
changed_files, summary) and `MaxFiles`/`MaxBytes` caps. No git context
provider exists in `contextproviders/`.

**Impact:** Coding agents don't get automatic git state in context. The
`tools/git` package exists for tool use, but there's no passive context
injection.

**Recommendation:** This is a high-value provider for coding agents. Implement
`contextproviders.Git` with the plan's Option C (provider-controlled,
configurable).

### G5 — Plugin fragment authority policy not implemented

The plan decides on Option B: plugins default to `user` authority, with
explicit `developer` grants via `FragmentAuthorityPolicy`. No such policy
exists.

**Impact:** Currently, any provider can set any authority. There's no
enforcement.

**Recommendation:** Low priority until plugins are a real concern. The
`AuthorityDeveloper` constant exists; enforcement can be added in the manager's
`buildProviderRecord`.

### G6 — No `capability.detached` flow exercised

`capability.Detached` event type exists, `DetachEvent` helper exists, and
`Manager.Replay` handles it. But there's no `Manager.Detach` method to detach
a live capability. Detachment only works through replay.

**Impact:** Runtime can't dynamically remove a capability from a running
thread.

**Recommendation:** Add `Manager.Detach(ctx, instanceID)` when needed. The
event infrastructure is ready.

### G7 — JSONL store deferred file creation

The plan says `LiveThread` should support "deferred file creation until first
persisted item." The JSONL store creates the file on `Create`, not on first
`Append`.

**Impact:** Empty thread files accumulate if threads are created but never
written to (e.g., discarded sessions).

**Recommendation:** Minor. The `Discard` method exists for cleanup. Deferred
creation is an optimization.

### G8 — No retry for buffered writes

The plan mentions "retry of buffered writes after transient I/O errors" for
`LiveThread`. Not implemented.

**Impact:** A transient disk error during append loses events silently.

**Recommendation:** Add retry with backoff in `liveThread.Append` when I/O
reliability becomes a concern.

---

## Induced Architecture Assessment

### What works well

1. **Clean layer separation.** `thread` owns lifecycle, `conversation` owns
   tree/items/projection, `agentcontext` owns context rendering, `capability`
   owns stateful extensions, `runtime` wires them together. Each package has a
   clear single responsibility.

2. **Event sourcing is real, not aspirational.** Thread events are the durable
   source of truth. The conversation tree, capability state, and context render
   records are all rebuilt from events on resume. This is the plan's central
   principle and it's working.

3. **Context manager design is excellent.** The pull-based provider model with
   manager-owned mechanical diffs, fingerprint fast paths, optimistic
   concurrency on commit, and the prepare/commit/rollback lifecycle is
   well-engineered. It handles the hard cases (native continuation vs. full
   replay, compaction re-render, removal tombstones) without leaking complexity
   to providers.

4. **Capability system is minimal but complete.** The generic
   `StatefulCapability[T]`, typed event validation, and the planner as first
   concrete implementation prove the design works end-to-end.

5. **Projection pipeline is sound.** Items → NormalizeItems → MessagesFromItems
   → ProjectionPolicy → provider request. Each step is testable and the
   normalization handles real edge cases (orphans, duplicates, missing results,
   cancellation recovery).

### Architectural concerns

#### A1 — `runtime` package is becoming a gravity well

`runtime/thread_runtime.go` is 755 lines and growing. It owns:
- Thread runtime lifecycle (create, resume, replay)
- Capability wiring
- Context manager wiring
- Context render → thread event serialization
- Context injection into requests (instructions vs. items)
- Request preparer chaining
- Tool merging
- Compaction orchestration

This is the package where every new feature lands because it's the integration
point. The plan's "materialized views" concept suggests these projections
should be separable, but in practice they're all in `thread_runtime.go`.

**Recommendation:** Consider extracting context render event serialization
(`contextRenderEvents`, `applyContextFragmentRecorded`, etc.) into a dedicated
`runtime/contextevents.go` or even a `runtime/contextbridge` internal package.
The context injection logic (`contextInjectionForRender`) could also be
separated from the thread runtime.

#### A2 — Two parallel history paths (thread-backed vs. standalone)

`runtime.History` works both with and without a `thread.Live`. When `live` is
nil, it's a standalone in-memory history. When `live` is set, it persists
conversation payloads as thread events. This dual mode means every commit path
has to check `if h.live != nil`.

The plan doesn't describe this dual mode — it assumes thread-backed operation.
The standalone mode exists for backward compatibility and simpler use cases.

**Impact:** Not a bug, but it means the "event sourcing is the foundation"
principle has an escape hatch. Standalone histories don't persist, don't
replay, and don't participate in the thread lifecycle.

**Recommendation:** This is fine as long as the standalone path is understood
as a convenience for testing and simple scripts. Document it explicitly as
"ephemeral mode" vs. "durable mode."

#### A3 — Context render events are serialized in `runtime`, not `agentcontext`

The `contextFragmentRecorded`, `contextFragmentRemovedRecorded`, and
`contextSnapshotRecorded` types are defined in `runtime/thread_runtime.go`, not
in `agentcontext`. This means `agentcontext` doesn't know how its render
results become durable events.

**Impact:** If another consumer of `agentcontext` (e.g., a remote runtime)
wants to persist context renders, it has to duplicate the serialization logic.

**Recommendation:** Consider moving the event payload types to `agentcontext`
or a shared `agentcontext/events` package. The `runtime` package would still
own the `thread.Event` envelope construction, but the payload schemas would be
portable.

#### A4 — No shared event kind registry across packages

Event kinds are scattered:
- `thread/event.go`: `thread.created`, `branch.created`, etc.
- `capability/events.go`: `capability.attached`, etc.
- `runtime/thread_runtime.go`: `conversation.context_fragment`, `harness.context_snapshot_recorded`, etc.
- `runtime/history_events.go`: `conversation.user_message`, `conversation.assistant_message`, etc.
- `runner/runner.go`: `provider.route_selected`, `provider.execution_metadata_recorded`

The plan's remaining item #3 asks: "Decide whether all durable non-capability
event kinds need a shared typed registry."

**Impact:** There's no single place to see all event kinds, validate payloads,
or ensure kind uniqueness. A new contributor adding an event kind could
accidentally collide with an existing one.

**Recommendation:** This is the plan's open question. For now, the scattered
approach works because the event set is small and owned by the SDK. When
plugins start emitting events, a registry becomes necessary. Consider at
minimum a `thread/eventkind` package that collects all known kinds as
constants, even if validation stays distributed.

#### A5 — Compaction projection is branch-local but tree is shared

The `Tree` is a shared mutable structure. Compaction events are appended to a
branch, and `ProjectItems` handles them during projection. But the tree itself
doesn't know about compaction — it just stores nodes. The compaction logic
lives entirely in `conversation/item.go`'s `ProjectItems` → `NormalizeItems`
pipeline.

This is correct per the plan ("compaction is an event and projection choice,
not deletion"), but it means:
- Every projection must handle compaction correctly
- The tree grows monotonically (original nodes are never removed)
- Long-running threads will accumulate nodes

**Recommendation:** This is fine for the current scale. The plan acknowledges
this and defers indexing/repair. When tree size becomes a concern, consider a
`Tree.Prune` method that removes nodes below a compaction boundary while
keeping the compaction event as the new root.

---

## Plan Accuracy

### Items the plan should update

1. **Remove `GhostSnapshotItem`** from the internal item types list, or define
   its purpose.

2. **Mark Phase 7 as substantially complete.** The dual `Messages`/
   `PendingMessages` path is removed. The remaining Phase 7 work is the `flai`
   → `agentsdk` rename (tracked in `review-improvements.md`).

3. **Update remaining work list.** Item #1 (stricter recovery for malformed
   streams) is partially addressed by the recovery transcript commit. The
   remaining gap is streams that fail *before* a complete assistant message.

4. **Add `Fork` to the `ThreadStore` sketch.** The implementation has it; the
   plan doesn't mention it.

5. **Note the standalone History mode** as an intentional escape hatch for
   non-durable use cases.

### Items the plan got right that should stay

1. **Option C for context snapshots** (hybrid with provider hashes) — implemented
   exactly.
2. **Option C for git context** (provider-controlled) — not yet implemented but
   the design is ready.
3. **Option B for plugin authority** (default user, explicit developer grants) —
   infrastructure exists, enforcement deferred.
4. **Option C for capability context granularity** (granular full-state
   fragments) — planner implements this exactly.
5. **Option B for event schema strictness** (typed registration) — implemented
   with `DefineStateEvent[T]`.

---

## Priority Recommendations

| Priority | Item | Effort |
|---|---|---|
| **High** | G4: Implement git context provider | Medium — the `tools/git` package has the primitives |
| **Medium** | A1: Extract context event serialization from `thread_runtime.go` | Small — mechanical refactor |
| **Medium** | A3: Move context event payload types to `agentcontext` | Small — type moves + imports |
| **Medium** | A4: Collect all event kind constants in one reference file | Small — constants only |
| **Low** | G1: Add individual harness state-change events | Medium — needs consumers |
| **Low** | G2: Define typed `HarnessState` struct | Small — when first plugin needs it |
| **Low** | G5: Add `FragmentAuthorityPolicy` enforcement | Small — when plugins arrive |
| **Low** | G6: Add `Manager.Detach` for live capability removal | Small — event infra ready |
| **Low** | G7: Deferred JSONL file creation | Small — optimization |
| **Low** | G8: Retry for buffered writes | Medium — needs error classification |

---

## Overall Verdict

The implementation is a faithful, well-executed realization of the plan. The
core event-sourced architecture is working end-to-end: events are durable,
state is rebuilt from replay, context is diffed mechanically, capabilities are
stateful and replayable, and projection is normalized. The gaps are in
hardening and features the plan explicitly deferred.

The main architectural risk is `runtime` package growth (A1). Everything else
is either low-priority or waiting for a concrete consumer to justify the work.

**Plan status: active implementation, phases 1–6 complete, phase 7 in
progress.** The plan's remaining work items are accurate and correctly
prioritized.
