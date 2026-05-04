# Workflows, triggers, and daemon mode

## Workflow lifecycle

This section documents the current workflow lifecycle shape.

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

## Execution primitive boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps this rule explicit: workflows orchestrate `action` refs and do not import agent, app, harness, terminal, daemon, channel, or tool packages. Harness remains the session-bound workflow execution and lookup owner.

## Workflow execution semantics

This section documents workflow execution semantics.

## Default execution model

The default workflow executor remains a deterministic, sequential, topologically ordered pipeline. `Executor.Execute(...)` still uses `MaxConcurrency <= 1` unless callers opt into parallelism with:

```go
workflow.WithMaxConcurrency(n)
```

This keeps existing dogfood and CLI behavior stable while making independent DAG execution available for hosts that are ready for concurrent action calls.

## DAG, fan-out, and fan-in

Workflow definitions continue to use `Step.DependsOn` as the dependency graph. The executor validates:

- required step IDs;
- required action names;
- duplicate step IDs;
- unknown dependencies;
- dependency cycles.

For sequential execution, steps run in deterministic topological order. For parallel execution, each scheduling round finds ready steps whose dependencies are complete and runs up to `MaxConcurrency` of them. Fan-out is represented by multiple ready independent steps; fan-in is represented by a downstream step depending on multiple upstream step IDs.

The default dependency input rules remain:

- no dependencies: initial workflow input;
- one dependency: that dependency's output;
- multiple dependencies: `map[string]any` keyed by dependency step ID.

## Dataflow and input mapping

Steps can now override default dependency input behavior with structured mapping:

```go
workflow.Step{
    ID:        "render",
    Action:    workflow.ActionRef{Name: "render"},
    DependsOn: []string{"fetch", "classify"},
    InputMap: map[string]string{
        "doc":      "steps.fetch.output",
        "category": "steps.classify.output.name",
        "original": "input",
    },
}
```

`InputMap` builds an object from expressions. `InputTemplate` and `Input` also support `{{ ... }}` substitutions for string fields or whole-string values:

- `input` — initial workflow input;
- `steps.<id>.output` / `steps.<id>.data` — prior step output;
- `steps.<id>.output.<field>` — map or exported struct field lookup;
- `steps.<id>.error` — prior step error text.

This is intentionally small and boring. It is not a full templating language; add one only if real workflow definitions need conditionals/loops inside mapping expressions.

## Retry, timeout, and error policy

Each step can declare:

```go
Retry: workflow.RetryPolicy{MaxAttempts: 3, Backoff: 100 * time.Millisecond}
Timeout: 30 * time.Second
ErrorPolicy: workflow.StepErrorContinue
```

`MaxAttempts` includes the first attempt. `Timeout` wraps the action context for that attempt. The default error policy is fail-fast. `StepErrorContinue` records the failed result and allows downstream steps whose dependencies are complete to continue.

Attempts are projected into `StepState.Attempts`, so run detail can show failed attempts followed by a later successful attempt.

## Conditional execution

`Step.When` supports a small condition model:

```go
When: workflow.Condition{StepID: "classify", Equals: "ready"}
```

Conditions can test whether a prior step exists, whether it has data, whether its output equals a value, and can be negated. Skipped steps emit `workflow.step_skipped` and project to `StepSkippedStatus`.

## Idempotency and value references

`Step.IdempotencyKey` is carried on step start/completion/failure/skipped events and projected into `StepState`. The executor does not deduplicate by key yet; the field is the durable contract future stores/runners can use when replaying or resuming work.

Action outputs that are already `workflow.ValueRef` are now preserved rather than wrapped as inline values. This keeps external and redacted references intact:

```go
return action.Result{Data: workflow.ExternalValue("s3://bucket/output.json", "application/json")}
```

## Resumability and deterministic replay constraints

The code now records enough step-level facts for later resumability: run ID, definition identity, action identity/version, idempotency key, attempts, value references, and terminal/skipped status. A full durable resume runner is intentionally not added in this slice because the current thread-backed run store is an append-only event source, not a distributed work queue.

Replay constraints are explicit:

- sequential execution with `MaxConcurrency <= 1` has deterministic scheduling for a fixed definition and deterministic actions;
- parallel execution may interleave live events differently across runs, but projected run state remains keyed by run/step IDs;
- deterministic replay of external side effects requires action-level idempotency keyed by `Step.IdempotencyKey`;
- retries and timeouts are observable in events but are not replayed from historical timing data.

## Coverage

The workflow test suite now covers:

- stable sequential pipeline semantics;
- multiple-dependency fan-in input behavior;
- parallel independent steps with fan-in;
- retry attempts;
- step timeout;
- continue-on-error policy;
- conditional skipped steps;
- structured input mapping;
- typed action input/output validation;
- external/redacted value reference projection;
- idempotency key projection.

