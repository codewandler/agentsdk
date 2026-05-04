# Architecture overview

## Purpose

This document describes a target architecture for agentsdk that grows out of the current implementation. It is intentionally migration-oriented: most of the required foundations already exist, but their boundaries are blurred.

The goal is not to replace the current packages with a theoretical architecture. The goal is to make existing responsibilities explicit, move them gradually to clearer homes, and add missing concepts such as workflows, actions, channels, triggers, and harness lifecycle where the current model does not yet express them.

## Review order

Use the detailed aspect docs in this order when reviewing architecture from high level to low level:

1. Product and docs surface: `README.md`, `docs/README.md`, vision, roadmap, tasklist.
2. App/resource/plugin composition: `app`, `resource`, `agentdir`, `plugins/*` ([`28_APP_RESOURCE_PLUGIN_BOUNDARY.md`](28_APP_RESOURCE_PLUGIN_BOUNDARY.md)).
3. Harness/session/channel hosts: `harness`, `daemon`, `channel/*`, `terminal/*` ([`29_HARNESS_CHANNEL_BOUNDARY.md`](29_HARNESS_CHANNEL_BOUNDARY.md)).
4. Agent/runtime boundary: `agent`, `runtime`, `runner` ([`30_AGENT_RUNTIME_BOUNDARY.md`](30_AGENT_RUNTIME_BOUNDARY.md)).
5. Execution primitives: `action`, `tool`, `command`, `workflow`.
6. Persistence/state/context: `thread`, `conversation`, `agentcontext`, `capability`, `skill`.
7. Policy/observability/memory: `safety`, `usage`, compaction paths.

The package-level import review lives in [`27_PACKAGE_BOUNDARY_ANALYSIS.md`](27_PACKAGE_BOUNDARY_ANALYSIS.md), the app/resource/plugin composition review lives in [`28_APP_RESOURCE_PLUGIN_BOUNDARY.md`](28_APP_RESOURCE_PLUGIN_BOUNDARY.md), the harness/session/channel review lives in [`29_HARNESS_CHANNEL_BOUNDARY.md`](29_HARNESS_CHANNEL_BOUNDARY.md), and the agent/runtime review lives in [`30_AGENT_RUNTIME_BOUNDARY.md`](30_AGENT_RUNTIME_BOUNDARY.md). Their current conclusion is that there are no blocking low-level-to-host dependency violations, but `agent.Instance` remains the main fan-in/fan-out ownership problem.

## Current architecture summary

agentsdk currently has these major subsystems:

```text
cmd/agentsdk
  -> terminal/cli
  -> app
  -> agentdir/resource
  -> agent.Instance
  -> runtime.Engine
  -> runner.RunTurn
  -> conversation/thread/tool/capability/agentcontext
```

The important point: **agentsdk already has a runtime, app composition layer, resource discovery layer, plugin system, thread persistence model, capability model, context system, safety seed, and terminal channel.**

The future architecture should mainly clarify ownership:

- `runtime`/`runner` should stay focused on model/tool turns.
- `conversation`/`thread` should remain the durable state foundation.
- `app`/`agentdir`/`resource`/`plugins` should remain the app composition and discovery foundation.
- `terminal` should evolve into one channel over a general harness/session API and should not hardcode product/use-case composition such as planner or “standard” tool bundles.
- `agent.Instance` should shrink or become a compatibility façade around clearer session/runtime/harness concepts.
- datasources/workflows/actions should extend resource/plugin/app composition instead of introducing a separate parallel ecosystem.

## Current execution path

The current `agentsdk run` path is already a prototype harness, but the responsibilities are spread across packages:

```text
cmd/agentsdk
  -> terminal/cli.Config
  -> terminal/cli.Load
     -> Resources.Resolve(policy)
     -> agentdir/resource resolution
     -> harness.LoadSession(...resource bundle, plugins, load policy, middleware...)
        -> app.New(...resource bundle, plugins, middleware...)
        -> app.InstantiateDefaultAgent(...agent options...)
     -> agent.Instance
        -> model routing / policy
        -> agent-owned tool activation state
        -> skill repository and activation state
        -> context providers
        -> optional JSONL thread/session store
        -> optional ThreadRuntime for capabilities/context replay
        -> runtime.Engine
  -> terminal/repl or one-shot task
  -> runner events rendered by terminal/ui
```

A future harness should not throw this away. It should extract and name the reusable parts:

- resource/app loading;
- session open/resume;
- event subscription;
- turn/workflow dispatch;
- channel lifecycle;
- trigger lifecycle;
- safety policy installation.

## Existing bounded contexts

### Tooling

Current packages:

- `tool`
- `toolactivation`
- `tools/*`
- `toolmw`

Current strengths:

