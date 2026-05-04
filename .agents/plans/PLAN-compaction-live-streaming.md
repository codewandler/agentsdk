# PLAN: Live Compaction Summary Streaming and Session Visibility

## Status

Planned. This plan captures the roadmap-aligned solution for making conversation compaction observable while it runs, including live summary text deltas and a clear record of what the continued session will use as memory.

## Context

Current compaction is functionally complete but mostly invisible while it is running:

- `agent.Instance.RunTurn` calls `maybeAutoCompact` after a successful turn.
- `agent.CompactWithOptions` generates a summary through `generateCompactionSummary` and then commits `conversation.CompactionEvent` through `runtime.Engine.Compact`.
- `generateCompactionSummary` calls `unified.Client.Request` directly, accumulates `unified.TextDeltaEvent` internally, and returns only the final string.
- The terminal and harness see only final counters / notices, not the summary text being generated.
- The final `conversation.compaction` thread event does contain the committed summary, but only after the model call and compaction commit complete.

This creates a visibility gap: during auto-compaction or `/compact`, users and API clients cannot see the exact summary that future turns will continue with.

## Goals

1. Stream the generated compaction summary live as text deltas.
2. Make compaction lifecycle explicit: start, summary deltas, summary completed, committed, failed.
3. Preserve `conversation.compaction` as the authoritative persisted final state.
4. Expose enough metadata to explain why compaction happened and what changed.
5. Align with the long-term output event model in `docs/08_OUTPUT_EVENT_MODEL.md` instead of adding more ad-hoc `io.Writer` output paths.
6. Support both terminal users and harness/API subscribers.
7. Keep compaction semantically separate from a normal assistant turn.

## Non-goals

- Do not rewrite conversation projection or compaction floor behavior.
- Do not make compaction an agent-callable tool/action.
- Do not duplicate full summary text into diagnostic metadata events when `conversation.compaction` already stores authoritative state.
- Do not introduce a terminal-only fix that writes deltas directly to `agent.Out()`.

## Recommendation

Implement **agent-level output/lifecycle events** for compaction, with bridges to terminal rendering and harness session subscriptions.

Do not model compaction as a normal `runner` turn. Compaction is memory maintenance owned by `agent.Instance`; it happens between user turns or through `/compact`, and its result changes the projected session state. Reusing `runner.TextDeltaEvent` directly would blur turn semantics and make `runner` describe lifecycle work it does not own.

The future-proof direction is:

```text
agent compaction
  -> agent output/lifecycle events
      -> terminal/ui renderer
      -> harness Session.Subscribe / API stream
      -> optional future unified output Sink
  -> runtime.Compact
      -> conversation.compaction thread event (authoritative final state)
```

`agent.WithOutput` remains a compatibility bridge for coarse notices until replaced by the output event model.

## Target event model

Add a small, typed agent event surface. Exact package names can evolve, but the events should be owned above `runner` and below terminal/harness.

Candidate package/API:

```go
package agent

type Event interface{}
type EventHandler func(Event)

type CompactionTrigger string

const (
    CompactionTriggerManual CompactionTrigger = "manual"
    CompactionTriggerAuto   CompactionTrigger = "auto"
)

type CompactionStage string

const (
    CompactionStageSelect         CompactionStage = "select"
    CompactionStageSummaryRequest CompactionStage = "summary_request"
    CompactionStageSummaryStream  CompactionStage = "summary_stream"
    CompactionStageCommit         CompactionStage = "commit"
)

type CompactionStartedEvent struct {
    TurnID          int
    Trigger         CompactionTrigger
    KeepWindow      int
    EstimatedTokens int
    Threshold       int
    TokensBefore    int
    ReplacedCount   int
}

type CompactionSummaryDeltaEvent struct {
    TurnID  int
    Trigger CompactionTrigger
    Text    string
}

type CompactionSummaryCompletedEvent struct {
    TurnID        int
    Trigger       CompactionTrigger
    SummaryBytes  int
    SummaryTokens int
}

type CompactionCommittedEvent struct {
    TurnID           int
    Trigger          CompactionTrigger
    CompactionNodeID conversation.NodeID
    ReplacedCount    int
    TokensBefore     int
    TokensAfter      int
}

type CompactionFailedEvent struct {
    TurnID  int
    Trigger CompactionTrigger
    Stage   CompactionStage
    Err     error
}
```

