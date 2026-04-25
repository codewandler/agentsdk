# Agentsdk Conversation Runtime + llmadapter Continuation Plan

Status date: 2026-04-25

## Summary

`agentsdk` should grow an optional conversation/session layer and a small runner layer, but it should not copy the full `flai` `core/` versus `runtime/` directory split yet.

The existing root packages are already the core:

```text
tool/
tools/
markdown/
skill/
interfaces/
```

Add new capabilities beside them:

```text
conversation/   durable session, event-log tree, projection, turn fragments
runner/         iterative model/tool loop over llmadapter unified.Client
```

`llmadapter` should remain stateless. It should not know about the event-log tree, branches, agent sessions, or durable history. It only needs enough request/event surface to:

- accept explicit per-request continuation controls
- accept explicit provider cache controls where supported
- return provider response IDs and usage/cost data
- keep request routing independent from conversation state

## Design Decision

Do not put the tree model in `llmadapter`.

The tree changes how a session projects history into the next request. That is an `agentsdk/conversation` responsibility. For `llmadapter`, each request is still just:

```text
unified.Request -> unified.Client -> unified.Event stream
```

If a branch can use a provider-native continuation handle, `agentsdk/conversation` places that handle into `unified.Request.Extensions`. If not, it sends replayed canonical messages. The provider client either encodes the extension or ignores/warns/errors according to its mapping policy.

## Terms

Use these names consistently.

- `ConversationID`: durable user/application conversation identity. It names the event-log tree.
- `SessionID`: runtime execution identity for one active agent/session. It may map to provider prompt-cache keys, but it is not the whole history.
- `BranchID`: a named or generated branch in the event-log tree.
- `NodeID`: one appended payload event in the tree.
- `Head`: current node selected for the next turn.
- `ProviderContinuation`: provider-scoped handle such as OpenAI Responses `previous_response_id`.

Do not assume one global provider continuation per conversation. Continuation handles are branch-head and provider-endpoint scoped.

## Why Tree State Is Local To agentsdk

The event-log tree provides:

- append-only durable history
- branching/forking
- stable node lineage
- replay projection from any selected head
- persistence through structural events

`llmadapter` cannot safely own this because:

- gateway/router code is intentionally stateless per request
- provider continuation handles are not portable across providers
- fallback routing may change provider/API/model
- a branch can fork from an older provider response ID
- persisted tree semantics belong above provider transport

The tree only affects the projected request:

```text
selected branch head
  -> path to root
  -> payload event projection
  -> unified.Request messages/instructions/tools/extensions
  -> llmadapter unified.Client
```

## agentsdk/conversation Target

Initial package responsibilities:

```text
conversation/
  ids.go              ConversationID, SessionID, BranchID, NodeID
  event.go            payload events: message, compaction, annotation, state marker
  tree.go             append-only in-memory event-log tree
  storage.go          structural storage events and replay interfaces
  session.go          user-facing session wrapper
  request.go          high-level request builder
  projection.go       tree path -> unified.Request inputs
  turn_fragment.go    transactional in-flight turn accumulation
  events.go           agent-facing stream events
```

First public API sketch:

```go
sess := conversation.New(
    conversation.WithConversationID("conv_..."),
    conversation.WithSessionID("sess_..."),
    conversation.WithModel("default"),
    conversation.WithTools(tool.UnifiedToolsFrom(tools)),
)

_ = sess.AddUser("fix the failing tests")
events, err := runner.RunTurn(ctx, sess, client, runner.WithToolExecutor(exec))
```

The package should also support a lower-level direct request:

```go
events, err := sess.Request(ctx, client, conversation.NewRequest().User("...").Build())
```

## Turn Fragment Rule

Reuse the best idea from `agentapis/conversation`: one model response is a transactional `TurnFragment`.

The session must not mutate committed history until:

1. upstream stream has closed
2. no stream error occurred
3. a terminal completion event was observed
4. assistant output/tool calls were finalized
5. commit policy validates the fragment as replay-safe
6. commit callbacks accept the exact append payload

Only then append request inputs, assistant output, reasoning side-channel, and provider continuation metadata atomically.

Rejected/interrupted turns leave committed history untouched.

This is especially important for tool loops. A partial assistant tool call without matching tool results can poison replay for Anthropic/OpenAI-style APIs.

## Tree Commit Model

Payload events should be semantic and provider-independent.

Initial payload event types:

```go
type MessageEvent struct {
    Message unified.Message
}

type AssistantTurnEvent struct {
    Message       unified.Message
    FinishReason  unified.FinishReason
    Usage         unified.Usage
    Continuations []ProviderContinuation
}

type ToolResultEvent struct {
    Result unified.ToolResult
}

type CompactionEvent struct {
    Summary string
    Replaces []NodeID
}
```

