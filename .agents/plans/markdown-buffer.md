# Markdown Buffer Plan

## Status

Implemented in `agentcore/markdown`.

Current delivered state:

- `markdown` package cleanup completed
- frontmatter helpers live in coherent `package markdown`
- streaming markdown `Buffer` implemented
- top-level block parsing uses **goldmark**
- callback-based block emission implemented
- concurrency safety added
- callback serialization added
- conservative streaming rules documented
- broad regression test coverage added

This document is now updated from an initial design plan into a **current-state + next-steps plan**.

---

## Goal

Provide a small streaming markdown utility for agent / LLM output:

- callers append partial markdown chunks as they arrive
- the buffer keeps incomplete trailing content internally
- whenever one or more **stable, fully renderable top-level markdown blocks** become available, a callback is invoked
- emitted output is **append-only** and never retracted

Primary target remains later integration into `../miniagent`.

---

## Implemented package layout

Current files:

- `markdown/frontmatter.go`
- `markdown/frontmatter_test.go`
- `markdown/buffer.go`
- `markdown/buffer_test.go`

Cleanup completed:

- old mixed package naming removed
- markdown directory now consistently exposes `package markdown`
- frontmatter file naming normalized

---

## Implemented API

Current public API:

```go
package markdown

type Block struct {
    Markdown string
    Kind     BlockKind
}

type BlockKind string

const (
    BlockParagraph  BlockKind = "paragraph"
    BlockHeading    BlockKind = "heading"
    BlockList       BlockKind = "list"
    BlockCodeFence  BlockKind = "code_fence"
    BlockCode       BlockKind = "code"
    BlockBlockquote BlockKind = "blockquote"
    BlockHTML       BlockKind = "html"
    BlockThematic   BlockKind = "thematic_break"
    BlockOther      BlockKind = "other"
)

type BlockHandler func([]Block)

type BufferOption func(*Buffer)

func WithMarkdown(md goldmark.Markdown) BufferOption
func NewBuffer(handler BlockHandler, opts ...BufferOption) *Buffer
func (b *Buffer) Write(p []byte) (int, error)
func (b *Buffer) WriteString(s string) (int, error)
func (b *Buffer) Flush() error
func (b *Buffer) Pending() string
func (b *Buffer) Reset()
```

### Current contract

Documented and implemented:

- use `NewBuffer`; zero value is **not** ready for use
- output is **append-only**
- callback receives **whole top-level markdown blocks**, never inline fragments
- emitted block markdown preserves the original source slice used by the buffer
- emitted markdown may include trailing blank lines up to the next emitted block boundary
- `Write` is conservative and may keep the trailing block buffered
- `Flush` is end-of-stream, best-effort finalization
- callback delivery is serialized even if `Write` is called concurrently
- buffer state methods are concurrency-safe

---

## Implemented parser approach

### Parser choice

Implemented with **goldmark**.

### Current parsing model

The buffer parses the current pending source with goldmark and inspects the top-level AST blocks.

Current implementation uses:

- goldmark for top-level block structure
- source slicing based on block starts
- conservative streaming heuristics for deciding whether the trailing block is stable

Important current framing:

> This is not a fully incremental markdown parser.
> It is a **goldmark-assisted conservative streaming block buffer**.

That is the correct description of the implemented system.

---

## Current stability model

### Stable enough to emit

Implemented conservative policy:

- a complete heading line can emit
- thematic breaks can emit
- code blocks / fenced code blocks can emit once considered closed
- most paragraph / list / blockquote / html tails require stronger boundary conditions
- blank line after a block usually makes it stable
- `Flush()` emits any remaining parsed tail best-effort

### Explicitly buffered during streaming

- incomplete trailing line without terminating newline
- content inside an open fenced code block
- most trailing container blocks without a closing / separating boundary

### Special fence handling

Implemented helper:

- `firstUnclosedFenceStart(src string) int`

Purpose:

- conservatively prevent emission of anything that starts inside an unclosed fence
- compensate for the fact that an open CommonMark fence can absorb the rest of the document until EOF

This is still the most hand-rolled part of the implementation.

