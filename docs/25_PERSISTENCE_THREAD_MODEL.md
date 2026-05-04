# Persistence / thread model

Section 21 keeps `thread` as the durable event foundation and tightens the
inspection/replay contracts around it.

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