Structural storage events should be separate:

```text
conversation_created
branch_created
node_appended
head_moved
branch_deleted_or_archived
```

The in-memory tree can append payloads directly, but persistence should journal structural events.

## Provider Continuation Model

Store provider continuation handles on committed assistant turn nodes.

Shape:

```go
type ProviderContinuation struct {
    ProviderName string
    APIKind      string
    APIFamily    string
    NativeModel  string
    ResponseID   string
    Extensions   unified.Extensions
}
```

Rules:

1. A continuation handle is valid only for the same compatible provider endpoint/API/model scope.
2. If route selection changes, fall back to replay projection.
3. If branch head is an older node, use the continuation handle attached to that node, not the newest handle in the conversation.
4. Do not put continuation handles in global session state.
5. Store provider-specific details as opaque extension values where possible.

## Projection Strategies

`agentsdk/conversation` should choose one of these per request.

### Replay

Always works for stateless providers.

Projection:

```text
root -> selected head path -> []unified.Message
```

No provider continuation extension is set.

### Native Previous Response

For OpenAI Responses-compatible providers that support `previous_response_id`.

Projection:

```text
pending user/tool messages only
+ unified.ExtOpenAIPreviousResponseID = branchHead.ResponseID
```

Only use when the branch head has a matching provider continuation.

Native continuation and replay budgeting must be treated as different request
economics:

- If the provider supports a valid branch-head native continuation such as
  `previous_response_id`, do not apply projection trimming/compaction to the
  committed history before choosing that continuation. Sending only the pending
  user/tool messages plus the provider continuation is already the intended
  projection.
- Token/context budgeting is primarily for replay-only providers or fallback
  paths where the full canonical history would otherwise be resent.
- Trimming replay messages for a native-continuation route usually does not save
  money in the way replay trimming does, because provider-side state/cache is
  still the continuity mechanism. It can instead break provider cache behavior,
  continuity semantics, or branch-head assumptions.
- Prompt-cache usage returned by providers should be used for observability and
  estimator calibration, not as the pre-request budgeting source. Usage arrives
  after projection has already been chosen.

### Provider Session ID

For providers with a session/conversation field, such as OpenRouter `session_id`.

Projection:

```text
normal messages or native continuation
+ provider-specific session extension
```

This is not a replacement for local tree state. It is a provider hint/cache affinity key.

### Prompt Cache Key

For OpenAI Responses and similar APIs.

Projection:

```text
request cache controls derived from SessionID/ConversationID
```

This improves cost/latency but does not make the provider responsible for conversation correctness.

## llmadapter Changes Needed

Keep these changes small and stateless.

### 1. Encode OpenAI Responses continuation extensions

`llmadapter/unified/extensions.go` already defines:

```go
ExtOpenAIPreviousResponseID = "openai.responses.previous_response_id"
ExtOpenAIStore              = "openai.responses.store"
```

But provider request encoding currently does not appear to consume them.

Add wire fields for OpenAI/OpenRouter Responses-compatible request encoding:

```text
previous_response_id
store
```

Then update:

```text
providers/openrouter/responses/
endpoints/openairesponses/ if downstream decode/encode needs round-trip support
future providers/openai/responses/ when added
```

Tests:

- request with `ExtOpenAIPreviousResponseID` encodes `previous_response_id`
- response `CompletedEvent.MessageID` is captured and can be reused in the next request
- replay strategy still works without the extension

### 2. Add canonical cache hint support or explicit cache extensions

`agentapis` had `CacheHint` and Responses extras:

```text
prompt_cache_key
prompt_cache_retention
```

`llmadapter` currently reports cache usage, but request-side cache controls are not modeled.

Minimum viable path:

```go
const (
    ExtOpenAIPromptCacheKey       = "openai.responses.prompt_cache_key"
    ExtOpenAIPromptCacheRetention = "openai.responses.prompt_cache_retention"
)
```

Encode these only in Responses-compatible providers.

Later, if multiple provider families need shared cache semantics, add canonical fields:

```go
type CacheHint struct {
    Enabled bool
    TTL     string
}

type Message struct {
    ...
    CacheHint *CacheHint
}

type Instruction struct {
    ...
    CacheHint *CacheHint
}

type Request struct {
    ...
    CacheHint *CacheHint
}
```

Do not add canonical cache fields until at least two provider mappings need them.

### 3. Preserve response IDs consistently

`unified.CompletedEvent.MessageID` already exists and OpenRouter Responses already sets it from response IDs.

Audit all provider clients:

- Anthropic Messages should set `MessageID` when message ID is available.
- OpenAI Chat should set `MessageID` from completion ID.
- Responses providers must set `MessageID` from response ID.

The conversation layer depends on this field to store provider continuations.

