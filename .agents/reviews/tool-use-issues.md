# Tool use issues encountered during skill feature implementation

This document records concrete problems encountered while using the available tools during the implementation of the runtime skill discovery/activation feature in `agentsdk`.

The goal is to distinguish between:
- unclear or misleading tool descriptions
- actual tool bugs
- operator misuse / mismatch between tool behavior and my expectations

## 1. `plan` tool state / behavior issues

### Symptom
Repeated attempts to create or update plans produced confusing responses such as:
- `planner: plan already created`
- `planner: step "inspect-state" not found`

### Observed scenarios
- Creating a new plan after a previous plan existed in the session sometimes failed even when the new plan had a different intended purpose.
- Updating step status failed because the planner response did not appear to preserve or return all created step IDs consistently.
- The first plan response only returned a subset of expected steps, which made later `set_step_status` calls brittle.

### Likely cause
Possibly one or more of:
- the tool allows only one active plan per session but does not make that explicit enough
- the tool response does not reliably reflect the full internal plan state after `create_plan` + `add_step`
- step IDs may not be persisted exactly as sent, or the response shape is not sufficient for reliable follow-up updates

### Impact
- I could not reliably use planner state as the single source of truth for multi-step progress updates.
- I had to continue work without depending on accurate planner mutations.

### Possible improvement
- Make the single-plan constraint explicit in the description if that is the intended behavior.
- Ensure responses always return the full current plan, including all step IDs/statuses.
- If `create_plan` is all-or-nothing, document whether a second call replaces, merges, or fails.

## 2. `multi_tool_use.parallel` misuse risk / ergonomics issue

### Symptom
I used `multi_tool_use.parallel` for duplicate or non-independent calls more than once while exploring the codebase.

Examples:
- running the same `grep` twice in parallel
- mixing calls where one result would have been better inspected before deciding the next call

### Likely cause
This appears to be primarily an operator/ergonomics issue, not necessarily a tool bug.
However, the tool does not guard against obviously duplicate parallel calls.

### Impact
- wasted tool calls
- noisier transcript
- slightly higher cognitive load when reading outputs

### Possible improvement
- optional duplicate-call detection or warning when two identical tool invocations are included in the same parallel batch
- stronger guidance in the tool description about using parallel only for truly independent calls

## 3. `file_edit` exact-match fragility

### Symptom
Several `file_edit` operations failed because the target `old_string` no longer matched exactly after earlier edits or because another operation in the same batch changed the surrounding text.

Examples encountered:
- `old_string not found`
- operation conflicts when insert target fell inside a region already modified by another operation in the same call
- batched edits across multiple files where one file succeeded and another failed due to exact-match drift

### Likely cause
This is mostly expected behavior from an exact-match editing primitive, but it is easy to hit during iterative refactors.
The structured operation model is strict, which is good for safety, but it becomes brittle when:
- editing the same region multiple times in one call
- combining insert/replace operations that depend on each other
- reusing text snippets captured before the file changed

### Impact
- repeated repair loops
- need to re-read files more often between edits
- increased chance of accidental malformed insert locations when trying to patch quickly

### Possible improvement
- better conflict diagnostics, e.g. reporting which earlier operation invalidated the later one
- an option to reference a previously targeted line anchor instead of only exact string blocks
- clearer guidance in the description that multiple operations in one call are resolved against the original file content and may conflict if they overlap semantically

## 4. `file_edit` multi-file call ambiguity

### Symptom
Using `file_edit` with `path` as multiple files and a shared operations list is easy to misuse.

### Observed issue
At one point a batched `file_edit` call attempted to apply conceptually separate edits to multiple files in one request. The failure mode was not always obvious from the initial intent.

### Likely cause
The tool is powerful, but the mental model for multi-file operations is less obvious than for single-file edits.

### Impact
- harder to predict which part failed
- more cumbersome recovery

### Possible improvement
- stronger description/examples for multi-file behavior
- warning when the same operations array is used against multiple files and exact-match replacements are likely file-specific

## 5. `file_write` worked well, but can be deceptively safer than `file_edit`

### Observation
When large file rewrites were needed, `file_write` with `overwrite=true` was more reliable than complex patch/edit sequences.

### Issue type
Not a bug; more of a workflow insight.

