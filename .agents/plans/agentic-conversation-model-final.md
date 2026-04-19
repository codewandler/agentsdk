# Agentcore Conversation / Session Layer — Design + Implementation Doc

## Status

This document is the implementation-ready design for a new `agentcore/conversation` layer.

It is intended to be handed to another engineer and should contain enough context, rationale, terminology, API direction, persistence model, and implementation order to build the feature safely.

This document is the single source of truth for this design.

---

# 1. Executive summary

`agentcore` should gain a **small, agent-first, append-only event-log tree layer** that sits above `codewandler/llm`.

Its job is to provide:

- a durable conversation/session abstraction
- branching/forking history
- resumable sessions via pluggable storage
- a projection step from history to request messages
- a simple interaction-oriented API

It should **not** expose low-level request-shaping concerns as its main user API.

The main intended user experience is:

- create or resume a session
- add a user message
- run one turn
- receive a result

Conceptually:

```go
sess := conversation.New(
    conversation.WithModel("sonnet"),
    conversation.WithTools(filesystem.Tools()...),
)

_, _ = sess.AddUser("fix the failing tests")
res, err := sess.Run(ctx, runner)
```

Everything involving `llm.Request`, cache behavior, tool definitions, and message linearization should be mostly internal.

---

# 2. Why this layer exists

## Current state in `llm`

`codewandler/llm` is intentionally stateless.

That is good and should remain true.

`llm` provides:

- request primitives
- messages/parts
- tool definitions
- streaming/event processing
- provider-specific capabilities behind a normalized API

But it does **not** provide the higher-level agent concepts we need here:

- a durable session
- resumable history
- branch/fork support
- a simple “add message and run” interaction model

## Why not just expose `llm.Request`

Because that would force users of this layer to think about:

- request assembly
- cache hints
- api hinting
- output format
- tool choice details
- request rebuilding on every turn

That is too low-level for the agent/session layer being designed here.

## Why event-first, not message-first

A plain message history is too narrow.

Today the main history item is a message, but later we may need to persist non-message semantic history items too, for example:

- compaction records
- annotations
- state markers
- replay/control markers
- future sub-agent lineage metadata

So the correct durable base is an **event-log tree**.

`MessageEvent` is the primary event kind in v1, but not the only possible future kind.

---

# 3. Design goals

The new layer should:

1. provide a durable session abstraction
2. support append-only history with branching
3. support resume from persisted storage
4. support projection from history to request-ready messages
5. keep user-facing configuration minimal
6. hide most request-level details
7. stay portable and reusable across runtimes
8. remain smaller and less opinionated than `flai`

---

# 4. Non-goals

The first version should **not** try to implement:

- a full runtime framework
- a built-in context compaction policy engine
- a workflow engine
- a plugin/app framework
- full multi-agent orchestration
- provider-specific UX policies
- a fixed filesystem storage policy

This should be a reusable core, not a full product runtime.

---

# 5. Core concepts and terminology

These terms should be used consistently in code and docs.

## Event-log tree

The durable in-memory history structure.

Properties:

- append-only
- branch-aware
- node-based
- each node stores one payload event
- parent links define lineage

## Payload event

A semantic history item stored in a node.

Examples:

- `MessageEvent`
- future: `CompactionEvent`

In-memory APIs primarily work with payload events.

## Structural storage event

A persisted mutation of the event-log tree.

Examples:

- `node_appended`
- `branch_created`
- `head_moved`

Persistence/replay works primarily with structural storage events.

## Projection

A mapping from a selected path in the event-log tree to a linear `[]Message` sequence suitable for request building.

## Session

A wrapper around:

- event-log tree
- model/tools configuration
- optional llm passthrough options
- metadata
- optionally persistence integration

## Runner

An execution component that performs one turn using a `Session`.

---

# 6. Architectural split

The design has five conceptual pieces.

## 6.1 Event-log tree

Responsible for:

- in-memory durable history
- node append
- branch awareness
- current head selection
- path traversal
- snapshots

## 6.2 Session

Responsible for:

- owning the conversation/event-log tree
- holding model/tools/llm passthrough defaults
- exposing convenience methods like `AddUser`
- providing the primary interaction surface

## 6.3 Projection

Responsible for:

- selecting a path from the event-log tree
- mapping payload events to request messages
- preserving room for future context shaping

## 6.4 Runner

Responsible for:

- building an internal request
- sending it to the LLM runtime/provider
- appending resulting events back into the session
- returning a high-level result

## 6.5 Persistence

Responsible for:

- storing structural storage events
- loading them back
- replaying them into a fresh in-memory event-log tree

---

# 7. Key design rules

## 7.1 Event first, message second

The history model stores payload events, not just messages.

Reason:

- future-proofing
- non-message history items will likely appear later
- cleaner separation between semantic history and persistence mechanics

## 7.2 Event-log tree first, sequence second

The primary model is not a flat message list.

Reason:

- retries branch
- sub-agents will branch
- speculative execution may branch
- branch-awareness is easier to build in now

A flat message list should only exist as a projection result.

## 7.3 Append is the core write primitive