### 4. Add provider endpoint identity on routed clients or returned events

For direct clients, `agentsdk` can be configured with a known provider name/API kind.

For routed clients, the conversation layer needs to know what continuation scope produced a response. Options:

1. Wrap routed clients in `agentsdk` and attach configured endpoint identity.
2. Add a lightweight event processor in `llmadapter` that emits a `RawEvent` or `WarningEvent`-like metadata event.
3. Extend `unified.MessageStartEvent` with provider/API metadata.

Preferred first pass: keep this outside `llmadapter`. `agentsdk/runner` should accept an explicit `ProviderIdentity` option when constructing the client/session. Only add event metadata if routing/fallback makes it unavoidable.

### 5. Do not add tree or session stores to llmadapter

Explicit non-goals for `llmadapter`:

- no branch storage
- no conversation persistence
- no hidden previous-response state
- no session registry in gateway/router
- no mutation of route health based on conversation IDs

## agentsdk/runner Target

`runner` owns the loop that `miniagent` currently implements locally.

Responsibilities:

- build `unified.Request` from session projection
- call `unified.Client.Request`
- stream text/reasoning/tool/usage events to caller
- collect tool calls
- execute tools with `tool.Ctx`
- append tool results
- continue until done or max steps
- commit only complete fragments

Event surface:

```go
type Event interface{}

type TextDeltaEvent struct { Text string }
type ReasoningDeltaEvent struct { Text string }
type ToolCallEvent struct { Call unified.ToolCall }
type ToolResultEvent struct { CallID, Name string; Output string; IsError bool }
type UsageEvent struct { Usage unified.Usage }
type CompletedEvent struct { FinishReason unified.FinishReason }
type ErrorEvent struct { Err error }
```

The runner should be usable by `miniagent` without forcing a full app framework.

## miniagent Migration Path

This is part of the same overall migration, not a follow-up afterthought. `miniagent` is the first serious consumer that proves the new `agentsdk` shape works.

Current `miniagent` dependencies:

```text
agentsdk      tools/markdown
agentapis     conversation/session/request/events
llmproviders  provider discovery/session creation
```

Target dependencies:

```text
agentsdk/tool
agentsdk/tools/*
agentsdk/markdown
agentsdk/conversation
agentsdk/runner
llmadapter/unified
llmadapter/providers/*
```

### Current miniagent seams to replace

Current important files:

```text
miniagent/main.go
miniagent/agent/agent.go
miniagent/agent/options.go
miniagent/agent/toolexec.go
miniagent/agent/activation.go
miniagent/agent/testing.go
miniagent/agent/usage/usage.go
miniagent/agent/display/
miniagent/tests/integration/
miniagent/tests/e2e/
```

Current responsibilities:

- `main.go` builds `llmproviders.Service` and handles model/provider CLI flags.
- `agent/agent.go` owns provider lookup, `agentapis/conversation.Session`, the step loop, event consumption, tool continuation, usage recording, and reset behavior.
- `agent/toolexec.go` executes `agentsdk/tool.Tool` and converts tool definitions into `agentapis/api/unified.Tool`.
- `agent/options.go` exposes inference settings using `agentapis/api/unified` enums.
- `agent/usage` tracks provider/model/token/cost data from `agentapis` usage events.
- `agent/display` renders streamed text/reasoning/tool calls and uses `agentsdk/markdown`.

Target responsibilities:

- `main.go` should build explicit `llmadapter` clients or a small local provider factory.
- `agent/agent.go` should mostly wire workspace, tools, session, runner, display, and usage.
- `agentsdk/conversation` should own history, branches, projection, and committed state.
- `agentsdk/runner` should own the model/tool loop that currently lives in `Agent.RunTurn` and `runStep`.
- `agent/toolexec.go` should shrink or disappear once `runner.ToolExecutor` exists.
- `agent/options.go` should use `llmadapter/unified` inference types, not `agentapis/api/unified`.

### Miniagent migration phases

#### M0: prepare module wiring

- Add `replace github.com/codewandler/llmadapter => ../llmadapter` to `miniagent/go.mod` while `agentsdk` depends on local `llmadapter`.
- Keep `replace github.com/codewandler/agentsdk => ../agentsdk`.
- Do not remove `agentapis` or `llmproviders` yet.
- Verify `go test ./agent ./agent/display ./agent/usage` still compiles before behavioral changes.

#### M1: adopt agentsdk tool conversion

- Replace `convertUnifiedToolDefinition` in `agent/toolexec.go`.
- Use `tool.UnifiedToolsFrom(active)` from `agentsdk/tool`.
- Switch imports from `agentapis/api/unified` to `llmadapter/unified` only where tool definitions are involved.
- Keep the current `agentapis/conversation.Session` for this phase if needed by adding a temporary conversion shim from `llmadapter/unified.Tool` to `agentapis/api/unified.Tool`.

