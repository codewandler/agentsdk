# AGENTS.md - agentsdk notes

This file is for developers and AI agents working on agentsdk and its nearby
consumer repos.

## Resource format references

When adding or changing filesystem resource layouts for agents, commands,
plugins, or skills, update `docs/RESOURCES.md` with the external standard or
compatibility source being followed. Prefer established formats over new
agentsdk-specific layouts.

## Testing

Run the full test suite before committing:

```bash
go test ./...
```

For focused work, run tests for the package you changed:

```bash
go test ./runtime/...
go test ./conversation/...
go test ./tool/...
```

See `.agents/reviews/` for detailed review notes and follow-up recommendations.

## Apps and examples

The `apps/` directory contains first-party dogfood apps:

- `apps/engineer/` — Resource-only coding/architecture/code-review/DevOps agent used to build agentsdk itself.
- `apps/builder/` — Reserved for the planned `agentsdk build` builder app.

The `examples/` directory contains small instructional agent applications:

- `examples/devops-cli/` — CLI agent with custom tool wiring.
- `examples/research-desk/` — Multi-source research agent with resource bundles.
- `examples/release-notes-agent/` — Planned: release notes generation agent.
- `examples/repo-maintainer/` — Planned: repository maintenance agent.

When adding or changing SDK APIs, check whether an existing app or example should
be updated to reflect the change.

## Branding: flai → agentsdk

Several files still reference the predecessor project name "flai". Public
constants (`tools/toolmgmt.KeyActivationState`, `skill.RegistryKey`) retain
`flai.` prefixes for downstream compatibility.

When writing new code, always use `agentsdk` naming. Do not introduce new
`flai` references.

## Dependency update process

When upgrading `llmadapter`, pass the released version through the dependency
chain deliberately:

1. Verify or cut the `llmadapter` release.
2. Update `agentsdk` to that released `llmadapter` version.
3. Run `go test ./...` in `agentsdk`.
4. Commit, tag, and push the `agentsdk` release.
5. Update consumers such as `../miniagent` to the released `agentsdk` version
   and the same direct `llmadapter` version when they import it directly.
6. Run consumer tests.
7. For CLI consumers, reinstall the compiled binary before smoke testing.

Important: `miniagent` is a compiled Go binary. Updating `go.mod`, tagging, or
pushing repos does not update the already installed `$GOPATH/bin/miniagent`.
After dependency-chain updates, run `task install` in `../miniagent` before
checking installed-binary behavior.

If `llmadapter resolve <model>` works but `miniagent -m <model>` fails, first
verify that the installed `miniagent` binary was rebuilt after the dependency
update. Otherwise the binary may still contain older routing behavior.

## Plugin architecture

First-party plugins live under `plugins/`. Each plugin bundles related tools,
context providers, and skill sources behind the `app.Plugin` interface.

- `plugins/gitplugin` — git tools + git context provider.
- `plugins/skillplugin` — skill tool + discovery + skill inventory context.
- `plugins/toolmgmtplugin` — tool management tools + active-tools context.
- `plugins/standard` — pre-assembled plugin sets for common configurations.

Plugin interfaces are defined in `app/plugin.go`. There are two context
provider facets:

- `ContextProvidersPlugin` — app-scoped, stateless providers created at
  registration time.
- `AgentContextPlugin` — agent-scoped, factory-based providers that receive
  per-agent state (skill repo, toolset) during instantiation.

See `.agents/plans/PLAN-plugin-architecture-and-bundling.md` for the full
design rationale.

## Cross-references

- `README.md` — public API overview, runtime stack, CLI resource bundles.
- `CHANGELOG.md` — release history and migration notes.
- `docs/RESOURCES.md` — external format references and compatibility layouts.
- `.agents/plans/` — design plans and architecture decisions.
- `.agents/reviews/` — detailed architecture and implementation review notes.