- `tool.Tool` is a clean model-callable interface.
- Typed tools already generate JSON schemas.
- `tool.Result` supports deterministic model-facing output and persistence.
- `tool.Intent` and `IntentProvider` provide side-effect declaration.
- Middleware can wrap tools for logging, risk gates, timeouts, and approval.
- Some concepts are not inherently model-only; execution, intent, result, events, context, and middleware should move into a top-level `action` package centered on `action.Action`, `action.Ctx`, `action.Result`, action intent, and action middleware. JSON schema/provider projection remains a tool-surface specialization unless an action explicitly provides optional `*jsonschema.Schema` metadata.
- `toolactivation.Manager` already models active/inactive tool visibility.

Evolution:

- Add a top-level `action` package.
- Introduce `action.Action` as the stable Go-native executable primitive that owns name, description, input/output `action.Type`, intent declaration, `action.Ctx`, `action.Result`, emitted events, result contract, and middleware chain.
- Move middleware completely to `action.*`; `tool` can alias or adapt it during migration.
- Treat `tool.Tool` as embedding or wrapping `action.Action`, adding only LLM-facing concerns such as guidance, provider/tool-call projection, activation/visibility, and transcript rendering.
- Keep `tool` as the public LLM-facing API during migration, but avoid adding parallel compatibility aliases unless explicitly needed; prefer deleting/collapsing duplicated paths.
- Keep generic local tools under `tools/` while adding action-backed constructors over time.
- Compose tool defaults through named use-case/environment plugins (`development`, `local_cli`, `research`, first-party apps), not generic standard bundles.
- Move product/service/environment-specific integrations toward adapters or integration packages as they appear.

### Turn runtime

Current packages:

- `runtime`
- `runner`
- `usage`

Current strengths:

- `runner.RunTurn` is the low-level model/tool loop.
- `runtime.Engine` is the higher-level turn facade.
- `runtime.History` integrates conversation projection and provider continuations.
- `runtime.OpenThreadEngine`, `CreateThreadEngine`, and `ResumeThreadEngine` already provide durable thread-backed engines.
- `runtime.ThreadRuntime` already binds live threads to capability replay and context render replay.
- Runtime options already accept tools, context providers, capabilities, event handlers, request observers, model settings, cache policy, and tool context factories.

Evolution:

- Preserve `runtime.Engine` and `runner.RunTurn` as the low-level turn runtime.
- Remove remaining concrete tool construction/imports from runtime over time.
- Treat workflow execution as a runtime-adjacent orchestration layer above turn execution.
- Let harness/session code own multi-session lifecycle and channel/trigger dispatch.

### Conversation and thread persistence

Current packages:

- `conversation`
- `thread`
- `thread/jsonlstore`

Current strengths:

- `conversation.Tree` supports branchable history.
- `conversation.Payload` and projected `Item`s separate stored events from request projection.
- `conversation.TurnFragment` provides a transaction-like turn commit object.
- Provider continuations are modeled explicitly.
- Compaction is explicit and semantic rather than silent history rewriting.
- `thread.Event` provides append-only event persistence.
- `thread.Store` has memory and JSONL implementations.
- Thread events already store conversation, capability, context render, usage, and lifecycle records.

Evolution:

- Reuse thread events for workflow execution records rather than inventing a separate persistence mechanism.
- Add datasource/workflow/action event kinds through the existing `thread.EventDefinition` pattern where state changes need persistence.
- Keep provider history immutability and explicit compaction as design constraints.

### Capabilities

Current packages:

- `capability`
- `capabilities/planner`

Current strengths:

- Capabilities are attachable modules that provide context, optional tools, and optional state.
- Stateful capabilities apply event-sourced state events.
- `capability.Manager` and `capability.Registry` already support factory-based creation and replay.
- Capability context providers render state back into the agent context.
- Planner is a working built-in stateful capability: plan mutations are state events; planner context renders current plan state; the `plan` tool is a model-facing projection over that state.

Evolution:

- Keep capabilities for attached stateful agent/session features with lifecycle, context, and optional event-sourced state.
- Do not overload capabilities to mean workflows, actions, plugins, or datasources.
- Let capabilities expose actions for workflow/app use and tool projections for LLM use, but do not assume the two sets are identical.
- Some capability actions may be internal, workflow-only, or unsuitable for direct model invocation; tools remain the deliberate LLM-facing subset/projection.
- In the action migration, add a capability action facet while keeping `Capability.Tools()` as the LLM-facing compatibility/projection surface.
- Allow workflows to require an attached capability or call capability actions when needed.
- Consider workflow execution state as either thread events or a capability only if there is a concrete need; default should be workflow-specific thread events.

### Context

Current packages:

- `agentcontext`
- `agentcontext/contextproviders`

