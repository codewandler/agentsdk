# 08 — Structured Output and Event Model

This note completes the section-5 writer/output replacement design from
`docs/04_TASKLIST.md`. It defines the target model before replacing remaining
writer fields. The implementation rule is intentionally conservative: new code
should target these shapes, while existing writer paths are migrated only in
small follow-up slices that keep terminal dogfood stable.

## Goals

- Keep execution code structured: agent, harness, command, workflow, and safety
  code should publish typed data, not terminal strings.
- Keep rendering at channel boundaries: terminal, TUI, HTTP/SSE, JSON clients,
  and LLM-facing tool summaries choose their own representation.
- Preserve current dogfood behavior while removing writer-only seams gradually.
- Avoid one renderer interface that tries to fit every channel equally poorly.

## Non-goals

- This is not the full risk/safety policy design.
- This is not asynchronous workflow lifecycle.
- This is not a terminal UI rewrite.
- This does not remove every `fmt.Fprintf` that formats a payload into a
  `strings.Builder`; payload-local formatting is acceptable until a renderer
  registry is justified.

## Core event envelope

The target publication unit is an output event:

```go
type Event struct {
    ID        string
    Time      time.Time
    Scope     Scope
    Kind      Kind
    Severity  Severity
    Payload   any
    Source    Source
    Trace     Trace
}
```

Recommended fields:

- `ID`: optional stable event ID for streams and persisted events.
- `Time`: producer timestamp.
- `Scope`: app/session/turn/step/workflow identifiers.
- `Kind`: semantic event kind, such as `command.result` or `usage.recorded`.
- `Severity`: debug/info/warn/error.
- `Payload`: typed payload; never pre-rendered terminal text unless the payload
  is explicitly a legacy text payload.
- `Source`: component and package that emitted the event.
- `Trace`: correlation IDs for request, run, command path, or tool call.

## Scope shape

```go
type Scope struct {
    AppName      string
    SessionName  string
    SessionID    string
    AgentName    string
    TurnID       string
    WorkflowName string
    WorkflowRun  string
    StepID       string
}
```

This keeps channel consumers from parsing IDs out of display strings.

## Event kinds and payloads

### Displayable payloads

Displayable payloads represent user-facing information. The payload should be
serializable and should carry enough structure for non-terminal renderers.

```go
type DisplayPayload struct {
    Title       string
    Summary     string
    Body        any
    Fields      []Field
    Items       []Item
    Tables      []Table
    Attachments []AttachmentRef
}
```

Rules:

- Prefer typed domain payloads (`SessionInfoPayload`, `WorkflowRunPayload`) when
  a command already has one.
- Use `DisplayPayload` for generic notices, short summaries, and simple lists
  only after repeated shapes justify it.
- Large values should be references, not embedded strings.

### Notices

Notices are structured status messages that are not command results by
themselves.

```go
type NoticePayload struct {
    Code    string
    Level   Severity
    Message string
    Details []Field
}
```

Examples:

- fallback plugin selected
- no workflows configured
- compatibility evidence unavailable
- session resumed from path

### Command results

Command execution should publish both the raw `command.Result` and command
metadata:

```go
type CommandResultPayload struct {
    Path       []string
    Descriptor command.Descriptor
    Result     command.Result
    Error      string
}
```

`command.Result` stays the trusted in-process result type. The output event adds
channel-independent context around it.

### Workflow events

Workflow output publication mirrors the workflow event stream:

```go
type WorkflowEventPayload struct {
    RunID        workflow.RunID
    WorkflowName string
    Status       workflow.RunStatus
    StepID       string
    ActionName   string
    InputRef     string
    OutputRef    string
    Error        string
}
```

Rules:

- Synchronous `Session.ExecuteWorkflow` publishes a summary event today.
- Future async workflow lifecycle should publish started/running/queued/canceled
  events using the same envelope.
- Full workflow event history remains in workflow/thread storage; channel events
  are the live stream/read-model projection.

### Usage records

Usage is already structured in `usage.Record`; output publication should wrap it:

```go
type UsageRecordPayload struct {
    Record usage.Record
    Totals usage.Aggregate
}
```

Rules:

- Usage persistence errors are diagnostics, not usage records.
- Terminal can continue printing per-step and per-session usage summaries.
- JSON/SSE clients should receive raw usage dimensions and totals.

### Diagnostics

Diagnostics are for operational or configuration issues:

```go
type DiagnosticPayload struct {
    Code      string
    Component string
    Message   string
    Error     string
    Details   []Field
}
```

Examples:

- invalid resources
- plugin resolution failures
- usage persistence append failure
- renderer failure
- compatibility policy diagnostic

### Debug events

Debug events are explicit and opt-in:

```go
type DebugPayload struct {
    Label string
    Data  any
}
```

Rules:

- Debug-message output currently printed by terminal should become
  `KindDebug`/`DebugPayload`.
- Debug payloads may be redacted by channel policy.
- Debug events are not model-visible unless an LLM summary renderer explicitly
  includes them.

