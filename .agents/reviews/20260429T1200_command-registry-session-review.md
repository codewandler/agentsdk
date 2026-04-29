# Session Review: Command Registry & Fish History Classification Gaps

Session: 2026-04-29T11:14 – 12:29 CEST (~75 minutes)
Context: `codewandler/cmdrisk` — declarative command registry, 30+ executables, parser fixes
Agent: Claude, ~75 min session, 4 commits, ~120 tool calls

## Summary

User asked to run cmdrisk against fish history, derive adjustments, then
implement everything end-to-end without pausing. Session produced:
- Fish history analysis (56,530 commands)
- Design plans for classification gaps and registry architecture
- Full implementation of declarative command registry (`internal/cmdinfo/`)
- 30+ executable registrations with subcommand-level granularity
- ENV_VAR=value prefix handling, go tool/flag support, git 30 subcommands
- Parser fixes for export/time/comments
- README.md
- Rejection rate: 60.4% → 26.6%

4 commits: `b56be07` (registry), `c4b719f` (parser), `fb5595c` (README),
plus one earlier cleanup.

---

## What Went Well

### 1. Data-driven approach was highly effective

Running the fish history first and analyzing rejection patterns before
writing any code was the right call. The analysis identified exactly which
executables to prioritize (git 5,894, docker 2,843, yarn 2,937, cargo
1,914) and which patterns to fix (ENV prefix ~2,000, go tool 47). Every
implementation decision was backed by frequency data.

### 2. Architecture design before implementation

Writing the registry design plan (003) before coding prevented the
"monkey patching everywhere" problem the user explicitly wanted to avoid.
The `CommandDef`/`SubcommandDef` types, `CustomOps` escape hatch, and
`FallbackBehavior` for script runners were all designed upfront and
implemented cleanly.

### 3. Incremental verification loop

The pattern of: implement → `go test` → fix → `go test` → measure with
fish history was tight. Tests caught regressions immediately (git status
now benign instead of rejected, source string changes). The fish history
re-measurement after each major change gave concrete feedback.

### 4. Scope management

The user said "implement ALL until the end, only get back to me if blocked
or DONE." The session delivered on this — no unnecessary pauses, no
questions, continuous forward progress across registry, semantic layer,
classification, build layer, policy input, decision layer, tests, and
cleanup.

---

## What Went Wrong / Could Be Better

### 1. Initial fish history command failed silently

The first attempt at `./cmdrisk --history=fish` produced empty output
because `--history=fish` reads from stdin, not the fish history file
directly. Took 3 tool calls to diagnose. The CLI help text says
`--history < ~/.bash_history` but the flag name `--history=fish` implies
it might auto-read the file.

**Lesson:** Should have read the CLI help or source first before running.

### 2. JSON output format confusion

Spent several tool calls discovering that `--format json` only applies
with `--explain`, and that batch output uses `writeBrief` (pretty) by
default. Then discovered the JSON output is multi-line (not JSONL), which
broke the initial Python analysis script.

**Lesson:** The output format behavior is non-obvious. The CLI could
benefit from `--format` applying to brief output too, and JSONL for batch.

### 3. Python analysis scripts were verbose

Used Python extensively for analyzing the fish history JSON output (~6
separate Python scripts). Each was 30-60 lines. This worked but was
slow — each script required parsing the full 96MB JSON file.

**Alternative:** Could have written a small Go test or CLI flag that
produces the analysis directly. Or used `jq` for simpler queries.

### 4. `declare -x FOO=bar` panic

The DeclClause handler crashed on `declare -x` because `assign.Name` was
nil for flag arguments. This was caught by a smoke test, not by unit
tests. Should have written parser tests before the smoke test.

**Lesson:** Always test edge cases in the parser layer before integration.

### 5. `git commit` test expectation wrong

Initially wrote `require.Equal(t, ActionAllow, ...)` for `git commit`,
but `persistence_modify` correctly triggers `requires_approval`. Had to
fix the test expectation. This is a design understanding issue, not a
code issue — I should have thought through the policy implications before
writing the test.

### 6. Dead code cleanup was interleaved with feature work

Removed legacy functions (`gitOperations`, `goOperations`, etc.) during
the main implementation rather than in a separate pass. This made the
diff larger and harder to review. Would have been cleaner as a separate
commit.

### 7. Plan tool friction (recurring)

Hit `planner: plan already created` again. Had to add steps to the
existing plan instead of creating a new one. Step IDs from the new batch
didn't conflict but the UX was confusing.

---

## Tool Use Analysis

### Tool call distribution (estimated)

| Tool | Calls | Purpose |
|---|---|---|
| bash | ~45 | Build, test, fish history analysis, smoke tests, Python scripts |
| file_edit | ~20 | Code modifications across 10+ files |
| file_read | ~15 | Understanding existing code, checking test expectations |
| file_write | ~10 | New files (cmdinfo/, plans, README, tests) |
| grep | ~10 | Finding function references, dead code detection |
| git_status/diff | ~5 | Pre-commit verification |
| plan | ~5 | Step status updates |
| glob/dir_tree | ~3 | File discovery |

