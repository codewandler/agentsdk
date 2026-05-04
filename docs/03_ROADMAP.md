# 03 — agentsdk Roadmap

## Purpose

This roadmap turns the vision and architecture into incremental work. It is grounded in what agentsdk already has: runtime, runner, thread persistence, resource discovery, app/plugin composition, terminal execution, tools, skills, capabilities, context providers, and safety primitives.

The roadmap should therefore be read as an evolution plan, not a greenfield build plan.

## Guiding rules

1. Keep `go test ./...` green after every change.
2. Preserve current `agentsdk run` and `agentsdk discover` behavior while internals evolve.
3. Reuse existing packages before creating new parallel systems.
4. Extend `resource.ContributionBundle`, `app.Plugin`, thread events, and app manifests rather than bypassing them.
5. Add missing boundaries before moving large amounts of code.
6. Let real apps validate abstractions before generalizing them; document them publicly as hypothetical or anonymized case studies.
7. Because the project is pre-1.0, prefer deleting dirty transitional APIs over preserving compatibility shims unless a shim is explicitly requested.

Plugin/contribution invariant: do not introduce a second, unrelated harness plugin system alongside `app.Plugin`. Session-owned features may be projected into agent-facing tools/context providers through session projection seams, but packaging should remain one conceptual plugin/contribution model. If session-scoped contributions need pluginization later, evolve the existing plugin model or move plugin ownership upward into `harness.Service` with app-level and session-level facets under one concept.

## Current foundation to reuse

Before adding anything, recognize the reusable pieces already present:

| Existing subsystem | Reuse for future work |
| --- | --- |
| `runtime.Engine`, `runner.RunTurn` | model/tool turn execution; workflow model-step action implementation. |
| `conversation`, `thread`, `thread/jsonlstore` | durable session and future datasource/workflow/action event persistence. |
| `runtime.ThreadRuntime` | thread-bound capability and context replay; future harness sessions. |
| `tool`, `toolactivation`, `tools/*`, `toolmw` | model-callable schema/projection plus reusable execution, intent, middleware, risk assessment patterns to migrate into action; `toolactivation.Manager` owns mutable tool activation state. |
| `capability`, `capabilities/planner` | attachable stateful agent/session features; planner remains a capability because it is event-sourced session state plus context plus action/tool projection, not a workflow. |
| `agentcontext` | selected context for turns and future workflow steps. |
| `skill` | instruction/reference resources; not a workflow replacement. |
| `command` | slash commands, caller policy, channel-result semantics, `command.Tree` for declarative subcommands/args/flags/validation/descriptors, harness `session_command` projection for agent-callable commands, and possible action-backed command adapters. |
| `agentdir`, `resource` | datasource/workflow/action resource discovery should extend this. |
| `app`, `plugins/*` | datasource/workflow/action registration should extend this plugin/app model. |
| `terminal/*` | first channel; should migrate onto harness/session APIs. |
| `agent.Spec`, `agent.Instance` | current blueprint/session façade; migrate responsibilities gradually. |

## Milestone 0 — Documentation and alignment

Status: complete.

Deliverables:

- `docs/01_VISION.md`
- `docs/02_ARCHITECTURE.md`
- `docs/03_ROADMAP.md`

Acceptance criteria:

- The project has a top-level product direction.
- Existing subsystems and their future roles are documented.
- The runtime/workflow/harness/channel/trigger distinction is documented.
- The default workflow resource location is documented as `.agents/workflows/*.yaml`.
- Migration paths are described, not just new package ideas.

Verification:

```bash
go test ./...
```

## Phasing

The milestones are ordered to avoid a rewrite:

```text
Document and preserve current behavior
  -> move dogfood apps into the right product category
  -> extend existing resource/app/plugin models for datasources/workflows/actions
  -> add execution for workflows on top of existing runtime/thread/tool systems
  -> extract harness/channel seams from the current terminal run path
  -> validate with an anonymized support-assistant case study and builder
  -> reduce package coupling after the seams are proven
```

This order is deliberate. Workflow resources and app/plugin registration should be added before a full harness rewrite, because they naturally extend systems that already exist. The harness can then host both ordinary agent turns and workflow runs.

## Milestone 1 — Promote and preserve dogfood apps

Status: complete.

Goal: distinguish real first-party agentic apps from small examples without breaking current workflows.

Current state:

- `apps/engineer` is the first-party dogfood coding/architecture/devops app.
- The stale `examples/engineer` compatibility copy has been deleted; examples stay instructional, apps stay durable dogfood products.
- It uses current agentdir/app manifest/resource behavior.

Tasks:

1. Create an `apps/` directory for first-party dogfood apps. ✅
2. Move or copy `examples/engineer` to: ✅

   ```text
   apps/engineer/
   ```

3. Delete stale compatibility documentation for the old example path. ✅
4. Add or reserve: ✅

   ```text
   apps/builder/
   ```

5. Keep `examples/` for small instructional examples. ✅
6. Update README, AGENTS notes, and example references. ✅

Acceptance criteria:

- `agentsdk run apps/engineer` works.
- The engineer app remains resource-only unless/until it needs Go extensions.
- Documentation explains why engineer is a dogfood app, not just a tiny example.
- Existing examples continue to run or have clear current-path notes.

