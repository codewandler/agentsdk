# 13 — Action/tool convergence

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
