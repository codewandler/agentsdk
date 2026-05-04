# Execution primitives

## Action/tool convergence

This checkpoint settles the boundary between `action.Action` and `tool.Tool` after the workflow and command refactors.

## Decision summary

- `action.Action` is the surface-neutral, Go-native execution primitive.
- `tool.Tool` remains the LLM-facing projection used by model tool-call loops.
- New reusable execution should prefer actions; model exposure should be explicit through `tool.FromAction(...)`.
- Existing tools do not need a broad rewrite. Adapt them to actions only when the action is useful outside the LLM loop or removes duplicated workflow/command code.
- Tool concepts should not re-declare action concepts unless they are genuinely LLM-facing projection concerns.

## Boundaries

### Action

An action owns reusable execution metadata and behavior:

- `action.Spec.Name` / `Description`
- input and output `action.Type`
- Go-native execution through `Execute(action.Ctx, any) action.Result`
- surface-neutral intent through `action.Intent`
- action middleware through `action.Middleware`
- emitted runtime/workflow events through `action.Result.Events`

Actions intentionally do not know about model provider formats, tool-call transcript rendering, activation state, guidance strings, or LLM-safe result text.

### Tool

A tool owns LLM-facing projection concerns:

- provider/tool-call schema projection
- JSON argument decoding from model tool calls
- LLM-visible guidance
- deterministic LLM-visible result rendering via `tool.Result.String()`
- model-facing tool error signaling via `tool.Result.IsError()`
- activation and risk middleware used during agent turns

`tool.Ctx` embeds `action.Ctx` but keeps tool/session metadata (`WorkDir`, `AgentID`, `SessionID`, `Extra`) because legacy tools and model-facing execution need those fields.

## Adapter policy

### Action to tool

Use `tool.FromAction(action.Action, ...)` when an action should be callable by the LLM.

The adapter:

- decodes JSON input using `action.Type.DecodeJSON` when an input type exists;
- exposes `action.Spec` name, description, and input schema as the tool metadata;
- maps `action.Result` into `tool.Result`;
- supports LLM guidance with `tool.WithActionGuidance(...)`;
- supports action middleware with `tool.WithActionMiddleware(...)` and `tool.ApplyAction(...)`.

This is the safe default adapter because it projects from the more general execution primitive into the narrower LLM surface.

### Tool to action

Use `tool.ToAction(tool.Tool)` only for migration paths or workflow reuse of a legacy tool.

Trade-off: a legacy tool may depend on `tool.Ctx` and may encode LLM-facing behavior in `tool.Result`. The adapter therefore requires the runtime context to satisfy `tool.Ctx`; callers that only have `context.Context` should create a native action instead of adapting a tool.

### Action to command

Do not add generic action-to-command adapters unless they delete real boilerplate. Commands remain the user/channel projection. The existing command action seams are sufficient for current harness and workflow usage.

## First-party tool migration

First-party tools should gain action-backed constructors only when useful. The current small example is `tools/jsonquery`:

- `jsonquery.Action()` provides a reusable action for workflows or app composition.
- `jsonquery.Tool()` remains the model-facing tool projection via `tool.FromAction(jsonquery.Action())`.

Do not mass-migrate shell, filesystem, web, git, or capability-management tools in one pass. Those tools carry mature LLM-facing result and risk semantics; converting them should be incremental and justified by actual reuse outside model tool calls.

## Intent model

`tool.Intent` aliases `action.Intent`. `action.Intent.Normalize(...)` mirrors action/tool compatibility fields so both action-native and legacy tool policy code read the same semantic intent during migration.

Recommended authoring style:

- new action code sets `Action` and `Class`;
- legacy tool code may still set `Tool` and `ToolClass`;
- adapters and extractors normalize both forms.

## Middleware model

`action.Middleware` is the reusable execution middleware model. It wraps `action.Action` and can observe or transform:

- spec metadata;
- input;
- execution context;
- intent;
- result.

`tool.Middleware` remains for LLM-facing behaviors that depend on raw JSON arguments, guidance/schema projection, approval UX, or `tool.Result` rendering.

Use `tool.ApplyAction(...)` only on action-backed tools. It deliberately leaves legacy tools unchanged so migration code can be safe and explicit.

## Result contract

`action.Result` now has an explicit execution status contract:

- `action.OK(data, events...)` for successful execution;
- `action.Failed(err, events...)` for failed execution;
- `Result.IsError()` for status/error checks;
- `Result.Err()` for status-aware error retrieval.

`action.Result` remains execution-oriented. It does not replace `tool.Result`; the tool result contract is still model-facing and persistence/rendering oriented.

## JSON schema projection

Action input/output schema projection belongs in `action.Type` and `action.SchemaFor(...)`. Tool schemas reuse those action schema helpers instead of maintaining a separate reflection path.

## Compatibility promise

Pre-1.0 APIs can still move, but the direction should remain stable:

- workflows orchestrate actions;
- commands project user/channel operations;
- tools project LLM-callable operations;
- actions carry reusable execution metadata, intent, results, and middleware.

## Execution primitive boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps the action/tool direction explicit: `action` has no internal agentsdk imports, while `tool` depends on `action` as the narrower LLM-facing projection. Keep action-backed tool migration incremental and justified by workflow/command reuse.

## Command system

This document describes the current command-system conventions.

## Current command shape

