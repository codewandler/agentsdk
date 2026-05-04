# Safety, compaction, and usage

## Safety and risk policy

The safety policy seam is defined without moving the existing terminal
risk log opportunistically.

## Ownership

Safety policy is surface-neutral and lives at execution boundaries:

- `action.Intent` describes side effects before execution.
- `safety.Assessor` evaluates the intent and returns a normalized decision.
- `safety.Gate` is action middleware that enforces allow / approval / reject /
  error outcomes.
- Harness, daemon, terminal, HTTP, and future channels own approval UX because
  only the channel knows whether a human is present and how to ask them.
- Existing `toolmw.RiskGate` remains the compatibility path for LLM-facing tools.
  `toolmw.SafetyAssessment` and `toolmw.ToolAssessment` bridge old tool
  middleware shapes to the new `safety` package while preserving shell/cmdrisk
  behavior.

Do not hide policy in declarative app/resource metadata alone. Metadata can
advertise expected risk, but enforcement belongs where execution actually starts.

## Decision model

`safety.DecisionAction` has four values:

- `allow` — execute without asking.
- `requires_approval` — call a boundary-supplied `safety.Approver` before
  execution.
- `reject` — deny before execution.
- `error` — deny because assessment itself produced an error decision.

Assessment failures fail closed by default. `Gate.FailOpen` exists for explicitly
trusted hosts that choose observation-only behavior, but should not be the
terminal or daemon default for unattended work.

## Approval boundaries

Approval is modeled as:

```go
type Approver func(ctx action.Ctx, request safety.ApprovalRequest) (bool, error)
```

Channels may implement it differently:

- terminal: prompt the user interactively;
- daemon/background trigger: deny approval-required work unless trusted config
  installs a non-interactive approver;
- HTTP/SSE: return an approval-required event or coordinate with a client-side
  flow;
- tests/embedded hosts: inject allow/deny functions.

The current section provides the seam and action-level enforcement. It does not
force every channel to expose a full approval UI in this slice.

## Events and audit trail

`safety.Event` is intentionally usable as `action.Event`. The gate records:

- `safety.assessed`
- `safety.allowed`
- `safety.approved`
- `safety.rejected`
- `safety.denied`
- `safety.errored`

`InMemoryAuditStore` gives tests and embedded hosts a concrete audit target.
Thread-backed durability can implement `safety.AuditStore` later without
changing the action or tool middleware contracts.

## Commands

`command.Policy` now includes descriptive safety fields:

- `SafetyClass`
- `RequiresApproval`

These fields are exported through command descriptor APIs so terminal, HTTP, TUI,
and model-facing command catalogs can show that a command may trigger sensitive
work. They are not by themselves an enforcement mechanism; command handlers that
start workflows/actions/tools must still run through the relevant safety gate.

## Shell/tool analyzer integration

The local CLI plugin continues to wire `tools/shell.WithRiskAnalyzer(...)` using
`cmdrisk`. The terminal still installs its existing observation-only risk logger
in `terminal/cli/load.go`. This section deliberately keeps that presentation path
in place until a later channel-specific approval UX section replaces it.

## Non-goals in this slice

- No global rules engine.
- No sandbox implementation.
- No terminal prompt UI migration.
- No daemon trust config format.
- No datasource-specific policy work.

## Execution primitive boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps safety primitive-level: `safety` and `actionmw` are the enforcement seam, while `toolmw` is a compatibility bridge for LLM-facing tools and existing shell/cmdrisk behavior.

## Compaction and memory

Compaction is a default, visible memory-management behavior rather than a hidden maintenance operation.

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

Compaction floor and resume behavior remain runtime/conversation concerns. Existing floor behavior is preserved and tested so that thread-backed sessions still replay compacted memory correctly after resume.

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
