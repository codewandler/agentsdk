# App/resource/plugin boundary review

This is a docs-only review of the app/resource/plugin composition layer. It follows the package boundary analysis in [`27_PACKAGE_BOUNDARY_ANALYSIS.md`](27_PACKAGE_BOUNDARY_ANALYSIS.md) and focuses on whether `app`, `resource`, `agentdir`, and `plugins/*` match the intended architecture.

No code changes are part of this batch.

## Review commands

```bash
go list -f '{{.ImportPath}} {{join .Imports " "}}' ./app ./resource ./agentdir ./plugins/...
```

Internal imports observed:

```text
app
  -> action
  -> agent
  -> agentcontext
  -> capability
  -> command
  -> datasource
  -> resource
  -> skill
  -> tool
  -> workflow

resource
  -> agent
  -> command
  -> skill

agentdir
  -> agent
  -> capability
  -> command
  -> command/markdown
  -> markdown
  -> resource
  -> skill

plugins/gitplugin
  -> agentcontext
  -> agentcontext/contextproviders
  -> tool
  -> tools/git

plugins/localcli
  -> agent
  -> app
  -> capabilities/planner
  -> capability
  -> plugins/plannerplugin
  -> tool
  -> tools/filesystem
  -> tools/git
  -> tools/jsonquery
  -> tools/notify
  -> tools/phone
  -> tools/shell
  -> tools/skills
  -> tools/todo
  -> tools/toolmgmt
  -> tools/turn
  -> tools/vision
  -> tools/web
  -> websearch

plugins/plannerplugin
  -> capabilities/planner
  -> capability

plugins/skillplugin
  -> agentcontext
  -> agentcontext/contextproviders
  -> app
  -> skill
  -> tool
  -> tools/skills

plugins/toolmgmtplugin
  -> agentcontext
  -> agentcontext/contextproviders
  -> app
  -> tool
  -> tools/toolmgmt

plugins/visionplugin
  -> tool
  -> tools/vision
```

## Intended boundary

The composition layer has three related but separate jobs:

| Package/group | Job | Must not own |
| --- | --- | --- |
| `resource` | Declarative metadata model normalized from files/manifests/external roots. | Live runtime state, sessions, channel rendering, plugin factory resolution. |
| `agentdir` | Filesystem/resource loader that turns `.agents`, compatibility roots, and plugin roots into `resource.ContributionBundle`. | App instantiation, session lifecycle, execution, process policy. |
| `app` | Go composition root for reusable definitions and runtime registries. | Live session/channel lifecycle, daemon process policy, terminal rendering. |
| `plugins/*` | Named contribution bundles for concrete use cases/environments. | A generic default/standard bundle, session plugin system, hidden global composition. |

This boundary is still the right one. Declarative resources and Go plugins both feed `app.App`; `harness.Session` remains the live execution boundary.

## Current state assessment

### `resource`

`resource.ContributionBundle` currently carries:

- `AgentSpecs`
- Markdown `Commands`
- `CommandResources`
- `Workflows`
- `Actions`
- `Triggers`
- `DataSources`
- `Skills` / `SkillSources`
- plugin refs and diagnostics through source/manifest loading paths

Import assessment:

- `resource -> agent` is currently acceptable because `AgentSpecs` uses `agent.Spec` as the canonical declarative agent definition.
- `resource -> command` is currently acceptable because structured command resources use `command.JSONSchema`, `command.OutputDescriptor`, and `command.Policy` as metadata.
- `resource -> skill` is currently acceptable because skill metadata/source references are loaded as declarative resources.

Watch point: `resource` should stay metadata-only. It should not import `app`, `harness`, `runtime`, `terminal`, `daemon`, `tool`, or concrete plugins. The current graph satisfies that.

### `agentdir`

`agentdir` parses filesystem resources into `resource.ContributionBundle`.

Import assessment:

- `agentdir -> agent` is acceptable for parsing agent frontmatter into `agent.Spec` and compaction config.
- `agentdir -> capability` is acceptable for parsing capability attach specs.
- `agentdir -> command` and `command/markdown` are acceptable for Markdown command resources and structured command metadata.
- `agentdir -> markdown` is expected for frontmatter parsing.
- `agentdir -> resource` is the primary output model.
- `agentdir -> skill` is expected for skills and skill references.

Watch point: `agentdir` currently knows a few agent-specific frontmatter details, including auto-compaction config. That is acceptable because the file format describes agents, but it increases coupling to `agent` package option structs. If `agent.Instance` cleanup splits spec/config from live runtime, `agentdir` should depend only on the spec/config side.

