# HTTP/SSE channel and AG-UI compatibility

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