- New broad, user-facing SDK command groups should be implemented as `command.Tree` roots.
- Flat `command.New(...)` commands remain supported for small app-specific commands and compatibility, but broad namespaces such as `session`, `workflow`, `help`, and control commands should stay tree-backed so descriptors, validation, help, schemas, and structured execution all share one source of truth.
- `harness.Session.ExecuteCommand(...)` and the command envelope APIs execute tree commands with structured input instead of reconstructing slash-command strings.

## Descriptor metadata

Command descriptors now carry:

- input descriptors (`Descriptor.Input`) projected from `Arg(...)`, `Flag(...)`, and `TypedInput[...]()`;
- output descriptors (`Descriptor.Output`) set with `command.Output(...)`;
- caller policy (`Policy`) kept on the descriptor and exposed through export/catalog projections where appropriate.

Output descriptors intentionally describe the result payload rather than forcing typed output binding. The current result path remains:

1. command handler returns `command.Result` with a structured payload;
2. descriptor declares the expected payload kind/media types/schema;
3. terminal, TUI, API, JSON, or LLM-facing renderers render at the boundary.

Typed output binding is deferred because it would add reflection/generic complexity without reducing enough boilerplate yet. A future typed-output helper should only be added if command authors repeatedly duplicate output descriptor declarations.

## Schema and export surfaces

- `command.CommandInputSchema(desc)` remains the object-schema projection for structured command input.
- `command.OutputDescriptor.Schema` is the output schema source of truth.
- `command.ExportDescriptors(...)` flattens executable descriptor trees for HTTP/OpenAPI-like channels and includes policy, input schema, and output schema.
- `harness.Session.CommandCatalog(...)` exposes the same input/output schema projection for session-scoped clients.

The generic `harness.CommandEnvelope` remains the execution envelope. Per-command details are provided by the catalog/export metadata rather than by generating a large `oneOf` tool schema. This keeps `session_command` stable and avoids bloating LLM tool definitions.

## Policy decisions

The current policy model is sufficient for this slice:

- `command.UserPolicy()` marks user-callable commands.
- `command.AgentPolicy()` marks agent-callable commands.
- `command.InternalPolicy()` and `command.TrustedPolicy()` mark internal/trusted commands.
- No workflow-callable policy is added yet. Workflows use trusted action/command envelope seams today, and a separate workflow policy should only be added if workflows need a restricted command subset distinct from trusted internal execution.

## Agent command projection

`harness.AgentCommandToolName` / `session_command` is the preferred agent-facing command projection. It uses a path array plus structured input object and is backed by the session command catalog.

The older `command.Tool(...)` / `command_run` slash-string tool remains for compatibility only. New harness integrations should prefer `session_command`; `command_run` can be removed in a future breaking release after downstream callers migrate.

## Execution primitive boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps `command` as the user/channel projection layer. `command.Tree`, descriptors, result envelopes, schemas, and policies remain core command concepts.

The main watch item is `command -> tool` for the older command-to-tool projection. New agent-facing command execution should continue to prefer harness `session_command`; the old slash-string projection can be deleted once dogfood no longer needs it.

## Command rendering

This section documents command rendering and result payload conventions.

## `command.Text(...)` usage

Production command handlers no longer use `command.Text(...)` for broad harness commands. Remaining `command.Text(...)` calls are in the `command` package itself and tests that exercise flat commands, tree dispatch, and typed binding. This keeps plain text available for small app-specific commands without making it the default harness result shape.

## Usage classification

- **Simple messages**: use `command.Notice(...)`, `command.NotFound(...)`, or `command.Unavailable(...)`. The `/context` command now uses the generic notice payload instead of a one-off context payload.
- **Structured notice candidates**: not-found and unavailable paths already use `NoticePayload` with level/resource/id metadata where useful.
- **Typed payload candidates**: session metadata, workflow definitions, workflow starts, workflow run lists, workflow run detail, agents, skills, and compaction stay as typed payloads because their JSON form is useful to API/TUI/LLM clients.
- **Error candidates**: command validation remains `HelpPayload{Error: *ValidationError}` so terminal help and JSON validation details share one payload. Execution errors that are command outcomes are carried by typed payload fields such as `WorkflowStartPayload.Error` or `SkillActivationPayload.Error`.

## Generic payload decisions

- A generic message payload already exists as `command.NoticePayload`; no additional message type is needed.
- A generic validation detail payload is not added because `command.ValidationError` plus `command.HelpPayload` already covers descriptor-backed validation.
- A generic table/list payload is deferred. Lists currently have domain-specific fields (`workflow.Definition`, `workflow.RunSummary`, `agent.Spec`, `skill.Item`) that are more useful to JSON clients than a generic row/cell model. Add a table payload only if multiple unrelated commands start duplicating row rendering without needing domain JSON.

## Rendering boundary

Command handlers should return structured payloads or generic notices. String formatting belongs in payload `Display(mode)` methods or terminal/UI renderers. The one practical inline formatting cleanup in this slice moved `/context` from a one-off payload to `command.Notice(...)`; remaining `fmt.Fprintf` usage in harness command result code is inside payload display methods or catalog-context rendering.

## Coverage

Added rendering coverage for:

- golden terminal rendering of `SessionInfoPayload`;
- golden terminal rendering of `WorkflowDefinitionPayload`;
- JSON rendering of harness structured payloads.

These tests intentionally cover stable payload rendering without snapshotting volatile timestamps, generated run IDs, or provider-specific output.
