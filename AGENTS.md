# AGENTS.md - agentsdk notes

This file is for developers and AI agents working on agentsdk and its nearby
consumer repos.

## Resource format references

When adding or changing filesystem resource layouts for agents, commands,
plugins, or skills, update `docs/reference/resources.md` with the external
standard or compatibility source being followed. Prefer established formats over
new agentsdk-specific layouts.

The project is pre-1.0 and currently has no external legacy users. Prefer
deleting stale paths over compatibility shims unless explicitly requested.

## Testing

Run the full CI check before committing or handing work back:

```bash
./scripts/ci-check.sh
```

This includes `go test ./...` plus repository guards. For focused work, run tests
for the package you changed:

```bash
go test ./runtime/...
go test ./conversation/...
go test ./tool/...
```

See `docs/architecture/99_REVIEW_AND_IMPROVEMENTS.md` for consolidated review
notes and follow-up recommendations.

## Apps and examples

The `apps/` directory contains first-party dogfood apps:

- `apps/engineer/` — Resource-only coding/architecture/code-review/DevOps agent used to build agentsdk itself.
- `apps/builder/` — First-party `agentsdk build` dogfood app.

The `examples/` directory contains small instructional agent applications:

- `examples/local-quickstart/` — Minimal resource-only app.
- `examples/workflow-app/` — Declarative workflow and command exposure example.
- `examples/command-tree/` — Markdown and YAML command examples.
- `examples/datasource/` — Go-native datasource registry example.
- `examples/action-tool-adapter/` — Action/tool adapter example.
- `examples/devops-cli/` — CLI agent with custom tool wiring.
- `examples/research-desk/` — Multi-source research agent with resource bundles.

When adding or changing SDK APIs, check whether an existing app or example should
be updated to reflect the change.

## Documentation structure

- Root `README.md` is end-user facing only.
- `docs/` is publishable; do not put temporary tasklists, planning logs, release
  gates, or handoff notes there.
- `docs/README.md` is the publishable docs index.
- `docs/architecture/` contains stable infrastructure and ownership-rule docs.
- Review findings and improvement backlog items belong only in
  `docs/architecture/99_REVIEW_AND_IMPROVEMENTS.md`.
- Temporary task tracking, readiness gates, plans, and archived internal notes
  live under `.agents/`.
- Prefer consolidating existing docs over adding new files. Do not add new
  per-area architecture review logs.

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
- `plugins/plannerplugin` — planner capability factory.

Named host/plugin composition lives under `plugins/`; for example, `plugins/localcli` assembles the local terminal plugin.

Plugin interfaces are defined in `app/plugin.go`. There are two context
provider facets:

- `ContextProvidersPlugin` — app-scoped, stateless providers created at
  registration time.
- `AgentContextPlugin` — agent-scoped, factory-based providers that receive
  per-agent state during instantiation.

See `.agents/plans/PLAN-plugin-architecture-and-bundling.md` for the full
design rationale.

## Cross-references

- `README.md` — public end-user overview.
- `CHANGELOG.md` — release history and migration notes.
- `docs/README.md` — publishable docs index.
- `docs/reference/resources.md` — external format references and compatibility layouts.
- `docs/architecture/README.md` — architecture docs index.
- `docs/architecture/99_REVIEW_AND_IMPROVEMENTS.md` — consolidated review and improvement backlog.
- `.agents/TASKLIST.md` — temporary internal task tracking.
- `.agents/plans/` — design plans and architecture decisions.
- `.agents/archive/` — archived internal notes preserved outside publishable docs.
