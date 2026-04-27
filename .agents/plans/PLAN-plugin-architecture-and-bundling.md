# Plan: Plugin architecture — bundling tools, context providers, commands, and skills

## Problem statement

Features in `agentsdk` are currently organized by **kind** (tools in `tools/`,
context providers in `agentcontext/contextproviders/`, commands wired ad-hoc,
skills in `skill/`). Related functionality for a single domain is scattered:

- **Git**: tools in `tools/git/`, context provider in
  `agentcontext/contextproviders/git.go`, no shared wiring point.
- **Skills**: tool in `tools/skills/`, context provider in
  `agentcontext/contextproviders/` (skill inventory), skill sources in
  `app/skills.go`, command wired in `terminal/cli/`.
- **Tool management**: tool in `tools/toolmgmt/`, context provider in
  `agentcontext/contextproviders/` (tools provider).

Adding a new cross-cutting feature requires touching 3–4 packages and updating
the assembly in `tools/standard/standard.go` and `agent/agent.go`. The
`app.Plugin` interface already exists (`app/plugin.go`) with facets for
commands, tools, agent specs, and skill sources — but no facet for context
providers, and no first-party plugin implementations.

## Goal

Introduce a `plugins/` directory containing first-party plugin implementations
that bundle all contributions for a domain behind the `app.Plugin` interface.
Extend the plugin interface to support context provider contributions. Keep the
existing low-level packages (`tools/*`, `agentcontext/contextproviders/`) as
reusable building blocks that plugins compose.

## Confirmed design decisions

1. **`app/plugin.go` gets a `ContextProvidersPlugin` facet.**
   ```go
   type ContextProvidersPlugin interface {
       Plugin
       ContextProviders() []agentcontext.Provider
   }
   ```
   `RegisterPlugin` detects this facet and collects providers. The `App` exposes
   them so that agent instantiation can pass them through to the runtime context
   manager.

2. **Plugins compose existing packages — they don't replace them.**
   `plugins/gitplugin` imports `tools/git` and
   `agentcontext/contextproviders` and bundles them. The low-level packages
   remain importable for consumers who don't want the plugin abstraction.

3. **Only multi-facet domains become plugins initially.**
   Single-facet packages (`tools/shell`, `tools/notify`,
   `agentcontext/contextproviders/time.go`) stay where they are. A plugin is
   justified when a domain contributes two or more of: tools, context providers,
   commands, skill sources.

4. **`tools/standard/standard.go` gains a plugin-oriented API alongside the
   existing `Tools()` API.** Backward compatibility is preserved; the new path
   is additive.

5. **The empty `plugin/` directory is removed.** All plugin-related code lives
   under `plugins/` (plural) for implementations and `app/plugin.go` for
   interfaces.

6. **`agent.Instance.contextProviders()` evolves.** Today it hardcodes the
   provider list. After this work, plugin-contributed providers flow through
   `app.App` → agent options → context manager registration. The agent still
   adds its own baseline providers (environment, time) but plugin providers
   are additive.

## Context provider timing: app-scoped vs agent-scoped

This is the central design constraint. Context providers fall into two
categories:

### App-scoped (stateless / config-only)

These can be created at plugin registration time because they depend only on
configuration, not on per-agent runtime state:

| Provider | Current location | Depends on |
|---|---|---|
| `Environment` | `agent.Instance.contextProviders()` | workspace dir (config) |
| `Time` | `agent.Instance.contextProviders()` | interval (config) |
| `Git` | `agent.Instance.contextProviders()` (not present today) | workspace dir, git mode (config) |
| `Model` | `agent.Instance.contextProviders()` | model name, provider, effort (config — known at spec time) |
| `AgentsMarkdown` | `agent.Instance.contextProviders()` | instruction paths (config — from spec) |

These are safe to contribute from `ContextProvidersPlugin.ContextProviders()`.

### Agent-scoped (runtime state dependent)

These depend on per-agent-instance mutable state and **cannot** be created at
plugin registration time:

| Provider | Current location | Depends on |
|---|---|---|
| `SkillInventoryProvider` | `agent.Instance.contextProviders()` | `*skill.Repository`, `*skill.ActivationState` — created per agent instance |
| `Tools` (active tools) | `agent.Instance.contextProviders()` | `*standard.Toolset` — per agent instance, changes at runtime |

These must remain constructed by the agent during instantiation. Plugins that
contribute these providers need a different facet — a factory that receives
agent-instance state:

