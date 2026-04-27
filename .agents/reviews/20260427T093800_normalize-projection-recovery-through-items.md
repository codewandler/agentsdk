# Code Review: Normalize projection recovery through items

**Commit:** `8f6d8b3` — Normalize projection recovery through items
**Reviewed:** 2026-04-27T09:38:00+02:00
**Verdict:** Request Changes (minor)

**Resolution:** The blocking cleanup items were addressed in the final plan
closeout batch: tool-call filtering now uses a separate allocation,
`completedToolCalls` mutation has a single owner, recovery commits document the
empty finish reason, and repeated tool-call ID edge cases are covered.

## One-Sentence Summary

This commit eliminates the dual `Messages`/`Items` projection paths in
`ProjectionInput`, adds duplicate tool-call/result deduplication to
`NormalizeItems`, persists a recovery transcript when a runner turn aborts
mid-tool-execution, wires the planner capability into `DefaultSpec`, and
expands documentation (README package index, AGENTS.md testing/examples
sections).

---

## Blocking Issues

### B1 — `filtered := msg.ToolCalls[:0]` aliases the backing array from `sanitizeToolCalls`

**File:** `conversation/item.go:195`

`sanitizeToolCalls` returns a freshly allocated slice, so `msg.ToolCalls[:0]`
reuses that allocation. The subsequent `for _, call := range msg.ToolCalls`
iterates the *same* backing array that `filtered` is appending into. This is
safe **only** because `filtered` can never grow beyond the original length (it
only keeps or drops elements). However, this is a subtle correctness invariant
that is easy to break in a future edit. If someone later adds a call that
*inserts* elements (e.g. synthetic calls), the range loop would read mutated
data.

**Recommendation:** Use a separate allocation or iterate a copy:

```go
sanitized := sanitizeToolCalls(msg.ToolCalls)
filtered := make([]unified.ToolCall, 0, len(sanitized))
for _, call := range sanitized {
    ...
}
msg.ToolCalls = filtered
```

**Severity:** Low-blocking. Correct today, but fragile. A one-line change to
`make(…)` removes the hazard.

### B2 — Recovery transcript commits a second, overlapping fragment on abort

**File:** `runner/runner.go:207-216`

`commitRecoveredTranscript` creates a *new* `TurnFragment`, adds the full
`transcript` (including the original user message and all intermediate
assistant/tool messages), marks it `Complete("")`, and commits it. The
*original* fragment at line 70 is then `Fail(err)`'d at line 190/202 — which
means `Payloads()` on it will return an error, so it's never committed. That
part is fine.

However, the recovery fragment commits with `Complete("")` (empty finish
reason) and no assistant message, no usage. This means:

1. The tree gets nodes for all request messages. The `isZeroMessage` check at
   `turn_fragment.go:61` skips the `AssistantTurnEvent` because there are no
   continuations, no usage, and finish reason is `""`. So the payloads are just
   the raw `MessageEvent`s. This is probably intentional but worth a comment.
2. On the *next* turn, these messages will be in the tree and projected into
   the conversation. The assistant message with tool calls and the tool results
   will appear, but there's no final assistant response. This is the desired
   recovery behavior (the provider will see the partial transcript and can
   continue).
3. The pending tool calls will be resolved (tool calls have matching results),
   so normalization handles this correctly.

**Verdict:** Not actually blocking after analysis, but the empty-finish-reason
`Complete("")` pattern deserves a comment explaining it's intentional recovery,
not a normal turn completion.

### B3 — `completedToolCalls` is mutated inside `sanitizeToolResults` AND in the caller

**File:** `conversation/item.go:209-213` and `conversation/item.go:316-332`

`sanitizeToolResults` adds entries to `completed` (line 330), and then the
caller *also* adds entries to `completedToolCalls` (lines 211-212). This means
every completed tool call ID gets added to `completedToolCalls` **twice** —
once inside `sanitizeToolResults` and once in the loop after it returns. The
second write is a no-op (same value), so it's not a bug, but it's confusing
and suggests the ownership of the `completed` map mutation isn't clear.

**Recommendation:** Either let `sanitizeToolResults` own the mutation (and
remove lines 211-212), or keep `sanitizeToolResults` pure and do the mutation
only in the caller. Pick one owner.

---

## Maintainability Concerns