Verification:

```bash
go test ./...
go run ./cmd/agentsdk discover apps/engineer
go run ./cmd/agentsdk run apps/engineer --help
```

## Milestone 2 — Extend resource discovery for datasources and workflows

Status: complete.

Goal: introduce datasource and workflow specs through the existing resource pipeline.

Current state:

- `agentdir` loads agents, commands, skills, datasource specs, and workflow specs from `.agents`, compatibility roots, and plugin roots.
- `resource.ContributionBundle` normalizes discovered contributions.
- `agentsdk discover` reports datasource and workflow resources.
- Datasource/workflow loading is declarative-only; execution remains later work.
- `app.App` consumes current runnable contribution types; datasource/workflow app registries are added in Milestone 4.

Tasks:

1. Add datasource and workflow resource representations to `resource.ContributionBundle`. ✅
2. Add datasource/workflow metadata and source provenance similar to skills/commands. ✅
3. Extend `agentdir` to discover: ✅

   ```text
   .agents/datasources/*.yaml
   .agents/workflows/*.yaml
   ```

4. Keep datasource/workflow loading declarative-only at first; do not require execution. ✅
5. Update `agentsdk discover` to show discovered datasource and workflow resources. ✅
6. Update `docs/RESOURCES.md` with the chosen datasource/workflow resource conventions and note whether they are agentsdk-specific. ✅

Acceptance criteria:

- A datasource YAML file under `.agents/datasources/` is discoverable.
- A workflow YAML file under `.agents/workflows/` is discoverable.
- Discovery output includes datasource/workflow name/source/diagnostics.
- Existing agent/command/skill discovery is unchanged.

Verification:

```bash
go test ./agentdir/... ./resource/... ./cmd/agentsdk/...
go run ./cmd/agentsdk discover testdata-or-example-path
```

## Milestone 3 — Add datasource/workflow/action core model

Goal: define datasources, workflows, and actions as first-class domain concepts, with a top-level `action` package as the central execution primitive. Actions must be independent of LLMs/tools; tool and command surfaces wrap or trigger actions.

Status: mostly complete for the initial Go-defined model.

Current implementation includes a top-level `action` package, standalone `datasource` package, `workflow` definitions/executor, `action.Ref`, `workflow.ActionRef`, action/tool adapters, datasource/workflow validation, and tests. Remaining Milestone 3 work is mostly refinement: richer datasource schema semantics, broader adapter coverage, and continued migration of legacy tool concepts where useful.
Current state to reuse:

- `tool.TypedTool` already has typed params, schema generation, execution, result formatting, intent declaration, context, and middleware patterns. Execution, result, intent, context, events, and middleware are action responsibilities currently living near the tool package; JSON schema generation and transcript formatting are tool-surface responsibilities.
- `command.Command` already models slash-command invocation with command-specific policy and channel result kinds. Commands should trigger actions/workflows or render prompts; they should not become the base execution abstraction, and their params/results should not be confused with typed action schemas/results.
- `thread.EventDefinition` already supports registering typed event payloads.
- `capability` already provides attach/replay/state-event machinery for ambient session features such as planner; capabilities should be able to expose actions for workflow/app use and a separate deliberate tool subset/projection for LLM use.
- A representative support-assistant case study has a concrete datasource-shaped pipeline: documentation API client, HTML parser, vision extraction, condensation, and markdown/index output.

Tasks:

1. Add `workflow` package. ✅
2. Define domain types: ✅ initial Go-defined model exists

   ```text
   Datasource
   DatasourceKind
   Config schema
   Record/item schema
   Provenance metadata
   Credential/config references
   Paging/cursor/checkpoint state
   Consistency/freshness expectations
   Workflow
   Pipeline
   Step
   Edge
   ActionRef
   Action
   Description
   Input action.Type
   Output action.Type
   Intent declaration
   action.Ctx
   action.Result: Status, Data any, Error error, Events []action.Event where action.Event is an alias for any
   Result contract
   Middleware chain
   ```

3. Define datasource as a data boundary accessed by actions, including config schema, record/item schema, provenance, credential/config references, paging/cursor/checkpoint state, and consistency/freshness expectations. ✅ initial standalone `datasource` package exists
4. Define how datasources provide or reference standard actions such as `fetch`, `list`, `search`, `sync`, `map`, and `transform`. ✅ via `datasource.Actions` and `action.Ref`
5. Define `action.Action` as the owner of Go-native execution metadata currently mixed into tools: name, description, input/output `action.Type`, intent declaration, `action.Ctx`, `action.Result`, execution function, emitted events, and middleware chain. ✅
6. Define `action.Type` as the reusable input/output contract value that carries Go `reflect.Type` plus optional `*jsonschema.Schema` metadata, with helper methods for creating values, encoding, decoding, validation, and schema projection added as needed. ✅ initial implementation exists
7. Move middleware concepts completely to `action.*`; keep `tool` middleware as aliases/adapters during migration. ✅ action middleware exists; compatibility migration remains incremental
8. Define `tool.Tool` as embedding or wrapping `action.Action`, adding LLM-facing concerns such as guidance, activation, provider/tool-call projection, serializable schema constraints, and transcript rendering. ✅ via action-backed tool projection
9. Decide how `tool.Ctx`, `tool.Result`, and `tool.Intent` alias/adapt to `action.Ctx`, `action.Result`, and action intent for compatibility, while keeping tool JSON serialization/schema constraints as tool-specific projection concerns. ✅ initial aliases/adapters exist
10. Add adapters: action-to-tool, tool-to-action for legacy tools, command-triggering-action/action-backed-command where useful, keep harness `session_command` as the deliberate agent-callable command projection, and define command-result mapping for channel-triggered actions/workflows. ✅ command-triggered workflows and command envelope adapters now exist; further command/action adapters remain future refinements
11. Support Go-defined datasources, workflows, and actions first. ✅
12. Add tests for model validation, `action.Type` construction/validation, datasource action references, step references, adapters, middleware ordering, and simple pipeline construction. ✅ initial coverage exists