The essential write operation is:

```go
Append(parent NodeID, ev Event) (NodeID, error)
```

Not multiple role-specific append methods.

Those may exist as convenience helpers at the session layer, not as the core interface shape.

## 7.4 Projection is required

A projection step is unavoidable because:

- the durable model is a tree
- the request format is linear
- not all payload events necessarily become request messages

## 7.5 Persistence is append-only

Append-only persistence gives:

- easy replay
- inspectability
- simpler failure handling
- good JSONL compatibility
- natural resumability

## 7.6 Public configuration should stay tiny

At this layer, the main user-facing config should be only:

- model
- tools
- optional extra `llm.Option`s

Everything else should stay internal unless a strong need emerges.

## 7.7 No implicit storage directory

`agentcore/conversation` must not choose a default filesystem location.

If a consumer later wants `~/.miniagent/sessions/`, that policy belongs to the consumer.

---

# 8. Public API design

## 8.1 Identifiers

```go
type SessionID string
type NodeID string
type BranchID string

func NewSessionID() SessionID
func NewNodeID() NodeID
func NewBranchID() BranchID
```

Implementation guidance:

- keep these string-based
- generation method can be simple/random/uuid-like
- no dependency on `flai` ID types

---

## 8.2 Message model

The message model should stay structurally close to `llm/msg`.

```go
type Role string

const (
    RoleSystem    Role = "system"
    RoleUser      Role = "user"
    RoleAssistant Role = "assistant"
    RoleTool      Role = "tool"
)

type Message struct {
    Role  Role
    Parts []Part
    Meta  map[string]any
}

type Part interface {
    PartType() string
}

type TextPart struct {
    Text string
}

type ThinkingPart struct {
    Text      string
    Signature string
}

type ToolCallPart struct {
    ID   string
    Name string
    Args json.RawMessage
}

type ToolResultPart struct {
    CallID   string
    ToolName string
    Content  string
    IsError  bool
}
```

Implementation guidance:

- v1 only needs these four part kinds
- keep JSON compatibility straightforward
- do not add image/audio/file payloads yet
- keep conversion to `llm/msg` simple and explicit

---

## 8.3 Payload event model

```go
type Event interface {
    EventKind() string
}

type MessageEvent struct {
    Message Message
}

func (MessageEvent) EventKind() string { return "message" }

type CompactionEvent struct {
    Summary  Message
    Replaces []NodeID
}

func (CompactionEvent) EventKind() string { return "compaction" }
```

Implementation guidance:

- only `MessageEvent` must be implemented in v1
- `CompactionEvent` is not required in v1
- but code structure should make adding new payload events obvious and safe

---

## 8.4 Event-log tree model

```go
type Node struct {
    ID        NodeID
    ParentID  NodeID
    BranchID  BranchID
    Event     Event
    CreatedAt time.Time
    Meta      map[string]any
}

type Branch struct {
    ID        BranchID
    FromNode  NodeID
    CreatedAt time.Time
    Meta      map[string]any
}
```

Implementation guidance:

- use a synthetic root node internally
- all real history items are appended below some node
- root itself should not need to correspond to a real user-visible payload event
- parent links are the true lineage model
- `BranchID` is metadata, not the primary source of truth

---

## 8.5 Event-log tree interface

```go
type Conversation interface {
    SessionID() SessionID

    Root() NodeID
    Head() NodeID
    SetHead(NodeID) error

    Get(NodeID) (Node, bool)
    Children(NodeID) []NodeID

    Append(parent NodeID, ev Event) (NodeID, error)
    Fork(from NodeID) (BranchID, error)

    Path(to NodeID) []Event
    PathNodes(to NodeID) []Node
    Messages() []Message

    Snapshot() Snapshot
}
```

Semantics:

- `Head()` is the currently selected node in the active line of history
- `Messages()` means projection of the current head path to messages
- `Fork(from)` may initially be lightweight metadata support
- `Append` is the only required write primitive in the core interface

Implementation guidance:

- thread-safety is recommended for the in-memory implementation
- `Get`, `Children`, `Path`, and `Snapshot` should return safe copies where appropriate
- appending under an unknown parent should return an error
- setting head to an unknown node should return an error

---

## 8.6 Snapshot

```go
type Snapshot struct {
    SessionID SessionID
    RootID    NodeID
    HeadID    NodeID
    Nodes     []Node
    Branches  []Branch
}
```

Why it matters:

- tests
- debugging
- replay verification
- UI/runtime inspection

Implementation guidance:

- snapshot should be deterministic enough for tests
- consider stable ordering of nodes/branches in snapshot output

---

## 8.7 Model configuration

```go
type ModelConfig struct {
    Model      string
    Tools      []tool.Tool
    LLMOptions []llm.Option
}
```

This is the preferred v1 public configuration surface.

Why this shape:

- small
- future-proof
- avoids mirroring all of `llm.Request`
- lets advanced callers pass through extra low-level options when needed

Explicitly not part of this layer’s normal public API:

- api type hint knobs
- output format knobs
- cache toggles as the primary config surface
- request assembly details

