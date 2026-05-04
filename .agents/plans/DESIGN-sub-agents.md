# DESIGN - Sub-Agents in the Harness

## Purpose

This design proposes sub-agent support for agentsdk as a harness/session
control-plane feature. A sub-agent is a separately running agent session,
usually spawned by another agent turn, with its own context window, tools,
capabilities, thread state, lifecycle, and event stream.

The design intentionally builds on existing agentsdk primitives:

- `harness.Service` owns process/app/session lifecycle.
- `harness.Session` owns per-session dispatch and session projections.
- `app.App` remains the composition root for agents, tools, actions, commands,
  workflows, plugins, context providers, and resources.
- `agent.Spec` remains the declarative agent blueprint.
- `runtime.Engine` and `runner.RunTurn` remain the low-level turn runtime.
- `thread` remains the durable event log.
- `AgentProjection` remains the way a harness session projects session-owned
  tools/context into an agent-facing surface.

The result should not be a second plugin system, a second workflow engine, or a
second model/tool loop.

## Research Summary

### External Patterns

Sub-agent implementations converge on the same themes:

- **Context isolation:** sub-agents get a separate context window so focused work
  does not pollute the parent thread. Claude Code documents subagents as using
  separate context windows plus independently configured prompts and tool sets.
  Source: https://docs.claude.com/en/docs/claude-code/subagents
- **Explicit specialization:** sub-agents are usually role or task specific.
  Claude Code stores subagent definitions as Markdown files with YAML
  frontmatter; Google ADK describes multi-agent systems as hierarchies of
  specialized agents coordinated by a parent. Sources:
  https://docs.claude.com/en/docs/claude-code/subagents,
  https://google.github.io/adk-docs/agents/multi-agents/
- **Controlled handoff/delegation:** OpenAI Agents SDK handoffs are explicit
  operations with input filters and tool-like routing semantics, rather than
  implicit model text conventions. Source:
  https://openai.github.io/openai-agents-js/guides/handoffs/
- **Orchestration as a separate concern:** LangChain/LangGraph distinguishes
  subagents, handoffs, skills, routers, and custom workflows, with context
  engineering as the central design problem. Source:
  https://docs.langchain.com/oss/python/langchain/multi-agent/index
- **Lifecycle and observability:** Microsoft Agent Framework separates agents,
  orchestration, workflows, memory, and observability so multi-agent systems can
  be inspected and controlled. Source:
  https://learn.microsoft.com/en-us/agent-framework/overview/

Important implication for agentsdk: prompting alone is not enough. The harness
must provide lifecycle, limits, state isolation, result contracts, and
observability so delegation is reliable instead of just suggestive.

### Codex CLI Lessons

Codex has a mature sub-agent implementation worth mirroring selectively.
Relevant source paths in `/tmp/codex/codex-rs`:

- `core/src/tools/handlers/multi_agents.rs` describes the collaboration tool
  surface (`spawn_agent`, `send_input`, `wait_agent`, `close_agent`,
  `resume_agent`) and states that spawned agents inherit the live turn config.
- `core/src/agent/control.rs` owns `AgentControl`, the shared root-scoped
  control plane for spawning, messaging, status, closing, and spawn-tree
  traversal.
- `core/src/agent/registry.rs` enforces max-thread limits, tracks active
  spawned agents, reserves paths/nicknames, and releases slots.
- `core/src/agent/mailbox.rs` provides ordered inter-agent communication with
  pending-message detection.
- `core/src/tools/handlers/multi_agents/wait.rs` waits for final child status
  with clamped timeouts.
- `core/src/context/subagent_notification.rs` injects model-visible child
  completion notifications back into the parent context.
- `core/src/agent/role.rs` layers role-specific configuration on top of the
  parent effective config while preserving runtime selections.

Key Codex design choices to reuse:

- Sub-agents are separate threads, not hidden continuations inside one thread.
- A root-scoped control object is shared by the parent and all descendants.
- Spawned agents inherit runtime policy such as cwd, sandbox/approval, provider,
  model defaults, and environment selection unless explicitly overridden.
- Role/model overrides are constrained and explicit.
- Spawn depth and total spawned thread count are limited.
- Parent-child spawn edges are persisted and can be resumed.
- Wait returns when any requested child reaches a final status or times out; it
  does not busy-poll.
- Child completion is model-visible to the parent through a structured context
  notification, not only UI telemetry.
- Sub-agent tools emit typed collaboration events for UI/client rendering.

Codex choices to avoid or adapt:

- Do not make Codex-specific `ThreadManagerState` assumptions. agentsdk should
  use `harness.Service`, `harness.Session`, and `thread.Store`.
- Do not create a parallel role file format if existing `.agents/agents/*.md`
  and `agent.Spec` can express the specialization.
- Do not make sub-agent tooling a global default. It should be a named
  harness/session projection or app/plugin-selected surface.

## Goals

- Allow a parent agent to spawn, message, wait for, resume, and close
  sub-agents through opt-in model tools.
- Keep every sub-agent as a normal `harness.Session` backed by a normal
  `agent.Instance` and `runtime.Engine`.
- Preserve context isolation while allowing explicit context forking when the
  parent asks for it.
- Persist parent-child relationships, lifecycle events, and enough metadata to
  resume a sub-agent tree.
- Expose typed session events for terminal/TUI/API clients.
- Surface child status/results back to the parent through context providers.
- Enforce depth, count, tool, model, approval, workspace, and timeout policy.
- Align with existing app/resource/plugin/harness/action/workflow boundaries.

## Non-Goals

- Durable distributed workers. Sub-agent execution can start as in-process work
  owned by `harness.Service`, matching current async workflow trade-offs.
- Autonomous delegation without model-visible tools. Delegation should be an
  explicit tool call or host API operation.
- General workflow replacement. Workflows remain deterministic DAG/action
  orchestration; sub-agents are model-driven concurrent workers.
- A new `.agents/subagents` resource layout in v1. Existing agent resources are
  sufficient unless compatibility research later proves otherwise.
- Cross-process remote sub-agent routing in v1. The event/state model should not
  prevent it later.

## Design Principles

1. **Harness owns lifecycle.** Spawning another agent is a session lifecycle
   operation, so it belongs above `agent.Instance` and `runtime.Engine`.
2. **App owns composition.** Available sub-agent roles are `agent.Spec` values
   registered in `app.App`; plugins and resource bundles contribute them the
   same way they contribute other agents.
3. **Thread events own durability.** Spawn edges, lifecycle, messages, and
   status changes are durable thread events, not a separate database.
4. **Session projection owns model-facing tools.** `spawn_agent` and friends are
   projected into the parent agent through `harness.AgentProjection`, just like
   `session_command`.
5. **Sub-agents are normal sessions.** A sub-agent has an agent name, session ID,
   thread ID, context providers, tool activation state, skills, capabilities,
   usage, and workflow visibility like any other session.
6. **Context sharing is explicit.** Default spawn starts from the child agent's
   instructions plus task input and ambient environment context. Forked context
   is a deliberate option with configurable history scope.
7. **Delegation is bounded.** Max depth, max children, max active sub-agents,
   wait timeout, and role/tool restrictions are required runtime policy.
8. **Results are structured.** Tool outputs should return agent IDs, canonical
   names, status, timeout flags, and summaries rather than prose-only text.

## Concept Model

```text
harness.Service
  owns app.App and root-scoped SubAgentControl

harness.Session
  owns one agent.Instance
  may have parent/child metadata
  projects sub-agent tools/context when enabled

SubAgentControl
  registry of live sub-agent sessions under a root session tree
  spawn/message/wait/resume/close operations
  spawn-edge persistence and replay

SubAgentRecord
  session name/id
  agent name
  parent session/thread
  thread id / branch id
  depth
  role/spec metadata
  lifecycle status
  last task summary
```