Current strengths:

- Context providers return fragments with keys, roles, markers, authority, fingerprints, snapshots, and cache hints.
- Context manager records render state and diffs.
- Thread runtime replays context render records.
- Built-in providers already cover environment, git, time, file, command, project inventory, model info, project instructions, and skill inventory.

Evolution:

- Reuse context providers for workflow steps with selected context.
- Add step-level context selection in workflow execution rather than creating a separate context system.
- Keep context render records replayable and observable.

### Skills and commands

Current packages:

- `skill`
- `command`
- `command/markdown`
- `tools/skills`

Current strengths:

- Skills support external Agent Skills-compatible filesystem resources.
- Skill references under `references/` can be activated exactly and persisted.
- Commands support slash-command parsing, aliases, command policy, and command result semantics.
- Command results are channel instructions: render text, run an agent turn, reset, exit, or mark handled.
- Harness command projections expose only agent-callable session commands through `session_command` plus catalog context and deliberately reject channel-only command paths from agent context.

Evolution:

- Keep skills as instruction/reference resources, not workflows.
- Keep commands as user/app/channel invocation surfaces and prompt templates, not workflow replacement.
- Prefer action-backed commands for typed work, with command-specific metadata for aliases, argument hints, caller policy, slash-command wiring, and channel/user visibility.
- Treat command parsing (`command.Params`) as UX input, not as the canonical typed action input schema.
- Allow commands to trigger actions or workflows where useful, but keep command result semantics distinct from action execution results.
- Keep the harness `session_command` projection as the explicit agent-callable command path. Treat the older `command_run` bridge as compatibility unless a concrete use case proves otherwise.
- Keep agent-callable commands opt-in; do not expose reset/exit/session-control commands through LLM-facing command tools unless there is an explicit policy reason.

### App/resource/plugin composition

Current packages:

- `app`
- `agentdir`
- `resource`
- `plugins/*`
- `markdown`

Current strengths:

- `agentdir` resolves `.agents`, compatibility roots, app manifests, global/user resources, local roots, embedded/FS roots, and declarative git sources.
- `resource.ContributionBundle` normalizes discovered contributions.
- `app.App` composes bundles, plugins, commands, tools, skill sources, context providers, middleware, agent specs, actions, datasource definitions, and workflow definitions.
- `app.Plugin` has facets for commands, agent specs, tools, capability factories, skill sources, context providers, agent-context providers, tool middleware, actions, datasources, and workflows.
- App manifests already declare default agent, discovery policy, model policy, and sources.

Evolution:

- `agentdir` discovers `.agents/datasources/*.yaml` and `.agents/workflows/*.yaml`.
- `resource.ContributionBundle` includes datasource and workflow contributions.
- `app.Plugin` includes datasource/workflow/action facets and capability-factory facets so concrete capabilities such as planner are contributed by named plugins instead of hidden defaults in `agent`.
- `app.App` registers datasources/workflows/actions the same way it registers commands/tools/skills today.
- `app.App` is the registry/executor composition host. User input dispatch, command-result application, workflow thread recording, and command-triggered workflow entry points now live in `harness.Session`, so app does not need terminal/session-state command shims.


### Plugin and session projection invariant

Do not introduce a second, independent harness plugin system alongside `app.Plugin`. Session-scoped surfaces such as harness command tools and their context providers are valid extension points, but they should not create a parallel `harness.Plugin` concept with separate lifecycle semantics.

The near-term pattern is a **session projection**, not a plugin: a harness session may project session-owned capabilities into agent-facing surfaces such as `tool.Tool` values and `agentcontext.Provider` values. For example, the command layer can project the same declarative command tree into:

```text
harness command tree
  -> terminal slash command
  -> structured command envelope
  -> workflow command action
  -> agent command tool
  -> agent command catalog context provider
```

The invariant is that packaging/contribution remains unified. App/resource/plugin composition should not fork into `app.Plugin` plus unrelated `harness.Plugin`. If session-scoped contributions become pluginizable later, the resolution should be to evolve the existing plugin model or move plugin ownership upward into `harness.Service` as the host, with facets for app-level and session-level contributions under one plugin concept.

A likely intermediate seam is:

```go
type AgentProjection struct {
    Tools            []tool.Tool
    ContextProviders []agentcontext.Provider
}
```

Default harness sessions attach the command projection automatically, which makes `session_command` and the agent command catalog context available to the next agent turn while still enforcing per-command `AgentCallable` policy. This lets `harness.Session` attach session-owned agent projections without prematurely naming a second plugin system. Once harness owns more lifecycle, that projection can become a session-level facet of the unified plugin/contribution model.


### Default composition invariant