Those can still exist internally or through llm options, but they should not dominate this layer.

---

## 8.8 Session

```go
type Session interface {
    ID() SessionID
    Conversation() Conversation
    ModelConfig() ModelConfig
    Metadata() map[string]any
}

type AgentSession struct {
    id       SessionID
    conv     Conversation
    model    ModelConfig
    metadata map[string]any
}
```

Implementation guidance:

- session owns model/tools/default llm passthrough options
- conversation owns event-log tree state
- session should be intentionally small

---

## 8.9 Projection

```go
type Projection interface {
    Project(conv Conversation, head NodeID) ([]Message, error)
}

type ActivePathProjection struct{}
```

V1 requirement:

- implement only `ActivePathProjection`

Semantics of `ActivePathProjection`:

- walk the selected path from root to head
- read payload events in order
- include only `MessageEvent` values in the resulting message list
- ignore unknown future event kinds by default or fail explicitly, but behavior must be documented

Implementation guidance:

- v1 behavior should be conservative and explicit
- simplest safe behavior: only project `MessageEvent`; ignore nothing silently unless intentionally documented
- if a non-projectable event kind appears, returning an error is acceptable in v1

---

## 8.10 High-level interaction API

This is the most important user-facing refinement.

### Result

```go
type Result struct {
    Message Message
    NodeID  NodeID
}
```

### Runner

```go
type Runner interface {
    Run(ctx context.Context, sess Session) (Result, error)
}
```

This separation is good because:

- session stores state
- runner performs execution
- different runtimes/providers can implement runner differently

### Session convenience methods

```go
func (s *AgentSession) Add(ev Event) (NodeID, error)
func (s *AgentSession) AddUser(text string) (NodeID, error)
func (s *AgentSession) Run(ctx context.Context, r Runner) (Result, error)
```

Implementation guidance:

- `Add(ev)` should append under the current head by default
- on successful append, new node becomes head
- `AddUser(text)` should be sugar around `Add(User(text))`
- `Run(ctx, r)` should delegate to `r.Run(ctx, s)`

Target UX:

```go
sess := conversation.New(
    conversation.WithModel("sonnet"),
    conversation.WithTools(filesystem.Tools()...),
)

_, _ = sess.AddUser("fix the failing tests")
res, err := sess.Run(ctx, runner)
```

---

## 8.11 Constructors and options

```go
func New(opts ...Option) *AgentSession
func Resume(id SessionID, opener StoreOpener, opts ...Option) (*AgentSession, error)
```

### Options

```go
type Option func(*config)

func WithSessionID(id SessionID) Option
func WithModel(model string) Option
func WithTools(tools ...tool.Tool) Option
func WithLLMOptions(opts ...llm.Option) Option
func WithMetadata(key string, value any) Option
func WithStore(opener StoreOpener) Option
func WithProjection(p Projection) Option
```

Implementation guidance:

- keep options short and obvious
- `WithProjection` is acceptable because projection is a core concept
- `Resume` should load persisted structural events and replay them into a fresh session
- do not make storage path policy part of this package

---

# 9. Internal request-building behavior

This layer will internally need to build `llm.Request`, but that should be treated as a supporting internal concern.

Internal behavior should include:

1. choose the projection
2. project the selected event-log tree path to `[]Message`
3. convert those messages to `llm/msg.Messages` / request messages
4. attach tool definitions automatically
5. apply sensible tool-choice defaults when tools are present
6. enable caching by default
7. apply optional additional `llm.Option`s from `ModelConfig`

Implementation guidance:

- avoid exposing a big `RequestOptions` struct publicly here
- prefer internal helper(s) for request assembly
- if a low-level request-builder helper exists, document it as secondary / integration-focused

---

# 10. Helper constructors

These should exist as convenience helpers.

```go
func System(text string) MessageEvent
func User(text string) MessageEvent
func AssistantText(text string) MessageEvent
func AssistantTurn(text string, thinking []ThinkingPart, calls []ToolCallPart) MessageEvent
func ToolResult(callID, toolName string, result tool.Result, err error) MessageEvent
```

Implementation guidance:

- these should return `MessageEvent`, not bare `Message`
- keep them out of the core interface
- keep them deterministic and easy to test

Suggested behavior:

- `System` -> `MessageEvent{Message{Role: RoleSystem, ...}}`
- `User` -> `MessageEvent{Message{Role: RoleUser, ...}}`
- `AssistantText` -> assistant role with text part
- `AssistantTurn` -> assistant role with text/thinking/tool-call parts
- `ToolResult` -> tool role with tool result part derived from `tool.Result`

---

# 11. Persistence design

## 11.1 Core persistence interfaces

```go
type Store interface {
    Load() ([]StoredEvent, error)
    Append(StoredEvent) error
    Close() error
}

type StoreOpener interface {
    Open(sessionID SessionID) (Store, error)
}
```

V1 guidance:

- this is sufficient
- do not add a more elaborate session manager abstraction yet

## 11.2 No implicit storage directory

This package must not choose a default session directory.

For example:

- `miniagent` may later use `~/.miniagent/sessions/`

But that is a caller decision.