Sub-agent hierarchy is a session tree, not a workflow DAG. A workflow may call an
action that spawns a sub-agent, but the spawned agent's turns remain model-driven
session execution.

## Public API Sketch

### Harness Types

```go
type SubAgentControl struct {
    // root-scoped registry and policy
}

type SubAgentPolicy struct {
    Enabled          bool
    MaxDepth         int
    MaxActive        int
    MaxTotal         int
    DefaultForkMode  ForkMode
    MaxForkTurns     int
    WaitMinTimeout   time.Duration
    WaitMaxTimeout   time.Duration
    AllowedAgents    []string
    AllowedTools     []string
}

type SubAgentSpawnRequest struct {
    ParentSession string
    Name          string
    AgentName     string
    Task          string
    Items         []InputItem
    Fork          ForkOptions
    Model         string
    Effort        string
    ToolPolicy    ToolPolicyOverride
}

type SubAgentHandle struct {
    SessionName string
    SessionID   string
    AgentName   string
    ThreadID    string
    Depth       int
    Status      SubAgentStatus
    Nickname    string
}
```

The initial implementation can keep these internal until the shape is proven by
the tool projection and tests.

### Service Methods

```go
func (s *Service) SpawnSubAgent(ctx context.Context, req SubAgentSpawnRequest) (SubAgentHandle, error)
func (s *Service) SendSubAgentInput(ctx context.Context, target string, input SubAgentInput) error
func (s *Service) WaitSubAgents(ctx context.Context, targets []string, timeout time.Duration) (WaitSubAgentsResult, error)
func (s *Service) CloseSubAgent(ctx context.Context, target string) (SubAgentStatus, error)
func (s *Service) ResumeSubAgentTree(ctx context.Context, root SessionOpenRequest) error
```

These methods should delegate to the same session open/resume/send/close
mechanics used by normal sessions.

### Model-Facing Tools

Add a session projection:

```go
func (s *Session) SubAgentProjection(policy SubAgentPolicy) AgentProjection
```

Projected tools:

- `spawn_agent`: start a child session for a bounded task.
- `send_input`: send additional input to an existing child.
- `wait_agent`: wait for one or more child status changes.
- `close_agent`: close one child and any descendants.
- `resume_agent`: resume a previously persisted child, if supported.
- `list_agents`: optional v1.1 tool for discoverability.

Tool descriptions must include behavioral guidance, but enforcement belongs in
the control plane. Descriptions should explicitly discourage spawning for the
current critical path unless useful parallel work exists.

### Agent Specs as Roles

Sub-agent roles should reuse `agent.Spec`:

```yaml
---
name: explorer
description: Focused codebase exploration agent
tools: [bash, grep, glob, file_read]
max-steps: 30
---
Answer one specific codebase question. Do not edit files.
```

This avoids a second role format and aligns with `docs/17_RESOURCE_APP_MANIFESTS.md`.
If compatibility with Claude Code subagent files is added later, it should be a
resource loader that converts those files into `agent.Spec`.

## Execution Semantics

### Spawn

1. Validate parent session is open and sub-agent policy is enabled.
2. Resolve `AgentName` from `app.App` registered agent specs.
3. Reserve a child slot and canonical name before opening the session.
4. Build child `agent.Option`s from parent runtime policy:
   - workspace/cwd;
   - session store directory;
   - provider/model policy defaults;
   - approval and tool middleware context;
   - tool timeout;
   - skill discovery scope;
   - thread store.
5. Apply explicit spawn overrides only when allowed by policy.
6. Open a child `harness.Session` through `Service.OpenSession`.
7. Record a durable spawn event and parent-child edge.
8. Attach sub-agent projection to the child if recursive delegation is allowed.
9. Dispatch the initial child input.
10. Return a `SubAgentHandle` immediately; the parent can continue or call
    `wait_agent`.

