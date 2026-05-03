# 10 — Command Result and Rendering Cleanup

This audit closes section 7 of `docs/04_TASKLIST.md`.

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
