# Compaction / memory

Section 22 makes compaction a default, visible memory-management behavior rather than a hidden maintenance operation.

## Goals

- Keep `/compact`, `agent.Compact`, `agent.CompactWithOptions`, and runtime compaction APIs stable enough for dogfood.
- Keep auto-compaction enabled by default for normal agent sessions.
- Configure auto-compaction by context-window percentage, not by absolute token thresholds.
- Default auto-compaction to 85% of the resolved model context window.
- Allow explicit opt-out when hosts need deterministic no-extra-model-call behavior.
- Make both manual and automatic compaction visible while the summary is generated.
- Keep the final `conversation.compaction` event as the authoritative persisted memory state.

## Policy model

Compaction is memory maintenance owned by `agent.Instance` and backed by `runtime.Engine.Compact`. It is not an agent-callable tool/action and should not run in the middle of a model/tool turn.

Default policy:

```text
auto_compaction.enabled = true
auto_compaction.context_window_ratio = 0.85
auto_compaction.keep_window = 4
```

The trigger threshold is computed as:

```text
threshold_tokens = resolved_model_context_window * context_window_ratio
```

The context window comes from resolved model metadata/modeldb through the existing route identity path. When context-window metadata is unavailable, agentsdk uses a documented fallback context window solely to preserve protection. The fallback must be visible in policy/state output so hosts know the threshold was not modeldb-backed.

Absolute token thresholds are not the preferred public configuration surface. If kept for source compatibility, they should be deprecated/backcompat-only and not appear in new docs or resource examples.

## Opt-out

Hosts can opt out explicitly with:

```go
agent.WithAutoCompaction(agent.AutoCompactionConfig{Enabled: false})
```

Resource/app configuration should expose the same semantic flag:

```yaml
auto_compaction:
  enabled: false
```

Opt-out is for hosts that cannot tolerate background summary model calls, such as deterministic tests, offline demos, or externally managed memory policies.

## Visibility and event model

Compaction should publish lifecycle events that can be rendered by terminal clients and streamed through harness/API channels:

```text
compaction.started
compaction.summary_delta
compaction.summary_completed
compaction.committed
compaction.skipped
compaction.failed
```

Events should include enough metadata to answer:

- was compaction manual or automatic?
- why did it run?
- what percentage threshold was used?
- what context window was used, and was it resolved or fallback?
- how many estimated tokens existed before compaction?
- how many messages/nodes were replaced?
- how many estimated tokens were saved?
- what compaction node was committed?

Live summary deltas are presentation events. They should not be persisted by default. The final summary is persisted once in the canonical `conversation.compaction` event.

## Terminal behavior

Manual `/compact` and automatic compaction should render through the same visible path.

Terminal output should:

- clearly label compaction as memory maintenance, not a normal assistant answer
- show why compaction started
- stream the summary text while it is generated
- show final committed counters and saved-token estimates
- show failures without failing the already-completed user turn for auto-compaction

## Harness/API behavior

Harness sessions should expose compaction policy/state and publish compaction lifecycle events through `Session.Subscribe`. HTTP/SSE and future channels should consume that session event stream rather than duplicating terminal slash parsing.

The authoritative persisted state remains the thread/conversation event log. Dropped live deltas do not corrupt memory state because clients can fetch/replay the final `conversation.compaction` summary.

## Persistence

The durable state split is:

- `conversation.compaction` — authoritative final summary and replaced node IDs
- `conversation.auto_compaction` — diagnostic/metrics event for automatic compaction decisions
- live summary delta events — transient presentation stream, not persisted by default

Compaction floor and resume behavior remain runtime/conversation concerns. Section 22 should preserve existing floor behavior and add tests that thread-backed sessions still replay compacted memory correctly after resume.

## Tests

Coverage should include:

- default-enabled auto-compaction
- 85% context-window threshold calculation
- explicit opt-out
- invalid percentage validation/clamping
- fallback behavior when context-window metadata is unavailable
- manual `/compact` structured payload and visible summary
- auto-compaction with session/thread persistence and resume
- lifecycle events for started, summary deltas, summary completed, committed, skipped, and failed
- harness subscriber visibility
- terminal/CLI smoke coverage where practical
