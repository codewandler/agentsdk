# Harness, sessions, and channels

## Harness session lifecycle

This note records the section-4 harness/session lifecycle decisions from
`.agents/TASKLIST.md`.

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

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps `harness` as the live session aggregation point. The important cleanup candidate is explicit: centralize session/thread store selection and open/resume behavior in harness/session when that can remove duplicated `agent.Instance` lifecycle ownership.

`harness.LoadSession` remains useful as a loading convenience, but core `harness.Service` should stay channel-neutral and avoid terminal-like policy.

## Terminal CLI channel

Section 15 keeps the terminal package as a channel and presentation boundary. It
should adapt resources, app composition, harness sessions, and command results to
CLI behavior without becoming the canonical runtime owner.

## Boundary

`terminal/cli` owns CLI policy:

- one-shot versus interactive mode selection;
- terminal help text and grouped flag presentation;
- local terminal fallback plugin policy;
- resource-path argument handling;
- command-line flag parsing;
- terminal rendering and warning output.

It should not own reusable execution semantics:

- `app` composes reusable app definitions and registries;
- `harness.Session` executes session-bound commands, workflows, and agent turns;
- `daemon` owns long-running service/process conventions;
- `command` owns descriptors, policies, schemas, and result rendering contracts.

## One-shot and interactive modes

`agentsdk run [path] [task]` has two modes:

- if `[task]` is non-empty, it is sent once through `harness.Session.Send(...)`;
- if `[task]` is empty, the terminal REPL opens over the loaded session.

Slash commands use the same session execution path in both modes. The terminal
host renders returned `command.Result` values in one-shot mode and the REPL
renders command/turn output interactively.

Examples:

```bash
agentsdk run .
agentsdk run . /session info
agentsdk run . /workflow list
agentsdk run . /workflow start session_summary hello
```

## Plugin flags

The terminal host may activate a named local fallback plugin when resources do
not provide an agent. This is CLI host policy, not `app.New` default behavior.

```bash
agentsdk run . --plugin local_cli
agentsdk run . --plugin git --plugin skill
agentsdk run . --no-default-plugins
```

Conventions:

- `--plugin <name>` activates a named app plugin through the configured
  `app.PluginFactory`; it can be repeated.
- `--no-default-plugins` disables only the terminal host's built-in `local_cli`
  fallback policy.
- `--no-default-plugins` does not disable manifest plugin refs or explicit
  `--plugin` refs.
- Unknown plugin names should fail during load with a plugin-resolution error.

App manifests can declare plugin refs too:

```json
{
  "sources": [".agents"],
  "plugins": [
    "local_cli",
    {"name": "git", "config": {"mode": "read_only"}}
  ]
}
```

Manifest plugin refs are resource/app configuration. CLI `--plugin` refs are
operator overrides. Both flow through the same plugin factory path.

## Model/source API policy flags

The terminal command exposes model/source policy flags as channel policy knobs:

```bash
agentsdk run . --source-api openai.responses
agentsdk run . --model gpt-4.1
agentsdk run . --model-use-case agentic_coding --model-approved-only
agentsdk models --source-api anthropic.messages --thinking
```

Conventions:

- `--source-api` selects the model provider API compatibility path.
- `--model-use-case` and `--model-approved-only` constrain compatibility
  selection when configured.
- `agentsdk models` is the inspection surface for source API and model policy
  decisions; normal `agentsdk run` should only apply them.

## Debug and risk presentation

Debug-message output and risk-log presentation remain terminal concerns for now.
The terminal host currently wires log-only tool risk middleware and writes risk
observations to stderr so the TUI/REPL output remains readable. Do not move this
into `app.Plugin` or `harness.Session` until the safety policy model is designed.

## Command help and inspect surfaces

There are two command concepts exposed in terminal UX:

- command catalog descriptors from executable command trees;
- structured command resources from `.agents/commands/*.yaml`.

Executable command descriptors power `/help`, command catalog context for agents,
`session_command`, and future channel/API exports. Structured command resources
are declarative metadata; they become executable only when a harness/session
projection binds their target.

Current conventions:

```bash
agentsdk run . /help
agentsdk run . /workflow list
agentsdk discover .
```

`agentsdk discover` is the debugging surface for resource/app manifests. It
should distinguish:

- Markdown/app commands (`Commands`);
- structured command resources (`Structured commands`);
- workflows, actions, triggers, datasources, plugin refs, skills, and diagnostics.

The next high-value CLI improvement is executable structured command resources:

```text
resource.CommandContribution -> harness command projection -> Session target execution
```

That should remain a session projection rather than a new terminal runtime.

## Workflow UX

The CLI already renders async workflow starts, workflow run listings, run detail,
workflow events, reruns, and cancellation through structured command payloads.
Further workflow polish should happen only when dogfood finds concrete gaps.

Examples:

```bash
agentsdk run . /workflow start nightly_check --async
agentsdk run . /workflow runs
agentsdk run . /workflow run <run-id>
agentsdk run . /workflow events <run-id>
agentsdk run . /workflow cancel <run-id>
```

## Section 15 decisions

- Keep `terminal/cli.Load` as the shared CLI prelude unless another extraction
  deletes duplicated code.
- Keep local CLI fallback policy in terminal.
- Keep debug/risk presentation in terminal until the safety model is designed.
- Do not make terminal slash parsing the canonical API for future HTTP/SSE
  channels; use `harness.Session.ExecuteCommand` and command descriptors instead.
