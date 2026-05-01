# 01 — agentsdk Vision

## Purpose

agentsdk is a Go framework, runtime, and builder for secure, reliable agentic applications.

It is already more than a tool library: the repository contains a turn runtime, durable thread/session state, resource discovery, app manifests, plugins, skills, commands, terminal execution, standard tools, safety primitives, context providers, and a dogfood engineer agent. The future design should evolve these existing pieces into a clearer product architecture rather than replace them with unrelated new abstractions.

agentsdk should support three related product roles:

1. **Agent runtime** — the reusable execution substrate for model turns, tools, context, state, safety, capabilities, events, and persistence.
2. **Agent development kit** — the APIs, resource formats, plugins, examples, and app composition model used by developers to build agentic apps in Go and with declarative resources.
3. **Agent builder** — an agent-powered product surface (`agentsdk build`) that interviews users about a use case and creates agents, workflows, actions, plugins, repositories, deployment assets, and operational configuration.

The long-term product promise:

> A user describes an agentic use case — for example, ticket triage across a helpdesk or issue tracker — and agentsdk helps turn that use case into a working, safe, observable, deployable agentic application.

agentsdk should provide the runtime, safety model, datasource/workflow/action model, app packaging, resource discovery, and builder experience so each application does not reinvent the same infrastructure.

## Evolution thesis

The product direction is not "build a new runtime next to the old one." It is:

```text
existing turn runtime + existing app/resource/plugin system + existing thread log
  -> clearer harness/session/channel boundaries
  -> datasource/workflow/action orchestration on top
  -> builder that generates resources, code, and deployment assets using those same primitives
```

This matters because current users can already build agents in Go and run resource bundles with `agentsdk run`. Future work should preserve that path while making the underlying host reusable for daemons, web/TUI channels, scheduled triggers, and generated apps.

## Product north star

agentsdk should make agentic applications boring to build and safe to operate.

A good agentsdk application should have:

- declarative agent/app configuration where useful;
- Go-code construction paths for embedded applications;
- typed actions as the central executable primitive, with tools and commands as invocation surfaces;
- explicit side-effect policy and approval gates;
- durable thread/session state;
- reproducible context rendering;
- observable runtime and workflow events;
- reliable workflows with schemas and constrained execution;
- multiple channels for human/system interaction;
- triggers for scheduled or event-driven background work;
- packaging/deployment paths suitable for real services.

## What exists today

The current repository already provides the foundation for the vision:

- `runtime` and `runner` execute model/tool turns over `llmadapter/unified.Client`.
- `conversation` models branchable history, projected items, compaction, and provider continuations.
- `thread` provides append-only thread events, branches, stores, memory store, and JSONL persistence.
- `runtime.ThreadRuntime` binds live threads to capability state and context render replay.
- `tool` defines the current model-callable tool abstraction, including JSON schemas, results, intent declaration, middleware hooks, and unified conversion; these are the concepts most likely to migrate toward a central `action.Action` core.
- `tools/*` provides filesystem, shell, git, web, vision, phone, JSON query, todo, turn, skill, and tool management tools.
- `toolmw` and `cmdrisk` integration provide an early risk/safety layer.
- `capability` and `capabilities/planner` provide attachable, event-sourced capability state.
- `agentcontext` and `agentcontext/contextproviders` provide context managers, render records, diffs, and built-in context providers.
- `skill` supports Agent Skills-compatible skill directories, references, repositories, and activation events.
- `command` supports slash commands and a `command_run` tool bridge for agent-callable commands.
- `agentdir` and `resource` load `.agents`, compatibility roots, app manifests, local/global resources, declarative git sources, and normalized contribution bundles.
- `app` composes resource bundles, commands, plugins, tools, skills, context providers, middleware, and agent specs into running app instances.
- `plugins/*` already define first-party plugin bundles for git, skills, tool management, vision, and standard plugin sets.
- `terminal/cli`, `terminal/repl`, and `terminal/ui` provide the current terminal channel and `agentsdk run` experience.
- `apps/engineer` is a practical dogfood resource bundle used as a coding/architecture/devops agent; `examples/engineer` remains as a compatibility copy during the transition.

The product vision is therefore evolutionary: clarify boundaries, reuse these foundations, and add missing concepts only where the current model cannot express the future product.

## What agentsdk is

agentsdk is both a library and an executable product surface.

