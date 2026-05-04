# Discover / introspection

Section 23 keeps `agentsdk discover` as the debugging surface for understanding what a resource directory, app manifest, and plugin set contribute before anything is run.

## Human-readable discovery

`agentsdk discover [path]` remains optimized for quick terminal inspection. It reports:

- resource sources considered by discovery;
- agent specs and their resource IDs;
- configured capability attachments per agent;
- executable command descriptors, including caller/safety policy labels when present;
- skill definitions and skill references with trigger metadata;
- datasource, workflow, action, trigger, and structured command resource descriptors;
- manifest plugin refs and whether structured config is present;
- disabled suggestions and diagnostics.

This command is intentionally a debugging surface, not a runtime API. Channels that need machine-readable command execution should use harness/session command APIs instead of parsing this text.

## Machine-readable discovery

`agentsdk discover --json [path]` emits a stable JSON snapshot for tooling and smoke tests. The payload includes the same high-level sections as the text output:

```json
{
  "sources": [],
  "agents": [],
  "commands": [],
  "skills": [],
  "skillReferences": [],
  "datasources": [],
  "workflows": [],
  "actions": [],
  "triggers": [],
  "structuredCommands": [],
  "plugins": [],
  "capabilities": [],
  "diagnostics": []
}
```

Use `--local` with `--json` for reproducible project-only snapshots:

```bash
agentsdk discover --local --json .
```

## Descriptor conventions

- `commands` are executable slash-command descriptors from the loaded app command registry.
- `structuredCommands` are declarative command resources from `.agents/commands/*.yaml`; they are metadata until a harness/channel projects them into an execution surface.
- `workflows`, `actions`, `triggers`, and `datasources` are declarative resource descriptors from the resolved bundle.
- `plugins` reports manifest refs and whether config was provided; it does not expose resolved runtime internals or secrets.
- `capabilities` reports configured agent attachments from specs. Live runtime state remains a session/harness concern.

## Compatibility notes

The JSON shape is meant for diagnostics and CI smoke assertions. It should be additive: new descriptor sections or fields can be added, but existing top-level names should remain stable unless a task-list section explicitly migrates them.