Preferred if possible: do this after `miniagent` moves off `agentapis`, avoiding a temporary shim.

#### M2: introduce agentsdk runner behind the current Agent API

- Add a `runner.Runner` or `runner.RunTurn` call inside `Agent.RunTurn`.
- Keep `miniagent.Agent` public API stable.
- Map `runner.Event` to existing display calls:
  - `TextDeltaEvent` -> `StepDisplay.WriteText`
  - `ReasoningDeltaEvent` -> `StepDisplay.WriteReasoning`
  - `ToolCallEvent` -> `StepDisplay.PrintToolCall`
  - `ToolResultEvent` -> `display.PrintToolResult`
  - `UsageEvent` -> `usage.Tracker.Record`
  - `CompletedEvent` -> step/turn summary
  - `ErrorEvent` -> current diagnostic error path
- Preserve `ErrMaxStepsReached` behavior.
- Preserve cancellation semantics from current tests:
  - cancel during first tool emits canceled results for remaining tool calls
  - follow-up flush prevents invalid tool-use history

#### M3: replace agentapis conversation session

- Replace `agent.session *conversation.Session` from `agentapis/conversation` with `agentsdk/conversation.Session`.
- Replace request building:
  - `conversation.NewRequest().User(...)`
  - `ToolResultWithError(...)`
  - `Tools(...)`
  with `agentsdk/conversation` request/session APIs.
- Preserve `Agent.Reset()`:
  - reset conversation tree or create a new conversation/session depending on desired CLI behavior
  - reset usage tracker
  - generate new `SessionID`
- Preserve system prompt behavior from `BuildSystemPrompt`.

#### M4: replace llmproviders service/provider construction

- Remove `llmproviders.Service` from `Agent`.
- Replace:

```go
service.ProviderFor(model)
provider.CreateSession(...)
```

with a local miniagent provider factory:

```go
type ClientTarget struct {
    Client        unified.Client
    ProviderName  string
    APIKind       string
    APIFamily     string
    NativeModel   string
    Capabilities  runner.Capabilities
}
```

- First implementation can be explicit and env-based:
  - Anthropic Messages using `llmadapter/providers/anthropic/messages`
  - OpenAI Chat using `llmadapter/providers/openai/chatcompletions`
  - OpenRouter Responses or Chat using `llmadapter/providers/openrouter/...`
- Keep model aliases local:
  - `fast`
  - `default`
  - `powerful`
  - existing direct model IDs
- Do not wait for a generic `llmadapter` provider registry unless this becomes painful.

#### M5: migrate inference options

- Replace `agentapis/api/unified` types in `agent/options.go`.
- Map current fields to `llmadapter/unified.Request` fields:
  - `Model` -> `Request.Model`
  - `MaxTokens` -> `Request.MaxOutputTokens`
  - `Temperature` -> `Request.Temperature`
  - `Thinking` -> `Request.Reasoning` or a small miniagent enum until `llmadapter` exposes matching ergonomics
  - `Effort` -> `Request.Reasoning` budget/effort if supported
- Be explicit about lossy mapping. Do not silently pretend every provider supports reasoning/effort.

#### M6: migrate usage and cost

- Replace `agentapis/api/unified.StreamUsage` handling with `llmadapter/unified.Usage`.
- Keep `agent/usage.Tracker` initially; only change its input conversion.
- Map token categories:
  - `input.new`
  - `input.cache_read`
  - `input.cache_write`
  - `output`
  - `output.reasoning`
- Map cost items if present.
- If cost data is absent, keep usage output without costs.

#### M7: remove old dependencies

Remove from `miniagent/go.mod`:

```text
github.com/codewandler/agentapis
github.com/codewandler/llmproviders
github.com/codewandler/llm
```

Also remove local imports from:

```text
main.go
agent/agent.go
agent/options.go
agent/testing.go
tests/integration/*
```

Run:

```sh
env GOCACHE=/tmp/go-cache go mod tidy
env GOCACHE=/tmp/go-cache go test ./...
```

### Miniagent file-level target

Target package shape after migration:

```text
miniagent/agent/
  agent.go          public Agent wiring, reset, params summary
  options.go        CLI/inference options mapped to agentsdk/llmadapter
  provider.go       local llmadapter client factory and model alias resolution
  runner.go         thin adapter from agentsdk/runner events to display/usage
  activation.go     keep unless moved into agentsdk later
  system.go         keep
  repl.go           keep
  usage/            keep initially
  display/          keep
```

`agent/toolexec.go` should either disappear or only contain miniagent-specific `tool.Ctx` construction if that is not moved into `agentsdk/runner`.

### Miniagent compatibility requirements

Keep these behaviors stable:

