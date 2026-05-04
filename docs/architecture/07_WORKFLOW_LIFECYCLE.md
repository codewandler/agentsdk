# 11 — Workflow Lifecycle

This note closes section 8 of `docs/04_TASKLIST.md` and documents the current workflow lifecycle shape after the synchronous-only dogfood path.

## Lifecycle states and events

Workflow execution now materializes these durable run statuses:

- `queued` — recorded before asynchronous execution starts.
- `running` — recorded from `workflow.Started` when the executor begins.
- `succeeded` — recorded from `workflow.Completed`.
- `failed` — recorded from `workflow.Failed`.
- `canceled` — recorded from `workflow.Canceled`.

The same concrete event payloads are used for live callbacks, thread-backed persistence, and projected read models:

- `workflow.queued`
- `workflow.started`
- `workflow.step_started`
- `workflow.step_completed`
- `workflow.step_failed`
- `workflow.completed`
- `workflow.failed`
- `workflow.canceled`

`workflow.Projector` materializes `workflow.RunState` and `workflow.RunSummary` from those events. Thread-backed run storage remains the read-model source for harness/CLI run lookup because it preserves run history across `--sessions-dir`/`--continue` sessions without introducing a second persistence layer.

## Harness APIs and CLI commands

The harness keeps synchronous execution via:

```go
session.ExecuteWorkflow(ctx, "name", input)
```

and adds asynchronous start/cancel helpers:

```go
runID := session.StartWorkflow(ctx, "name", input)
session.CancelWorkflow(ctx, runID, "reason")
```

The command tree exposes the same lifecycle operations:

```text
/workflow start <name> [input...] [--async <async>]
/workflow runs [--workflow <workflow>] [--status <queued|running|succeeded|failed|canceled>] [--limit <limit>] [--offset <offset>]
/workflow run <run-id>
/workflow rerun <run-id> [--async <async>]
/workflow events <run-id>
/workflow cancel <run-id> [reason...]
```

`/workflow start --async` records a queued event, starts execution in a goroutine, and immediately returns the run ID. Cancellation uses the stored cancel function for active in-process async runs and appends a cancellation event when the projected run is not already terminal.

## Metadata persisted with runs

Run state and summaries now include:

- session ID;
- agent name;
- thread ID and branch ID;
- trigger/source string;
- invoking command path;
- workflow input as `workflow.ValueRef`;
- workflow definition hash and version;
- per-step action name and action version.

Inline values are represented with `workflow.InlineValue(...)`; external and redacted value references are available through `workflow.ExternalValue(...)` and `workflow.RedactedValue(...)` for later large-output/redaction work.

## Validation and identity

Workflow execution validates action inputs and outputs when action specs provide JSON-schema-backed `action.Type` contracts. The current implementation validates at the workflow boundary before and after each step and stops at the first validation failure.

Workflow definition identity is recorded as:

- `workflow.DefinitionHash(def)` — SHA-256 over a canonical workflow definition subset; and
- `Definition.Version` — caller-provided semantic/version string.

Action identity is recorded on step events using `action.Ref{Name, Version}`.

## Trade-offs

- Async execution is intentionally in-process. It is enough for CLI/TUI dogfood but not a durable background worker model.
- Cancellation is cooperative through `context.Context`; actions must observe context cancellation to stop promptly.
- Pagination is offset/limit over the projected summaries. That is simple and deterministic for thread-backed reads, but it is not cursor-stable if future storage supports concurrent appends from multiple workers.
- Thread-backed run storage remains sufficient for now because it avoids duplicating event persistence. A separate indexed store should wait until run lookup volume or cross-thread querying makes replay too expensive.