- Use "structured command resources" for resource YAML commands and "command
  descriptors" for executable catalog metadata. Avoid reviving the removed
  `command-descriptors/` resource directory concept.

## Host boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps terminal as a host/channel package but flags its direct `agent`, `runner`, and `tool` dependencies as cleanup candidates. Terminal should keep CLI policy, local fallback plugin policy, slash parsing, and rendering; live runtime/session state should move behind harness/session APIs as those APIs become sufficient.

## HTTP/SSE channel

Section 16 adds a small HTTP/SSE channel over `harness.Service` while keeping the
harness core protocol-neutral.

## Protocol namespaces

The channel intentionally separates the native Agents SDK API from compatibility
surfaces:

```text
/api/agentsdk/v1/...   native harness/session HTTP API
/ag-ui/v1/...          AG-UI compatibility namespace
```

The native API is the first implemented surface. The AG-UI namespace is reserved
and documented so future compatibility work does not leak AG-UI concepts into
`harness` internals or the native API shape.

## Native API

Implemented endpoints:

```text
GET  /api/agentsdk/v1/health
GET  /api/agentsdk/v1/sessions
POST /api/agentsdk/v1/sessions
POST /api/agentsdk/v1/sessions/{session}/commands
GET  /api/agentsdk/v1/sessions/{session}/context
GET  /api/agentsdk/v1/sessions/{session}/events
POST /api/agentsdk/v1/sessions/{session}/workflows/{workflow}/start
GET  /api/agentsdk/v1/sessions/{session}/workflows/runs
```

`{session}` may be either the harness registry name or the session ID.

### Command execution

Command execution uses structured command paths and maps directly to
`Session.ExecuteCommand(...)`:

```json
{
  "path": ["workflow", "list"],
  "input": {}
}
```

Terminal slash strings are intentionally not the native API:

```json
{"command": "/workflow list"}
```

That shape is rejected. Terminal slash parsing remains terminal policy; HTTP and
future channels should use command descriptors and structured paths.

The response preserves structured command data and renders the payload for
machine/presentation clients:

```json
{
  "kind": "display",
  "payload": {},
  "display": "terminal-oriented rendering",
  "json": "{...}"
}
```

The `json` field is currently the command package's JSON rendering text. Future
iterations can replace it with an object once command result serialization is
made explicit.

### Sessions

Session open/list endpoints are thin wrappers around harness service APIs:

```json
POST /api/agentsdk/v1/sessions
{
  "name": "web",
  "agent_name": "coder",
  "store_dir": "./var/sessions",
  "resume": "session-id-or-jsonl-path"
}
```

`harness.Service` still owns lifecycle and registry semantics.

### Workflow endpoints

Workflow endpoints are convenience wrappers around session APIs:

- sync start calls `Session.ExecuteWorkflow(...)`;
- async start calls `Session.StartWorkflow(...)`;
- run listing calls `Session.WorkflowRuns(...)`.

The command endpoint can also drive workflow commands through structured command
paths, so workflow-specific endpoints are additive convenience, not a separate
canonical workflow runtime.

### Context inspection

`GET /api/agentsdk/v1/sessions/{session}/context` returns provider descriptors
and the last committed context render snapshot. The endpoint is a channel
inspection surface over `agentcontext.Manager`; it does not render providers or
mutate context state.

### SSE events

`GET /api/agentsdk/v1/sessions/{session}/events` subscribes to
`Session.Subscribe(...)` and emits server-sent events:

```text
event: session
data: {"type":"command", ...}
```

The event payload is a JSON-safe projection of `harness.SessionEvent`; errors are
serialized as strings.

## AG-UI compatibility

`GET /ag-ui/v1` currently advertises the compatibility boundary. It does not yet
implement AG-UI run/event mapping.

Design rules for the future AG-UI adapter:

- Keep AG-UI types at the channel boundary.
- Do not make `harness.Session` speak AG-UI directly.
- Map `harness.SessionEvent`, `command.Result`, workflow events, and agent turn
  lifecycle events into AG-UI events inside `channel/httpapi` or a sibling
  `channel/agui` package.
- Preserve `/api/agentsdk/v1` as the native API; do not change native payloads to
  chase one compatibility protocol.

## A2UI relationship

A2UI is treated as a future generative UI payload format, not the runtime
transport API. It can be carried inside native displayable events or AG-UI events
later, for example:

```json
{
  "type": "ui.a2ui",
  "payload": {
    "version": "v0.9",
    "createSurface": {"surfaceId": "summary", "catalogId": "..."}
  }
}
```

This keeps AG-UI as the likely runtime compatibility target and A2UI as a
renderable payload shape.

## Current limitations

- No authentication, authorization, CORS, or deployment server lifecycle yet.
- AG-UI endpoint is a compatibility boundary placeholder, not a full adapter.
- Command JSON rendering is still text; future command result serialization can
  make this more structured.
- Workflow run listing requires thread-backed sessions, as in the harness API.
- SSE is session-scoped and does not yet multiplex service-wide events.

## Host boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps HTTP/SSE as a thin channel adapter over `harness.Service` and `harness.Session`. Protocol-specific concepts, including future AG-UI/A2UI mappings, should stay in channel packages and not leak into core harness semantics.