Notes:

- Avoid importing `agent` into `runner`; keep these events in `agent` or a future neutral output package.
- Keep full summary content in deltas and in the final `conversation.CompactionEvent`, not in every metadata payload.
- Include trigger and turn/session metadata so renderers can label the event and clients can correlate it.
- If privacy/redaction policies are added later, apply them at sink/rendering boundaries, not at conversation state persistence.

## Public API shape

Add an event hook without replacing existing runner event handling immediately:

```go
func WithEventHandler(handler agent.EventHandler) Option
```

or, if this lands alongside the broader output model:

```go
func WithOutputSink(sink output.Sink) Option
```

Preferred migration path:

1. Add an `agent.EventHandler` now, deliberately scoped to agent lifecycle/output events.
2. Keep `WithEventHandlerFactory` for `runner.Event` until the output model subsumes normal turn streaming too.
3. Later adapt `WithOutput` and `WithEventHandlerFactory` into the unified output sink.

`CompactOptions` can be extended source-compatibly:

```go
type CompactOptions struct {
    KeepWindow int
    Summary    string

    // New optional metadata. Empty values get sensible defaults.
    Trigger CompactionTrigger
    TurnID  int
}
```

Auto-compaction should pass `Trigger: CompactionTriggerAuto` and the current turn id if available. `/compact` should pass `Trigger: CompactionTriggerManual`.

## Summary generation changes

Current `generateCompactionSummary` should become an observable streaming operation.

Required behavior:

1. Build the same transcript and request.
2. Set `Stream: true` unless explicitly disabled for provider compatibility.
3. Dispatch the request through a shared helper that can call request observers.
4. For each provider event:
   - `unified.TextDeltaEvent`: append text and emit `CompactionSummaryDeltaEvent`.
   - `unified.ReasoningDeltaEvent`: either ignore initially or add a separate reasoning event if exposed reasoning is needed.
   - `unified.RouteEvent`: update/emit provider route visibility if the agent event model includes provider metadata.
   - `unified.ProviderExecutionEvent`: emit or record execution metadata.
   - `unified.UsageEvent`: record usage as compaction usage, not as a normal assistant turn.
   - `unified.WarningEvent`: publish diagnostic/warning output.
   - `unified.ErrorEvent`: emit `CompactionFailedEvent` and return error.
   - `unified.CompletedEvent`: mark stream completion.
5. Validate that non-empty summary text was produced.
6. Emit `CompactionSummaryCompletedEvent` before commit.

Longer term, factor the stream consumption logic so compaction does not duplicate all of `runner.consumeEvents`. The shared helper should be careful not to commit request/assistant messages to conversation history.

## Persistence model

Keep the existing authority split:

- `conversation.compaction`
  - authoritative state event
  - contains final summary and replaced node ids
  - used by resume/projection

- `conversation.auto_compaction`
  - diagnostic/metrics event
  - keep current metadata and add optional fields only if needed:
    - trigger
    - turn id
    - duration
    - failure stage for failed auto-compactions if a separate event is not added

Do not store live deltas in the thread event log by default. Deltas are presentation/stream events; the final summary is already persisted once as state.

If future audit requirements demand reconstructing live generation, add a separate opt-in provider transcript/debug event stream with redaction controls.

## Harness and API streaming

Extend `harness.Session.Subscribe` to surface agent output events.

Candidate additions:

```go
const SessionEventAgent SessionEventType = "agent"

type SessionEvent struct {
    // existing fields...
    AgentEvent any
}
```

or a more explicit variant:

```go
const SessionEventOutput SessionEventType = "output"

type SessionEvent struct {
    // existing fields...
    OutputKind string // "compaction.started", "compaction.delta", ...
    TextDelta  string
    Payload    any
}
```

Recommendation: prefer a generic output/agent event carrier over a compaction-only session event. The same path can later carry normal assistant deltas, diagnostics, debug payloads, and tool/output events.

Important delivery decision:

- Current `Session.publish` is non-blocking and drops events when subscriber channels are full.
- That is acceptable for coarse lifecycle notices but risky for text deltas.

Roadmap-aligned approach:

1. Keep `Session.Subscribe` best-effort for compatibility.
2. Document that token/text deltas can be dropped unless the subscriber provides enough buffer.
3. Add a future dedicated streaming API with backpressure if API clients require reliable byte-for-byte live text.
4. The final committed summary remains retrievable via the conversation/thread state even if live deltas were dropped.

## Terminal rendering

Update `terminal/ui` to render compaction events distinctly from assistant answers.

Target UX:

```text
[compacting conversation: replacing 42 messages]
Summary of previous work...
...
[compacted: replaced 42 messages, ~71000 tokens saved]
```

Requirements:

- Label auto-compaction clearly; do not make it look like the assistant is answering the user.
- Reuse the streaming markdown renderer path where practical, but keep compaction display state separate from normal `StepDisplay` state.
- End/flush the compaction display on committed or failed events.
- Manual `/compact` and auto-compaction should render through the same code path.

## Request/usage/provider visibility

Compaction summary generation is a model call and should be observable like one:

- Call `WithRequestObserver` for compaction summary requests.
- Record usage with a distinct kind/component, for example `operation=compaction`.
- Preserve route/provider execution metadata where available.
- Avoid updating current provider identity incorrectly if compaction uses a different model/provider in the future.

Future-proof detail: usage records should be able to distinguish:

- normal assistant turn
- compaction summary generation
- tool/model calls inside workflows/actions

If the existing `usage.Record` schema cannot represent this cleanly, add an operation/component field in a separate usage plan rather than overloading turn ids.

## Implementation phases

### Phase 1: Agent event surface

Files likely touched:

- `agent/events.go` or `agent/output.go`
- `agent/agent.go`
- `agent/options.go`

Tasks:

- Define agent event types for compaction lifecycle.
- Add event handler option.
- Add internal `emitEvent` helper.
- Ensure existing `WithEventHandlerFactory` for `runner.Event` remains unchanged.

Verification:

- Unit test handler registration and event ordering with synthetic events.

### Phase 2: Observable compaction lifecycle

Files likely touched:

- `agent/compact.go`
- `agent/agent.go`
- `harness/control_command.go`

Tasks:

- Extend `CompactOptions` with trigger/turn metadata.
- Emit started before summary request.
- Emit failed with stage-specific errors.
- Emit committed after `runtime.Compact` succeeds.
- Pass `manual` trigger from `/compact`.
- Pass `auto` trigger from `maybeAutoCompact`.

Verification:

- `agent` tests for manual and auto lifecycle events.
- Assert no committed event when summary generation fails.

### Phase 3: Stream summary text

Files likely touched:

- `agent/compact.go`

Tasks:

- Set compaction summary request `Stream: true`.
- Emit `CompactionSummaryDeltaEvent` for each text delta.
- Emit `CompactionSummaryCompletedEvent` after non-empty summary is assembled.
- Handle errors and context cancellation with failed events.
- Optionally require/track `CompletedEvent` similarly to `runner` stream handling.

Verification:

- Fake client emits multiple text chunks; test receives matching deltas in order.
- Final committed `conversation.CompactionEvent.Summary` equals concatenated deltas.
- Context cancellation emits failed event and does not commit compaction.

