# Cleanup / Restructuring Plan

## Current cleanup status

Cleanup is no longer in the early ownership-drift phase. The recent work removed most of the dirty compatibility seams that were blocking the new architecture from being usable:

- `toolactivation.Manager` owns mutable tool registry / activation state.
- `agent.Instance` no longer installs hidden standard tools, hidden planner factories, terminal UI rendering, or a default product agent.
- Generic `tools/standard` and `plugins/standard` are gone.
- Default composition is explicit and named through plugins, currently most visibly `plugins/localcli`.
- `app.App` is a registry/composition host, not a channel target and not a dumping ground for agent pass-through helpers.
- `harness.Session` is the channel/session boundary for sends, commands, workflow starts, workflow run lookup, command projections, and terminal one-shot command output.
- Commands are declarative `command.Tree` roots with descriptors, structured execution, typed input binding, catalog projection, JSON rendering, and command envelope execution.
- Workflow execution options live in `workflow`, not `app`; app resolves definitions/actions, while harness records session-scoped runs to the live thread.
- `command.Result` remains structured and is rendered at presentation boundaries.
- Current writer-based output fields are transitional compatibility seams; do not expand them without a designed displayable/event publication model.

## Completed cleanup batches

- **Tool ownership and standard-bundle removal** ✅
  - Moved mutable tool activation out of old standard bundle ownership and into `agent.Instance` via `toolactivation.Manager`.
  - Renamed the old generic activation package to `toolactivation`.
  - Removed `tools/standard` and `plugins/standard` entirely.
  - Removed hidden default tools from `app.New` and `agent.New`.
  - Removed ambiguous `app.WithTools(...)`; hosts choose `app.WithDefaultTools(...)`, `app.WithCatalogTools(...)`, or plugin facets explicitly.

- **Agent/app default composition cleanup** ✅
  - Removed context-free `agent.DefaultSpec()` and moved local CLI fallback composition to `plugins/localcli`.
  - Added context-aware `app.PluginFactory` and plugin-ref loading through harness.
  - Removed concrete default planner construction from generic agent/app setup.
  - Terminal chooses named default plugins and can disable them with `--no-default-plugins`.

- **Terminal/harness loading boundary** ✅
  - Moved reusable app/default-agent/service/session loading into `harness.LoadSession`.
  - Harness now applies grouped app/agent/session load settings including source API, model policy, resume paths, plugin refs, loaded plugins, fallback-agent mechanics, and default-agent preparation.
  - Terminal remains responsible for CLI flags, local CLI default-plugin policy, terminal event handlers, debug-message output, risk-log presentation, and fallback spec selection.

- **Command system cleanup** ✅
  - Stopped growing switch/case slash-command namespaces.
  - Migrated harness commands onto declarative `command.Tree` roots.
  - Unified command metadata on `command.Descriptor` and `command.Registry`.
  - Added structured command execution via `Session.ExecuteCommand` and command envelopes.
  - Added structured notice payloads and JSON command rendering.
  - Removed redundant command surfaces such as `Session.CommandDescriptors()` and `Session.AgentCommandCatalog()`.
  - Kept agent-facing command adapters behind `Session.AgentCommandProjection()` rather than separate public tool/context constructors.

- **Workflow/session cleanup** ✅
  - Added workflow run IDs, live workflow events, `workflow.ValueRef`, run-state projection, memory/thread run stores, and thread recording.
  - Moved workflow thread recording from `app.App` to `harness.Session.ExecuteWorkflow`.
  - Removed app-level workflow command shims (`RegisterWorkflowCommand` / `WorkflowCommand`).
  - Moved workflow execution options from `app` into `workflow` (`workflow.WithRunID`, `workflow.WithEventHandler`).
  - Removed redundant `app.App.WorkflowAction(...)`; callers can use `workflow.WorkflowAction` directly when action adaptation is needed.
  - `/workflow start`, `/workflow runs`, `/workflow run <id>`, filtering, status output, and one-shot terminal rendering are in place.

- **Runtime/public-surface pruning** ✅
  - Removed stale runtime request/turn/history option shims that had no production callers.
  - Removed app/agent/harness pass-through helpers and dead accessors where there was a clearer current surface.
  - Removed concrete `tools/skills` and `tools/toolmgmt` imports from `runtime`; runtime receives neutral activation state through owner packages.

## What is intentionally still present

These are not immediate cleanup targets unless a later slice clearly deletes more than it adds:

- `agent.Instance` remains the running session-backed façade. It is still large, but the worst ownership drift has been removed.
- `agent.Instance.RegisterTools(...)` and `RegisterContextProviders(...)` remain for session projection attachment.
- `runtime.Engine.RegisterContextProviders(...)` remains because runtime owns the active context manager for future turns.
- `harness.Session.Info()`, `ParamsSummary()`, `SessionID()`, `Tracker()`, and `Out()` remain because terminal/REPL presentation still depends on them.
- `harness.Session.WorkflowRunStore`, `WorkflowRunState`, and `WorkflowRuns` remain because they are the current workflow read model used by harness commands.
- `app.App` discovery surfaces such as agents, commands, workflows, tools, skills, diagnostics, and resources remain because `agentsdk discover`, harness loading, and tests use them legitimately.
- `command.Tool` / `command_run` remains as an older command-tool bridge, while harness sessions now use the newer `session_command` projection. Do not expand the old bridge unless needed; prefer the harness projection path.

## Remaining cleanup candidates

- **Agent responsibility split**
  - Inspect before changing. Candidate responsibilities: session lifecycle, context provider lifecycle, capability registry/session ownership, skill activation persistence, and writer-shaped output.
  - Only move code if the slice deletes/collapses a real path; avoid file-only churn.

- **Displayable/event publication design**
  - Design typed displayable events/publications before replacing writer seams.
  - Goal: terminal, TUI, HTTP/SSE, JSON, and LLM-facing channels render the same structured content differently.
  - Do not touch risk logging in this cleanup pass; it is experimental and needs separate design.

- **Workflow lifecycle**
  - Current `/workflow start` is synchronous. Async lifecycle, cancellation, rerun, pagination, richer trigger/input metadata, and raw event inspection remain future work.
  - Do not create a workflow database until thread-backed run storage proves insufficient.

- **Command output descriptors / renderer design**
  - Current payload `Display(mode)` works. Add a renderer registry only if it reduces code and keeps domain models presentation-neutral.
  - Future command descriptors may include output payload metadata.

- **SDK dogfood and release readiness**
  - Keep `agentsdk run` usable as the main dogfood path.
  - Add a small worked example for the refactored local CLI plugin + command tree + workflow run UX once documentation is stable.
  - Before treating the refactor as ready for broader use, run end-to-end manual checks in addition to `go test ./...`.

## Guardrails for any next slice

- No new harness plugin system beside `app.Plugin`.
- No new command switch namespaces; use declarative `command.Tree`.
- No generic `tools/standard` or `plugins/standard` default composition packages.
- No separate profile system for plugin composition; named composition is `app.Plugin` plus `app.PluginFactory`.
- No hidden default tool bundles in `app.New` or `agent.New`.
- No command output discarded at terminal/channel boundaries.
- Do not expand writer-shaped output in harness/runtime; long-term output should become structured displayable events/publications.
- New seams should delete or collapse an old path.
- Commit only after focused and full verification pass.
