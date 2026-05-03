# 12 — Workflow Execution Semantics

This note closes section 9 of `docs/04_TASKLIST.md`.

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
