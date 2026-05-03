# 09 — Command System Follow-ups

This note closes the section 6 command-system decisions from `docs/04_TASKLIST.md`.

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