- CLI flags and default model behavior.
- REPL behavior.
- `--workspace` behavior.
- tool activation defaults.
- cancellation during tool execution.
- max-step handling and `ErrMaxStepsReached`.
- display rendering of text, reasoning, tool calls, tool results, and step usage.
- markdown streaming display via `agentsdk/markdown`.

Known acceptable changes:

- provider/model diagnostic wording may change
- exact cost display may be absent until `llmadapter` cost enrichment is wired
- model alias backing may change during the first provider-factory pass

### Miniagent tests to migrate or add

Update existing tests:

- `agent/agent_test.go`
  - fake `unified.Client` instead of fake `agentapis/conversation.Streamer`
  - cancellation/tool-result tests preserved
- `agent/testing.go`
  - fake `llmadapter/unified.Client`
- `tests/integration/markdown_render_test.go`
  - use runner fake client events
- `tests/integration/cancel_tool_use_test.go`
  - keep as live/provider test if credentials are present

Add tests:

- `RunTurn` commits no history on stream failure.
- `RunTurn` commits no history on missing completed event.
- multi-turn replay preserves previous assistant/tool messages.
- branch/fork behavior if exposed in miniagent.
- provider continuation fallback: if target provider changes, replay is used.

### Miniagent boilerplate extraction map

After another pass over `miniagent`, these are the main pieces that should move into `agentsdk`.

#### B1: turn loop and tool continuation

Current source:

```text
../miniagent/agent/agent.go
  RunTurn
  runStep
  flushToolResultsAfterCancel
```

Move to:

```text
agentsdk/runner
```

Why:

- This is generic agent loop logic, not miniagent product logic.
- It handles the repeat model -> tool -> model loop.
- It knows how to detect missing terminal completion.
- It owns cancellation repair semantics.
- Multiple future consumers will need exactly this behavior.

Target API sketch:

```go
type Runner struct {
    Client   unified.Client
    Session  *conversation.Session
    Tools    ToolProvider
    Executor ToolExecutor
    MaxSteps int
}

func (r *Runner) RunTurn(ctx context.Context, input string) (<-chan Event, error)
```

or:

```go
func RunTurn(ctx context.Context, cfg Config, input string) (<-chan Event, error)
```

`miniagent` should only translate `runner.Event` into terminal output and usage records.

#### B2: tool executor and tool context

Current source:

```text
../miniagent/agent/toolexec.go
  executeTool
  toolResultFromError
  agentcoreToolContext
```

Move to:

```text
agentsdk/runner
```

or, if useful independently:

```text
agentsdk/tool/executor.go
```

Why:

- Executing a `tool.Tool` from a model-emitted `unified.ToolCall` is SDK-level glue.
- Error normalization for cancellation/deadline should be consistent across consumers.
- `tool.Ctx` construction is currently repeated application wiring.

Target API sketch:

```go
type ToolContextConfig struct {
    WorkDir   string
    AgentID   string
    SessionID string
    Extra     map[string]any
}

type ToolExecutor struct {
    Tools   func() []tool.Tool
    Context ToolContextConfig
}

func (e ToolExecutor) Execute(ctx context.Context, call unified.ToolCall) (ToolResult, error)
```

`ToolResult` should include:

```go
type ToolResult struct {
    CallID  string
    Name    string
    Output  string
    IsError bool
}
```

Keep miniagent-specific timeout defaults in `miniagent`; pass deadlines through context.

#### B3: activation manager

Current source:

```text
../miniagent/agent/activation.go
agentsdk/interfaces/toolactivation.go
agentsdk/tools/toolmgmt/
```

Move to:

```text
agentsdk/activation
```

Why:

- `tools/toolmgmt` already assumes an activation-state abstraction.
- The concrete implementation in `miniagent` is generic.
- Registered versus active tool state is a common SDK concern.

Target API sketch:

```go
package activation

type Manager struct {
    // registered tools + active set
}

func New(tools ...tool.Tool) *Manager
func (m *Manager) Register(tools ...tool.Tool) error
func (m *Manager) AllTools() []tool.Tool
func (m *Manager) ActiveTools() []tool.Tool
func (m *Manager) Activate(patterns ...string) []string
func (m *Manager) Deactivate(patterns ...string) []string
```

Migration detail:

- Move or alias `interfaces.ActivationState` into `activation`.
- Keep compatibility type alias in `interfaces` for one release if needed.
- `tools/toolmgmt` should depend on the interface, not concrete manager.

#### B4: standard tool bundle

Current source:

```text
../miniagent/agent/agent.go
  setupTools
```

Move to:

```text
agentsdk/tools/standard
```

Why:

- Every consumer currently has to remember the right default tool set.
- `miniagent` should not hardcode standard bundle assembly.
- It centralizes `web.DefaultSearchProviderFromEnv()`.

