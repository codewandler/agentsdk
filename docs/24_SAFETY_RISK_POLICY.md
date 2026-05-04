# Safety / risk policy

Section 20 defines the safety policy seam without moving the existing terminal
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