As a **library**, it lets Go developers build agents, tools, actions, workflows, plugins, channels, triggers, and app harnesses.

As a **runtime**, it runs agent turns, executes tools/actions, renders context, persists state, enforces safety policies, and emits events.

As a **resource/app format**, it can load agent directories, app manifests, skills, commands, workflow specs, and plugin-provided contributions.

As a **builder**, it should eventually generate full agentic apps from requirements: resource-only apps, mixed YAML/Go apps, custom Go code, tests, and deployment artifacts.

## What agentsdk is not

agentsdk should not become a monolithic SaaS product or a pile of product-specific integrations inside the core runtime.

The core should stay reusable. Helpdesk systems, issue trackers, chat systems, email, browser automation, hosted web UIs, deployment targets, and cloud-specific integrations should be modeled as adapters, plugins, bundled apps, or generated application code unless they are genuinely universal primitives.

## Product surfaces

### `agentsdk run`

Runs an agentic app from Go code or from resources such as agent directories and app manifests.

This exists today through `cmd/agentsdk`, `terminal/cli`, `app`, `agentdir`, `resource`, and `agent.Instance`. It should evolve without breaking current behavior. Internally, more of the shared app/session/channel lifecycle should move behind a harness boundary so terminal, HTTP/SSE, TUI, WebSocket, gRPC, telnet, and proprietary channels can reuse the same host logic.

### `agentsdk discover`

Inspects discovered resources without running an agent.

This already exists and should remain a key debugging/product surface for app manifests, resource roots, external sources, skills, commands, agents, and future datasource/workflow/action resources.

### `agentsdk build`

An agent-powered builder that helps users create agentic apps.

The builder should be able to:

- ask requirements questions;
- identify required agents, tools, actions, connectors, workflows, channels, and triggers;
- create agent specs and app manifests;
- create workflow YAML specs where declarative resources are enough;
- scaffold Go plugins/actions/tools/connectors where code is needed;
- generate tests;
- create Docker/build/deployment assets from templates;
- initialize or update a git repository;
- run verification commands and report gaps.

Builder output should support multiple complexity levels:

1. **Resource-only** — YAML/Markdown/spec files only.
2. **Hybrid** — declarative resources plus generated Go plugins/actions/tools.
3. **Full app** — custom Go app/harness code, tests, packaging, and deployment assets.
4. **Deployment-ready** — Docker, Helm, CI/CD, service manifests, and environment-specific configuration generated from templates.

The builder itself should be an agentsdk app and dogfood the runtime, workflows, safety, tools, and packaging model.

### Harness / daemon

The harness is the host for complete agentic applications.

Today, this role is partially split across `terminal/cli.Load`, `app.App`, and `agent.Instance`. Those existing pieces should evolve into a clearer harness/session abstraction rather than being discarded.

The harness should initialize configured agents, channels, stores, datasources, triggers, tools, actions, policies, and workflows. It owns process-level lifecycle and exposes a control plane to channels and external systems.

The harness may run embedded in a CLI process or as a daemon/service. The public concept should be **harness**. **Daemon** is one deployment mode.

## Concept hierarchy

Several agentsdk concepts intentionally overlap because they are different views of the same underlying work. The goal is not to eliminate overlap, but to define the direction of composition:

```text
Adapter / connector
  -> implements actions and/or datasources against an external system

Datasource
  -> configured data boundary: collection, stream, API, corpus, database, or index
  -> owns schema, provenance, credentials/config references, and sync/checkpoint semantics
  -> is accessed by actions; it is not itself the execution primitive
  -> may provide standard actions such as fetch, list, search, sync, transform

Action
  -> smallest typed unit of execution in package `action`
  -> completely independent of LLMs, agents, and tools
  -> owns stable execution metadata: name, description, input/output `action.Type`, intent, `action.Ctx`, `action.Result`, emitted events, and middleware chain
  -> can be called by workflows/pipelines, triggers, commands, tools, datasources, or app code

Tool
  -> LLM-facing exposure of executable power
  -> embeds or wraps `action.Action`
  -> adds only model-facing concerns such as guidance, provider/tool-call projection, activation/visibility, and transcript rendering

Workflow / pipeline
  -> reliable, inspectable composition of `workflow.ActionRef`s
  -> orchestrates action execution, dataflow, control flow, retries, and durable progress
  -> pipeline is the linear case; workflow is the general DAG/control-flow case
  -> may itself be wrapped as an action for commands, triggers, tools, or parent workflows

Command
  -> human/app/channel-facing invocation surface, usually slash-command shaped
  -> owns UX metadata such as aliases, argument hints, caller policy, and result instructions to the channel
  -> can call an action, start a workflow, or execute/render a prompt/model-turn action
  -> is not the typed execution primitive; command params/results are channel semantics around execution

Capability
  -> attachable agent/session feature with lifecycle, context, and optional event-sourced state
  -> may expose actions, tools, and context, but those are surfaces over the attached state/feature
  -> not a workflow; capabilities extend what an agent/session has available, workflows define what happens

Bundle / plugin
  -> packaging and contribution mechanisms
  -> bundle is declarative/discovered; plugin is Go-code contribution

Channel
  -> user/system ingress and event egress for the harness

Event
  -> observable and/or durable fact emitted by runtime, workflow, datasource, tool/action, channel, or trigger
  -> thread events are the durable event log; runner events are live execution/UI events

Trigger
  -> event/time source that starts or resumes work through the harness
```