### Forked Context

Forking parent context is optional:

- `none`: child gets only task input plus normal context providers.
- `summary`: child gets a parent-provided or system-generated summary.
- `last_n_turns`: child gets a bounded recent branch projection.
- `full`: only allowed by explicit policy because it can leak context and waste
  tokens.

Forking must preserve provider-history immutability. Do not copy transient tool
call fragments or partial assistant commentary into the child. Prefer
conversation/tree projection helpers rather than ad hoc transcript slicing.

### Messaging

`send_input` should append normal user input to the child session and optionally
interrupt an active turn. For a future v2, distinguish:

- queue-only message;
- follow-up task that starts/wakes a turn;
- interrupt-and-replace task.

Inter-agent messages should be structured events and model-visible context
fragments. They should not only be hidden harness state.

### Waiting

`wait_agent` should:

- clamp timeout to configured min/max;
- subscribe to status/event changes instead of polling;
- return once any target reaches a terminal status or the timeout expires;
- include current status for every target it knows about;
- never block the whole service event loop;
- record begin/end events for clients.

Terminal statuses:

- `completed`;
- `failed`;
- `canceled`;
- `closed`;
- `not_found`.

Non-terminal statuses:

- `pending`;
- `running`;
- `interrupted`.

### Closing

Closing a sub-agent should:

- mark the spawn edge closed;
- cancel/close the child session;
- recursively close descendants by default;
- flush thread state before removing live registry entries;
- return the previous/final status.

## Persistence and Events

Add durable thread events under the parent and child threads:

```text
subagent.spawned
subagent.input_sent
subagent.status_changed
subagent.completed
subagent.failed
subagent.closed
subagent.edge_closed
```

Event payloads should carry:

- root session ID;
- parent session ID and thread ID;
- child session ID and thread ID;
- child agent name;
- child display name/nickname;
- spawn call/tool call ID when model initiated;
- depth;
- fork mode;
- status;
- last task summary;
- error string;
- timestamps.

Thread-backed sessions should be able to reconstruct:

- open child edges;
- closed child edges;
- current child status;
- child display names;
- last task summary;
- descendant tree for resume/close.

Live `SessionEvent` should gain sub-agent variants or move to the structured
output envelope described in `docs/08_OUTPUT_EVENT_MODEL.md`:

```text
subagent.spawn.begin
subagent.spawn.end
subagent.interaction.begin
subagent.interaction.end
subagent.wait.begin
subagent.wait.end
subagent.close.begin
subagent.close.end
```

## Context Projection

The parent needs a compact, model-visible inventory of active children:

```text
subagents:
- worker_a: running - "update parser tests"
- explorer_b: completed - "found config loader path"
```

Add a session context provider backed by `SubAgentControl`:

- `harness/subagent_inventory_context.go`
- role: user or developer depending on final context authority policy;
- stable key such as `harness/subagents`;
- fingerprint includes child IDs, statuses, and last task summaries.

Completion notifications should be injected into the parent history or provided
as context fragments so the parent notices child completion even if it did not
wait. Codex uses a `<subagent_notification>` contextual user fragment; agentsdk
can use the existing context provider system first and add persisted message
injection only if model behavior requires it.

## Safety and Policy

Sub-agent policy must be explicit and host-owned.

Required controls:

- enable/disable sub-agent projection per app/session/agent;
- max depth;
- max active children;
- max total children per root session;
- allowed child agent specs;
- allowed model overrides;
- allowed tool overrides;
- allowed fork modes;
- max fork turns;
- wait timeout clamp;
- child workspace restrictions;
- child approval policy inheritance;
- close-on-parent-close behavior.

Security defaults:

- inherit or narrow approval policy; never silently broaden it;
- inherit or narrow workspace/sandbox; never silently expand writable roots;
- inherit risk/approval middleware from the parent app/session;
- do not expose secrets through forked context;
- redact sub-agent tool outputs before parent-visible summaries;
- require explicit opt-in before child agents can write to shared files.