The file store backend must always receive its base path explicitly.

---

# 12. Structural storage events vs payload events

This distinction should be explicit in the implementation.

## 12.1 Structural storage events

These describe mutations of the durable event-log tree.

```go
type StoredEvent struct {
    Version   int       `json:"version,omitempty"`
    Kind      string    `json:"kind"`
    TS        time.Time `json:"ts"`
    SessionID SessionID `json:"session_id,omitempty"`

    NodeID   NodeID   `json:"node_id,omitempty"`
    ParentID NodeID   `json:"parent_id,omitempty"`
    BranchID BranchID `json:"branch_id,omitempty"`

    Payload    *StoredHistoryEvent `json:"payload,omitempty"`
    HeadNodeID NodeID              `json:"head_node_id,omitempty"`
    Meta       map[string]any      `json:"meta,omitempty"`
}
```

V1 structural kinds:

- `node_appended`
- `branch_created`
- `head_moved`

Why `node_appended` and not `message_appended`:

- because the payload is not permanently limited to message payloads

## 12.2 Payload event envelope

These describe the semantic event stored in an appended node.

```go
type StoredHistoryEvent struct {
    Kind       string            `json:"kind"`
    Message    *StoredMessage    `json:"message,omitempty"`
    Compaction *StoredCompaction `json:"compaction,omitempty"`
}

type StoredMessage struct {
    Role  Role           `json:"role"`
    Parts []StoredPart   `json:"parts,omitempty"`
    Meta  map[string]any `json:"meta,omitempty"`
}

type StoredCompaction struct {
    Summary  *StoredMessage `json:"summary,omitempty"`
    Replaces []NodeID       `json:"replaces,omitempty"`
}

type StoredPart struct {
    Type string `json:"type"`

    Text      string          `json:"text,omitempty"`
    Signature string          `json:"signature,omitempty"`
    ID        string          `json:"id,omitempty"`
    Name      string          `json:"name,omitempty"`
    Args      json.RawMessage `json:"args,omitempty"`
    CallID    string          `json:"call_id,omitempty"`
    ToolName  string          `json:"tool_name,omitempty"`
    Content   string          `json:"content,omitempty"`
    IsError   bool            `json:"is_error,omitempty"`
}
```

V1 payload kinds:

- `message`

Reserved/future kinds:

- `compaction`
- `annotation`
- `state_marker`

Implementation guidance:

- only `message` must be supported in v1
- the payload envelope should make future additions obvious

---

# 13. Replay design

```go
func Replay(events []StoredEvent, conv Conversation) error
```

Replay should conceptually do two things:

1. apply the structural storage event
2. if that event is `node_appended`, decode and attach the payload event

Replay requirements:

- preserve node IDs
- preserve parent links
- preserve branch IDs
- preserve active head
- reconstruct semantic payload events, not flattened text only

Implementation guidance:

- keep replay logic deterministic
- validate invariants during replay
- fail clearly on impossible sequences
- do not silently repair malformed logs in v1

Examples of malformed sequences that should error:

- append under unknown parent
- move head to unknown node
- duplicate node IDs
- unknown payload kind when no fallback is defined

---

# 14. JSONL store design

`conversation/jsonlstore` should be the first backend.

## Required behavior

- one session per JSONL file
- one structural storage event per line
- explicit base directory provided by caller
- `Load()` returns `[]StoredEvent`
- replay happens in `conversation`, not in `jsonlstore`

## Recommended path convention

The package itself should not enforce a policy.

A caller may choose a convention like:

```text
<baseDir>/<sessionID>.jsonl
```

or

```text
<baseDir>/sessions/<sessionID>.jsonl
```

That decision belongs to the opener/backend caller.

## Failure behavior

- partial/corrupt line should return a decode/load error
- append failure should propagate to caller
- store should not hide filesystem errors

---

# 15. Package layout

Recommended layout:

```text
conversation/
  doc.go
  ids.go
  message.go
  part.go
  event.go
  node.go
  branch.go
  snapshot.go
  conversation.go
  memory.go
  helpers.go
  projection.go
  projection_default.go
  session.go
  options.go
  persist.go
  replay.go

conversation/jsonlstore/
  doc.go
  store.go
  opener.go
  encode.go
  decode.go
```

## File responsibilities

### `doc.go`
Package overview and terminology.

### `ids.go`
Session/node/branch IDs and generators.

### `message.go`
Roles and `Message` definition.

### `part.go`
Part interface and concrete part types.

### `event.go`
Payload event interface and concrete payload events.

### `node.go`
Node type and helpers.

### `branch.go`
Branch type and helpers.

### `snapshot.go`
Snapshot type.

### `conversation.go`
Core event-log tree interface definitions.

### `memory.go`
In-memory implementation.

### `helpers.go`
Convenience constructors for payload events.

### `projection.go`
Projection interface.

### `projection_default.go`
`ActivePathProjection` implementation.

### `session.go`
Session and `AgentSession`.

### `options.go`
Option types and construction logic.

### `persist.go`
Stored structural events + payload envelopes + store interfaces.

### `replay.go`
Replay logic.