```go
// AgentContextPlugin contributes context providers that depend on per-agent
// runtime state. The factory is called during agent instantiation, not at
// plugin registration time.
type AgentContextPlugin interface {
    Plugin
    AgentContextProviders(AgentContextInfo) []agentcontext.Provider
}

// AgentContextInfo carries the per-agent state needed to construct
// agent-scoped context providers.
type AgentContextInfo struct {
    SkillRepository  *skill.Repository
    SkillState       *skill.ActivationState
    ActiveTools      func() []tool.Tool  // closure over toolset
    Workspace        string
    Model            string
    Provider         string
    Effort           string
}
```

This keeps the plugin interface clean: `ContextProvidersPlugin` for stateless
providers, `AgentContextPlugin` for providers that need instance state.

**Decision:** Phase 1 implements only `ContextProvidersPlugin` (app-scoped).
The `AgentContextPlugin` factory facet is deferred to phase 3+ when
`skillplugin` and `toolmgmtplugin` need it. The git plugin only needs the
app-scoped facet.

## Scope: initial plugin set

| Plugin package | Tools | Context providers (app-scoped) | Context providers (agent-scoped) | Commands | Skills |
|---|---|---|---|---|---|
| `plugins/gitplugin` | git_status, git_diff | git state | — | — (future) | — |
| `plugins/skillplugin` | skill (activation) | — | skill inventory | /skills, /skill (future) | skill source discovery |
| `plugins/toolmgmtplugin` | tools_list, tools_activate, tools_deactivate | — | tools inventory | — | — |

Domains that remain as-is (single facet, no plugin wrapper needed):

- `tools/shell` — bash tool only
- `tools/filesystem` — file tools only
- `tools/web` — web tools only (search provider injected)
- `tools/notify` — notification tool only
- `tools/todo` — plan tool only
- `tools/turn` — turn_done tool only
- `agentcontext/contextproviders/environment.go` — env provider only
- `agentcontext/contextproviders/time.go` — time provider only

## Implementation phases

### Phase 1: Plugin interface extension + `ContextProvidersPlugin`

**Files changed:**
- `app/plugin.go` — add `ContextProvidersPlugin` interface
- `app/app.go`:
  - Add `contextProviders []agentcontext.Provider` field to `App`
  - In `RegisterPlugin`: detect `ContextProvidersPlugin`, collect providers
  - Add `ContextProviders() []agentcontext.Provider` accessor
  - In `InstantiateAgent`: forward collected providers via new agent option
- `agent/options.go` — add `WithContextProviders(...agentcontext.Provider)`
- `agent/agent.go`:
  - Add `extraContextProviders []agentcontext.Provider` field to `Instance`
  - In `contextProviders()`: append `a.extraContextProviders` after baseline
  - In `initThreadRuntime()`: same — extra providers are included

**Wiring path:**
```
app.RegisterPlugin(p)
  → if ContextProvidersPlugin → a.contextProviders = append(...)

app.InstantiateAgent(name)
  → base options include agent.WithContextProviders(a.contextProviders...)

agent.New(opts...)
  → stores extraContextProviders

agent.Instance.contextProviders()
  → [environment, time, model, tools, skills, agents_md] ++ extraContextProviders
```

**Key constraint:** Provider keys must be unique across the context manager.
If a plugin contributes a provider with key `"git"` and the agent also
hardcodes one, registration fails with a duplicate key error. This is
intentional — it surfaces misconfiguration early. The agent's hardcoded
providers use keys `environment`, `time`, `model`, `tools`, `skills`,
`agents_markdown`. Plugin providers must not collide with these.

**Tests:**
- `app/app_test.go`: register a plugin implementing `ContextProvidersPlugin`,
  verify `app.ContextProviders()` returns the contributed providers.
- `app/app_test.go`: verify `InstantiateAgent` passes providers through (mock
  agent option capture or verify via agent accessor).
- `agent/agent_test.go` or `agent/options_test.go`: verify
  `WithContextProviders` stores providers and `contextProviders()` includes
  them.

### Phase 2: First plugin — `plugins/gitplugin`

**New files:**
- `plugins/gitplugin/gitplugin.go`
- `plugins/gitplugin/gitplugin_test.go`