Target API sketch:

```go
package standard

type Options struct {
    WebSearchProvider web.SearchProvider
    IncludeToolManagement bool
    IncludeNotify bool
    IncludeTodo bool
}

func Tools(opts Options) []tool.Tool
func DefaultTools() []tool.Tool
```

Suggested default for `miniagent`:

```go
standard.Tools(standard.Options{
    WebSearchProvider: web.DefaultSearchProviderFromEnv(),
    IncludeToolManagement: true,
})
```

Keep product-specific activation defaults in `miniagent` if they diverge.

#### B5: usage accounting primitives

Current source:

```text
../miniagent/agent/usage/usage.go
../miniagent/agent/agent.go
  unifiedToUsageTokens
  mergeUsageRecord
  aggregateTurn
```

Move to:

```text
agentsdk/usage
```

Why:

- The tracker is generic: token items, cost items, dimensions, filters, aggregation, drift.
- `llmadapter/unified.Usage` already has structured token/cost categories.
- Future SDK consumers need usage tracking without importing `miniagent`.

Target API sketch:

```go
package usage

type Record struct {
    Usage      unified.Usage
    Dims       Dims
    IsEstimate bool
    RecordedAt time.Time
}

type Tracker struct { ... }

func FromUnified(u unified.Usage, dims Dims) Record
func Merge(records ...Record) Record
```

Avoid duplicating token enums if possible. Prefer storing `unified.TokenItems` and `unified.CostItems` directly, then add presentation helpers:

```go
func InputTotal(tokens unified.TokenItems) int
func OutputTotal(tokens unified.TokenItems) int
```

Miniagent can keep terminal-specific formatting, or it can later move into `agentsdk/usagefmt`.

#### B6: fake unified client test harness

Current source:

```text
../miniagent/agent/testing.go
../miniagent/agent/agent_test.go
```

Move to:

```text
agentsdk/runnertest
```

or:

```text
agentsdk/runner/runner_test.go internal helpers first
```

Why:

- Fake stream clients are necessary for deterministic runner/conversation tests.
- Current miniagent fakes are tied to `agentapis`; the llmadapter migration needs new fakes anyway.

Target API sketch:

```go
type FakeClient struct {
    Requests []unified.Request
    Streams  [][]unified.Event
    Errs     []error
}

func (f *FakeClient) Request(ctx context.Context, req unified.Request) (<-chan unified.Event, error)
```

Add helpers:

```go
func TextStream(responseID, text string) []unified.Event
func ToolCallStream(responseID string, calls ...unified.ToolCall) []unified.Event
func ErrorStream(err error) []unified.Event
```

#### B7: inference option mapping

Current source:

```text
../miniagent/agent/options.go
```

Mostly keep in `miniagent` for now.

Reason:

- Defaults like `codex/gpt-5.4`, `temperature=0.1`, and `max_tokens=16000` are product choices.
- Reasoning/effort UX is CLI/product-facing.

But add small SDK helpers if needed:

```text
agentsdk/runner
  RequestOptions -> apply to unified.Request
```

Target API sketch:

```go
type RequestOptions struct {
    Model string
    MaxOutputTokens int
    Temperature float64
    Reasoning *unified.ReasoningConfig
}

func (o RequestOptions) Apply(req *unified.Request)
```

#### B8: terminal streaming markdown display

Current source:

```text
../miniagent/agent/display/step.go
../miniagent/agent/display/markdown.go
../miniagent/agent/display/format.go
../miniagent/agent/display/usage.go
```

Keep in `miniagent` initially.

Reason:

- Terminal output style is product UX.
- ANSI colors, glyphs, step headers, and CLI summaries should not be forced on SDK consumers.
- `agentsdk/markdown.Buffer` is already the reusable piece.

Possible later extraction:

```text
agentsdk/terminal/streammarkdown
agentsdk/usagefmt
```

Only do this if a second terminal app reuses it.

#### B9: system prompt builder

Current source:

```text
../miniagent/agent/system.go
```

Keep in `miniagent`.

Reason:

- This is product policy: tool preferences, tone, batching guidance, workspace wording.
- SDK should provide tools and optional guidance strings, not one global coding-agent prompt.

Possible SDK support:

- expose tool guidance aggregation helpers from `agentsdk/tool`
- standard bundle can provide recommended guidance blocks

#### B10: REPL and CLI completion

Current source:

```text
../miniagent/agent/repl.go
../miniagent/main.go
```

Keep in `miniagent`.

Reason:

- Signal behavior, `/new`, shell completion install paths, and cobra command layout are product/app concerns.

Possible SDK support later:

```text
agentsdk/app
```

Only after multiple apps need the same shell/REPL abstraction.

### Extraction priority

Do these first:

