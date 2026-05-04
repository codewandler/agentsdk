# 17 — Resource and app manifest follow-ups

Section 13 completes the next resource/app-manifest slice after daemon and triggers.

## Resource bundle coverage

Native `.agents` resource discovery now includes:

```text
.agents/workflows/*.yaml
.agents/actions/*.yaml
.agents/triggers/*.yaml
.agents/commands/*.yaml
```

Plugin-root manifest sources support the same layouts without the `.agents/` prefix:

```text
workflows/*.yaml
actions/*.yaml
triggers/*.yaml
commands/*.yaml
```

Workflow resources are converted into executable `workflow.Definition` values when loaded into `app.App`. The converter now preserves more workflow semantics from YAML, including version, input maps/templates, retry/backoff, timeout, step error policy, idempotency keys, and simple conditions. Workflow resources may also include `expose.commands` and `expose.triggers` sugar; the loader normalizes those projections into command and trigger contributions targeting the workflow.

Action resources are metadata only. They document host/plugin-provided actions for discovery and UX; they do not create executable actions by themselves.

Trigger resources are declarative trigger definitions. `agentsdk serve` converts interval trigger resources into in-process daemon jobs using the event-source/matcher/executor model from section 12. Trigger targets may reference an existing workflow or declare an inline workflow that is normalized into the same workflow contribution model.

Structured command YAML resources live alongside Markdown commands in `commands/`. They describe command path/tree placement, input/output schema metadata, caller policy, and a target (`workflow`, `action`, or `prompt`). Command targets may reference an existing workflow or declare an inline workflow that is normalized before execution surfaces consume it.

## Manifest plugin refs

App manifests support both short and structured plugin refs:

```json
{
  "plugins": [
    "local_cli",
    {"name": "planner", "config": {"mode": "safe"}}
  ]
}
```

Plugin refs now validate that every ref has a non-empty name. Path-based plugin refs remain rejected; use named plugin refs and plugin factory resolution instead.

## Discover output

`agentsdk discover` now reports:

- workflows;
- actions;
- triggers;
- structured command resources;
- manifest plugin refs and whether structured config is present.

Invalid resources and invalid manifest plugin refs return clearer read/parse/validate errors with file context.

## Examples

Added examples:

- `examples/resource-only-app` — `.agents` resources only, including a trigger targeting a workflow.
- `examples/hybrid-app` — manifest + `.agents` resources + structured plugin config.

## Datasource ordering

Datasource resource expansion remains intentionally deferred. Existing datasource discovery stays in place, but new runtime behavior should wait until daemon/triggers produce a concrete background ingestion/synchronization case study.

## Composition boundary review

The follow-up boundary review in [`28_APP_RESOURCE_PLUGIN_BOUNDARY.md`](28_APP_RESOURCE_PLUGIN_BOUNDARY.md) confirms this resource/app model is still acceptable: `resource` and `agentdir` stay metadata/spec loaders, `app` remains the reusable definition/registry composition root, and `harness.Session` remains the live execution boundary.

The main watch item is that `app.App` still stores live `*agent.Instance` values. That is a current bridge, not the desired end state. When the `agent.Instance` cleanup starts, keep app definitions/registries in `app` and move live session/runtime ownership toward harness/session.