**Implementation:**
```go
package gitplugin

import (
    "github.com/codewandler/agentsdk/agentcontext"
    "github.com/codewandler/agentsdk/agentcontext/contextproviders"
    "github.com/codewandler/agentsdk/tool"
    "github.com/codewandler/agentsdk/tools/git"
)

type Option func(*Plugin)

type Plugin struct {
    gitMode contextproviders.GitMode
    workDir string
}

func New(opts ...Option) *Plugin {
    p := &Plugin{gitMode: contextproviders.GitMinimal}
    for _, opt := range opts {
        if opt != nil {
            opt(p)
        }
    }
    return p
}

func WithMode(mode contextproviders.GitMode) Option {
    return func(p *Plugin) { p.gitMode = mode }
}

func WithWorkDir(dir string) Option {
    return func(p *Plugin) { p.workDir = dir }
}

func (p *Plugin) Name() string { return "git" }

func (p *Plugin) Tools() []tool.Tool {
    return git.Tools()
}

func (p *Plugin) ContextProviders() []agentcontext.Provider {
    opts := []contextproviders.GitOption{
        contextproviders.WithGitMode(p.gitMode),
    }
    if p.workDir != "" {
        opts = append(opts, contextproviders.WithGitWorkDir(p.workDir))
    }
    return []agentcontext.Provider{contextproviders.Git(opts...)}
}
```

**Interface compliance:** `*Plugin` satisfies `app.Plugin`, `app.ToolsPlugin`,
and `app.ContextProvidersPlugin`.

**Effect on `agent.Instance.contextProviders()`:** Today the agent does **not**
include a git context provider. The git plugin adds one. No duplicate key
conflict.

**Effect on `tools/standard/standard.go`:** Today `standard.Tools()` includes
`git.Tools()` only when `opts.IncludeGit` is true. The git plugin replaces
this opt-in with a plugin registration. Both paths remain valid.

**Tests:**
- Verify `Name()`, `Tools()`, `ContextProviders()` return expected values.
- Compile-time interface assertion:
  ```go
  var _ app.ToolsPlugin = (*Plugin)(nil)
  var _ app.ContextProvidersPlugin = (*Plugin)(nil)
  ```
- Integration: register `gitplugin.New()` via `app.WithPlugin`, verify the
  git tools and git context provider are both available.

### Phase 3: `AgentContextPlugin` facet + `plugins/skillplugin`

**Files changed:**
- `app/plugin.go` — add `AgentContextPlugin` interface and `AgentContextInfo`
  struct
- `app/app.go`:
  - Collect `AgentContextPlugin` implementations in `RegisterPlugin`
  - In `InstantiateAgent`: after building the skill repo and toolset, call
    each `AgentContextPlugin.AgentContextProviders(info)` and forward the
    results via agent option

**New files:**
- `plugins/skillplugin/skillplugin.go`
- `plugins/skillplugin/skillplugin_test.go`

**Implementation:**
```go
package skillplugin

type Plugin struct {
    discoveries []app.SkillSourceDiscovery
}

func (p *Plugin) Name() string { return "skills" }

func (p *Plugin) Tools() []tool.Tool {
    return skills.Tools()
}

func (p *Plugin) SkillSources() []skill.Source {
    // Discover from configured directories
    var sources []skill.Source
    for _, d := range p.discoveries {
        discovered, _ := app.DiscoverDefaultSkillSources(d)
        sources = append(sources, discovered...)
    }
    return sources
}

func (p *Plugin) AgentContextProviders(info app.AgentContextInfo) []agentcontext.Provider {
    if info.SkillRepository == nil && info.SkillState == nil {
        return nil
    }
    return []agentcontext.Provider{
        contextproviders.SkillInventoryProvider(contextproviders.SkillInventory{
            Catalog: info.SkillRepository,
            State:   info.SkillState,
        }),
    }
}
```

**Effect on `agent.Instance.contextProviders()`:** The hardcoded skill
inventory provider block in `contextProviders()` is removed. The skill plugin
contributes it instead via `AgentContextProviders`. The agent no longer needs
to import `contextproviders.SkillInventoryProvider` directly.

**Duplicate key guard:** The agent's hardcoded `contextProviders()` must stop
adding the skill inventory provider when one is contributed by a plugin. Two
approaches:
- (a) The agent checks whether any extra provider already has key `"skills"`
  and skips its own.
- (b) The agent always defers to plugins for skill/tool providers and only
  adds them when no plugin is registered.

**Decision:** Approach (a) — the agent's `contextProviders()` skips any
provider whose key is already present in `extraContextProviders`. This is a
simple key-set check and keeps the agent self-contained.

**Tests:**
- Verify skill plugin contributes skill inventory provider with correct state.
- Verify agent does not duplicate the `"skills"` provider key when skill
  plugin is registered.
- Verify backward compat: agent without skill plugin still adds its own
  skill inventory provider.

### Phase 4: `plugins/toolmgmtplugin`

**New files:**
- `plugins/toolmgmtplugin/toolmgmtplugin.go`
- `plugins/toolmgmtplugin/toolmgmtplugin_test.go`

**Implementation:**
- Bundles `tools/toolmgmt.Tools()`.
- Implements `AgentContextPlugin` to contribute the tools context provider
  using `info.ActiveTools()`.