1. `agentsdk/runner`: move `RunTurn`/`runStep` behavior.
2. `agentsdk/activation`: move concrete activation manager.
3. `agentsdk/tools/standard`: move standard tool bundle assembly.
4. `agentsdk/usage`: move generic usage tracker and unified usage conversion.
5. `agentsdk/runnertest`: move fake client helpers after runner API settles.

Defer:

- terminal display
- system prompt
- CLI/REPL
- high-level app abstraction
- plugins/modes/hooks

### Migration sequence summary

1. Replace miniagent's local tool schema conversion with `tool.UnifiedToolsFrom`.
2. Add an adapter in miniagent that maps current display/usage code from `runner.Event`.
3. Switch session construction from `agentapis/conversation` to `agentsdk/conversation`.
4. Replace `llmproviders.Service` with explicit `llmadapter` client construction for first supported providers.
5. Keep model aliases/config local to miniagent until `llmadapter` has a stable CLI/provider registry package.
6. Remove `agentapis` and `llmproviders` dependencies.

First backend target:

- OpenRouter Responses or Anthropic Messages, whichever has the best current live tool-loop behavior in `llmadapter`.

## Implementation Order

### Phase 1: llmadapter continuation primitives

- Encode `ExtOpenAIPreviousResponseID`.
- Add/encode prompt cache key/retention extensions for Responses.
- Add tests for request encoding.
- Audit `CompletedEvent.MessageID` coverage.

### Phase 2: agentsdk/conversation in-memory core

- IDs and event-log tree.
- Append/path/head traversal.
- Session with model/tools/system defaults.
- Replay projection into `unified.Request`.
- Tests for branch path projection.

### Phase 3: turn fragments

- Port the `agentapis` fragment concept.
- Validate complete/replay-safe turns.
- Commit request inputs + assistant output atomically.
- Preserve provider continuation handles per committed assistant node.
- Tests for stream failure, cancellation, reasoning-only discard, tool-call commits.

### Phase 4: native continuation projection

Status: completed 2026-04-25.

- Added provider continuation lookup at the selected branch head.
- Added provider-aware request projection through `Session.BuildRequestForProvider`.
- Set `ExtOpenAIPreviousResponseID` for matching Responses-family provider continuations.
- Fall back to replay when provider/API/model does not match or no branch-head continuation exists.
- Added tests for matching continuation projection, Responses API alias matching, provider/model mismatch replay, branch fork using an older response ID, and runner request projection.

### Phase 5: runner

- Implement model/tool loop.
- Tool execution with `tool.Ctx`.
- Event stream for UI consumers.
- Usage propagation.
- Tests using fake `unified.Client`.

### Phase 6: miniagent migration

- M0: add local `llmadapter` module wiring.
- M1: adopt `agentsdk/tool.UnifiedToolsFrom`.
- M2: route current `Agent.RunTurn` through `agentsdk/runner`.
- M3: replace `agentapis/conversation.Session` with `agentsdk/conversation.Session`.
- M4: replace `llmproviders.Service` with explicit `llmadapter` client construction.
- M5: migrate inference options to `llmadapter/unified`.
- M6: migrate usage/cost conversion to `llmadapter/unified.Usage`.
- M7: remove `agentapis`, `llmproviders`, and legacy `llm` dependencies.

## Follow-ups From flai To Reuse

Reuse selected `flai` designs after the first `agentsdk/conversation` + `agentsdk/runner` + `miniagent` migration is working. Do not copy the whole `flai` architecture into `agentsdk`.

### F1: runtime loop policy model

Source:

```text
~/projects/flai/runtime/loop/
~/projects/flai/core/loop/
```

Reuse:

- max-iteration policy
- context-cancellation policy
- stop/continue/error decision shape
- careful handling of incomplete provider streams
- cancellation semantics around tool calls

Adaptation:

- replace `coreloop.Provider.Stream` with `llmadapter/unified.Client.Request`
- replace `coreloop.StreamEvent` with `llmadapter/unified.Event`
- keep policy interfaces small enough for `miniagent`

Do not copy:

- full flai event bus integration
- hook wiring
- runtime-specific token-counting path

### F2: conversation decay and context budgeting

Source:

```text
~/projects/flai/runtime/conversation/decay.go
~/projects/flai/core/conversation/conversation.go
```

Reuse later:

- protected recent token budget
- recency half-life
- old tool-result clearing
- budget allocation model

Adaptation:

- apply decay during `agentsdk/conversation` projection, not during tree storage
- preserve original event-log nodes even when projection emits a compacted/cleared view
- make decay optional and disabled by default in v1
- only apply budget/decay to replay projections and replay fallback paths; do
  not compact committed history when a valid provider-native continuation is
  being used
- use provider usage response data to calibrate future token estimates per
  provider/model/session, but keep a preflight estimator because request usage is
  only known after the call completes