Acceptance criteria:

- Datasources/workflows/actions can be constructed in Go.
- A datasource can expose at least one typed action.
- A pipeline is represented as a workflow with sequenced edges.
- Invalid datasource/action/workflow references are caught by validation.
- `action.Action`, `action.Ctx`, `action.Result`, emitted events, action intent, and action middleware are the canonical Go-native execution design, with existing tool concepts aliased or compatibility-adapted rather than duplicated.

Verification:

```bash
go test ./workflow/...
go test ./tool/... ./command/...
```

## Milestone 4 — Extend app/plugin composition for datasources/workflows/actions

Goal: make datasources/workflows/actions part of the existing app/plugin model.

Status: complete for the initial app/plugin composition slice.

Current implementation adds app/plugin facets and app-level registries for actions, datasource definitions, and workflow definitions. `app.App` consumes resource-bundle datasource/workflow contributions and plugin-contributed actions/datasources/workflows. `app.App` is currently the composition root/registry host; longer-term harness/session code should own process and execution lifecycle rather than expanding `app.App` into a permanent god object.
Current state:

- `app.Plugin` facets already contribute commands, agent specs, tools, skill sources, context providers, and middleware.
- `app.App` owns command registry, agent specs, tool catalog, skill sources, context providers, plugin registrations, and resource bundles.

Tasks:

1. Add plugin facets for datasources/workflows/actions. ✅
2. Add app-level registries for datasources/workflows/actions. ✅
3. Register datasource/workflow/action contributions from resource bundles. ✅ datasource/workflow resources; action implementations are Go/plugin-defined
4. Register datasource/workflow/action contributions from plugins. ✅
5. Add app APIs to list/get datasources/workflows/actions. ✅
6. Keep existing tool/command/skill behavior unchanged. ✅

Acceptance criteria:

- A plugin can contribute a datasource and action implementation.
- A resource bundle can contribute datasource and workflow definitions.
- `app.App` can resolve datasource definitions, workflow definitions, and action implementations.
- No separate datasource/workflow plugin system exists.

Verification:

```bash
go test ./app/... ./plugins/... ./resource/...
```

## Milestone 5 — Minimal workflow executor

Goal: execute a simple sequential pipeline using `action.Action` while adapting existing runtime/tool/command infrastructure.

Current state to reuse:

Status: partially complete.

Current implementation supports Go-defined sequential workflows over `action.Action`, app-owned workflow execution through `App.ExecuteWorkflow`, workflow-as-action exposure, harness-owned slash-command workflow triggers through `command.Tree`, and concrete workflow event payload structs returned through `action.Result.Events` plus an optional live event handler. Workflow event definitions are registered in the same `thread.EventDefinition` style used elsewhere so persistence adapters can map concrete payloads into thread events.

These workflow events are live telemetry shaped for persistence. Workflow now has run identity, materialized `workflow.RunState`, `workflow.RunSummary`, started/completed timing, duration, step attempt metadata, `workflow.ValueRef` output references, a projector that rebuilds run/step/attempt status and outputs from concrete workflow events, a context-aware `workflow.RunStore` contract, an in-memory implementation, a thread event recorder, and a thread-backed run store scoped to a thread/branch. Workflow-level execution options cover run ID and event handlers; `App.ExecuteWorkflow` accepts those options but stays registry/executor-focused and does not auto-record to a default agent thread. Harness session execution wires `workflow.ThreadRecorder` for the session agent's live thread when available, and harness session APIs expose single-run lookup and run-summary listing from the thread-backed projection. Runtime step dataflow remains Go-native `any`; workflow events and projected state use inline, external, or redacted value references. Remaining work includes richer validation/output contracts, chronological/indexed run listing, and harness-owned multi-session workflow lifecycle.

- `runtime.Engine` can run model/tool turns and can be wrapped by prompt/model-turn actions.
- Existing `tool.Tool` values can be adapted to actions during migration, but new workflow code should depend on `action.Action` through `workflow.ActionRef` resolution.
- Existing `command.Command` values can trigger actions or workflows where appropriate; command parsing and channel-result semantics remain outside `action.Result`.
- `agentcontext.Manager` can provide context fragments.
- Capabilities can provide context and state that workflow steps may require, but workflow execution state should remain workflow events by default.
- `thread.Event` can persist execution records.