Generic packages must not define product defaults. `agent` should not define the default terminal/development/research agent, `runtime` should not install default tools or capabilities, and `app.New` should not silently install bundles. Terminal may provide a local CLI fallback for convenience, but that fallback should be a declared app/plugin/resource bundle plus plugin references. Concrete plugins are registered by the host; active plugins are selected by app/resource configuration or explicit CLI flags.

### Terminal as current channel

Current packages:

- `terminal/cli`
- `terminal/repl`
- `terminal/ui`

Current strengths:

- `terminal/cli.Load` resolves terminal/CLI concerns such as resources, CLI overrides, local default-plugin policy, session flags, and terminal UI adapters; reusable app/default-agent/session/plugin load mechanics live behind harness loading helpers.
- REPL and UI are functional and dogfooded.
- Runner events already provide a channel-friendly stream of text/tool/usage/step/error events.

Evolution:

- Treat terminal as the first channel.
- Keep generic resource/app/session/plugin setup behind harness loading helpers while keeping terminal-specific policy in terminal.
- Keep terminal rendering in `terminal/ui` and at terminal command-result boundaries.
- Make terminal call harness/session APIs instead of constructing the whole stack directly.

### Agent package

Current package:

- `agent`

Current strengths:

- `agent.Spec` is the resource-level blueprint.
- `agent.Instance` is a useful high-level session-backed agent object.
- Model policy, inference options, auto mux, compatibility evidence, compaction, session persistence, skill activation, context providers, agent-owned tool activation, and runtime creation are already integrated.

Current problem:

`agent.Instance` is doing too much. It mixes:

- declarative spec interpretation;
- model routing/policy;
- tool activation setup;
- skill repository/state setup;
- context provider setup;
- session/thread store setup;
- capability registry setup;
- terminal UI concerns;
- runtime engine construction;
- usage tracking and compaction.

Evolution:

- Keep `agent.Spec` as core public API.
- Keep `agent.Instance` initially for compatibility and as the current high-level façade.
- Gradually move host/session lifecycle concerns toward `harness`.
- Move terminal-specific behavior out toward terminal channel.
- Keep model policy/inference pieces close to agent spec/runtime construction unless a clearer boundary emerges.

## Target architecture

The target dependency direction:

```text
cmd/agentsdk
  -> terminal/channel adapters
  -> harness
  -> app/resource/plugin composition
  -> workflow execution
  -> runtime.Engine / runner.RunTurn
  -> core abstractions

channels/*
  -> harness/session/control APIs

triggers/*
  -> harness trigger sink/session APIs

workflow
  -> datasource/action registry + runtime/tool/command/context/thread abstractions

runtime
  -> conversation/thread/tool/capability/agentcontext abstractions

adapters/*
  -> core interfaces + third-party/environment systems
```

Avoid:

```text
runtime -> concrete tools
runtime -> terminal UI
core domain -> product-specific adapters
new workflow discovery -> separate from existing resource discovery
new plugins -> separate from existing app.Plugin facets
new persistence -> separate from thread events
```

## Concept composition model

The core domain should make overlapping concepts compositional rather than competing:

```text
Adapter / connector
  owns integration details for an external system or runtime environment
  may provide datasources, actions, tools, context providers, or triggers

Datasource
  owns a configured data boundary: collection/API/index/stream, schema, provenance, credentials/config references, sync state
  is accessed by actions; it is not the execution primitive
  may provide standard action definitions such as read/sync/search/map
  may be exposed to a model only indirectly through tool-wrapped actions

Action
  is the smallest typed executable unit in package `action`
  is independent of LLMs, agents, and tools
  owns stable execution metadata: name, description, input/output `action.Type`, implementation, intent, `action.Ctx`, `action.Result`, emitted events, result contract, and middleware chain
  can be called by workflows, tools, commands, triggers, datasources, or app code

Tool
  embeds or wraps `action.Action`
  is the LLM-facing projection of executable power
  adds model/provider-specific exposure: activation, guidance, serializable schema projection, provider conversion, and transcript-oriented result rendering

Workflow / pipeline
  composes `workflow.ActionRef`s with data flow and control flow
  orchestrates action execution, retries, step policy, context selection, and durable progress
  pipeline is the linear workflow case
  may itself be exposed through an action wrapper

Command
  is a human/app/channel-facing invocation surface
  owns slash-command UX metadata, caller policy, parsing, and channel result instructions
  may call an action, start a workflow, or execute/render a prompt/model-turn action
  is not the canonical typed execution/result contract

Capability
  is attached agent/session feature state plus lifecycle/context
  may expose actions for workflow/app use and tools for LLM use; these sets need not be identical
  defining feature is stateful extension of an agent/session, often replayed from thread events

Plugin / bundle
  are packaging/contribution mechanisms, not execution primitives
  plugin is Go-code contribution; bundle/resource is declarative/discovered contribution

Channel
  is ingress/egress between users/systems and the harness

Event
  is either live telemetry (runner/channel events) or durable state/history (thread events)
  should be reused for workflow/action/datasource observability before adding a separate event bus

Trigger
  is an event/time source that asks the harness to start/resume work
```