## Execution primitive boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps workflow in the execution layer: it owns action orchestration, DAG scheduling, retry/timeout/error policy, input mapping, validation, and event projection without owning app/resource loading or channel presentation.

## Daemon trigger scheduling design

This checkpoint captures the open questions behind daemon mode and scheduled/background agent work before implementation starts.

## Decision already made

- **Harness** is the product concept. **Daemon** is one deployment mode of the harness.
- A **trigger** is not a channel. It is a time/event source that asks harness/session APIs to start or resume work.
- Datasource implementation work is deferred until daemon/service mode and trigger scheduling prove the long-running host shape.

## Decisions from checkpoint

### 1. CLI command shape

Question: should the long-running host be exposed as `agentsdk serve`, `agentsdk daemon`, both, or only as a Go API initially?

Decision: use `agentsdk serve` for the user-facing command. Keep “daemon” as documentation language for deployment mode. `serve` is less Unix-specific and maps to future HTTP/SSE/control-plane behavior.

### 2. Process lifecycle ownership

Question: does daemon mode reuse `harness.Service` directly, or should there be a thin `daemon` package above harness?

Decision: add a slim daemon package wrapper above `harness.Service` for long-running process concerns: config loading, lifecycle orchestration, trigger/job ownership, and service/CLI integration. Keep `harness.Service` as the runtime/session owner. The daemon wrapper must not become a second app/runtime/plugin system.

### 3. Session target for scheduled prompts

Question: when a trigger says “run this prompt every X,” which session receives it?

Options:

- fixed configured session ID;
- default app/session per trigger;
- create a new session per fire;
- resume last successful session if present, create otherwise.

Decision: support configurable session targeting because real use cases differ:

- `shared`: reuse one configured session over time, allowing normal compaction/memory behavior;
- `trigger_owned`: use a named session derived from trigger ID;
- `ephemeral`: create a fresh session per fire/event;
- `resume_or_create`: resume a configured session if present, otherwise create it.

The config model must make this explicit. Do not silently attach background work to an interactive user session unless the user starts it from that session, such as through a REPL `/triggers` or `/jobs` command.

### 4. Trigger target type

Question: can a trigger target an agent prompt, a workflow, an action, or all three?

Decision: support all target kinds conceptually, with workflow as the preferred normalization layer:

- `agent_prompt`: send a prompt into a session/agent;
- `workflow`: start a named workflow with input;
- `action`: execute an action directly only when policy and context are explicit.

Implementation should prioritize workflow targets because workflows can already wrap prompt turns, actions, and structured orchestration. Direct action targets can come later or remain an advanced API surface.

### 5. Trigger resource/config location

Question: where should trigger definitions live?

Options:

- app manifest only;
- `.agents/triggers/*.yaml`;
- host/daemon config only;
- Go/plugin-only initially.

Decision: support Go API plus host/daemon config first. Config must be expressive enough to manage trigger targets, session mode, interval, overlap policy, and prompt/workflow/action input without code. Defer `.agents/triggers/*.yaml` discovery until the runtime/config shape is proven.

### 6. Persistence model

Question: should trigger fires be persisted as trigger events, workflow metadata, thread events, or all of those?

Decision: persist the minimum useful metadata in the target thread/run:

- trigger ID;
- trigger type;
- scheduled/fire timestamp;
- target kind/name;
- source/config reference;
- fire/run correlation ID.

Do not create a separate trigger database until thread-backed projection is insufficient.

### 7. Backoff and overlap policy

Question: what happens if interval fires while the previous fire is still running or repeatedly fails?

Decision: only one run is allowed by default, with no overlap-policy config in the first implementation. If the prior fire is still running, the interval fire is skipped and recorded/logged. Add queue/parallel/backoff configuration only after the simple behavior is proven and observable.

### 8. Safety and approval

Question: can scheduled work execute high-risk actions without a user present?

Decision: scheduled/background triggers need a stricter policy than interactive CLI. Default background policy should deny approval-required operations unless a trusted policy/config is explicitly installed.

### 9. Observability and control

Question: how does a user see and stop running triggers?

Decision: logs/running terminal output are enough for the first proof, but define a small daemon API so operators can see what is going on:

- list configured triggers/jobs;
- list active trigger loops/jobs;
- disable/stop a trigger/job;
- inspect last fire/error;
- subscribe to trigger/job events.

Expose the same concepts in normal `agentsdk run` REPL sessions through slash commands such as `/triggers` or `/jobs`, so background/repeating work can run inside the current harness/session. Daemon mode is then effectively the same host without the main interactive agent I/O.

### 10. HTTP/SSE dependency

Question: does daemon mode require HTTP/SSE first?

Decision: no. Build daemon/service lifecycle and interval trigger with in-process APIs, logs, REPL commands, and CLI smoke tests first. HTTP/SSE is a later channel/control-plane projection over the same harness/daemon service.

## Proposed first implementation slice

