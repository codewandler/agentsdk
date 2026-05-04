# App architecture boundaries

## Resource-only vs hybrid apps

**Resource-only apps** contain no Go code. They are pure `.agents/` resource bundles
discovered and executed by `agentsdk run <dir>`. Use this when:

- The agent only needs prompt engineering, skills, workflows, and commands.
- All required tools are provided by the host (filesystem, shell, git, web, vision, etc.).
- No custom actions or data transformations are needed.

**Hybrid apps** combine resource bundles with Go code that registers custom
actions, tools, plugins, or datasources. Use this when:

- The agent needs typed Go actions with structured input/output.
- Custom tool implementations are required (API clients, database queries, etc.).
- Plugin composition is needed (bundling tools + context providers + skill sources).

## Plugin architecture

Plugins implement `app.Plugin` (just `Name() string`) plus optional facet interfaces:

| Facet | Interface | Purpose |
|-------|-----------|---------|
| Tools (default) | `DefaultToolsPlugin` | Tools active when agent spec has no `tools:` field |
| Tools (catalog) | `CatalogToolsPlugin` | Tools selectable by agent spec `tools:` patterns |
| Actions | `ActionsPlugin` | Typed Go actions exposed as tools or workflow steps |
| Commands | `CommandsPlugin` | Programmatic slash commands |
| Workflows | `WorkflowsPlugin` | Programmatic workflow definitions |
| Skills | `SkillsPlugin` | Skill discovery sources |
| Context | `ContextProvidersPlugin` | App-scoped context providers |
| Agent context | `AgentContextPlugin` | Per-agent context providers (receive runtime state) |
| Capabilities | `CapabilityFactoriesPlugin` | Capability factories (e.g. planner) |
| Middleware | `ToolMiddlewarePlugin` | Global tool middlewares |

## Composition patterns

- **Actions → Tools**: `tool.FromAction(action)` wraps any action as an LLM-callable tool.
- **Workflows → Commands**: Workflow YAML `expose.commands` projects workflows as slash commands.
- **Skills → Context**: Activated skills inject their content into the agent context.
- **Plugins → App**: `app.WithPlugin(plugin)` registers all plugin facets at once.

## When to escalate from resource-only to hybrid

1. You need structured input validation beyond what YAML/Markdown provides.
2. You need to call external APIs with authentication, retries, or error handling.
3. You need custom data transformations between workflow steps.
4. You need to register a datasource that queries a database or API.
5. You need tool middleware (logging, rate limiting, approval gates).