This model avoids treating every concept as a synonym for "function." The smallest executable unit is the action; tools, commands, workflows, triggers, datasources, and channels are different ways to expose, compose, or initiate actions. The current `tool.Tool` package already contains much of this action machinery, so the migration should factor it into `action.*` carefully rather than introduce a duplicate workflow-only action stack.

## Datasource/workflow/action architecture

Datasource/workflow/action support should be an extension of existing app/resource/plugin/runtime concepts.

### Definitions

```text
Datasource
  name
  kind/type
  config schema
  record/item schema
  provenance metadata
  credential/config references
  paging/cursor/checkpoint state
  consistency/freshness expectations
  standard action refs: fetch/list/search/sync/map/transform

Action
  name
  description
  input action.Type
  output action.Type
  implementation
  intent declaration
  action.Ctx
  action.Result: Status, Data any, Error error, Events []action.Event where action.Event is an alias for any
  result contract
  middleware chain
  observability labels

Workflow
  name
  description
  input schema
  output schema
  steps with workflow.ActionRef
  edges / dataflow mappings
  policy
  retries / timeout / concurrency limits
  context selection
  durable progress semantics
  metadata

Pipeline
  workflow whose DAG is a simple sequence

workflow.WorkflowAction
  adapter that exposes a workflow definition as action.Action when a caller needs action-level composition
  useful for triggers, tools, parent workflows, or explicit action registration

Example: docs_refinement_loop
  command /refine-docs starts the docs_refinement_loop workflow through harness/session wiring
  steps: review_source -> challenge_docs -> identify_gaps -> propose_questions -> refine_docs -> report_summary
  each step resolves workflow.ActionRef -> action.Action
```

### Resource loading

Default resource location:

```text
.agents/workflows/*.yaml
```

But discovery should follow existing resource resolution rules:

- project `.agents` roots;
- compatibility roots only if the workflow format is explicitly supported there;
- plugin roots;
- app manifest sources;
- embedded filesystems;
- declarative remote git sources, where safe.

### Plugin contribution

Extend `app.Plugin` with facets such as:

```go
type DataSourcesPlugin interface {
    Plugin
    DataSources() []workflow.DataSource
}

type WorkflowsPlugin interface {
    Plugin
    Workflows() []workflow.Definition
}

type ActionsPlugin interface {
    Plugin
    Actions() []workflow.Action
}
```

Exact interface names should be decided when the workflow package is implemented; the architectural requirement is that datasource/workflow/action contributions extend the existing plugin facet model.

### Persistence

Workflow execution should emit concrete events. Harness/session adapters should persist only the subset needed for replay, resumability, audit, context reconstruction, or user-visible history using the existing thread event registry pattern.

Potential persistent event kinds:

```text
workflow.started
workflow.step_started
workflow.step_completed
workflow.step_failed
workflow.completed
workflow.failed
action.intent_declared
action.decision_recorded
action.result_recorded
```

Persistence should be driven by statefulness and timescale, not by package boundaries. Do not persist every short-lived command/action event by default: commands are usually invocation surfaces, actions are usually operation-scale execution, workflows are run-scale state, datasources may have sync/checkpoint-scale state, and capabilities may have session-scale state.

The initial workflow event model should follow the same persistent-event style as the rest of the codebase: concrete payload structs registered through `thread.EventDefinition`, not a single discriminator-bearing payload struct. Live workflow execution may pass those same concrete payload structs through `action.Result.Events` or an event handler; a persistence adapter can later choose the corresponding `thread.EventKind` when appending to a thread.

Live workflow events are telemetry payloads shaped to be compatible with future persistence. Workflow now has explicit run identity, step attempt metadata, `workflow.ValueRef` output references, and a projector/materializer that can rebuild `workflow.RunState` from concrete events. Runtime step dataflow remains Go-native `any` at the action boundary, while workflow events and projected run state store outputs as inline, external, or redacted value references.

`workflow.RunStore` defines the workflow-facing run access contract: append events for a `RunID`, read recorded events, and project current `RunState`. It accepts `context.Context` because durable implementations read and append through thread stores or future external storage. `workflow.MemoryRunStore` is the simple in-memory implementation and is intentionally not a durable database; it clarifies append/read/project semantics, run isolation, unknown-run behavior, and projector error handling. `workflow.ThreadRecorder` records live workflow events into a `thread.Live`, and `workflow.ThreadRunStore` satisfies the same run-store semantics by appending to and reading from a scoped thread/branch log.