Tasks:

1. Implement a minimal workflow executor for sequential pipelines. ✅
2. Define `workflow.ActionRef` as the graph-level reference to an `action.Action`; workflow owns references/dataflow, action owns execution. ✅ aliases `action.Ref`
3. Support initial action implementations:

   - prompt/model-turn action using `runtime.Engine` or `agent.Instance` initially; ✅ `agent.TurnAction` exists; app default-agent helper was removed in favor of explicit instance composition
   - legacy tool adapter action wrapping `tool.Tool` where needed;
   - workflow-as-action adapter so triggers, tools, explicit action registration, and parent workflows can start a workflow through the action layer; ✅ `workflow.WorkflowAction` exists; the redundant app helper was removed
   - command trigger invoking an action or workflow where appropriate, with explicit mapping from action/workflow result to command/channel result; ✅ harness `/workflow start` command exists on the declarative command tree
   - no-op/transform action for tests.

4. Add a dogfood workflow example for the documentation refinement loop used to evolve these docs: ✅ initial resource workflow fixture exercises workflow loading, agent-turn action execution, and thread-backed run recording

   ```text
   /refine-docs
     -> review_source
     -> challenge_docs
     -> identify_gaps_and_open_questions
     -> propose_refinements
     -> refine_docs
     -> report_summary
   ```

   This should be documented as a command-triggered workflow where each step is an action and the workflow can be rerun iteratively.

5. Emit workflow/action events to the existing thread event log when a thread is available; emit datasource events for sync/checkpoint state when relevant. ✅ workflow event payloads, `thread.EventDefinition`s, run IDs, `ValueRef` outputs, run-state projection, `workflow.ThreadRecorder`, `workflow.ThreadRunStore`, workflow execution options, and session-owned live-thread recording through `harness.Session.ExecuteWorkflow` exist; broader async lifecycle remains future work.
6. Add per-step input/output passing. ✅ initial dependency-output passing exists
7. Add a minimal workflow run store. ✅ context-aware `workflow.RunStore`, `workflow.MemoryRunStore`, and `workflow.ThreadRunStore` exist
8. Add workflow run summaries/listing. ✅ `workflow.RunSummary`, projected timing/duration, `ThreadRunStore.Runs`, `Session.WorkflowRuns`, and `/workflow runs` exist
9. Add harness workflow start command. ✅ `/workflow start <name> [input]` synchronously executes workflows and returns the run ID
10. Add basic output validation. Not started
11. Defer parallel DAG execution until sequential pipeline semantics are proven.

Acceptance criteria:

- A Go-defined pipeline can execute end-to-end. ✅
- Workflow steps use `workflow.ActionRef` resolved to `action.Action`. ✅
- Output from one step can feed the next. ✅
- A prompt/model-turn can run as an action. ✅ `agent.TurnAction` exposes an `agent.Instance` turn as an `action.Action`; hosts register the desired instance explicitly as a workflow action
- A workflow can be exposed as an action. ✅
- A command can trigger a workflow through `harness.Session`; an initial dogfood workflow resource fixture exercises the resource-defined workflow path.
- Action intent can be inspected before execution, including actions exposed as tools.
- Execution is observable through events. ✅ initial in-memory workflow events exist
- Workflow run event/state access has an explicit store boundary. ✅ context-aware `workflow.RunStore` with memory and thread-backed implementations exists
- Thread-backed runs can persist workflow events. ✅ `workflow.ThreadRecorder` and `workflow.ThreadRunStore` exist for thread logs; `harness.Session.ExecuteWorkflow` records to the session agent live thread when available, while `app.App` remains registry/executor-focused
- Thread-backed workflow runs can be started, listed, filtered, and inspected through the harness. ✅ `/workflow start <name> [input]`, `Session.WorkflowRuns`, `Session.WorkflowRunState`, `/workflow runs`, `/workflow runs --workflow <name>`, `/workflow runs --status <status>`, and `/workflow run <id>` exist

Verification:

```bash
go test ./workflow/... ./runtime/... ./thread/...
```

## Milestone 6 — Minimal harness wrapping existing app/agent flow

Goal: consolidate current app/session setup without rewriting runtime.

Current state:

- `terminal/cli.Load` resolves terminal/CLI policy, resources, plugin defaults/flags, session flags, and channel adapters, while reusable app/default-agent/session/plugin loading mechanics now live in `harness.LoadSession` and related helpers.
- `app.App` registers resources/plugins and instantiates agents.
- `agent.Instance` owns runtime/session setup.
- `harness.Service` and `harness.Session` wrap the existing app/default-agent stack.
- Harness exposes session metadata plus session-scoped workflow run lookup/listing over the default agent live thread.
- Terminal send paths route through `harness.Session`, so harness can own session-aware slash-command namespaces such as `/session` and `/workflow`.
- Harness commands are backed by declarative `command.Tree` definitions and exposed through `Session.Commands().Descriptors()` and `Session.ExecuteCommand` for structured, non-stringified command execution.
- Default harness sessions attach the command projection automatically: the `session_command` tool and agent command catalog context provider are available to agent turns, while `AgentCallable` policy still filters executable commands.
- Terminal one-shot mode renders returned `command.Result` payloads instead of discarding command output.
- Current harness load configuration still has writer-based compatibility fields for the existing app/agent terminal path. These exist only because the current app/agent/terminal stack still accepts writers; they should not become the long-term channel output model.