### `jsonlstore/store.go`
Concrete file-backed store.

### `jsonlstore/opener.go`
Path-based opener.

### `jsonlstore/encode.go`
Encoding helpers.

### `jsonlstore/decode.go`
Decoding helpers.

---

# 16. In-memory implementation guidance

A likely internal shape for `MemoryConversation`:

- mutex
- `sessionID`
- `rootID`
- `headID`
- map of `NodeID -> Node`
- map of `NodeID -> []NodeID` for children
- map of `BranchID -> Branch`

Key rules:

- synthetic root exists immediately
- `Add`/`Append` under current head should create a new node and advance head
- path traversal walks parent links back to root and reverses
- branch creation may be metadata-only initially

Need to decide and document whether:

- `Messages()` uses default projection directly
- or session owns projection and `Conversation.Messages()` is only convenience

Recommended v1 behavior:

- `Conversation.Messages()` may use a simple built-in active-path message extraction for convenience
- richer projection selection remains session/runner-level

---

# 17. Runner implementation guidance

The `Runner` interface is intentionally small.

A concrete runner implementation will likely do:

1. select projection
2. build message sequence from session
3. construct internal request
4. call provider/runtime
5. append returned assistant event(s)
6. append tool result event(s) if applicable
7. return final result

Questions intentionally left open for the actual runtime implementation:

- how tools execute during the loop
- whether multiple appended assistant/tool events can happen in one run
- how streaming maps into appended events

This doc does not require one specific loop design.

What matters is the session/result contract.

---

# 18. Testing plan

## 18.1 Core event-log tree tests

- create new tree with synthetic root
- append payload event under root
- append multiple payload events linearly
- branch by appending from an earlier node
- head movement works
- invalid parent errors
- invalid head errors
- path reconstruction works
- snapshot is correct

## 18.2 Message/payload tests

- helper constructors build expected `MessageEvent`
- tool result helper preserves `is_error`
- thinking/tool-call parts serialize as expected

## 18.3 Projection tests

- active path projection yields expected messages
- only `MessageEvent` projects in v1
- non-projectable event kind handling is explicit

## 18.4 Persistence tests

- append structural storage events to JSONL
- load and replay rebuilds identical tree shape
- node IDs preserved
- head preserved
- branch metadata preserved
- malformed logs fail clearly

## 18.5 Session tests

- options produce expected session config
- `AddUser` advances head
- `Run` delegates to runner

## 18.6 Integration tests

- create session -> append user -> run stub runner -> result appended
- persist -> load -> replay -> continue running works

---

# 19. V1 scope

V1 should include:

- identifiers
- message + parts
- payload event model (`Event`, `MessageEvent`)
- node/branch/snapshot
- in-memory event-log tree
- session wrapper
- active-path projection
- minimal `ModelConfig` (`Model`, `Tools`, `LLMOptions`)
- high-level session convenience methods (`Add`, `AddUser`, `Run`)
- structural storage events
- replay
- JSONL store

V1 should not include:

- additional projection strategies
- metadata mutation API beyond basic maps
- advanced branch management
- broad request-option surfaces mirroring `llm.Request`
- built-in compaction logic
- workflow or sub-agent runtime logic

---

# 20. Implementation order

Recommended implementation sequence:

1. `conversation/ids.go`
2. `conversation/message.go`
3. `conversation/part.go`
4. `conversation/event.go`
5. `conversation/node.go`
6. `conversation/branch.go`
7. `conversation/snapshot.go`
8. `conversation/conversation.go`
9. `conversation/memory.go`
10. `conversation/helpers.go`
11. `conversation/projection.go`
12. `conversation/projection_default.go`
13. `conversation/session.go`
14. `conversation/options.go`
15. `conversation/persist.go`
16. `conversation/replay.go`
17. `conversation/jsonlstore/store.go`
18. `conversation/jsonlstore/opener.go`
19. `conversation/jsonlstore/encode.go`
20. `conversation/jsonlstore/decode.go`
21. define first runner binding
22. integrate from `miniagent`

---

# 21. Open questions that should not block v1

These matter, but should not block the core implementation:

- best strategy for stable prefixes across providers
- exact caching defaults across providers/models
- future support for developer-role messages in this layer
- future semantics for compaction payload events
- future sub-agent lineage metadata
- how a specific runtime loop binds to the `Runner` interface

---

# 22. Final recommendation

Implement `agentcore/conversation` now as:

> an append-only event-log tree with session-level `Model + Tools + optional LLMOptions`, projection-based internal request building, a small runner abstraction, and replayable persistence.

And keep the user-facing interaction model centered on:

> add a message, run the session, get a result.

This is the smallest design that:

- improves on raw `llm.Request`
- hides most request-level mechanics from the normal user path
- stays reusable and portable
- supports resumable sessions
- leaves room for sub-agents
- avoids overcommitting to runtime policy too early


# 23. Engineering handoff invariants and failure semantics

This section upgrades the design into a stricter implementation handoff specification.

## 23.1 Core invariants

The implementation should preserve these invariants at all times.

### Event-log tree invariants