### Risk/safety events

Risk and safety events need a separate policy model. The minimum publication
shape is:

```go
type RiskPayload struct {
    Operation string
    Risk      string
    Decision  string
    Details   []Field
}
```

Do not migrate risk logging opportunistically. Keep current terminal risk logging
until the safety/risk policy section defines approval gates, audit trails, and
channel-specific UX.

## Renderer contracts

### Terminal renderer

Contract:

```go
type TerminalRenderer interface {
    RenderTerminal(Event, io.Writer) error
}
```

Requirements:

- May use ANSI color and progressive streaming.
- May render markdown live.
- Should never be called from core agent/runtime code.
- Owns terminal-specific wording, spacing, and status lines.

Current terminal writer paths map here:

- runner event display in `terminal/ui`
- one-shot command rendering in `terminal/cli/run.go`
- REPL command rendering in `terminal/repl`
- usage summaries in `terminal/ui/usage.go`

### TUI renderer

Contract:

```go
type TUIRenderer interface {
    Reduce(Event) TUIState
}
```

Requirements:

- Treats events as state updates, not line output.
- Keeps partial reasoning/text/tool state separately from final summaries.
- Needs stable event IDs and correlation scope.

### HTTP/SSE renderer

Contract:

```go
type StreamRenderer interface {
    EncodeEvent(Event) (name string, data []byte, err error)
}
```

Requirements:

- Uses event kind as SSE event name.
- Encodes JSON payloads without terminal formatting.
- Supports replay by event ID when persistence exists.

### JSON / machine-readable renderer

Contract:

```go
type JSONRenderer interface {
    MarshalEvent(Event) ([]byte, error)
}
```

Requirements:

- Stable field names.
- Typed payload discriminator.
- No ANSI, markdown-only formatting, or human-only labels as canonical data.

### LLM-facing summary renderer

Contract:

```go
type LLMSummaryRenderer interface {
    Summarize(Event) (string, error)
}
```

Requirements:

- Compact, safe, and explicit about failures.
- Uses command descriptors and payload schemas where available.
- Redacts debug/risk details unless the policy allows them.

## `Display(mode)` decision

`payload.Display(mode)` remains sufficient for current command payloads because
commands are the only mature structured-result boundary today. Keep it for:

- `command.DisplayTerminal`
- `command.DisplayJSON`
- `command.DisplayLLM`
- small payload-local renderers that already have focused tests

Do not expand `Display(mode)` into the general event rendering system. Event
rendering needs scope, severity, timing, stream/replay IDs, and channel policy;
those do not belong on every command payload.

## Renderer registry decision

Do not add a global renderer registry yet.

Trade-off:

- A registry would make pluggable channels possible.
- It would also add indirection before there are enough independent channel
  implementations to prove the abstraction.

Decision: keep direct renderer functions/interfaces per channel. Add a registry
only after at least terminal plus one non-terminal channel need the same dynamic
lookup behavior.

## Migration plan for existing writer paths

### `harness.SessionLoadConfig.App.Output`

Current role: passes an `io.Writer` to `agent.WithOutput` for verbose/diagnostic
agent output.

Target: replace with an output sink or event subscriber once `Event`/`Sink` is a
public package. Until then, keep the field as a compatibility bridge and document
it as unstable.

### `agent.WithOutput`

Current role: writer for usage persistence diagnostics and auto-compaction
messages.

Target: replace with `agent.WithOutputSink` or equivalent once the output event
package exists. `WithOutput` should become a compatibility adapter that converts
legacy text to diagnostic/notice events, then eventually deprecate.

### Terminal event handler writer paths

Current role: `terminal/ui.AgentEventHandlerFactory(out)` renders runner events
directly to `io.Writer`.

Target: terminal should subscribe to session/agent output events and render them
through a terminal renderer. Keep current handler until the event stream covers
runner events completely.

### Debug-message output

Current role: `terminal/cli/load.go` writes debug-message payloads directly.

Target: publish `DebugPayload` events and let terminal/JSON renderers decide
whether to display or redact them.

### Auto-compaction output

Current role: `agent.maybeAutoCompact` writes success/failure text to
`agent.Out()`.

Target: publish notice/diagnostic events with compaction metadata:
`replaced_count`, `tokens_before`, `tokens_after`, and `tokens_saved`.

### Usage persistence error output

Current role: verbose `fmt.Fprintf(a.Out(), ...)` for marshal/append failures.

Target: publish `DiagnosticPayload` with component `agent.usage_persistence` and
error details.

## Risk logging decision

Keep risk logging out of this slice. The current terminal risk log path remains
until the safety/risk policy section defines approvals, audit trail persistence,
and channel-specific UX.

## Acceptance checklist

This design intentionally completes section 5 by defining the model and migration
contracts. It does not mechanically replace every writer call in the same slice;
that would mix design with broad behavior changes and risk destabilizing the
terminal dogfood path.
