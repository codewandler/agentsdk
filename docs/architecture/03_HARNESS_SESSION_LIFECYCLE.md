# 07 — Harness Session Lifecycle

This note records the section-4 harness/session lifecycle decisions from
`docs/04_TASKLIST.md`.

## Long-term shape

`harness.Service` is the host-owned lifecycle boundary for an `app.App`. It should
own generic session APIs that every channel can reuse: open, list, resume, close,
and subscribe. Terminal, HTTP/SSE, TUI, and tests should provide channel policy
and rendering, not each invent their own app/agent/session lifecycle.

`harness.Session` is the per-session execution boundary. It owns dispatching user
turns, command execution, synchronous workflow starts, thread-backed workflow run
lookup, and session-scoped event publication. It should not yet own every detail
inside `agent.Instance`; thread store creation and resume still flow through
agent options until harness/session lifecycle is stable enough to absorb that
code without adding indirection.

## Decisions

- `harness.Service` now has stable APIs for:
  - `OpenSession(ctx, SessionOpenRequest)`
  - `ResumeSession(ctx, SessionOpenRequest)`
  - `Sessions()`
  - `Close()`
- `harness.Session` now has stable APIs for:
  - `Subscribe(buffer)`
  - `Close()`
- `SessionOpenRequest.StoreDir` and `SessionOpenRequest.Resume` are the stable
  harness-level inputs for persisted session creation/resume. Internally, these
  still map to `agent.WithSessionStoreDir` and `agent.WithResumeSession`.
- `SessionSummary` is intentionally small: session name, session ID, agent name,
  thread-backed flag, and closed flag.
- `SessionEvent` is intentionally minimal and channel-neutral. It currently
  covers opened, input, command, workflow, and closed events. It is not the final
  displayable/event model from section 5.

## Ownership boundaries

### Session open/resume

`harness.Service.OpenSession` and `ResumeSession` are now the public lifecycle
entry points. They instantiate the requested/default agent, attach the agent
command projection, and track the resulting session by name.

The direct JSONL/thread opening still lives under `agent.Instance` for now. Moving
that code into harness would be premature until the store abstraction and close
semantics are clearer.

### Thread lifecycle

`harness.Session` owns thread-backed workflow run lookup and passes persistence
options through open/resume requests. It does not yet own branch creation,
compaction replay, or store migration. Those stay in agent/runtime/thread until a
clearer lifecycle seam appears.

### Agent lifecycle

`harness.Service` owns opening named session agents and closing sessions. The app
still owns agent specs, plugins, tools, commands, actions, and workflows.
`agent.Instance` still owns model/runtime internals.

### Workflow lifecycle

`harness.Session.ExecuteWorkflow` remains synchronous. It records workflow events
to the session live thread when one exists and publishes a `SessionEventWorkflow`
for channel subscribers. Async start, cancellation, events-by-run, and richer
status remain section-8 workflow lifecycle work.

### Channel event publication

Subscriptions are now available at the session boundary. Events are
channel-neutral data, not terminal-rendered strings. The current API is a bridge
until the structured output/displayable design lands.

### Terminal lifecycle

No terminal-specific lifecycle code was moved in this slice because the remaining
terminal pieces are CLI policy:

- resolving `--sessions-dir`
- resolving `--session` and `--continue`
- choosing default plugin policy
- rendering command/workflow/session results

Those should stay in terminal until another channel needs the same behavior. The
generic lifecycle that other channels need is now exposed through `harness.Service`
and `harness.Session`.

## Host boundary review

The docs-only host boundary review in [`29_HARNESS_CHANNEL_BOUNDARY.md`](29_HARNESS_CHANNEL_BOUNDARY.md) confirms `harness` is correctly the live session aggregation point. The important cleanup candidate is now explicit: centralize session/thread store selection and open/resume behavior in harness/session when that can remove duplicated `agent.Instance` lifecycle ownership.

`harness.LoadSession` remains useful as a loading convenience, but core `harness.Service` should stay channel-neutral and avoid terminal-like policy.