`app.App` workflow helpers accept execution options for run ID generation and live event handling, which lets callers install persistence adapters without coupling `workflow.Executor` to storage. Session-owned workflow execution goes through `harness.Session.ExecuteWorkflow`, which composes `workflow.ThreadRecorder` when the active agent has a live thread and preserves caller-provided event handlers. Thread append integration builds on the run-state model rather than writing unprojectable telemetry. Do not create a separate workflow database until thread events prove insufficient.

### Workflow run read models and harness commands

Workflow execution remains owned by `workflow.Executor`; `app.App` resolves workflow definitions and actions, while `harness.Session` owns session-scoped workflow recording and history APIs. The current path is intentionally narrow:

```text
workflow.Executor
  -> live workflow events
  -> workflow.ThreadRecorder
  -> thread.Live append
  -> workflow.ThreadRunStore projection
  -> harness.Session read APIs
  -> terminal-facing /workflow namespace
```

`workflow.ThreadRunStore` reads workflow events from a scoped thread/branch and projects them into `workflow.RunState` for a single run or `workflow.RunSummary` rows for listing. `harness.Session` exposes that read model through:

```go
WorkflowRunStore() (*workflow.ThreadRunStore, bool)
WorkflowRunState(ctx context.Context, runID workflow.RunID) (workflow.RunState, bool, error)
WorkflowRuns(ctx context.Context) ([]workflow.RunSummary, bool, error)
```

The terminal path routes through `harness.Session.Send`, so harness can own session-aware workflow commands without making `app.App` aware of terminal/session state. Harness commands are declared through `command.Tree`; slash syntax is one projection over the same tree used for descriptors and structured command execution. Current commands are:

```text
/workflow list                 # registered workflow definitions
/workflow show <name>          # workflow definition detail
/workflow start <name> [input] # synchronously execute a workflow in this session
/workflow runs [--workflow <name>] [--status <running|succeeded|failed>]
                               # recorded workflow run summaries for this thread-backed session
/workflow run <id>             # projected run detail for this thread-backed session
```

The same command model is exposed programmatically through:

```go
ExecuteCommand(ctx context.Context, path []string, input map[string]any) (command.Result, error)
```

`ExecuteCommand` dispatches against the command tree directly and does not stringify structured input into slash-command text. This keeps terminal, future HTTP/JSON APIs, generated help, and LLM-safe projections aligned on one command contract.

Trade-off: `/workflow start` is synchronous today: it executes the workflow in the current command request and returns after completion or failure with the run ID. A future async harness lifecycle can keep the same command shape and return `running` once workflows can outlive the request. `/workflow runs` includes projected start/completion timing, duration, and harness-side filtering by workflow name and status, but is currently sorted deterministically by run ID, not by execution time. Chronological ordering requires sorting support on top of `RunSummary` or introducing a separate indexed read model. Until then, the read model favors simple projection from the append-only thread log over a second workflow database.

### Runtime relationship

Workflow belongs to the broader runtime system, but not inside the low-level model/tool loop.

```text
runner.RunTurn         = one model/tool loop; can be used by a prompt/model action
runtime.Engine         = high-level turn engine over history/thread/context
workflow.Executor      = DAG/pipeline orchestration over workflow.ActionRef -> action.Action
workflow.WorkflowAction = adapter exposing a workflow run as action.Action
app.App               = composition root and registry host for workflow definitions/actions
harness.Service        = host/session seam for command-triggered workflows, recording, channels, triggers
```

A prompt or model turn is also an action when treated as an executable unit: it has input, output, context, policy, and result semantics. The current `agent.TurnAction` adapter exposes an `agent.Instance` turn as an `action.Action` for workflow use, returning the latest assistant text after the turn commits. The fact that it calls an LLM is an implementation detail of that action implementation, not a reason to make workflows depend directly on LLM concepts.

## Harness architecture

Harness should consolidate what is currently split across `terminal/cli.Load`, `app.App`, and `agent.Instance`.

Initial harness should be modest. It should not replace `app.App`; it should host it.

Responsibilities:

- load or receive app/resource composition;
- own session registry/lifecycle;
- open/resume thread-backed sessions;
- expose a channel-facing send/subscribe API;
- route work to agent/runtime/workflow execution;
- host triggers;
- install safety policy/middleware;
- emit observable events.

A first implementation can wrap current objects:

```text
harness.Service
  contains app.App
  opens agent.Instance sessions
  forwards user input to Instance.RunTurn
  forwards runner events to channel subscribers
```

Then evolve:

```text
harness.Service
  contains app composition
  owns sessions directly
  creates runtime engines via runtime.OpenThreadEngine
  hosts datasources/workflows/actions
  supports multiple channels/triggers
```