### Patterns that worked

1. **`file_read` → understand → `file_edit` → `bash go test`** — the core
   loop. Read the code, understand the pattern, edit, verify. Fast and
   reliable.

2. **`bash` with Python for data analysis** — parsing the 96MB fish history
   JSON with Python was effective despite being verbose. The decoder loop
   pattern (`json.JSONDecoder().raw_decode()`) handled multi-line JSON well.

3. **`grep` for dead code detection** — `grep -rn "functionName"` to verify
   no callers before removing functions. Quick and reliable.

4. **`file_write` for new files** — creating `cmdinfo/registry.go`,
   `cmdinfo/commands.go`, `cmdinfo/registry_test.go` as complete files was
   cleaner than incremental edits.

### Patterns that didn't work

1. **`file_edit` with large `old_string` blocks** — several edits required
   matching 20+ line blocks. Fragile when the file had been modified by a
   previous edit in the same session. Had to re-read the file to get the
   exact current content.

2. **Multiple `file_edit` operations in one call** — when editing the same
   file with multiple operations, the operations resolve against the
   original content. This is documented but still trips me up when the
   second operation depends on the first.

3. **`bash` timeout for fish history** — the initial `--explain --format json`
   run over 56K commands took ~1s, well within limits. But I started it as
   a background process unnecessarily, adding complexity.

### Tool ratings for this session

| Tool | Rating | Notes |
|---|---|---|
| file_read | 5/5 | Essential, used constantly, range support excellent |
| grep | 5/5 | Fast, reliable, context_lines useful |
| bash | 4/5 | Core workhorse. Python-for-JSON is clunky but works |
| file_write | 5/5 | Perfect for new files |
| file_edit | 3/5 | Powerful but fragile for large edits. Multi-op conflicts |
| git_status | 4/5 | Good for pre-commit. Missing `git add` native support |
| plan | 2/5 | Single-plan limit, step ID confusion, can't create new plan |

---

## Architecture Observations

### What the registry design got right

1. **Single source of truth works.** Before: 5 files to touch per new
   executable. After: 1 map entry. The `classification/outcome.go` file
   went from 109 lines with hardcoded switches to 73 lines with zero
   command names.

2. **`CustomOps` escape hatch is essential.** 4 of 30+ commands need it
   (go, git, kubectl, and implicitly the legacy `commandPatterns` commands).
   The other 26+ are purely declarative. Good 80/20 split.

3. **`FallbackBehavior` solved the yarn/npm problem elegantly.** Unknown
   subcommands in script-runner tools fall through to dynamic_execution
   instead of rejecting. This alone rescued ~850 commands from rejection.

4. **Metadata-only registry entries** (empty `BaseBehavior`) let legacy
   arg-scanning commands (`rm`, `curl`, `ssh`, etc.) participate in the
   registry for `ExpectsTarget`/`NeedsEndpoint` without changing their
   operation resolution.

### What could be improved

1. **Two resolution paths coexist.** Registry commands use
   `resolveFromRegistry`, legacy commands use `operationKindForExecutable`
   + `commandPatterns`. The fallthrough from registry → legacy (when
   registry returns nil) works but is subtle. Eventually all commands
   should be in the registry with `CustomOps` replacing `commandPatterns`.

2. **`persistence_modify` is too broad.** `git commit`, `git tag`,
   `git branch`, `git config` all trigger `requires_approval` because
   `persistence_modify` is treated the same as `systemctl enable`. Local
   VCS operations should probably have a lower risk than system service
   modifications.

3. **Target extraction is still weak for many commands.** `rm -rf
   node_modules` rejects because `node_modules` doesn't look like a path
   to the target resolver. The `ExpectsTarget` check is too strict for
   relative paths without `./` prefix.

---

## Metrics

| Metric | Start | End | Δ |
|---|---|---|---|
| Allow | 15,886 (28.1%) | 23,491 (41.6%) | +13.5pp |
| Requires approval | 6,511 (11.5%) | 17,992 (31.8%) | +20.3pp |
| Reject | 34,133 (60.4%) | 15,047 (26.6%) | **-33.8pp** |
| Parse errors | 1,136 | 200 | -82% |
| Registered executables | 3 (git, go, kubectl) | 30+ | +27 |
| Tests added | — | ~80 new test cases | — |
| Lines added | — | ~3,400 | — |
| Lines removed | — | ~300 (dead code) | — |

---

## Self-Rating

**4/5** — High throughput session with good data-driven decisions. The
registry architecture is clean and extensible. Main deductions: the
`declare -x` panic should have been caught by unit tests before smoke
testing, and the Python analysis scripts could have been more efficient.
The session delivered exactly what was asked for with no unnecessary
pauses.