### M1 — `sanitizeToolResults` now has three responsibilities via side effects

The function filters results, sanitizes content, *and* mutates the `completed`
map. The name suggests it only sanitizes. Consider renaming to
`filterAndTrackToolResults` or splitting the tracking out.

### M2 — Recovery commit path bypasses `threadEventHistory` / prepared-request machinery

`commitRecoveredTranscript` calls `history.CommitFragment` directly, skipping
the `commitPreparedRequest` / `commitFinalFragment` paths that handle thread
events. If the history is a `threadEventHistory`, no thread events are emitted
for the recovery commit. This may be intentional (recovery is best-effort), but
it's an implicit contract that should be documented.

### M3 — `DefaultSpec` now couples `agent` → `capabilities/planner`

This creates a direct import dependency from the `agent` package to
`capabilities/planner`. If the planner capability grows heavy dependencies,
this will pull them into every consumer of `agent`. Currently fine, but worth
noting as an architectural coupling point.

### M4 — Removal of `Messages`/`PendingMessages` from `ProjectionInput` is a breaking API change

Any external code that was setting `ProjectionInput.Messages` or
`ProjectionInput.PendingMessages` will fail to compile. The `runner_test.go`
and `runtime/history.go` callers are updated, but downstream consumers of the
SDK will break. This should be called out in the CHANGELOG.

---

## Untested Paths and Suggested Test Cases

| # | Gap | Suggested Test |
|---|-----|----------------|
| T1 | Recovery commit when `CommitFragment` itself fails | Test that `commitRecoveredTranscript` error is joined with the original error (cancellation + commit failure). |
| T2 | `NormalizeItems` with a tool call that appears in three separate assistant messages (same ID) | Verify only the first occurrence survives and the others are dropped. |
| T3 | `NormalizeItems` with interleaved: call A, result A, call A again (re-use after completion) | Verify the second call A is dropped because it's in `completedToolCalls`. |
| T4 | Empty assistant message after dedup drops all tool calls | The new guard at line 218-220 should drop the entire message. Add a test where an assistant message has only duplicate tool calls. |
| T5 | Recovery transcript commit on cancellation when history is `threadEventHistory` | Verify thread events are (or aren't) emitted — document the expected behavior either way. |
| T6 | `DefaultSpec` with no planner capability registered in the capability registry | Verify graceful error if the planner factory isn't registered (or that it always is). |

---

## Nits and Style

| # | File | Line | Note |
|---|------|------|------|
| N1 | `runner/runner.go` | 72 | `recoveryTranscriptDirty` — consider `transcriptHasToolResults` or `hasUncommittedToolWork` for clarity. The word "dirty" is ambiguous. |
| N2 | `runner/runner.go` | 213 | `fragment.Complete("")` — empty string finish reason. Add a brief comment: `// Recovery-only: no assistant completion.` |
| N3 | `conversation/item.go` | 195 | `filtered := msg.ToolCalls[:0]` — even if safe, a `make` is clearer (see B1). |
| N4 | `agent/default.go` | 22-24 | The system prompt mentions "plan tool" by name. If the tool is ever renamed, this becomes stale. Consider a `planner.ToolName` constant reference in a comment. |
| N5 | `README.md` | Package index table | The `plugin` package row is missing — it exists in the tree but isn't in the table. |
| N6 | `conversation/item_test.go` | 77-120 | Test uses `t.Fatalf` instead of `require.*` like the rest of the file. Mixing styles in the same file. |

---

## Overall Assessment

**Request Changes** (minor)

The core changes are well-motivated and the architecture direction is sound —
collapsing the dual projection path removes a real source of bugs, the dedup
logic is correct, and the recovery transcript is a meaningful improvement for
cancellation/timeout resilience. The tests cover the main happy paths.

Changes to address before merge:

1. **B1** — Use `make` instead of `[:0]` aliasing to avoid a subtle future
   footgun (one-line fix).
2. **B3** — Pick one owner for `completedToolCalls` mutation (either
   `sanitizeToolResults` or the caller, not both).
3. **M4** — Add a CHANGELOG entry noting the `ProjectionInput.Messages` /
   `PendingMessages` removal as a breaking change.
4. **N2** — Add a comment on the recovery `Complete("")` explaining intent.

Everything else is minor polish. The documentation improvements are excellent.