**Effect on `agent.Instance.contextProviders()`:** The hardcoded `tools`
provider block is removed when the toolmgmt plugin is registered (same
key-set dedup as phase 3).

**Tests:**
- Verify tools context provider is contributed with correct active tools.
- Verify no duplicate `"tools"` provider key.

### Phase 5: Standard plugin assembly

**Files changed:**
- `tools/standard/standard.go`:
  - Add `DefaultPlugins() []app.Plugin`
  - Add `Plugins(Options) []app.Plugin`
  - These return the appropriate plugin instances based on options

**Implementation sketch:**
```go
func Plugins(opts Options) []app.Plugin {
    var out []app.Plugin
    out = append(out, skillplugin.New())
    out = append(out, toolmgmtplugin.New())
    if opts.IncludeGit {
        out = append(out, gitplugin.New())
    }
    return out
}

func DefaultPlugins() []app.Plugin {
    return Plugins(DefaultOptions())
}
```

**Relationship to `Tools()`:** `Plugins()` and `Tools()` are parallel APIs.
`Plugins()` is the recommended path for consumers using `app.App`.
`Tools()` remains for consumers wiring tools directly without the app layer.
They must not be used together for the same domain — that would cause
duplicate tool registrations.

**Backward compatibility:**
- `standard.Tools()` and `standard.DefaultTools()` continue to work unchanged.
- `standard.Plugins()` is additive.
- Existing consumers that wire tools directly are unaffected.
- The `app.App` constructor does not auto-register plugins; consumers opt in.

### Phase 6: Cleanup

- Remove empty `plugin/` directory.
- Update `docs/RESOURCES.md` if plugin layout introduces new conventions.
- Update `ROADMAP.md` to reflect completed plugin architecture.
- Update `AGENTS.md` cross-references if needed.
- Verify `go test ./...` passes.
- Verify `examples/engineer` and `examples/devops-cli` still build.

## File inventory

### New files
```
plugins/gitplugin/gitplugin.go
plugins/gitplugin/gitplugin_test.go
plugins/skillplugin/skillplugin.go
plugins/skillplugin/skillplugin_test.go
plugins/toolmgmtplugin/toolmgmtplugin.go
plugins/toolmgmtplugin/toolmgmtplugin_test.go
```

### Modified files
```
app/plugin.go          — ContextProvidersPlugin, AgentContextPlugin interfaces
app/app.go             — collect/forward plugin context providers
agent/options.go       — WithContextProviders option
agent/agent.go         — extraContextProviders field, key-set dedup in contextProviders()
tools/standard/standard.go — Plugins(), DefaultPlugins()
```

### Deleted files
```
plugin/                — empty directory, removed
```

## Trade-offs

### Domain cohesion vs package coupling
Plugins increase domain cohesion (everything git-related in one place) at the
cost of wider import graphs per plugin package. A `gitplugin` imports both
`tools/git` and `agentcontext/contextproviders`. Today those packages are
independent leaves. This is acceptable because the plugin is a thin composition
layer — it doesn't merge the underlying packages.

### Two plugin facets for context providers
Having both `ContextProvidersPlugin` (app-scoped) and `AgentContextPlugin`
(agent-scoped) adds interface surface. The alternative — a single factory
that always receives agent state — would force stateless providers like git
to accept and ignore state they don't need. Two facets keep each plugin honest
about its dependencies.

### Plugin ceremony for simple domains
Wrapping `tools/shell` in a `shellplugin` that only returns `shell.Tools()`
adds indirection without benefit. The decision to only create plugins for
multi-facet domains avoids this.

### Context provider key-set dedup in agent
The agent's `contextProviders()` must skip providers whose keys are already
contributed by plugins. This is a runtime check rather than a compile-time
guarantee. The duplicate-key error from `agentcontext.Manager.Register` serves
as a safety net — if dedup logic has a bug, registration fails loudly rather
than silently duplicating context.

### Import path stability
No existing import paths change. `tools/git`, `tools/skills`,
`agentcontext/contextproviders` all remain. The `plugins/` packages are new
additions. External consumers are not broken.

## Out of scope

- Plugin lifecycle hooks (init, shutdown, health checks).
- Dynamic plugin loading or plugin registries.
- Moving single-facet packages into plugins.
- Remote/external plugin support.
- Plugin dependency ordering or priority.
- Migrating existing consumers (miniagent, examples) to use `Plugins()` — that
  is a follow-up after the API stabilizes.

## Verification

After each phase:
```bash
go test ./...
```

After phase 5, verify the engineer example and miniagent still build:
```bash
cd examples/devops-cli && go build ./...
cd ../miniagent && go build ./...
```