### Impact
- For heavily changing files, `file_write` was often the safer option.
- For smaller changes, `file_edit` remained appropriate.

### Possible improvement
- no tool bug here
- perhaps add guidance that `file_write` is often preferable once a file has diverged significantly from the original captured state

## 6. `bash` command-array behavior was good, but failures in later commands can obscure earlier success context

### Symptom
Using `bash` with arrays and `failfast=true` was generally useful, but when the second command failed, the first command’s successful verification could be mentally overshadowed.

Example encountered:
- `go test ./app/...` succeeded
- `go test ./examples/...` failed with `no packages to test`

### Likely cause
Not a bug. This is expected behavior.

### Impact
- can make a verification sequence look more broken than it is
- requires careful reading of the per-command output blocks

### Possible improvement
- no major change needed
- maybe annotate failures in array mode with a brief summary that earlier commands succeeded

## 7. `bash` plus compile/test loops were essential for catching malformed edits quickly

### Observation
Not an issue, but worth noting: the compile/test loop was the most reliable way to validate whether a sequence of tool edits had left files in a syntactically valid state.

### Why record this
This suggests the tool stack works best when:
- read file
- edit small scope
- immediately run focused tests/build
- re-read if needed

rather than doing many speculative edits before verification.

## 8. `grep` output duplication when searching overlapping path sets

### Symptom
Some `grep` outputs included duplicated-looking context blocks or repeated matches where the same lines appeared multiple times.

### Likely cause
Partly operator-driven: repeated identical searches and overlapping path scopes.
Possibly also output formatting that does not collapse repeated adjacent matches clearly.

### Impact
- noisier output
- harder to quickly distinguish unique hits from repeated context display

### Possible improvement
- collapse duplicate adjacent match displays when the same match is emitted multiple times in one result
- or add a `dedupe` option

## 9. `file_read` range handling was good, but large omitted sections can hide insertion context

### Symptom
`file_read` with ranges is very useful, but when the tool elides large sections with omitted markers, it can be easy to miss whether an inserted block landed inside the correct function or just outside it.

### Impact
- I had to re-read narrower, more targeted ranges after some insertions
- this contributed to one malformed insertion episode during the persistence work

### Possible improvement
- no real bug
- perhaps expose a mode that shows a few anchor lines before and after omitted gaps even more aggressively

## 10. `multi_tool_use.parallel` should probably not be used for state-mutating developer tools

### Observation
I only used it for reads here, but the session reinforced that parallel batching is best for read-only independent inspection.
For editing or stateful planning, serialized execution is much safer.

### Possible improvement
- document that best practice more explicitly in the tool description

## 11. Tool descriptions vs actual workflow takeaway

### Main lesson
Most issues were not raw tool breakage. The biggest friction came from:
- strict exact-match editing semantics in `file_edit`
- planner lifecycle/state assumptions not being explicit enough
- ease of overusing `parallel` where serialized steps would be better

### Categorization summary

#### Likely tool-description / UX issues
- `plan` single-plan/session behavior and incomplete returned state
- `file_edit` conflict/error messaging could be more explicit
- `parallel` could warn on duplicate identical calls

#### Likely actual bugs or near-bugs
- `plan` step tracking / returned state may be inconsistent enough to count as a real bug if reproducible

#### Mostly operator/workflow issues
- using parallel for duplicate inspections
- batching too many dependent `file_edit` operations at once
- not re-reading exact target regions before some late edits

## 12. Concrete examples from this implementation

Examples of problematic outputs encountered during the session:
- `planner: plan already created`
- `planner: step "inspect-state" not found`
- `old_string not found`
- `operation[...] conflicts with operation[...]`
- malformed code insertion requiring cleanup after an insert landed in the wrong place due to stale assumptions

## 13. Suggested follow-up actions

1. Reproduce the `plan` tool issues in isolation and verify:
   - one-plan-per-session behavior
   - create/update semantics
   - returned step IDs/state completeness
2. Improve `file_edit` documentation around overlapping operations resolved against original content.
3. Consider adding duplicate-call warnings in `multi_tool_use.parallel`.
4. Add a short internal best-practices note for agents:
   - prefer serialized edits
   - re-read before patching the same region again
   - use `file_write` for large rewrites
   - use `parallel` only for independent read-only inspection
