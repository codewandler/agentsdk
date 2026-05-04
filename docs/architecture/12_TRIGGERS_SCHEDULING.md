# 16 — Triggers and scheduling

Section 12 adds the first trigger/scheduler layer on top of daemon/service mode. The core design is intentionally event-based:

```text
source emits event -> matcher accepts event -> executor starts workflow/session work
```

This keeps interval, file-watch, webhook, queue, and future host events on the same internal path. Public sugar can later wrap this model without creating source-specific mini-runtimes.

## Internal model

The `trigger` package owns generic scheduling primitives:

- `Event` — normalized event with ID, type, source, subject, timestamp, data, and causality/correlation fields.
- `Source` — producer interface for normalized events.
- `IntervalSource` — first source implementation, emitting `timer.interval` events.
- `Matcher` — predicate interface for deciding whether a rule accepts an event.
- Built-in matchers:
  - `MatchAll`
  - `EventType(...)`
  - `SourceIs(...)`
  - `SubjectGlob(...)`
  - `All{...}`
  - `Any{...}`
- `Rule` — source + matcher + target + session policy + job policy.
- `Target` — execution target descriptor.
- `Scheduler` — in-process registry/runner for active trigger jobs.
- `JobSummary` / `JobEvent` — observability and control-plane state.

The model is deliberately not a generic expression-language automation engine yet. Conditions are typed Go matchers for now. That keeps behavior auditable and avoids prematurely committing to CEL/Starlark/JS/shell-style expressions.

## Targets

Initial target support is intentionally narrow:

1. `workflow` — preferred target for scheduled/background work.
2. `agent_prompt` — supported for direct scheduled prompts into harness sessions.
3. `action` — represented in the type model, but direct action execution is rejected until explicit policy/context design exists.

Direct action targets are risky because background events can otherwise execute arbitrary behavior without workflow-level validation, observability, or safety boundaries. Prefer workflow targets first.

## Session modes

Rules carry a `SessionPolicy` with these modes:

- `shared`
- `trigger_owned`
- `ephemeral`
- `resume_or_create`

The first implementation resolves those modes through `harness.TriggerExecutor`, using `harness.Service` as the only runtime/session owner. Trigger-owned sessions default to names derived from the rule ID, such as:

```text
trigger-daily-summary
```

Daemon hosts pass their configured sessions directory into the executor, so trigger-owned and shared sessions can be thread-backed and workflow runs can be queried later.

## Overlap policy

The default policy is:

```text
skip_if_running
```

If a source emits a matching event while the previous execution for that rule is still active, the scheduler records a skipped fire instead of launching overlapping work. Queueing, parallel execution, retries, and backoff remain future extensions.

## Daemon integration

`daemon.Host` now owns an in-process trigger scheduler attached to its existing `harness.Service`:

```go
host, _ := daemon.New(daemon.Config{Service: service, SessionsDir: ".agentsdk/sessions"})
_ = host.AddTrigger(ctx, trigger.Rule{...})
jobs := host.Jobs()
_ = host.StopJob("daily-summary")
```

Daemon status includes active jobs and last-fire metadata. Shutdown stops all jobs before closing the harness service.

## Harness integration

`harness.TriggerExecutor` maps trigger executions to existing harness APIs:

- workflow target -> `Session.ExecuteWorkflow(...)`
- agent prompt target -> session agent turn
- action target -> explicit rejection until policy exists

Workflow executions caused by triggers persist metadata through existing workflow run storage:

```text
metadata.trigger = "trigger"
metadata.command_path = ["trigger", "<rule-id>"]
metadata.session_id / thread_id / agent_name populated from the target session
```

`harness.Service` can attach a trigger registry, and `/jobs` is available as an internal REPL command for listing and stopping active trigger jobs in a normal `agentsdk run` session when a scheduler is attached.

## CLI smoke path

`agentsdk serve` exposes a small smoke-testable interval path:

```bash
agentsdk serve . \
  --status \
  --trigger-interval 1h \
  --trigger-workflow daily_summary \
  --trigger-input "hello"
```

For prompt targets:

```bash
agentsdk serve . \
  --status \
  --trigger-interval 1h \
  --trigger-prompt "Summarize the current session"
```

`--status` starts the host, registers the trigger, waits for the immediate interval fire, prints service/job status, and exits. Long-running `agentsdk serve` keeps the trigger active until interrupt/shutdown.

## Future event sources

A future file watcher should only need to implement `trigger.Source` and emit normalized events such as:

```text
type: fs.changed
source_id: docs-watch
subject: docs/guide.md
data.path: docs/guide.md
data.op: write
```

Rules can then reuse the same matcher/executor path:

```text
EventType("fs.changed") && SourceIs("docs-watch") && SubjectGlob("docs/*.md")
  -> workflow.start("review_docs")
```

That is the main benefit of the event-first model: adding a new source should not require a new execution runtime.

## Known intentional limits

- No cron syntax yet.
- No webhook/file/queue sources yet.
- No persisted trigger registry DB yet; active jobs are in memory.
- No generic expression language yet.
- No direct action execution without explicit policy.
- No retry/backoff/queue/parallel overlap policies yet.
- No external HTTP/SSE control plane yet.