1. every node except the synthetic root has exactly one parent
2. the synthetic root has no parent
3. every non-root node stores exactly one payload event
4. `NodeID` values are unique within a session
5. `BranchID` values are unique within a session
6. `Head()` always points to an existing node
7. `Root()` always points to the synthetic root node
8. parent links must never form cycles
9. `Children(parent)` must only return existing node IDs
10. if a node lists `ParentID = x`, then that node must appear in `Children(x)`

### Session invariants

1. every session has exactly one `SessionID`
2. every session owns exactly one event-log tree
3. `ModelConfig.Model` must be non-empty before a real run is attempted
4. tools on the session are treated as session-scoped defaults
5. `LLMOptions` are additive passthrough configuration, not a replacement for the high-level session model

### Replay invariants

1. replayed node IDs must match persisted node IDs exactly
2. replayed parent/child relationships must match persisted structure exactly
3. replayed head must match persisted head exactly
4. replay must not silently invent missing nodes or branches
5. replay must either succeed deterministically or fail clearly

---

## 23.2 Concurrency expectations

The in-memory implementation should be safe for normal concurrent use.

### Required guarantees

- concurrent reads should be safe
- concurrent append/read should be safe
- snapshotting during reads/appends should be safe
- head reads during appends should be safe

### Recommended implementation approach

- use a mutex or RWMutex internally
- return copies for slices/maps exposed from public methods
- do not expose internal maps directly

### Non-goals for v1 concurrency

The implementation does not need to guarantee lock-free behavior or transactional multi-step workflows across separate calls.

For example, this is acceptable in v1:

- `AddUser(...)` is atomic as one call
- `Get(...)` + later `Append(...)` is not guaranteed to remain logically stable if another goroutine changes head in between

That is normal and should be documented.

---

## 23.3 Exact default behaviors

These defaults should be implemented consistently unless a stronger reason emerges during coding.

### `New(...)`

- creates a new session
- creates a fresh event-log tree with a synthetic root
- creates a new session ID unless `WithSessionID(...)` was supplied
- installs `ActivePathProjection` unless another projection was provided
- stores model/tools/llm options from supplied options
- does not require persistence

### `Resume(...)`

- creates a fresh in-memory session/tree
- opens the provided store via the provided opener
- loads structural storage events from the store
- replays them into the fresh tree
- applies non-storage options passed to `Resume(...)`
- fails if load or replay fails
- does not silently ignore malformed persisted history

### `Add(ev)`

- appends under the current head node
- on success, the appended node becomes the new head
- returns the new node ID
- if append persistence is enabled and the persistence append fails, the call must return an error

Implementation note:

For v1, prefer strong consistency of in-memory + persistence result for a single append operation. If persistence append fails, do not report success.

### `AddUser(text)`

- equivalent to `Add(User(text))`
- empty string handling should be explicit; recommended v1 behavior is to reject empty user text with an error

### `Run(ctx, runner)`

- delegates to `runner.Run(ctx, session)`
- does not hide runner errors
- does not mutate state itself except through whatever the runner appends

### `Messages()`

- projects the current head path to messages
- should use the default active-path behavior
- in v1, only `MessageEvent` is included in the output

---

## 23.4 Projection behavior and errors

Projection behavior should be explicit.

### V1 projection rule

`ActivePathProjection` should:

1. traverse the selected path from root to head
2. inspect payload events in order
3. include only `MessageEvent` values in the projected `[]Message`

### Handling unknown/non-projectable payload events

Recommended v1 behavior:

- if a payload event kind is encountered that the projection does not know how to project, return an error

This is safer than silently dropping semantic history.

Only if there is a documented reason should the implementation choose silent skipping, and that behavior should then be tested explicitly.

---

## 23.5 Persistence failure semantics

Persistence behavior must be unsurprising.

### Load failures

- malformed JSON line => `Load()` error
- unknown required storage event shape => `Load()` or replay error
- filesystem read error => `Load()` error

### Append failures

- filesystem write failure => append error returned to caller
- encode failure => append error returned to caller
- caller should be able to decide what to do next

### Close failures

- `Close()` should return underlying close/sync errors
- no silent swallowing of close errors

---

## 23.6 Replay failure semantics

Replay should fail clearly on invalid histories.

Replay should error on:

- duplicate node ID append
- append under unknown parent
- head move to unknown node
- branch creation from unknown node
- structurally invalid event ordering when ordering matters
- unknown payload event kind when no decoder behavior is defined

Replay should not:

- auto-create missing parents
- auto-rewrite malformed branch IDs
- silently skip impossible structural events

---

## 23.7 Suggested validation rules

These validations should be implemented either in constructors/helpers or at append/build time.

### Message validation

Recommended v1 checks:

- role must be non-empty
- parts must be non-empty for real messages
- tool result parts require call ID and content
- tool call parts require ID and name
- thinking part may have empty signature, but not empty text

### Model configuration validation

Recommended v1 checks:

- `Model` required before `Run(...)`
- tools may be empty
- `LLMOptions` may be empty

### Event validation

Recommended v1 checks:

- `MessageEvent` must contain a valid message
- future payload event kinds should define their own validation rules