The preferred mental model is:

```text
External system -> adapter/connector -> datasource -> actions -> workflows/commands/tools/triggers -> harness
```

## Core product concepts

### Agent

An agent is a configured actor with instructions, tools/actions, skills, capabilities, model policy, context sources, and persistent session state.

Today this is represented by `agent.Spec` for the declarative blueprint and `agent.Instance` for a running session-backed object. The future architecture should preserve that distinction while reducing the amount of terminal/app wiring inside `agent.Instance`.

### App

An app is a composition of agents, commands, plugins, tools, skill sources, context providers, middleware, and resource bundles.

Today this is `app.App`. It is already a user-facing composition root. The future harness should build on this role, not bypass it. Over time, process/session/channel lifecycle may move from terminal/agent packages into harness while `app.App` remains the composition model.

### Resource bundle

A resource bundle is a normalized set of discovered contributions: agent specs, commands, skill sources, tool contributions, hooks, permissions, diagnostics, and future datasource/workflow/action contributions.

Today this is `resource.ContributionBundle`, produced by `agentdir` and consumed by `app.App`. Workflow/action resources should extend this existing discovery model.

### Plugin

A plugin is a Go-code contribution bundle.

Today `app.Plugin` and its facets contribute commands, agent specs, tools, skill sources, context providers, agent-scoped context providers, global tool middleware, and targeted tool middleware. Future datasource/workflow/action contributions should become additional plugin facets rather than a parallel plugin system.

### Action

An action is the core executable primitive in a top-level `action` package: a named atomic Go-native operation with typed input, typed output, declared intent, result semantics, execution context, optional emitted events, and middleware chain.

Actions are independent of LLMs, agents, and tools. A system can run commands, workflows, datasource syncs, scheduled jobs, or service integrations entirely through actions without any model involved.

Action metadata should live on the action, not on every surface that can invoke it. That includes name, description, input/output `action.Type`, intent declaration, `action.Ctx`, `action.Result`, middleware, observability labels, and safety policy hooks. Action implementations may execute command processes, HTTP requests, transforms, approvals, datasource reads/writes/syncs, workflow dispatches, model/agent turns, or domain-specific operations, but core actions do not need a built-in kind enum for that.

Actions are Go-native, not JSON-first. Core action handlers should accept typed values, not raw serialized bytes, and those values may be ordinary Go values such as interfaces, channels, readers, handles, or domain objects. Serialization constraints belong to surfaces such as tools, resource files, remote channels, and persisted workflow state. Tools are a specialization/projection because model inputs must be serializable and schema-described.

`action.Type` should represent the input or output contract for an action. It should track the Go type plus optional `*jsonschema.Schema` metadata, for example `reflect.Type` plus JSON schema when the type is serializable. `action.NewTyped[I, O]` should construct the input and output `action.Type` values for `I` and `O`, and should prefer handlers shaped like `func(action.Ctx, I) (O, error)` so ordinary Go functions can adapt naturally into actions. Over time `action.Type` can own helper methods for creating new values, encoding, decoding, validation, and schema projection, while still allowing Go-native values that cannot be serialized.

`action.Result` should be execution-oriented and minimal: `Data any`, `Error error`, and optional emitted `Events []action.Event`, where `action.Event` is just an alias for `any`. A result should not encode display concerns up front. Values that need display behavior can later implement a display-related interface, and channel/tool adapters can decide how to render them.