Tasks:

1. Add `harness` package. ✅
2. First implementation should wrap existing objects: ✅

   ```text
   harness.Service
     contains app.App
     opens/owns agent.Instance sessions
     forwards input to Instance.RunTurn
     emits runner events
   ```

3. Move the reusable parts of `terminal/cli.Load` toward harness loading functions. ✅ `harness.LoadSession` owns app/default-agent/service/session construction, loaded plugin application, and grouped app/agent/session load settings such as source API, model policy, and resume-session paths; `harness.ResolveAgentLoadConfig` owns model-policy/source-API load composition; `harness.ResolvePlugins` owns generic default/manifest/explicit plugin-ref resolution mechanics; `harness.EnsureFallbackAgent` owns fallback-agent injection mechanics; and `harness.PrepareResolvedAgent` owns generic default-agent selection plus agent-spec overrides.
4. Keep `terminal/cli.Load` as compatibility wrapper initially. ✅
5. Add session IDs and thread/session store handling through harness APIs where possible. ✅ `Session.Info`, `Session.AgentName`, `Session.ThreadID`, `/session info`, and workflow read APIs exist

6. Expose session-owned agent projections without creating a second plugin system. ✅ `harness.AgentProjection`, `Session.AgentCommandProjection`, explicit `Session.AttachAgentProjection`, default command-projection attachment in `harness.Service.DefaultSession`, and agent registration APIs exist for command tools and command-catalog context providers.

Acceptance criteria:

- Harness can load resources using existing `agentdir`/`resource`/`app` paths.
- Harness can instantiate the default agent through existing `app.App` APIs. ✅
- Harness can run a turn and expose events. ✅ initial send path exists; richer event subscription remains future work
- Harness can expose thread-backed workflow run state/history for the current session. ✅
- `agentsdk run` behavior remains unchanged for ordinary tasks, and one-shot command tasks now print structured command results. ✅

Verification:

```bash
go test ./harness/... ./terminal/... ./app/... ./agent/...
go run ./cmd/agentsdk run apps-or-examples-path
```

## Milestone 7 — Terminal becomes first channel over harness

Goal: make the existing terminal stack the first implementation of a channel boundary.

Current state:

- Terminal code works and now delegates reusable app/default-agent/session/plugin setup to harness while keeping CLI-specific policy in `terminal/cli.Load`.
- Runner events already map well to terminal rendering, but direct writer plumbing remains a transitional compatibility seam.
- Terminal one-shot and REPL sends route through `harness.Session`, which lets session-scoped commands such as `/workflow runs` use harness APIs.

Tasks:

1. Add minimal `channel` package.
2. Define channel host/session interfaces based on what terminal actually needs.
3. Adapt terminal REPL/UI to use harness APIs.
4. Keep terminal rendering in `terminal/ui`.
5. Design a structured displayable/publication model before replacing writer-shaped output seams. Future channels should receive typed displayables/events that terminal, HTTP/SSE, TUI, JSON, and LLM-facing frontends can render differently.
6. Keep CLI flags and UX stable.

Acceptance criteria:

- Terminal does not need to know low-level runtime construction details.
- Terminal still supports tasks, REPL, session resume, verbose/debug output, and slash commands.
- Harness can theoretically host another channel with the same session API.

Verification:

```bash
go test ./channel/... ./terminal/... ./harness/...
go run ./cmd/agentsdk run apps/engineer
```

## Command tree refactor gate

Status: initial gate complete.

The current command model has a declarative `command.Tree` in the existing `command` package. It supports subcommands, declared positional args, declared flags, enum/required validation, generated usage/help from descriptors, structured invocation handlers, and structured execution without converting input maps into slash-command strings. Harness `/workflow` and `/session` are tree-backed, and sessions expose:

```go
func (s *Session) Commands() (*command.Registry, error)
func (s *Session) ExecuteCommand(ctx context.Context, path []string, input map[string]any) (command.Result, error)
```

Remaining command-tree follow-ups:

```text
Add typed command input binding similar to action.NewTyped
Expose output payload metadata in descriptors
Project selected command trees into richer LLM-safe tool schemas only if the generic command envelope plus catalog context proves insufficient
Add more channels over Session.ExecuteCommand instead of adding channel-specific parsers
```

Do not add new broad command namespaces outside the command tree model.

## Workflow/harness follow-up backlog

Near-term workflow UX and read-model follow-ups:

- Make `/workflow start <name> [input]` asynchronous once harness owns workflow lifecycle beyond the current request.
- Add `/workflow runs --workflow <name>` filtering. ✅
- Add `/workflow runs --status succeeded|failed|running` filtering. ✅
- Add chronological ordering for `/workflow runs`; current ordering is deterministic by run ID.
- Carry started/completed timestamps and duration in `workflow.RunSummary`. ✅ basic projected timing exists; richer trigger/source/input metadata remains future work.
- Continue reducing presentation-specific command formatting by expanding structured payloads/renderers. Generic notice payloads, structured command result payloads, JSON rendering, and `Display(mode)` rendering exist; richer output payload descriptors and a renderer registry remain future work only if they reduce code.
- Design typed displayable events/publications for user-visible output. Treat `harness.SessionLoadConfig.Output`, app/agent output options, debug-message output, and terminal event handlers as compatibility seams until that design exists. Leave risk logging as a separate experiment that needs its own design before migration.
- Include richer workflow definition metadata in `/workflow show <name>` when definitions gain input/output schemas, defaults, policy, and step descriptions.

