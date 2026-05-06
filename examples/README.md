# Examples and dogfood apps

This directory contains small examples that exercise the blessed agentsdk paths. Keep examples intentionally narrow: reusable, daily-use dogfood agents belong under `apps/`; tutorial or integration samples belong under `examples/`.

## Current examples

| Path | Purpose | Primary surface |
|------|---------|-----------------|
| `local-quickstart/` | Minimal resource-only app for `agentsdk run` and `agentsdk discover`. | `.agents` resources |
| `resource-only-app/` | Resource-only workflow/command/trigger sample. | `.agents` resources |
| `workflow-app/` | Declarative workflow with command exposure. | workflows + commands |
| `command-tree/` | Structured YAML and Markdown command resources. | command resources |
| `hybrid-app/` | Resource bundle plus appconfig plugin composition. | appconfig + plugins |
| `datasource/` | Go-native datasource definition/registry example. | `datasource` package |
| `action-tool-adapter/` | Go-native action/tool adapter example. | `action` + `tool` packages |
| `devops-cli/` | Branded embedded CLI using `terminal/cli.NewCommand`. | separate Go module |
| `research-desk/` | Custom Cobra host using app/harness/repl directly. | separate Go module |
| `release-notes-agent/` | Resource-only prompt sketch for release notes. | `.agents` resources |
| `repo-maintainer/` | Resource-only repo maintenance sketch. | `.agents` resources |

## Apps vs examples

- Put durable first-party dogfood products in `apps/` when they are useful for daily development or intended to be launched directly by agentsdk commands.
- Put teaching samples in `examples/` when they demonstrate one SDK concept and should stay small.
- Avoid compatibility copies. If an example graduates into `apps/`, leave docs pointing at the app or remove the duplicate resource tree.

## Verification

From the repository root:

```bash
go test ./...
```

For nested Go-module examples, test them from their own directories:

```bash
(cd examples/devops-cli && go test ./...)
(cd examples/research-desk && go test ./...)
```

Resource examples can also be inspected with:

```bash
go run ./cmd/agentsdk discover --local examples/local-quickstart
go run ./cmd/agentsdk discover --json examples/workflow-app
```
