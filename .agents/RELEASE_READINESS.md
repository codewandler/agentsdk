# Release readiness checkpoint

This repository is still pre-1.0 and currently has one consumer: us. Release readiness therefore means a clean internal dogfood checkpoint, not backwards-compatible external support.

## Pre-1.0 public API boundary

Stable enough for dogfood:

- `app` for composing resource bundles, named plugins, actions, tools, workflows, commands, skills, and context providers.
- `agent.Spec` and explicit `agent.Options` for agent construction.
- `harness.Service` and `harness.Session` for open/resume/list/close, command execution, workflow execution, event subscriptions, and persisted thread inspection.
- `command.Tree`, `command.Descriptor`, descriptor export, and structured command results.
- `action.Action` as the surface-neutral Go execution primitive.
- `tool.Tool` as the LLM-facing projection surface, especially for action-backed tools.
- `workflow` definitions/executor and harness workflow lifecycle APIs.
- `trigger` and `daemon` for dogfood scheduling/service mode.
- `.agents` resource layouts documented in `docs/reference/resources.md`.
- `apps/engineer` and `apps/builder` as first-party dogfood apps.

Unstable/internal until dogfood proves otherwise:

- `agent.Instance` internals beyond the constructors/options/accessors used by current app/harness paths.
- Writer/output plumbing such as `agent.WithOutput` and `harness.SessionLoadConfig.Output`; channels should converge on structured events/results.
- HTTP/SSE and AG-UI compatibility endpoints; native `/api/agentsdk/v1` is a prototype channel API.
- Safety approval UI wiring; current safety package primitives are descriptive/enforcement building blocks.
- Datasource resource expansion; datasource remains Go/config metadata until a concrete case study needs more.
- Any package or doc that explicitly says prototype, design, follow-up, or checkpoint.

## Intentional breakage notes

There are no legacy users to preserve. Prefer deleting stale APIs and docs over keeping fallback paths.

### No standard tools/plugins

Removed concepts:

- `tools/standard`
- `plugins/standard`
- hidden default tools from `agent.New` / `app.New`

Current path:

- Use named plugins, e.g. `plugins/localcli`, `plugins/gitplugin`, `plugins/plannerplugin`, `plugins/skillplugin`, `plugins/toolmgmtplugin`, `plugins/visionplugin`.
- Resource agents opt into tools by name.
- Terminal `agentsdk run` may apply default local CLI policy at the terminal boundary unless `--no-default-plugins` is set.

### Local CLI plugin

Local terminal tools are owned by the named `local_cli` plugin. This keeps filesystem/shell/git/web policy out of generic `agent` and `app` construction.

Current paths:

```go
app.New(app.WithPlugin(localcli.New()))
```

```bash
agentsdk run .
agentsdk run . --no-default-plugins
agentsdk run . --plugin local_cli
```

### App vs harness responsibilities

- `app` composes reusable definitions and registries.
- `harness` owns live sessions, commands, workflows, events, persistence inspection, and session projections.
- `terminal`, `daemon`, and `channel/*` own presentation/process policy.

Do not add `harness.Plugin`; use `app.Plugin` plus session projections.

### Workflow options

Workflow execution policy belongs in `workflow` definitions/options and harness workflow APIs. Do not add ad hoc workflow flags to `agent` or terminal parsing when the workflow package can own the semantics.

### Commands and descriptors

Use `command.Tree`, `command.Descriptor`, `command.ExportDescriptor`, and resource command contributions. There is no separate `command-descriptors/` resource directory.

Canonical command resource path:

```text
.agents/commands/*.yaml
.agents/commands/*.md
```

Agent-side command execution should use the session-scoped `session_command` projection and catalog context. Do not expand slash-string command bridges.

### Agent projection path

The blessed path is:

1. Build an `app.App` from resources/plugins/options.
2. Open a `harness.Session`.
3. Use session projections for model-visible commands/tools/context.
4. Use channels to render structured results/events.

## CI/readiness gates

`scripts/ci-check.sh` is the local and CI entrypoint. It runs:

- `go test ./...`
- nested module tests for `examples/devops-cli` and `examples/research-desk`
- guards that `tools/standard` and `plugins/standard` stay deleted and are not imported by Go code
- a best-effort guard against obviously ignored command execution results at terminal/cmd boundaries

GitHub Actions runs the same script from `.github/workflows/ci.yml`.

## Internal checkpoint and cadence

Use an internal dogfood checkpoint tag after this release-readiness batch is reviewed and committed:

```bash
git tag dogfood-2026-05-04
```

Do not create an external semver release yet. External release cadence remains deferred until daily dogfood of `apps/engineer`, `agentsdk build`, daemon/triggers, and examples exposes no blocking friction.