Medium-term workflow lifecycle follow-ups:

- Persist richer run metadata: trigger/source, invoking command, input reference, agent/session identity, started/completed times, duration, and output reference.
- Add pagination for large thread logs and run lists.
- Add `/workflow rerun <id>` once inputs and action versioning are represented well enough.
- Add `/workflow events <id>` for debugging raw workflow event history.
- Add `/workflow cancel <id>` only after workflows can run asynchronously outside a single synchronous call stack.
- Decide whether `ThreadRunStore.Runs` should remain pure projection or whether a separate indexed read model is needed for large histories.

Longer-term workflow product follow-ups:

- Durable indexed run store separate from thread projection if thread-log scans become too expensive.
- Cross-session workflow run lookup for app/operator views.
- Web/UI workflow run browser.
- Workflow graph visualization.
- Approval gates, resumable checkpoints, and human-in-the-loop workflow steps.
- Trigger-owned workflow starts with durable source metadata.

## Milestone 8 — Safety policy expansion

Goal: evolve existing tool intent/middleware/cmdrisk into an action-centered safety layer that tools, commands, workflows, datasources, and triggers share.

Current state:

- `tool.Intent` exists.
- `IntentProvider` exists.
- `toolmw.CmdRiskAssessor` exists.
- Bash declares intent with cmdrisk analysis.
- Standard toolset can configure risk analyzer.
- Terminal currently has log-only risk middleware.

Tasks:

1. Define safety decision types: allow, deny, require approval, require sandbox, require network policy.
2. Move or adapt intent concepts to `action.*` so workflow actions, datasource actions, command-triggered actions, and tool-exposed actions share risk semantics.
3. Move middleware concepts to action execution; keep current tool middleware working through aliases/adapters.
4. Add approval interfaces that terminal channel can satisfy first.
5. Add audit event payloads using thread events.
6. Prepare adapter interfaces for Bubblewrap/network interception, but do not implement all sandboxing yet.

Acceptance criteria:

- Tool and action calls can be assessed before execution.
- Terminal can display/handle approval prompts for high-risk actions.
- Decisions are observable and, when thread-backed, persisted.
- Existing cmdrisk behavior still works.

Verification:

```bash
go test ./tool/... ./toolmw/... ./tools/shell/... ./workflow/...
```

## Milestone 9 — Harness daemon/service mode and triggers

Goal: support long-running background/event-driven work while reusing harness sessions.

This milestone is intentionally before concrete datasource expansion. Scheduled/background execution was one of the original refactor drivers, and it should prove the harness process/session lifecycle before more datasource-specific abstractions are added.

Current state:

- `harness.Service` and `harness.Session` already provide session open/resume/list/close, command execution, workflow execution, and event subscription seams.
- No generic trigger abstraction exists yet.
- The terminal CLI currently hosts a harness session for one-shot and REPL usage, but there is no service/daemon command shape or REPL job/trigger control yet.

Tasks:

1. Treat daemon as a harness deployment mode, not a separate product concept.
2. Use `agentsdk serve` as the long-running host command.
3. Add a slim daemon package wrapper above `harness.Service` for process/config/trigger ownership while keeping harness as the runtime/session owner.
4. Add service-like lifecycle coverage for long-running harness/daemon hosts.
5. Define trigger source and trigger/job sink interfaces.
6. Define config for trigger targets, session mode, interval, and input/prompt.
7. Implement interval trigger as first proof.
8. Route interval triggers to harness sessions using explicit session modes: shared, trigger-owned, ephemeral, or resume-or-create.
9. Prefer workflow targets for scheduled work; support prompt targets for simple repeated prompts and direct action targets only where policy/context are explicit.
10. Enforce one active run per trigger by default; skip overlapping fires with no overlap-policy config initially.
11. Persist/observe trigger-caused work with source metadata.
12. Expose trigger/job inspection through daemon APIs and normal REPL slash commands such as `/triggers` or `/jobs`.
13. Keep trigger implementation separate from channels; daemon mode is the same host without the main interactive agent I/O.
14. Defer datasource resource/runtime expansion until daemon and triggers are proven.

Acceptance criteria:

- A service-like harness/daemon host can run without an interactive REPL.
- A trigger can start/resume work through harness.
- An interval trigger can send a prompt into a configured target session mode.
- A trigger can start a workflow with source metadata.
- A normal `agentsdk run` REPL can start/list/stop repeating jobs in its current harness/session.
- Trigger source metadata appears in thread/runtime/workflow events where persistence is available.
- Trigger implementation is separate from terminal and HTTP channels.
- Datasource work remains deferred unless a concrete case study needs it.

Open-question checkpoint:

- See [`docs/14_DAEMON_TRIGGER_SCHEDULING.md`](14_DAEMON_TRIGGER_SCHEDULING.md) for settled decisions and remaining implementation notes around CLI shape, daemon wrapper, session modes, targets, config, persistence, overlap, safety, observability, and REPL jobs.

Verification:

```bash
go test ./trigger/... ./harness/... ./workflow/...
go test ./terminal/cli/... ./cmd/agentsdk/...
```

## Milestone 10 — Support-assistant case study validation

Goal: validate the architecture with an anonymized support-assistant case study that exercises datasources, workflows, actions, tools, resources, and channels without encoding company-specific details in this public repository.

The case study should represent a realistic support assistant, but all public docs and fixtures must use generic names and synthetic data. Private application repositories can keep their real connector names, prompts, endpoints, and customer-specific details.

Representative scaffold to validate:

- a resource app with `agentsdk.app.json`, a default orchestrator agent, and local-only discovery;
- one developer/configuration agent for implementation-heavy tasks;
- command resources for lookup, summarize, debug, and scaffold-style tasks;
- a documentation datasource pipeline:
  - documentation API client with pagination and metadata;
  - HTML parser preserving text/image interleaving;
  - optional vision extractor for screenshot descriptions;
  - optional condenser for retrieval-optimized documents;
  - output writer or indexer for searchable documents;
  - CLI entry point exposing bounded sync runs.

Recommended scope:

1. Treat the documentation corpus as the first concrete **datasource** case study.
2. Model the ingestion flow as a pipeline of actions:
   - fetch collections/documents;
   - parse document HTML or markdown;
   - extract screenshot knowledge where enabled;
   - condense/normalize document content;
   - write or index searchable documents.
3. Expose datasource actions as tools only where the agent needs direct access:
   - search documentation;
   - fetch source document;
   - cite source metadata.
4. Keep the resource app as the first app-level integration point.
5. Run the agent manually from CLI first; add triggers only after the datasource pipeline is reliable.
6. Add ticket/helpdesk triage later as a second datasource/workflow after the documentation datasource proves the model.

Suggested first workflow/pipeline:

```text
sync_documentation
  -> fetch_documents
  -> parse_document_content
  -> extract_image_knowledge
  -> condense_document
  -> write_documents
  -> build_or_refresh_search_index
```

Suggested first agent-facing tools over that datasource:

```text
docs_search(query) -> ranked source snippets
docs_fetch(document_id | url | title) -> full sourced document
```

Acceptance criteria:

- A bounded sync command can run the documentation pipeline against synthetic or sanitized fixtures.
- The datasource pipeline can process text and image-bearing documents without losing source URL/title/frontmatter/provenance.
- Vision extraction can be disabled for cheap deterministic runs and enabled for screenshot-sensitive runs.
- The orchestrator agent can answer at least five sourced documentation questions using generated documents or a search tool.
- At least one question must require screenshot-derived knowledge from a synthetic fixture.
- The case study identifies what belongs in agentsdk generically:
  - datasource model;
  - action/pipeline model;
  - search/index tool pattern;
  - provenance/citation metadata;
  - triggerable sync semantics.
- The case study identifies what remains app-specific/private:
  - product prompts;
  - connector quirks;
  - condensation prompts;
  - product-specific skills and commands;
  - real endpoints, credentials, customer data, and proprietary terminology.

Verification:

Use synthetic or sanitized fixtures only:

```bash
go test ./...
go run ./cmd/sync-docs --fixture ./testdata/docs --no-vision --no-condense --limit 5 -v
agentsdk run ./testdata/support-app "Summarize how to configure the sample workflow, citing sources."
```

## Milestone 11 — Builder app MVP

Goal: ship the first useful `agentsdk build` using existing app/resource/runtime pieces.

Current state to reuse:

- `apps/engineer`/dogfood agent pattern.
- Agentdir/app manifest/resource formats.
- Filesystem/shell/git tools.
- Planner capability as the first dogfood example of event-sourced session state exposed through context and action/tool surfaces.
- Terminal channel.
- Future datasource/workflow/action scaffolding.

Tasks:

1. Create `apps/builder` as an agentsdk app.
2. Add `agentsdk build` command path.
3. Start with guided scaffolding, not magic generation.
4. Generate resource-only apps first.
5. Add hybrid Go plugin/action/tool scaffolding next.
6. Add deployment templates later.
7. Use `agentsdk discover` and test commands as verification steps.

Builder output levels:

1. Resource-only YAML/Markdown.
2. Hybrid resources plus generated Go extension code.
3. Full app with custom Go harness code.
4. Deployment-ready app with Docker/Helm/CI assets.

Acceptance criteria for MVP:

- User can run `agentsdk build`.
- Builder asks basic requirements questions.
- Builder creates an agentdir/app manifest.
- Builder can create a workflow skeleton.
- Builder can run or print verification steps.

Verification:

```bash
go test ./...
go run ./cmd/agentsdk build
```

## Milestone 12 — HTTP/SSE channel

Goal: prove that terminal is not special by adding a second channel.

Prerequisite:

- Harness/session API is stable enough from terminal migration.

Tasks:

1. Add HTTP request/response channel.
2. Add SSE event stream for runtime/workflow events.
3. Keep protocol minimal and versioned.
4. Avoid full web UI until channel semantics are stable.