### `app`

`app.App` is the composition root for definitions and registries. It owns:

- command registry;
- agent specs and instantiated agent cache;
- action registry;
- datasource registry;
- workflow definitions;
- tool catalog/default tools;
- skill sources;
- context provider/plugin registration;
- capability factories;
- resource command metadata.

Import assessment:

- `app -> agent` is expected today for `agent.Spec`, `agent.Option`, and agent instantiation.
- `app -> resource` is expected for `WithResourceBundle` and diagnostics.
- `app -> action`, `command`, `tool`, `workflow`, `skill`, `capability`, `agentcontext`, and `datasource` reflect app-level registries/facets.
- `app` does **not** import `harness`, `terminal`, `daemon`, `channel`, or `cmd`, which is the most important boundary.

Cleanup point: `App` currently stores running `*agent.Instance` values. That is a practical bridge, but architecturally it blurs reusable app definitions with live session/runtime state. When cleaning `agent.Instance`, prefer moving live session/runtime ownership toward `harness` while keeping `app` focused on definitions, registries, and factories.

### `plugins/*`

First-party plugins are purpose-named contribution bundles. This remains correct:

- `plugins/gitplugin` contributes git context/tooling.
- `plugins/plannerplugin` contributes planner capability factories.
- `plugins/skillplugin` contributes skill context/tools.
- `plugins/toolmgmtplugin` contributes tool-management context/tools.
- `plugins/visionplugin` contributes vision tools.
- `plugins/localcli` aggregates the local terminal environment plugin.

`plugins/localcli` has high fan-out because it intentionally bundles the local CLI environment: filesystem, git, shell, skills, tool management, turn signaling, vision, web, phone, JSON query, notifications, and planner fallback. This is acceptable only because it is explicitly named and terminal-local. It must not become a generic `standard` plugin again.

Watch point: some plugins import `app` only for plugin facet helper types. That is acceptable for first-party plugins, but avoid making plugin packages depend on harness/session/channel concepts.

## Boundary findings

### OK / intended

- `app` does not import `harness`, `terminal`, `daemon`, `channel`, or `cmd`.
- `resource` does not import `app`, `harness`, `runtime`, `tool`, or plugins.
- `agentdir` does not instantiate apps or sessions.
- First-party plugins are purpose-named and do not form a second plugin system.
- Resource command contributions are metadata retained for harness/session/channel binding, not executable app commands.

### Watch

- `app.App` still stores live `*agent.Instance` values. This is the main composition-layer reflection of the broader `agent.Instance` ownership problem.
- `agentdir` depends on `agent.AutoCompactionConfig`; this is acceptable as agent file-format config but should move to a narrower spec/config type if `agent` splits live runtime ownership.
- `plugins/localcli` has intentionally high fan-out. Keep it local/terminal-specific and avoid using it as a default hidden bundle from lower layers.
- `skillplugin` and `toolmgmtplugin` import `app` for facet types; keep this as plugin-to-app contribution coupling only, not host/session coupling.

### Cleanup candidates

1. **Move live agent cache out of `app` when harness can own runtime/session construction.** `app` should retain agent specs/factories and reusable registries; live instances should be session-owned.
2. **Narrow resource/agent coupling after `agent` splits.** `resource` and `agentdir` should depend on agent spec/config, not live `agent.Instance` concerns.
3. **Keep structured command binding in harness.** Do not add executable command-resource behavior to `app.App`.
4. **Keep trigger execution host-owned.** Declarative triggers can live in resources/app metadata, but scheduler/process behavior remains daemon/harness-owned.
5. **Keep datasource postponed.** Existing datasource registry/resource metadata can remain, but do not expand runtime semantics until agent/session ownership cleanup is clearer.

## Documentation adjustments needed

The current docs are broadly aligned. The useful clarifications are:

- emphasize that `app` is still partly live because it caches `agent.Instance`, and that this should shrink;
- state that `resource` and `agentdir` are metadata/spec loaders, not runtime owners;
- state that plugins are app-level contribution bundles and should not grow harness/session/channel facets without a concrete repeated use case;
- keep `plugins/localcli` explicitly terminal-local, not generic default composition.

## Decision

The app/resource/plugin boundary is acceptable for the current dogfood checkpoint. No blocking import violations were found. The next implementation pressure remains the `agent.Instance` cleanup, especially moving live session/runtime ownership out of `app`/`agent` and into harness/session without adding compatibility shims.