1. Add a slim daemon package wrapper around `harness.Service`.
2. Add service-like harness/daemon lifecycle tests without HTTP/SSE.
3. Add minimal trigger/job interfaces and interval trigger support in Go.
4. Route interval trigger to a configured session target, starting with shared/trigger-owned/ephemeral session modes.
5. Prefer workflow targets for scheduled work; support prompt targets for simple repeated prompts.
6. Expose trigger/job inspection and control through the daemon API and REPL slash commands.
7. Persist trigger source metadata into session/workflow events where available.
8. Add one CLI smoke path after the Go API behavior is stable.

## What is explicitly deferred

- Datasource resource semantics.
- Webhook/file/queue triggers.
- Distributed scheduling.
- Hosted control plane.
- Direct arbitrary action targets as a first-class config target; prefer workflow targets first.
- Trigger resource discovery from `.agents/triggers/*.yaml` until the runtime shape proves itself.

## Daemon service mode

Section 11 establishes daemon mode as a deployment shape for the existing harness runtime, not a second SDK runtime.

## Decision summary

- The CLI command shape is `agentsdk serve [path]`.
- Daemon mode is a harness deployment mode: `harness.Service` remains the runtime/session owner.
- The new `daemon` package is intentionally slim. It wraps `harness.Service` for process-level conventions: storage paths, status snapshots, and graceful shutdown.
- Resource, app, plugin, agent, command, and workflow loading continue to use the same `cli.Load(...)` / `harness.LoadSession(...)` path used by `agentsdk run`.
- The first smoke-testable CLI path is `agentsdk serve [path] --status`, which opens the service stack, prints status, and exits without starting an interactive REPL.

## Ownership model

```text
agentsdk serve
  terminal/cmd CLI glue
    daemon.Host
      harness.Service
        harness.Session
          app.App
          agent.Instance
          command.Tree
          workflow.Registry/RunStore
```

`daemon.Host` must not grow its own app, runtime, plugin, command, or workflow system. If behavior belongs to sessions or runtime execution, it belongs in `harness.Service` / `harness.Session` or below.

## Public APIs

Harness service APIs for long-running hosts:

```go
service := harness.NewService(application)
session, err := service.OpenSession(ctx, harness.SessionOpenRequest{
    Name:      "daily",
    AgentName: "coder",
    StoreDir:  ".agentsdk/sessions",
})
status := service.Status()
_ = service.Close()
```

Daemon wrapper APIs:

```go
host, err := daemon.New(daemon.Config{
    Service:     service,
    SessionsDir: ".agentsdk/sessions",
    ConfigPath:  "agentsdk.app.json",
})
status := host.Status()
_ = host.Shutdown(ctx)
```

`daemon.Host.OpenSession(...)` and `daemon.Host.ResumeSession(...)` default the request `StoreDir` to the daemon sessions directory when the caller does not provide one.

## CLI conventions

Status smoke path:

```bash
agentsdk serve . --status
```

Expected output includes:

```text
agentsdk service
mode: harness.service
health: ok
sessions: .agentsdk/sessions
active_sessions: 1
- <session-name> id=<session-id> agent=<agent> thread_backed=true
```

Long-running mode:

```bash
agentsdk serve .
```

This starts the same harness stack and waits for interrupt. Future HTTP/SSE, trigger, and scheduler control planes should attach to this host shape instead of adding a parallel runtime.

Useful flags:

```bash
agentsdk serve . --sessions-dir ./var/sessions
agentsdk serve . --agent coder
agentsdk serve . --plugin local_cli
agentsdk serve . --no-default-plugins
```

## Storage conventions

Default service session storage is:

```text
<resource-path>/.agentsdk/sessions
```

Use `--sessions-dir` for explicit deployments. Daemon-owned sessions should be thread-backed by default so workflow runs, future trigger fires, and resume/continue operations have a durable correlation point.

## Config/resource conventions

`agentsdk serve` uses the same resource path and manifest loading behavior as `agentsdk run`:

- resource path argument defaults to `.`;
- app manifests such as `agentsdk.app.json` are resolved by the existing agentdir/resource loader;
- manifest plugin refs and explicit `--plugin` refs flow through the existing plugin factory path;
- `--no-default-plugins` disables only the built-in local CLI fallback plugin, not manifest or explicit plugin refs.

## Verification

Covered by tests for:

- harness service status and registry lookup;
- daemon host lifecycle, persisted-session defaults, status, and shutdown;
- `agentsdk serve --status` CLI smoke output without entering an interactive REPL.

## Follow-up boundary

Section 12 should add triggers/scheduling on top of this shape. Trigger loops should target `daemon.Host` / `harness.Service` APIs and publish trigger-caused session/workflow metadata rather than creating a separate scheduler runtime.

## Host boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps daemon imports intentionally thin: `daemon` depends on `harness` and `trigger`, not on a second app/runtime/plugin system. Future daemon behavior should continue to delegate reusable execution semantics to harness/session.

## Triggers and scheduling

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