Do not copy initially:

- flai's current flat history interface
- mandatory token-budget policy

### F3: HEAD / MIDDLE / STATE projection concept

Source:

```text
~/projects/flai/runtime/context/
~/projects/flai/core/conversation/conversation.go
```

Reuse:

- conceptual separation:
  - HEAD: stable instructions, skills, tool guidance
  - MIDDLE: conversation tree projection
  - STATE: volatile dynamic runtime state
- projector-style interface

Adaptation:

- implement HEAD/MIDDLE first
- defer STATE until a real consumer needs writable/dynamic state providers
- represent projected output as `llmadapter/unified.Instruction` and `unified.Message`

Do not copy initially:

- state provider framework
- synthetic `state_poll` machinery
- runtime context registry

### F4: tool activation registry

Source:

```text
~/projects/flai/runtime/tool/
../miniagent/agent/activation.go
```

Reuse:

- registered versus active tool distinction
- activation/deactivation by name
- always-active management tools

Adaptation:

- add a small `agentsdk/activation` package only after `miniagent` migration shows the API shape
- keep it independent of conversation/session
- have `tools/toolmgmt` depend on a minimal activation interface

Do not copy:

- mode-specific activation state until modes exist in `agentsdk`

### F5: SDK ergonomics

Source:

```text
~/projects/flai/sdk/
```

Reuse later:

- high-level `Run`/`NewAgent` style API
- functional options
- plugin-like setup ergonomics

Adaptation:

- only after `conversation` and `runner` stabilize
- keep it as a thin convenience layer over explicit lower-level packages

Do not copy now:

- full `App`
- slash-command registry
- app plugin lifecycle
- agent manager/subagent APIs

### F6: hooks and lifecycle events

Source:

```text
~/projects/flai/core/hook/
~/projects/flai/core/event/
~/projects/flai/runtime/agent/hooks.go
```

Reuse later:

- pre-request interception concept
- post-request observation concept
- structured lifecycle event vocabulary

Adaptation:

- start with simple runner event streaming
- add hooks only when a second consumer needs interception/observability

Do not copy now:

- full hook registry
- global app/runtime event bus

### F7: plugins, modes, and subagents

Source:

```text
~/projects/flai/core/plugin/
~/projects/flai/runtime/agent/
~/projects/flai/tools/agents/
```

Status:

- defer.

Reason:

- these are product/runtime features, not needed for making `agentsdk` a mature reusable base
- copying them now would recreate `flai` instead of producing a cleaner SDK

Possible later direction:

- `agentsdk/plugin` with only minimal capability bundles
- `agentsdk/mode` only if tool activation and prompt overlays need a shared representation
- subagents should remain a product-level feature until the runner API is stable

## Test Scenarios

Required unit tests:

- tree append and branch projection
- branch fork and independent heads
- replay request projection
- previous-response projection from matching branch head
- route/provider mismatch falls back to replay
- failed stream does not mutate history
- incomplete stream does not mutate history
- completed text turn commits
- completed tool-call turn commits
- canceled tool execution appends canceled tool results only through runner policy
- reasoning-only turn is rejected by default
- prompt cache key extension is encoded by Responses provider

Required integration tests:

- one-shot text through a fake client
- tool roundtrip through a fake client
- multi-turn memory through a fake client
- live text smoke for one llmadapter provider
- live tool-loop smoke for one llmadapter provider
- optional Responses previous-response live smoke when provider supports it

## Open Questions

1. Should `ConversationID` or `SessionID` ever be sent as `unified.Request.User`?
   - Default answer: no. `User` is an end-user/safety identity, not a conversation state key.

2. Should prompt cache keys use `ConversationID`, `SessionID`, or branch ID?
   - Default answer: use `SessionID` for runtime cache affinity. Add branch suffix only if provider cache pollution appears in tests.

3. Should `ProviderContinuation` store route names from `llmadapter/router`?
   - Default answer: yes when using routed clients. For direct clients, explicit provider identity is enough.

4. Should `agentsdk/conversation` support persistent storage in v1?
   - Default answer: define the storage interface now, implement memory first.

5. Should `llmadapter` expose a provider registry?
   - Default answer: not for this migration. Keep provider construction explicit until miniagent migration clarifies the needed config surface.

## Final Shape

`agentsdk` becomes the reusable agent SDK:

```text
tool + tools          portable actions
markdown + skill      context utilities
conversation          durable branchable history
runner                model/tool execution loop
```

`llmadapter` stays the stateless provider adapter:

```text
unified request/events
provider codecs
gateway/router
usage/pricing/model metadata
```

`miniagent` becomes a thin product shell:

```text
CLI flags
display
workspace defaults
model/provider config
self-improvement commands
```