---

## 23.8 Session persistence wiring expectation

If a session is configured with persistence, append-like operations should emit structural storage events as they occur.

Expected mapping:

- `Add(ev)` -> persist `node_appended`
- `Fork(from)` -> persist `branch_created`
- `SetHead(node)` -> persist `head_moved`

This makes resumed sessions reconstructable without snapshots.

---

## 23.9 Recommended implementation checkpoints

A colleague implementing this should consider these checkpoints in order.

### Checkpoint 1 — in-memory model works

Done when:

- session can be created
- synthetic root exists
- payload events append correctly
- head updates correctly
- path reconstruction works
- messages project from active path

### Checkpoint 2 — persistence roundtrip works

Done when:

- append operations write structural events
- load + replay reconstruct the same event-log tree
- resumed session continues appending correctly

### Checkpoint 3 — runner integration works

Done when:

- stub runner can inspect session messages
- runner can append assistant result
- `Run(...)` returns the assistant result and updated node ID

### Checkpoint 4 — miniagent integration works

Done when:

- `miniagent` can create/resume sessions
- add user input
- execute one turn
- persist/resume across restarts

---

## 23.10 Minimum acceptance criteria

This design should be considered successfully implemented when all of the following are true:

1. a session can be created with model/tools/llm options
2. a user message can be appended through a convenience API
3. the event-log tree supports branching via parent-linked nodes
4. the active path can be projected into request messages
5. a runner can execute one turn against the session
6. structural storage events can be written to JSONL
7. those events can be replayed into a fresh session
8. resumed sessions behave like live sessions
9. no fixed storage directory is assumed by `agentcore`
10. the main user-facing mental model remains: add message, run session, get result


# 24. Task breakdown / ticket checklist

This section turns the design into an implementation work breakdown that can be tracked directly.

---

## 24.1 Suggested epic structure

A practical split is:

- Epic A — core event-log tree model
- Epic B — session + interaction surface
- Epic C — persistence + replay
- Epic D — runner integration surface
- Epic E — test hardening + documentation

These can be done mostly in sequence, but parts of C/E can run in parallel once A is stable.

---

## 24.2 Epic A — core event-log tree model

### Ticket A1 — define identifiers

Deliverables:

- `SessionID`
- `NodeID`
- `BranchID`
- ID generation helpers

Acceptance criteria:

- IDs are string-based
- constructors exist
- tests verify non-empty IDs and uniqueness assumptions in practice

### Ticket A2 — define message + part types

Deliverables:

- `Role`
- `Message`
- `Part`
- `TextPart`
- `ThinkingPart`
- `ToolCallPart`
- `ToolResultPart`

Acceptance criteria:

- JSON representation is stable enough for persistence helpers
- validation behavior is defined and tested
- tool-call and tool-result parts preserve required identifiers

### Ticket A3 — define payload event model

Deliverables:

- `Event`
- `MessageEvent`
- placeholder/future-safe shape for additional payload event kinds

Acceptance criteria:

- `MessageEvent` is fully usable in v1
- event kind discrimination is explicit
- tests verify kind tagging and validation behavior

### Ticket A4 — define node / branch / snapshot types

Deliverables:

- `Node`
- `Branch`
- `Snapshot`

Acceptance criteria:

- structures reflect event-log tree terminology consistently
- snapshot shape is sufficient for tests/debugging

### Ticket A5 — define event-log tree interface

Deliverables:

- `Conversation` interface in its final v1 form

Acceptance criteria:

- append/head/path/snapshot surface matches design doc
- no role-specific append methods in the core interface

### Ticket A6 — implement in-memory event-log tree

Deliverables:

- `MemoryConversation` or equivalent concrete implementation
- synthetic root support
- append/head/path/children/get/snapshot behavior

Acceptance criteria:

- all invariants from section 23.1 hold
- thread-safety guarantees from section 23.2 hold
- append under unknown parent fails
- setting head to unknown node fails
- branching works via parent-linked nodes

### Ticket A7 — add helper constructors

Deliverables:

- `System(...)`
- `User(...)`
- `AssistantText(...)`
- `AssistantTurn(...)`
- `ToolResult(...)`

Acceptance criteria:

- all return `MessageEvent`
- deterministic output
- tested for correct role/part construction

---

## 24.3 Epic B — session + interaction surface

### Ticket B1 — define `ModelConfig`

Deliverables:

- `ModelConfig`
- validation behavior for model presence before run

Acceptance criteria:

- public config surface is only `Model`, `Tools`, `LLMOptions`
- no broad public request-options type exists in this layer

### Ticket B2 — define session type and constructors

Deliverables:

- `Session` interface
- `AgentSession`
- `New(opts ...Option)`
- option config plumbing

Acceptance criteria:

- session owns event-log tree + model config + metadata
- default projection is installed
- no persistence required for plain `New(...)`

### Ticket B3 — define and implement options

Deliverables:

- `WithSessionID`
- `WithModel`
- `WithTools`
- `WithLLMOptions`
- `WithMetadata`
- `WithStore`
- `WithProjection`