Actions can be implemented in Go and referenced by workflows, pipelines, tools, commands, triggers, datasources, and app code. This makes action the central reusable unit: workflow steps, datasource operations, model-callable tools, and slash commands should not each reinvent result, intent, middleware, and surface-specific schema/projection concepts.

### Tool

A tool is not the core execution primitive. A tool is the LLM-facing way to give an agent executable power over actions, workflows, commands, datasources, or other app capabilities.

Technically, the target shape is that `tool.Tool` embeds or wraps `action.Action` and adds only what an LLM/provider needs: guidance, provider/tool-call projection, activation/visibility, and transcript-oriented rendering. Middleware, intent declaration, context, and base results belong in `action.*`.

In the current code, `tool.Tool` owns metadata, schema, execution, intent, result, and middleware. The target direction is to factor those reusable parts into `action.Action`, then keep `tool.Tool` as the LLM-facing adapter over actions. Compatibility wrappers and aliases can preserve existing `tool.Tool`, `tool.Ctx`, `tool.Result`, `tool.Intent`, and middleware APIs while the internals migrate.

Tools and actions are related but not identical:

- an **action** is the reusable Go-native executable unit with typed input/output, intent, `action.Ctx`, `action.Result`, optional emitted events, and middleware;
- a **tool** is the model-callable exposure of executable power during a turn;
- one action may be exposed as zero, one, or many tools depending on model/channel needs;
- a tool may also expose higher-level executable surfaces such as workflows, commands, or datasource operations when that is the desired agent interface.

### Datasource

A datasource is a configured data boundary: collection, stream, API, document corpus, database, search index, or event source that agentsdk can ingest from, query, or synchronize.

A datasource is not an execution primitive and is not automatically a tool. It owns data-facing concerns: config, schema, provenance, credential references, paging/cursor/checkpoint semantics, sync state, and consistency expectations. Actions access datasources to do work such as `fetch`, `list`, `search`, `sync`, `map`, or `transform`. Those actions can then be used by workflows, commands, triggers/background jobs, app code, or wrapped as tools for agents.

A hypothetical support assistant is a concrete example: a documentation API plus parsing, vision extraction, condensation, and indexed search form a datasource pipeline. The agent-facing search tool is only one surface over that datasource.

### Workflow

A workflow is a reliable, typed, inspectable execution graph for agentic applications and non-agentic automation.

It exists for cases where an ad hoc command, free-form prompt, or one-off action is too ambiguous. Workflows compose `workflow.ActionRef`s into simple pipelines or more complex DAGs. `workflow.ActionRef` belongs to the workflow package because it is graph/reference/dataflow metadata; the executable target remains `action.Action`.

A workflow is not the core execution primitive. Actions are. A workflow orchestrates actions and owns dataflow, control flow, retries, step policy, context selection, and durable progress. A workflow can also be exposed as an action when commands, triggers, tools, or parent workflows need to start it through the same action execution layer.

A pipeline is the linear workflow case: a sequenced DAG with one primary path.

A concrete dogfood example is the documentation refinement loop used to evolve these docs: review source and docs, challenge the current model, identify gaps/open questions, propose refinements, apply edits, and repeat. In a harness, this could be triggered by a command such as `/refine-docs` and executed as a workflow over actions for reading source, searching docs, analyzing gaps, editing files, and reporting unresolved questions.

Workflow definitions may live wherever resource discovery can find them. The default filesystem convention for YAML specs should be:

```text
.agents/workflows/*.yaml
```

An embedded Go application may construct workflows directly in code instead of using YAML. Plugins and resource discovery should make both paths first-class.

### Capability

A capability is an attachable agent/session feature with lifecycle, context, and optional event-sourced state. Capabilities are useful for ambient, durable features such as planning, memory-like state, session-local registries, or interactive control state.

A capability is not an action, workflow, plugin, or datasource. It can expose actions for workflow/app use and separately expose tools for LLM use, but those sets are not identical: some capability actions may be internal, workflow-only, or unsafe/noisy for direct model invocation. Its defining feature is that it attaches stateful behavior to a live agent/session and can contribute context derived from that state.