Acceptance criteria:

- A client can send a request to an agent session over HTTP.
- A client can receive streamed events over SSE.
- Terminal and HTTP channels share harness/session behavior.

Verification:

```bash
go test ./channels/http/... ./harness/...
```

## Milestone 13 — Dependency cleanup

Goal: reduce coupling after new boundaries have proven useful.

Tasks:

1. Remove concrete tool imports from `runtime`. ✅
2. Move terminal-specific event rendering out of `agent` into the terminal boundary. ✅
3. Shrink `agent.Instance` toward a compatibility façade over harness/session/runtime pieces. In progress: terminal rendering, hidden standard tools, hidden planner factory construction, generic standard composition, and many pass-through public helpers have moved out or been deleted.
4. Move default-heavy app wiring out of `app.New` where appropriate. ✅ standard tools are now host-supplied; capability factories now flow through plugin facets.
5. Remove generic “standard” default composition. ✅ There is no context-free standard tool/plugin set; `tools/standard`, `plugins/standard`, and `agent.DefaultSpec` have been replaced by named use-case/environment plugins.
6. Move product/environment integrations into named plugins or adapters as they are added.

### Milestone 13a — Replace hardcoded default composition with plugins

Goal: stop hardcoding product/use-case composition in generic packages or the terminal channel.

Current issue:

- ✅ `agent/default.go` has been removed; the fallback terminal agent now lives with the named `local_cli` plugin.
- ✅ `terminal/cli` no longer imports `tools/standard` or concrete planner plugin wiring for defaults; it activates plugin refs.
- ✅ `tools/standard` and `plugins/standard` have been deleted; named plugins now own first-party composition.

Target:

- Default composition is declared by app/resource config, the local CLI plugin, or explicit CLI flags.
- Generic packages (`agent`, `runtime`, `app`) do not define default agents, default tools, or default capabilities.
- `terminal/cli` applies plugin refs and CLI overrides; it does not directly activate planner or standard tools in Go.
- First-party bundles are named by purpose, e.g. `local_cli`, `development`, `research`, or `apps/engineer`.

Tasks:

1. Define a minimal plugin declaration shape in app/resource config. ✅ JSON app manifests support plugin refs.
2. Add a host plugin factory registry so config can reference plugins by name without hardcoding active plugins in `terminal/cli`. ✅ `app.PluginFactory` is context-aware and the local CLI factory resolves built-in plugin refs.
3. Move the fallback terminal agent out of `agent.DefaultSpec` into the local CLI plugin. ✅ moved into `plugins/localcli` as the named local CLI default agent.
4. Replace terminal hardcoded `standard.DefaultTools`, `standard.CatalogTools`, and planner plugin activation with plugin-driven declarations plus optional CLI flags such as `--plugin` and `--no-default-plugins`. ✅
5. Introduce named first-party plugins only where they describe a real use case or environment. In progress: `plugins/localcli` exists and first-party concrete plugins remain purpose-named.
6. Stop using `tools/standard`/`plugins/standard`; delete them once callers are migrated. ✅

Acceptance criteria:

- `agent` has no product/use-case default spec and no concrete planner factory default. ✅
- `terminal/cli` does not import `tools/standard`, `plugins/standard`, or concrete capability plugins only to activate defaults. ✅
- Running without resources still works via an explicitly named local CLI fallback plugin. ✅
- Running with resources uses plugin declarations from config unless overridden by CLI flags. ✅ initial manifest/CLI refs exist

Verification:

```bash
go test ./agent/... ./app/... ./agentdir/... ./resource/... ./terminal/cli/... ./harness/...
go test ./...
```

Acceptance criteria:

- `runtime` no longer imports concrete `tools/*` packages.
- `agent` no longer imports terminal UI.
- Terminal is a channel over harness.
- Existing public API either still works or has explicit migration notes.

Verification:

```bash
go list -f '{{.ImportPath}} -> {{join .Imports " "}}' ./...
go test ./...
```

## Deferred work

- Full web UI.
- Advanced TUI.
- gRPC/WebSocket/telnet channels.
- Parallel DAG workflow executor.
- Durable workflow pause/resume/wait semantics.
- Workflow-as-action composition for nested workflows and reusable automations.
- Advanced sub-agent orchestration semantics.
- Bubblewrap sandbox adapter.
- Network interception adapter.
- Remote plugin distribution/trust model.
- Hosted control plane, if ever desired.

## Near-term recommendation

The root [`ROADMAP.md`](../ROADMAP.md) is the short contributor backlog. This file remains the canonical architecture roadmap.

The next practical sequence should be:

1. align the tasklist around daemon/service mode and trigger scheduling before touching datasource work;
2. prove service-like harness lifecycle and interval-triggered agent prompts through small tests;
3. add workflow trigger support only after prompt triggers work;
4. continue harness/channel cleanup only where it deletes or collapses remaining setup paths now that generic load mechanics are behind harness;
5. defer concrete datasource semantics until daemon/triggers and one concrete datasource case study justify the abstraction.

This keeps the architecture grounded in working code while addressing the original daemon/trigger motivation before adding fresh datasource surface area.
