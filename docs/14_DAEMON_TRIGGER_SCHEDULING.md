# 14 — Daemon, triggers, and scheduling checkpoint

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