Today the planner capability is a concrete example: the plan is event-sourced session state, planner context renders the current plan back into the agent context, and the `plan` tool is just an LLM-facing projection for mutating that state. Workflows should not replace capabilities; they address a different concern. Capabilities extend what an agent/session has available, while workflows orchestrate multi-step execution.

### Skill

A skill is an instruction/reference resource. Skills guide behavior and provide context. They are not the same as tools or workflows.

Today skills and exact `references/` paths can be activated at runtime and persisted across resumed sessions.

### Command

A command is a human/app/channel-facing trigger surface around an action, workflow, datasource operation, or prompt-rendering behavior.

Commands should wrap actions where they perform typed work, adding command-specific metadata such as aliases, argument hints, caller policy, slash-command wiring, and channel/user visibility. Command parsing and command results are UX/channel concerns: a command may ask the channel to render text, reset or exit an interactive loop, start an agent turn from a rendered prompt, or dispatch typed work.

Today `command.Command` has its own `Spec`, `Params`, and `Result`, and `command.Tool` exposes only explicitly agent-callable commands as `command_run`. `command.Tool` should continue to exist after the action migration as the deliberate agent-callable command projection and compatibility bridge. The target direction is not to delete commands, and not every command should become a model-callable tool. Instead, make executable command cores action-backed where useful while preserving command-specific policy, parsing, and channel result semantics.

### Channel

A channel exposes agents to users or external systems.

The current terminal CLI/REPL/UI stack is the first channel, although it is not yet factored as a generic channel. Future channels should reuse the same harness/session/control API.

Examples:

- terminal/REPL;
- TUI;
- web UI;
- HTTP request/response;
- SSE event streams;
- WebSocket;
- gRPC;
- telnet;
- proprietary RPC;
- chat surfaces.

A channel is not a tool. A channel is ingress/egress for humans or systems.

### Trigger

A trigger starts or resumes agent work from events rather than direct user input.

Examples:

- cron/scheduler;
- fixed interval loops;
- webhooks;
- file watchers;
- queue events;
- email events;
- chat events;
- system monitor ticks.

A trigger is not a channel. A channel is an interaction surface; a trigger is an event source.

### Event

An event is a fact emitted by the system. agentsdk already has two important event families:

- **runner events** — live execution events for UI/observability, such as text deltas, tool calls, usage, warnings, and errors;
- **thread events** — durable append-only events for conversation nodes, capability state, context render records, branch/session lifecycle, and future datasource/workflow/action state.

Triggers consume external events or time signals and turn them into harness work. Channels expose live events to users or systems. Workflows and datasources should emit events for observability and replay where needed.

### Adapter

An adapter connects agentsdk to a third-party service or environment-specific system.

Examples:

- helpdesk/issue-tracker/chat/email connectors;
- Tavily search;
- SIP/phone;
- Bubblewrap sandboxing;
- network interception;
- cloud/deployment providers.

## Safety as a differentiator

Safety should be a first-class product property, not a per-tool afterthought.

agentsdk already has a foundation: tool intent, middleware hooks, cmdrisk assessment, shell intent declaration, and standard toolset risk analyzer configuration. The future safety layer should generalize this across tools, actions, workflows, channels, and harness policies.

agentsdk should support:

- tool/action intent declaration;
- risk assessment;
- command risk analysis;
- approval gates;
- policy decisions;
- sandbox execution;
- filesystem boundaries;
- network policy/interception;
- secret handling/redaction;
- audit logs.

The intended flow:

```text
tool/action request
  -> declare intent
  -> assess risk
  -> apply policy
  -> optionally ask approval
  -> optionally constrain with sandbox/network policy
  -> execute
  -> persist/audit result
```

## Dogfood applications

The engineer agent is too important to remain only a tiny example. It is a dogfood app: the agent used to build agentsdk itself.

Current direction:

```text
apps/engineer/   first-party dogfood coding/architecture/devops agent
apps/builder/    first-party builder agent used by `agentsdk build`
examples/        small instructional examples only
```

Examples should remain small and educational. Dogfood apps can be larger and product-like.

## Design principle

Keep the layering simple and evolutionary:

```text
Existing runtime executes turns.
Existing thread/conversation packages persist state.
Existing app/resource/plugin packages compose applications.
Existing terminal code is the first channel.
Existing tool/intent/middleware code is the safety seed.
Workflow adds reliable orchestration of action references and durable progress.
Harness consolidates host/session/channel/trigger lifecycle.
Builder creates apps from these pieces.
```
