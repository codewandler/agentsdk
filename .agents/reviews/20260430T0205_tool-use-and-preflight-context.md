# Tool Use & Pre-Flight Context

Lessons from agent sessions on real codebases. These patterns reduce tool calls
by 40-60% on typical investigation tasks.

## Pre-Flight Context (AGENTS.md)

Shape the pre-context to the task domain. Correctness work needs different
context than performance work.

### Always include

- **File inventory with sizes**: `parser_impl.go (4557L, 127KB)` — one line
  per significant file. Agents waste 2-3 calls just discovering file sizes.
- **Recent benchmark baselines**: raw numbers with dates. Agents need to know
  where the pain is before reading code.
- **Hot paths**: name the 3-5 functions that dominate CPU/alloc profiles.
  Without this, agents read entire files linearly looking for patterns.

### Task-specific sections

Performance work needs: hot paths, known pathological inputs, allocation
budgets, benchmark commands with expected output ranges.

Correctness work needs: test classification counts, compliance process,
known gaps sorted by impact.

Architecture work needs: dependency graph, module boundaries, invariants
that must hold across changes.

## Tool Use Discipline

### Batch aggressively

`file_read` accepts multiple ranges in one call. After learning a file is
4557 lines, read it in one call with 8 ranges — not 4 separate calls with
2 ranges each.

```
// Bad: 4 calls
file_read(path, [{start:1, end:500}])
file_read(path, [{start:500, end:1000}])
file_read(path, [{start:1000, end:1500}])
file_read(path, [{start:1500, end:2000}])

// Good: 1 call
file_read(path, [
  {start:1, end:500}, {start:500, end:1000},
  {start:1000, end:1500}, {start:1500, end:2000}
])
```

### Read the metadata you already have

`file_read` returns `[Lines: 4557 total]` in the first response. Use that
to plan subsequent reads. Don't discover file size incrementally.

### Parallelize independent reads

When reading `parser.go`, `event.go`, and `bench_test.go` — none depend on
each other. Read all three in one tool call, not three sequential calls.

### Use grep before file_read for targeted work

For performance work, `grep` for `strings.Builder`, `make(`, `append(`,
`strings.TrimSpace` first. Then `file_read` only the functions that match.
Don't read 4500 lines looking for allocation patterns.

## Missing Tools (Wishlist)

### Symbol tree

A language-aware function/type index with line numbers. Compact format:

```
stream/parser_impl.go (4557L)
  structs: parser:26 savedList:70 pendingBlock:83 fenceState:125 inlineToken:2536
  block:   Write:139 Flush:165 processLine:214 closeParagraph:882
  inline:  tokenizeInline:2547 resolveEmphasis:2711 coalesce:3061
  link:    parseInlineLink:3172 parseRefLink:3273 parseAutolink:4191
  util:    leadingIndent:2471 stripIndent:2025 heading:2120
```

Six lines replaces reading an entire file. Go's `go/parser` AST makes this
trivial. Other languages have equivalent tools (tree-sitter, LSP).

### Profile tool

Runs `go test -bench=X -cpuprofile` and returns top-N hot functions with
cumulative percentages. Answers "where is time spent" without reading code.

### dir_tree with line counts

`dir_tree` should optionally show `(4557L, 127KB)` next to files.
Current `show_size` flag shows bytes only — line count is more useful
for planning read strategies.

## Anti-Patterns

- **Linear reading of large files**: reading a 4500-line file top to bottom
  when you need 3 functions. Use grep or a symbol tree to find targets first.
- **Sequential independent calls**: reading 3 unrelated files in 3 calls
  instead of 1 batched call.
- **Ignoring response metadata**: `file_read` tells you total lines on the
  first read. Use it.
- **Running benchmarks before reading code**: benchmarks tell you what's slow,
  but without code context you can't interpret why. Read the benchmark file
  first to understand what's being measured, then run.
- **Over-reading for the task**: for a performance task, you don't need to
  read test assertions or compliance classification. Scope reads to the
  hot path.
- **`git checkout -- <file>` as a scalpel when it's a sledgehammer**:
  NEVER use `git checkout -- <file>` to revert your own changes when the
  file contains other people's uncommitted work (e.g. benchmark results,
  generated output). This destroys their changes too. Instead, use
  `file_edit` to remove only the lines you added. This is a recurring
  mistake that has cost hours of re-running pipelines.