Acceptance criteria:

- options combine predictably
- metadata behavior is documented/tested
- projection override works

### Ticket B4 — session convenience methods

Deliverables:

- `Add(ev)`
- `AddUser(text)`
- `Run(ctx, r)`

Acceptance criteria:

- `Add(ev)` appends under current head and advances head
- `AddUser("")` behavior is explicit and tested
- `Run` delegates cleanly to runner

---

## 24.4 Epic C — persistence + replay

### Ticket C1 — define structural storage event model

Deliverables:

- `StoredEvent`
- `StoredHistoryEvent`
- `StoredMessage`
- `StoredCompaction`
- `StoredPart`

Acceptance criteria:

- structural event kinds are `node_appended`, `branch_created`, `head_moved`
- payload envelope supports `message` in v1
- future payload kinds fit without redesign

### Ticket C2 — define store interfaces

Deliverables:

- `Store`
- `StoreOpener`

Acceptance criteria:

- interfaces are minimal and sufficient for JSONL and future backends
- no filesystem policy embedded in the interface design

### Ticket C3 — implement replay

Deliverables:

- `Replay(events []StoredEvent, conv Conversation) error`
- internal helpers as needed

Acceptance criteria:

- replay preserves node IDs, parents, branches, head
- malformed histories fail clearly
- no silent repair of invalid logs

### Ticket C4 — wire persistence into session operations

Deliverables:

- `Add(ev)` emits `node_appended`
- `Fork(from)` emits `branch_created`
- `SetHead(node)` emits `head_moved`

Acceptance criteria:

- persistence wiring follows section 23.8 exactly
- append failure semantics match section 23.5
- strong consistency behavior is documented/tested

### Ticket C5 — implement JSONL store backend

Deliverables:

- file-backed store
- explicit-path opener
- JSON encode/decode helpers

Acceptance criteria:

- no built-in default storage directory
- one structural storage event per line
- load returns decoded `[]StoredEvent`
- close propagates close/sync errors

### Ticket C6 — implement resume flow

Deliverables:

- `Resume(id, opener, opts...)`

Acceptance criteria:

- opens store
- loads events
- replays into fresh session
- applies non-storage options
- fails clearly on load/replay errors

---

## 24.5 Epic D — runner integration surface

### Ticket D1 — define result + runner interfaces

Deliverables:

- `Result`
- `Runner`

Acceptance criteria:

- `Result` contains at least final message + node ID
- `Runner` shape matches design doc

### Ticket D2 — stub runner tests

Deliverables:

- test runner implementation used in unit/integration tests

Acceptance criteria:

- stub runner can inspect session state
- stub runner can append assistant result event
- `Run` behavior is verified end-to-end

### Ticket D3 — internal request-building helper(s)

Deliverables:

- internal conversion from projected messages to llm request input
- automatic tool definition wiring
- default caching enablement
- application of `LLMOptions`

Acceptance criteria:

- request-building stays internal/secondary
- public API does not regress toward broad request configuration
- tests verify tools + llm options are applied

---

## 24.6 Epic E — testing, docs, polish

### Ticket E1 — invariant tests

Acceptance criteria:

- all invariants from section 23.1 are covered by tests where practical

### Ticket E2 — concurrency tests

Acceptance criteria:

- concurrent read/append/snapshot behavior is exercised
- no data races in normal supported usage

### Ticket E3 — persistence roundtrip tests

Acceptance criteria:

- append -> persist -> load -> replay -> continue appending works
- head and branch state survive roundtrip

### Ticket E4 — documentation pass

Deliverables:

- package docs
- comments on core public types
- examples for `New`, `AddUser`, `Run`, `Resume`

Acceptance criteria:

- terminology matches this design doc
- docs emphasize interaction-oriented API and explicit-path persistence

### Ticket E5 — miniagent integration ticket

Acceptance criteria:

- `miniagent` can select its own session storage path policy
- `miniagent` can create/resume sessions through this layer
- `miniagent` can perform one-turn interaction via runner

---

## 24.7 Parallelization guidance

If multiple engineers work on this, recommended split is:

### Engineer 1

- Epic A (core model)
- parts of Epic B (session basics)

### Engineer 2

- Epic C (persistence + replay)
- JSONL backend

### Engineer 3

- Epic D (runner integration surface)
- Epic E docs/tests once base APIs stabilize

Constraint:

- Epic A must stabilize first because all other work depends on the core event-log tree interfaces and types.

---

## 24.8 Merge order guidance

Safest merge order:

1. core identifiers/messages/events/interfaces
2. in-memory event-log tree
3. session/options/helpers
4. persistence model + replay
5. JSONL backend
6. runner surface + request-building helpers
7. docs/examples/integration

This minimizes churn from interface changes.

---

## 24.9 Definition of done for the whole effort

The work is done when:

- the package exposes the final v1 public API described here
- event-log tree invariants hold
- a session can be created and resumed
- persistence is append-only and replayable
- no fixed storage directory is implied
- user-facing API remains interaction-oriented
- a runner can execute one turn and append results
- docs/tests are sufficient for another engineer to safely extend the feature