### Phase 4: Terminal bridge

Files likely touched:

- `terminal/ui/agent.go`
- `terminal/ui/events.go` or new `terminal/ui/compaction.go`
- `terminal/cli/load.go`
- example apps that install terminal handlers

Tasks:

- Add terminal renderer for agent compaction events.
- Install both runner event handler and agent event handler in CLI/app loading.
- Keep display visually separate from normal turn output.

Verification:

- Terminal UI unit tests or snapshot tests for start/delta/commit/failure.
- Focused package tests: `go test ./terminal/...`.

### Phase 5: Harness session streaming

Files likely touched:

- `harness/harness.go`
- `harness/load.go`
- `harness/harness_test.go`

Tasks:

- Publish agent output events through `Session.Subscribe`.
- Ensure event enrichment includes session name/id and agent name.
- Decide and document best-effort delivery semantics for deltas.
- Keep command result behavior unchanged.

Verification:

- Subscriber receives compaction start/delta/committed for `/compact`.
- Subscriber receives auto-compaction lifecycle during `Session.Send`.
- Tests use sufficient buffer to avoid intentional drops.

### Phase 6: Request observer, usage, provider metadata

Files likely touched:

- `agent/compact.go`
- `agent/agent.go`
- `usage/` if operation attribution is added
- `runtime/history_events.go` only if new persisted diagnostic events are needed

Tasks:

- Call request observer for compaction summary requests.
- Record compaction usage separately from normal turns.
- Surface provider route/execution metadata either as agent events or persisted provider metadata.
- Add failure diagnostics comparable to provider stream failure where useful.

Verification:

- Request observer sees compaction summary request.
- Usage tracker includes compaction usage with distinct operation/component.
- Existing usage totals remain backward-compatible or migration is documented.

## Acceptance criteria

- During `/compact`, terminal users see the summary text stream live and then a final compacted notice.
- During auto-compaction, terminal users see a clearly labeled compaction stream before the next prompt/turn continues.
- Harness/API subscribers can observe compaction lifecycle and text deltas.
- The final session state still comes from a single `conversation.compaction` event containing the committed summary.
- Dropped live deltas do not make state unrecoverable; clients can still fetch/replay final state.
- Existing normal turn streaming behavior remains unchanged.
- `go test ./...` passes.

## Trade-offs

### Agent events vs runner events

Agent events are the recommended ownership boundary. Runner events describe model/tool turn execution; compaction is agent memory maintenance. This avoids contaminating runner step semantics and lets future output events encompass more than model turns.

### Live deltas vs persisted deltas

Do not persist deltas by default. Persisting every chunk increases event log size and complicates redaction. Persist the final summary once as state; stream deltas as ephemeral presentation events.

### Best-effort session subscription vs reliable streaming

Reusing `Session.Subscribe` gets API visibility quickly but inherits non-blocking/drop behavior. The final summary event protects correctness. If exact live text delivery is required, add a dedicated streaming API with backpressure later.

### Full summary visibility vs privacy

The compaction summary is the actual future context, so showing it is important for transparency. Redaction belongs at sink/rendering boundaries, not in the canonical conversation state.

## Open questions

1. Should compaction summary requests require a `CompletedEvent`, matching normal runner behavior, or accept closed streams with text as today?
2. Should compaction usage be part of the existing usage tracker totals by default, or reported separately?
3. Should manual `/compact` allocate its own turn id, reuse the current session turn counter, or use a separate operation id?
4. Should future compaction support a separate model/policy from the main agent model?
5. Should harness expose raw typed Go events, JSON-safe payloads, or both?

## Related documents

- `.agents/plans/DESIGN-compaction.md`
- `.agents/plans/PLAN-compaction.md`
- `docs/08_OUTPUT_EVENT_MODEL.md`
- `docs/04_TASKLIST.md` section 21, Compaction / memory