The first harness implementation already wraps `app.App` and the default `agent.Instance` enough for terminal sends, session metadata, session-scoped workflow browsing, default session projections, and command-result application. It intentionally keeps command namespaces such as `/workflow` and `/session` in harness rather than app: app remains the composition/execution registry, while harness owns the channel/session context needed to answer questions such as "which thread-backed workflow runs belong to this session?". Harness command namespaces are now declarative `command.Tree` definitions; `Session.Send`, `Session.Commands().Descriptors()`, and `Session.ExecuteCommand` all use the same tree-backed command model instead of separate switch-based parsing paths. Terminal one-shot mode renders returned `command.Result` values at the terminal boundary rather than discarding them.

Harness loading now owns the reusable setup that used to sit in `terminal/cli.Load`: selecting and preparing the default agent from resolved resources, applying generic agent-spec overrides, resolving model-policy/source-API load settings, resolving default/manifest/explicit plugin refs through `app.PluginFactory`, applying loaded plugins, translating grouped session/app configuration into `app.Option` values, instantiating the default agent, creating the service, and opening the default session. Terminal remains responsible for terminal-only policy such as CLI flag interpretation, local CLI fallback/default-plugin policy, terminal event handlers, debug-message output, fallback spec selection, and risk-log presentation.

Current `harness.SessionLoadConfig` still carries `io.Writer` output because the existing `app`/`agent`/terminal path accepts writers for output and event adapters. Treat that as a transitional compatibility seam, not the target channel model. Long-term harness and runtime components should not spill arbitrary bytes to a writer. They should publish structured events, command results, usage records, notices, and renderable payloads that a channel/frontend can consume and render with its own renderer. Terminal may adapt those structured publications into text; HTTP/SSE, TUI, JSON, and LLM-facing channels should be able to choose different renderers over the same content.

## Package evolution map

| Current package | Future role |
| --- | --- |
| `action` | New top-level package for surface-neutral execution: Action, Ctx, Result, Intent, Middleware. |
| `tool` | Keep as public LLM-facing tool API; embed/wrap `action.Action` and alias/adapt action concepts for compatibility. |
| `tools/*` | Keep generic tools; expose some as actions where useful. |
| `toolmw` | Keep; gradually become part of broader safety architecture. |
| `runtime` | Keep turn runtime; remove concrete tool dependencies over time. |
| `runner` | Keep low-level model/tool loop. |
| `conversation` | Keep conversation projection/history model. |
| `thread` | Keep durable event/store model; add workflow events. |
| `capability` | Keep attachable stateful feature model; add action facet for workflow/app use while keeping tools as deliberate LLM-facing projections. |
| `capabilities/planner` | Keep as built-in capability and dogfood example of event-sourced session state plus context plus action/tool projection. |
| `agentcontext` | Keep context provider/render model; reuse for workflow steps. |
| `skill` | Keep instruction/reference resource model. |
| `command` | Keep slash command and channel-result model; use `command.Tree` for declarative subcommands, args, flags, validation, descriptors, and structured command execution; prefer harness `session_command` projection for agent-callable commands; add action-backed command adapters where useful without making every command model-callable. |
| `resource` | Extend contribution bundle with datasources/workflows/actions. |
| `agentdir` | Extend loader for `.agents/datasources` and `.agents/workflows`. |
| `app` | Keep composition root and resource/plugin registry host for agents, actions, datasources, workflows, commands, tools, skills, and diagnostics; hosted by harness for channel/session lifecycle. |
| `plugins/*` | Extend plugin facets; first-party bundles should be named by concrete capability/use case/environment, not generic “standard”. |
| `agent` | Keep spec and compatibility façade; migrate host/session duties outward. |
| `terminal/*` | First channel over harness; render command results at terminal boundaries. |
| `usage` | Keep runtime usage aggregation; integrate workflow attribution later. |

## Cleanup guardrails

Current cleanup work is enforcing these boundaries:

- `toolactivation.Manager` owns mutable tool registry and active/inactive state.
- There is no generic “standard tools” concept. Tool/capability defaults must be named by use case or environment and activated by app/resource config or explicit CLI flags, not hidden in `agent`, `app`, or `terminal/cli`.
- Session projections are not plugins; do not introduce `harness.Plugin` beside `app.Plugin`.
- New commands belong in declarative `command.Tree` definitions, not handwritten switch namespaces.
- Channel boundaries must render returned `command.Result` values instead of discarding them or formatting inside harness handlers.
- Harness/runtime code should publish structured content/events rather than writing arbitrary bytes to `io.Writer`; any current writer fields are transitional channel-adapter compatibility seams.
- Terminal event rendering lives in `terminal/*`; `agent.Instance` records runner events and delegates presentation through event handler factories.
- New seams should delete or collapse old paths; avoid labeling permanent complexity as "transitional" without paying it down.