For coding agents, parallel write ownership is a policy problem, not just a
prompting problem. If two child agents can edit files, the spawn request should
include an ownership scope and the tool middleware should be able to enforce or
warn on overlapping paths.

## Relationship to Workflows

Sub-agents and workflows should remain distinct:

- Use workflows for deterministic DAG/action execution with retries, timeouts,
  schemas, and replayable step state.
- Use sub-agents for model-driven exploration, synthesis, review, and parallel
  work where the execution path is not known ahead of time.

Possible adapters:

- `harness.SpawnSubAgentAction`: workflow step that starts a child and returns a
  handle.
- `harness.WaitSubAgentAction`: workflow step that waits for children.
- `harness.SubAgentResultAction`: workflow step that reads a completed child
  summary.

These should be actions over harness APIs, not special workflow engine behavior.

## Relationship to Plugins and Resources

Do not add `harness.Plugin`. Keep the invariant from
`docs/02_ARCHITECTURE.md`: packaging/contribution remains unified through
`app.Plugin`, resource bundles, and session projections.

Resource guidance:

- sub-agent roles are agent specs in `.agents/agents/*.md`;
- app manifests select named plugins and default agents as today;
- a future manifest field may enable sub-agent projection/policy, but avoid a
  new resource layout unless needed;
- if a new resource format is added, update `docs/RESOURCES.md` with the
  external compatibility source.

Possible future manifest shape:

```json
{
  "subagents": {
    "enabled": true,
    "allowed_agents": ["explorer", "worker"],
    "max_depth": 2,
    "max_active": 4,
    "fork_modes": ["none", "last_n_turns"]
  }
}
```

## Relationship to Current Stop Behavior

Sub-agents can reduce premature yielding only if the root loop has the right
contract. For batch tasks, parent agents should use planner/context state and
sub-agent status before ending the turn. The control plane should make child
status visible, and the prompt should state that spawning is not completion.

Recommended model guidance:

```text
If you spawn sub-agents, do not end the turn merely because they were spawned.
Continue independent local work. Wait only when their result is needed. End the
turn only when the full user task is complete, all required child results are
integrated, or you are blocked.
```

This should live in a high-authority harness/session instruction when sub-agent
tools are enabled.

## Implementation Plan

### Phase 1 - Internal Control Plane

- Add `harness.SubAgentControl` with an in-memory registry scoped to
  `harness.Service`.
- Add policy struct with conservative defaults: disabled, depth 1, max active 2.
- Add internal spawn/send/wait/close methods.
- Represent children as normal `harness.Session` values.
- Close child sessions when parent/root service closes.
- Unit test slot reservation, depth limits, close cascade, and status
  transitions.

### Phase 2 - Session Projection Tools

- Add `Session.SubAgentProjection(policy)`.
- Implement `spawn_agent`, `send_input`, `wait_agent`, and `close_agent` as
  model-facing tools.
- Return structured JSON tool results.
- Add context provider for active child inventory.
- Attach projection only when enabled by host/app/session policy.
- Test model-visible schemas, disabled policy, and tool result rendering.

### Phase 3 - Thread Events and Resume

- Define thread event payloads for spawn edges and lifecycle status.
- Persist parent-child edges to the parent live thread.
- Persist child metadata to the child live thread.
- Rebuild open child edges when resuming a persisted root session.
- Add `resume_agent` only after edge replay works.
- Test JSONL-backed resume, close-edge persistence, and descendant traversal.

### Phase 4 - Forked Context

- Add `ForkOptions` with `none`, `summary`, `last_n_turns`, and `full`.
- Start with `none` and `summary`; add `last_n_turns` after projection tests.
- Keep full-history fork disabled by default.
- Ensure context render records remain replayable.
- Test that tool-call fragments, transient commentary, and secrets are not
  copied into forked child input.

