# agentsdk

agentsdk is a Go SDK for building agentic command-line tools, local apps, and harness-driven agent runtimes.

It provides the reusable infrastructure for agent apps — tools, actions, commands, workflows, resource loading, sessions, persistence, triggers, safety policy, and runtime execution — while keeping product policy and presentation at the application boundary.

## What you can build

- Local agent CLIs with explicit tool/plugin composition.
- Resource-defined agent apps using `.agents` directories.
- Harness sessions that can run user turns, commands, and workflows.
- Daemon/service-style agents with triggers and scheduling.
- HTTP/SSE or terminal channels over the same harness/session model.
- First-party dogfood apps such as the builder app.

## Highlights

- **Explicit composition** — no hidden standard tool bundle; apps choose named plugins and tools deliberately.
- **Harness sessions** — one live session boundary for turns, commands, workflows, event subscriptions, and persistence.
- **Structured commands and workflows** — command trees, typed results, workflow run metadata, async lifecycle, and replayable events.
- **Resource loading** — declarative agents, commands, workflows, triggers, skills, and manifests from `.agents` resources.
- **Runtime primitives** — actions for Go-native execution, tools for LLM-facing projection, commands for user/channel projection, workflows for orchestration.
- **Durable threads** — append-only JSONL-backed session events with replay/read-model support.
- **Safety and memory** — risk-policy primitives, approval/audit seams, and visible default-on compaction.
- **Dogfood apps** — examples and first-party apps that exercise the public SDK path.

## Quick start

Install the CLI:

```bash
go install ./cmd/agentsdk
```

Run a local resource app:

```bash
agentsdk run examples/local-quickstart
```

Inspect available resources:

```bash
agentsdk discover --local examples/workflow-app
```

Start a workflow from the CLI:

```bash
agentsdk run examples/workflow-app /workflow list
agentsdk run examples/workflow-app /workflow start session_info_flow hello
```

## Documentation

Read [`docs/README.md`](docs/README.md) for guides, reference docs, architecture, examples, and internal notes.

## Status

agentsdk is pre-1.0 and under active dogfood development. APIs may change while the runtime, harness, and app/resource boundaries are still being simplified.

## License

MIT
