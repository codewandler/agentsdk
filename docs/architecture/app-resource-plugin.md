# App, resources, and plugins

## Resource and manifest model

Resource and manifest discovery now covers workflows, actions, triggers, and structured commands.

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

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps this resource/app model explicit: `resource` and `agentdir` stay metadata/spec loaders, `app` remains the reusable definition/registry composition root, and `harness.Session` remains the live execution boundary.

The main watch item is that `app.App` still stores live `*agent.Instance` values. That is a current bridge, not the desired end state. When the `agent.Instance` cleanup starts, keep app definitions/registries in `app` and move live session/runtime ownership toward harness/session.

## Plugin contribution model

The SDK uses one contribution model instead of adding parallel
plugin systems for harnesses, sessions, channels, or triggers.

## Boundary decision

Not every contribution lives in `harness`. The boundary is:

- `app` composes app-level, reusable definitions and runtime registries.
- `harness` owns live session execution, lifecycle, and projections.
- `resource` carries declarative contribution metadata.
- daemon/channel packages own process, presentation, and host policy.

A contribution can be declared or registered through `resource`/`app` and still
be executed through `harness.Session` when it needs a live session. Definitions
and metadata are app/resource concerns; session-bound dispatch is a harness
concern.

The SDK has four related but different concepts:

| Concept | Owner | Purpose |
| --- | --- | --- |
| `app.Plugin` | `app` package | Runtime/Go contribution bundle for app-level definitions and registries, with optional facets such as actions, tools, workflows, context providers, skills, and command handlers. |
| `resource.ContributionBundle` | `resource` + `agentdir` | Declarative contribution metadata loaded from `.agents`, compatibility roots, plugin roots, or manifests. |
| Session projections | `harness.Session` | Session-aware adapters that bind live session capabilities into command trees, agent tools, and context providers. |
| Host/daemon wiring | `daemon` and channel packages | Process, config, scheduling, storage, and presentation policy around a harness service. |

These concepts compose; they are not separate plugin systems. `app` remains the
composition root for reusable app capabilities, while `harness` remains the
execution boundary for session-aware behavior.

## Invariants

- Do not add `harness.Plugin` as a second plugin abstraction.
- Do not add `session.Plugin`, `channel.Plugin`, or `trigger.Plugin` until a
  concrete repeated use case proves the existing model cannot carry it.
- Keep session projections as projections. `harness/projection.go` is an adapter
  seam for session-owned command/tool/context exposure, not a packaging system.
- Keep first-party plugins purpose-named. `plugins/localcli`, `plugins/gitplugin`,
  `plugins/skillplugin`, `plugins/toolmgmtplugin`, `plugins/plannerplugin`, and
  `plugins/visionplugin` describe actual use cases or environments. A generic
  catch-all `standard` plugin should not return.
- Add new app plugin facets only when a real Go/plugin contribution cannot be
  represented by an existing facet.
- Add declarative resource contribution types when users need files/manifests to
  describe app behavior or deployment metadata.

## Current plugin facets

`app.Plugin` is intentionally small:

```go
type Plugin interface {
    Name() string
}
```

Optional facets live beside it:

- `CommandsPlugin`
- `AgentSpecsPlugin`
- `ToolsPlugin`
- `DefaultToolsPlugin`
- `CatalogToolsPlugin`
- `ActionsPlugin`
- `DataSourcesPlugin`
- `WorkflowsPlugin`
- `SkillsPlugin`
- `CapabilityFactoriesPlugin`
- `ContextProvidersPlugin`
- `AgentContextPlugin`
- `ToolMiddlewarePlugin`
- `ToolTargetedMiddlewarePlugin`

This is enough for current app-level Go/runtime contributions. There are no
app/session/channel/trigger plugin facets because there is no concrete need yet.

## Resource contributions are not plugins

Declarative resources are normalized into `resource.ContributionBundle`:

- `AgentSpecs`
- Markdown `Commands`
- structured `CommandResources`
- `Workflows`
- `Actions`
- `Triggers`
- `DataSources`
- `Skills` / `SkillSources`
- plugin refs from app manifests

A resource bundle can be consumed by `app.New(app.WithResourceBundle(...))`, by
`harness.LoadSession(...)`, or by daemon/channel hosts. It should not require a
resource-specific plugin wrapper.

Structured command resources are a good example:

```yaml
name: session-summary
path: [session, summary]
target:
  workflow: session_summary
```

`app.App.ResourceCommands()` exposes those structured command contributions as
load-time resource metadata retained for harness/session/channel consumers. It
does not make `app.App` the projection or execution owner, and these resources
are not automatically executable app commands. The executable binding belongs to
a session/channel projection because target execution needs session context:

- workflow targets should call `Session.ExecuteWorkflow(...)`;
- prompt targets should call `Session.Send(...)` or the session agent turn path;
- direct action targets need explicit trusted policy before execution.

That keeps command resources declarative: `app` can carry the metadata alongside
other app-level definitions, while `harness` remains the session-aware dispatcher.

## Trigger contributions are not plugins

Triggers have a runtime scheduler in `trigger` and daemon-owned execution wiring
in `daemon`. Declarative trigger resources normalize into
`resource.TriggerContribution`. A future trigger source ecosystem may need Go
extension points, but the first implementation should continue to use explicit
host configuration or app/plugin-contributed actions/workflows before adding a
`TriggerPlugin` facet.

## Session projections are not plugins

Session projections attach session-owned capabilities to existing surfaces:

- command catalog context for agents;
- `session_command` as an agent-callable tool;
- harness command trees such as `/session`, `/workflow`, and trigger commands;
- future structured command-resource bindings.

They are scoped to a live `harness.Session`, may close over session state, and
should remain adapters. Packaging, reusable definitions, and declarative
contribution metadata still live in `app.Plugin` or resource bundles.

## When to add a new facet

Add a new optional `app.Plugin` facet only when all of these are true:

1. At least two real plugins need to contribute the same kind of runtime object.
2. The object cannot be expressed as an action, workflow, command, tool,
   datasource, context provider, capability, or resource contribution.
3. The contribution is app-level or can be cleanly configured by app-level
   factories without depending on one live session.
4. Tests can show the facet reduces host-specific wiring rather than moving it
   into a vague abstraction.

If the contribution needs a live session, first try a session projection. If it
needs process ownership, first try daemon/channel host configuration.

## Next likely seam

The next useful implementation after this section is executable structured
command resources. That should be implemented as a harness/session projection,
not as a new plugin type:

```text
resource.CommandContribution -> harness command binding -> Session target execution
```

This preserves the current hierarchy: resource/app define and retain metadata;
harness/session binds that metadata to live execution; terminal, daemon, and
future channels present it.


## Composition boundary review

The consolidated review in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) keeps the plugin rule explicit: plugins are app-level named contribution bundles, resources are declarative metadata, and session projections are not a plugin system.

The important cleanup candidate is not a new plugin facet. It is reducing live runtime/session state in `app.App` and `agent.Instance` once harness/session can own that lifecycle directly.