### Phase 5 - Events and UI/API Integration

- Add sub-agent live events following `docs/08_OUTPUT_EVENT_MODEL.md`.
- Render terminal summaries for spawn/wait/close without leaking full child
  output.
- Add API/SSE-friendly payload structs once a non-terminal channel consumes
  them.
- Add usage aggregation by root session and child session.

### Phase 6 - Actions and Workflow Adapters

- Add action wrappers for spawn/wait/read-result if workflow use cases need
  them.
- Keep action wrappers thin over harness APIs.
- Avoid workflow-engine-specific sub-agent hooks until real orchestration needs
  exceed actions.

## Testing Strategy

Required coverage:

- spawn policy disabled/enabled;
- unknown child agent spec;
- max depth and max active enforcement;
- parent close closes descendants;
- wait timeout clamps and returns without busy polling;
- child completion appears in parent context;
- tool result JSON is stable;
- thread-backed spawn edge replay;
- resumed root sees open descendants;
- closed edge is not resumed as open;
- fork modes do not leak transient tool calls;
- child inherits or narrows workspace/tool/approval policy;
- multiple children with overlapping write scopes produce a policy warning or
  denial once write scopes exist.

Dogfood tests:

- parent spawns an `explorer` for a read-only code question and continues local
  work;
- parent spawns two explorers, waits for either result, then integrates both;
- parent spawns a worker with file ownership scope and closes it after result;
- persisted session resumes with an open child and can close the child tree.

## Open Questions

- Should child sessions share the parent `agent.Instance` skill activation state
  or start from the child spec plus app defaults? Default should be isolated
  state, with explicit inheritance later.
- Should child completion be injected as durable conversation content or only
  rendered by a context provider? Start with context provider; add injection if
  model behavior shows missed notifications.
- Should sub-agent policy live in `agentsdk.app.json`, agent frontmatter, CLI
  flags, or all three? Start with host/session Go options and CLI flags for
  dogfood; add manifest support after the API stabilizes.
- How should child file ownership be represented before agentsdk has a general
  sandbox/write-scope policy? Start with prompt/tool guidance and add middleware
  enforcement later.
- Should `spawn_agent` default to the same model or a lower-cost role model?
  Default should inherit the parent; role specs may intentionally override.

## Alignment With Existing Docs

- `docs/vision.md`: sub-agents fit the harness/runtime product role and the
  "agentic applications boring to build" goal. They are not product-specific
  integrations.
- `docs/02_ARCHITECTURE.md`: the design keeps `runtime` focused on turns,
  `conversation/thread` focused on durable state, `app` focused on composition,
  and `harness` focused on lifecycle. It uses session projections rather than a
  second plugin system.
- `docs/quickstart.md`: sub-agent support should be surfaced through
  `harness.Session`, like sends, commands, and workflows.
- `docs/07_HARNESS_SESSION_LIFECYCLE.md`: sub-agent lifecycle belongs in
  `harness.Service` and `harness.Session`, not in terminal or `agent.Instance`.
- `docs/08_OUTPUT_EVENT_MODEL.md`: sub-agent live events should use typed
  payloads and channel-boundary rendering.
- `docs/11_WORKFLOW_LIFECYCLE.md` and
  `docs/12_WORKFLOW_EXECUTION_SEMANTICS.md`: sub-agent execution starts
  in-process like current async workflows, but remains separate from workflow
  DAG semantics.
- `docs/13_ACTION_TOOL_CONVERGENCE.md`: sub-agent control operations can later
  become actions; the LLM-facing surface remains tools projected from the
  session.
- `docs/15_DAEMON_SERVICE_MODE.md`: daemon mode should attach to the same
  `harness.Service` APIs and must not add a parallel sub-agent runtime.
- `docs/17_RESOURCE_APP_MANIFESTS.md`: roles should reuse existing agent specs
  and manifest/plugin contribution paths.