---

## Concurrency behavior

Implemented:

- internal state protected by mutex
- callback delivery protected by separate mutex

Current guarantees:

- concurrent `Write`, `Flush`, `Pending`, and `Reset` are safe
- handler invocations do **not** overlap
- emitted callback batches are delivered serially

---

## Writer semantics

Current behavior:

- `Write` appends bytes into the internal pending buffer first
- if processing then fails, `Write` returns `(len(p), err)`
- invalid internal state returns a proper error instead of panic

This was an intentional hardening step.

---

## Current test coverage

### Core behavior

Covered:

- paragraph needs stable boundary
- heading emits on completed line
- multiple blocks in one write
- flush emits final buffered tail
- whitespace-only flush is preserved
- reset clears pending buffer
- internal error path returns `len(p), err`

### Fence behavior

Covered:

- open fence held until closed
- tilde fence held until closed
- longer closing fence accepted
- fence-like content inside body does not close fence
- indented fence up to three spaces
- unclosed fence emitted on `Flush`
- tab-indented fence / code behavior regression covered

### Container behavior

Covered:

- list needs closing boundary
- nested list needs closing boundary
- list continuation paragraph remains in list block
- paragraph followed by list across chunks
- blockquote needs closing boundary
- nested blockquote needs closing boundary
- blockquote lazy continuation

### Heading / HTML behavior

Covered:

- setext heading behavior
- ATX heading without trailing newline remains pending until flush
- html block needs closing boundary
- incomplete html block flushes at end

### Concurrency

Covered:

- callback delivery is serialized under concurrent writes

---

## What changed from the original design intent

Originally the idea was:

- rely heavily on the parser for block determination
- minimize custom markdown rules

Current reality:

- this is achieved **partially**, not completely
- block discovery is parser-backed
- stability determination still uses conservative custom logic

This is acceptable for now and matches the practical goal:

- safe, append-only streaming behavior for LLM output
- predictable block delivery
- no attempt to implement a fully incremental CommonMark engine yet

---

## Known limitations / honest framing

Current implementation should be described as:

> a conservative streaming markdown block buffer, tuned for realtime LLM output

Not as:

> a complete incremental markdown parser

Remaining limitations:

1. stability rules are still partly heuristic
2. fence detection logic is partially hand-written
3. source slicing is based on block starts and next boundaries, not exact parser-owned end spans
4. custom goldmark instances with extensions may interact with heuristics in surprising ways
5. some deep CommonMark edge cases may still require more tests before broad usage

---

## Recommended next steps

### 1. Miniagent integration

Primary next task:

Integrate the buffer into `../miniagent/agent/agent.go` so assistant text deltas flow through `markdown.Buffer` before display.

Current intended integration shape:

```go
buf := markdown.NewBuffer(func(blocks []markdown.Block) {
    for _, block := range blocks {
        sd.WriteMarkdown(block.Markdown)
    }
})

OnTextDelta(func(chunk string) {
    _, _ = buf.WriteString(chunk)
})

// at end of assistant response / step
_ = buf.Flush()
```

### 2. Miniagent display phase 1

Keep initial display simple:

- `WriteMarkdown` can initially forward to plain text output
- immediate UX win is stable buffering, not rich rendering

### 3. Miniagent display phase 2

Optional later enhancements:

- headings in bold / cyan
- fenced code blocks dimmed / indented
- nicer bullet rendering
- perhaps inline code styling later

### 4. Additional markdown hardening if needed

Only after observing real streamed output:

- more CommonMark edge-case tests
- more HTML block coverage
- more nested container coverage
- revisit fence detection if real failures appear

---

## Recommended integration note for miniagent

When integrating into miniagent, document the separation clearly:

- `agentcore/markdown.Buffer` handles **stabilization**
- `miniagent` handles **rendering**

That separation is still the right architecture.

---

## Final recommendation

Treat the markdown buffer as ready for controlled integration.

Use it as:

- a parser-backed,
- conservative,
- append-only,
- callback-driven markdown streaming utility.

The highest-value next step is no longer more design work — it is **integration into `miniagent`**.
