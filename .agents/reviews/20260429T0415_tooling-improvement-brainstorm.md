# Tooling Improvement Brainstorm

Session: 2026-04-29T02:14 - 04:15 CEST
Context: Building `competition/` pipeline package for `codewandler/markdown`
Agent: Claude Opus, ~2h session, ~24 plan steps, ~80+ tool calls

## Summary

After completing a large implementation session (competition pipeline with
4 stages, 2 CLIs, 15 Go files, Taskfile integration), the user asked for
an honest retrospective on tool use. This led to a brainstorm about
concrete improvements.

---

## 1. Bash Timeout is the #1 Pain Point

**Problem:** Default 30s timeout, max 300s. Real-world operations that
exceed this:
- `git clone` of 6 repos (~30s)
- `go test -bench` with count=3 (~5 min)
- Chained commands (metadata + compliance + bench)

**Impact:** Agent had to break work into smaller chunks, retry with
longer timeouts, and ultimately hand off to the user for long-running
tasks. ~10 minutes of wall-clock time wasted on timeout retries.

**Suggestion:** Either raise the max timeout for explicitly long
operations, or add a `background: true` mode that starts a process
and lets the agent poll for completion.

---

## 2. Missing Git Operations

**Problem:** Agent has `git_status` and `git_diff` (read-only) but
must use `bash` for all write operations: `git add`, `git commit`,
`git reset`, `git rm --cached`.

**Impact:** A git commit fumble occurred because `git add` for a new
directory didn't work as expected, and the agent couldn't verify
staging state without switching between `bash` and `git_diff`. The
commit went out with only 3 files instead of 37.

**Suggestion:** Extend `git` tool with write operations:
```
git(action="add", paths=["competition/", "Taskfile.yaml"])
git(action="commit", message="...", add=["..."]) // combined add+commit
git(action="reset", mode="soft", ref="HEAD~1")
```

---

## 3. Missing File Operations

**Problem:** No `file_copy`, `file_move`, or `dir_create`. Agent uses
bash for:
```bash
cp -r benchmarks/testdata/github-top10 competition/benchmarks/testdata/
mkdir -p competition/benchmarks/testdata competition/results
rm competition/results/results-*.json
```

**Suggestion:** Add `file_copy(src, dst, recursive)` and
`dir_create(path)`. These are common enough to warrant native tools.

---

## 4. Repetitive Command Patterns (Workflow Shortcuts)

**Problem:** The agent ran `cd competition && go build ./... && go vet ./...`
approximately 15 times in the session. Each time is a separate bash call
with timeout management.

**Suggestion: Session-scoped workflow aliases.**

The agent (or the runtime) can create named shortcuts for repeated
command sequences:

```
workflow(action="create", name="verify", steps=[
    "cd competition && go build ./...",
    "cd competition && go vet ./...",
    "go test ./stream ./terminal ./html . -timeout=60s"
], failfast=true)

// Later:
workflow(action="run", name="verify")
```

Properties:
- Ephemeral to the session (no Taskfile pollution)
- Created by the agent when it notices repetition
- Disappears when the session ends

### Auto-detection variant

Even better: the **runtime detects repetitions** and suggests them:

```xml
<provider key="workflows">
  <suggested name="verify-competition" frequency="12" last="2m ago">
    cd competition && go build ./...
    cd competition && go vet ./...
  </suggested>
</provider>
```

The runtime sees every bash call. After 3+ occurrences of the same
(or fuzzy-similar) command sequence, it surfaces a suggestion via
system context. The agent then just uses it.

Detection can be simple: exact match or common-prefix match on
sequential bash calls. The smart version catches near-misses
(e.g. same commands with/without `2>&1`).

---

## 5. Conversation History Access

**Problem:** The agent cannot efficiently search or retrieve its own
conversation history. When the context window compacts early messages,
they're gone. Even within the window, there's no structured index of
tool calls.

**Impact:**
- Git commit fumble (couldn't quickly list files created)
- Tool rating question (couldn't count bash calls or failure rates)
- Reconstructing decisions from earlier in the session

**Suggestion:** A `conversation` tool for on-demand retrieval:

```
conversation(action="search", query="file_write competition/")
-> returns matching messages with timestamps

conversation(action="fetch", range=[-20, -10])
-> returns messages 10-20 back from current

conversation(action="stats")
-> {messages: 147, tool_calls: 89, context_used: "62%"}
```

Key insight: the agent doesn't need full history in context all the
time. It needs it **searchable on demand**. Store everything, load
selectively.

---

## 6. JSON Data Inspection

**Problem:** Agent frequently shells out to Python for JSON queries:
```bash
python3 -c "import json; d=json.load(open('results.json')); ..."
```

**Suggestion:** A `json_query` tool:
```
json_query(path="results.json", expr=".candidates[].features.parser")
```

Covers the most common ad-hoc data inspection pattern without
requiring bash + Python.

---

## 7. Plan Tool Limitations

**Problem:**
- Cannot create a new plan when one exists (got "plan already created")
- Had to keep adding steps to the original plan (grew to 24 steps)
- No way to archive/close a plan and start fresh
- Step IDs must be unique across the entire plan lifetime

**Suggestion:** Support multiple plans, or plan archival:
```
plan(action="archive", plan_id="competition-scaffold")
plan(action="create_plan", plan={id: "compgen", title: "Build compgen"})
```

---

## 8. file_edit remove vs replace Confusion

**Problem:** The `remove` operation doesn't accept `new_string`, but
`replace` with `new_string: ""` does the same thing. The agent
tripped on this schema distinction.

**Suggestion:** Either:
- Allow `remove` to accept `new_string` (making it a replace)
- Or document more clearly that `remove` is delete-only and
  `replace` with empty `new_string` is the "find and delete" pattern

---

## Tool Ratings (from this session)

| Tool | Rating | Notes |
|------|--------|-------|
| file_read | 5/5 | Perfect, used constantly |
| grep | 5/5 | Fast, context_lines is great |
| file_write | 4/5 | Reliable for new files |
| file_edit | 4/5 | replace is excellent, remove schema confusing |
| git_status/diff | 4/5 | Good for read, missing write ops |
| dir_tree/glob | 4/5 | Good for exploration |
| bash | 3/5 | Essential but timeout is painful |
| plan | 3/5 | Useful but single-plan limit hurts |

Agent self-rating: 4/5 — solid output, ~5 corrections needed,
timeout management was the main time sink.