## Current coupling issues to reduce

Observed top-level dependency issues:

1. `agent` still imports many high-level and low-level packages: runtime, runner, thread/jsonlstore, usage, skill, context providers, and llmadapter routing. It no longer imports terminal UI, concrete planner construction, or any standard/default tool bundle; terminal rendering is attached through event handler factories and hosts pass explicit tools/capability registries.
2. `runtime` no longer imports concrete model-callable tool packages such as `tools/skills` or `tools/toolmgmt`; tool and skill activation state is injected through neutral state-owner packages.
3. `terminal` no longer imports generic standard bundles or concrete planner plugin wiring for defaults. It selects the named `local_cli` default plugin policy, but harness owns generic plugin-ref resolution and plugin application during session load. Terminal accepts manifest/CLI plugin refs and can disable default plugins with `--no-default-plugins`.
4. `app` no longer imports generic standard bundles; app hosts/plugins provide default and catalog tools explicitly, and capability factories now flow through plugin facets.

Migration strategy:

- Do not break these immediately.
- Add harness/channel/workflow boundaries alongside current code.
- Move setup paths gradually, deleting old/dirty paths instead of keeping compatibility shims unless a shim is explicitly requested.
- Use `go list` import checks to verify dependency direction improves.

## Verification approach

For every architectural change:

```bash
go test ./...
```

For resource/app/channel changes:

```bash
go run ./cmd/agentsdk discover [path]
go run ./cmd/agentsdk run [path]
```

For dependency direction:

```bash
go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./...
```

## Design trade-offs

### Evolution vs clean rewrite

A clean rewrite would produce prettier packages faster but would risk breaking working runtime, terminal, resource, plugin, skill, and safety behavior.

Recommendation: evolve in place. Add missing seams, then move code behind those seams.

### Harness vs app.App

`app.App` already composes agents, resources, commands, plugins, tools, skill sources, actions, datasources, and workflows. It is best understood as the composition root / registry host, not a channel target or final process runtime.

Recommendation: harness hosts `app.App` first and reuses its registries. Lifecycle-heavy responsibilities such as multi-session management, channel routing, trigger dispatch, and durable workflow execution should move to harness/session APIs over time. Keep `app.App` from becoming a permanent god object by splitting responsibilities deliberately when harness lands.

### Workflow as new system vs extension of resources/plugins/thread

A standalone workflow system would be faster to prototype but would duplicate discovery, plugin, persistence, and observability concepts.

Recommendation: datasource/workflow/action should extend `resource.ContributionBundle`, `app.Plugin`, app manifests/resource roots, thread events, and existing runtime/context/tool concepts. Workflow should own graph definitions and `workflow.ActionRef`; action should own execution.

### Actions vs tools

The current code puts several action-like responsibilities on `tool.Tool`: name, description, execution, result, intent declaration, context, middleware, and schema projection. That made sense because the LLM/tool loop was the first execution surface. As workflows, triggers, commands, and datasources become first-class, Go-native execution responsibilities should move to a surface-neutral top-level `action` package.

The trade-off: moving too aggressively risks breaking the clean existing tool API; moving too timidly will duplicate result, intent, middleware, event, and safety machinery in a separate workflow stack. The split should not make actions JSON-first: serializable JSON schemas are required for LLM tools and resource/remote surfaces, but core actions may accept real Go types such as interfaces, channels, readers, handles, or domain objects.

Recommendation: introduce `action.Action`, `action.Type`, `action.Ctx`, `action.Result`, action intent, emitted action events, and action middleware as the central executable layer. `action.Type` should carry the Go `reflect.Type` plus optional `*jsonschema.Schema` metadata, and later own helpers for creating values, encoding, decoding, validation, and schema projection. `action.NewTyped[I, O]` should construct input/output `action.Type` values for the typed handler and prefer handlers shaped like `func(action.Ctx, I) (O, error)` so ordinary Go functions can adapt naturally into actions. `action.Result` should stay execution-oriented: explicit execution `Status`, `Data any`, `Error error`, and optional `[]action.Event` payloads for the runtime to dispatch, where `action.Event` is an alias for `any`. Display/rendering concerns should be added later through interfaces implemented by returned data or by surface adapters, not by baking display variants into the core result. Then make `tool.Tool` embed or wrap `action.Action` and add only LLM-facing concerns such as guidance, activation, serializable schema/provider projection, and transcript rendering. Keep existing `tool.Tool` APIs only where they remain the active LLM-facing surface; avoid new compatibility wrappers, and delete/collapse duplicated paths as workflow/action code moves to the action abstraction.
