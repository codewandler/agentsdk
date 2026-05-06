# Resource Format References

This file tracks external resource-format references used by agentsdk design
and implementation decisions.

## Claude Code

- Claude Code subagents: https://docs.claude.com/en/docs/claude-code/subagents
- Claude Code slash commands: https://docs.claude.com/en/docs/claude-code/slash-commands
- Claude Code plugins: https://docs.claude.com/en/docs/claude-code/plugins
- Claude Code skills: https://docs.claude.com/en/docs/claude-code/skills

Relevant compatibility layouts:

```text
.claude/
  agents/
  commands/
  skills/

plugin/
  .claude-plugin/
    plugin.json
  agents/
  commands/
  skills/
```

## Agent Skills

- OpenCode skills: https://opencode.ai/docs/skills

Relevant compatibility layouts:

```text
.claude/skills/<skill>/SKILL.md
.agents/skills/<skill>/SKILL.md
~/.claude/skills/<skill>/SKILL.md
~/.agents/skills/<skill>/SKILL.md
```

`SKILL.md` directories are the canonical skill format for agentsdk. agentsdk
loads `.agents/skills` as a skill compatibility source, but `.agents/agents`
and `.agents/commands` are not ambient default layouts.

Within a skill directory, agentsdk also recognizes optional reference files under
`references/`, for example:

```text
.agents/skills/<skill>/SKILL.md
.agents/skills/<skill>/references/<file>.md
```

For runtime skill activation, only exact relative paths under `references/` are
eligible as activatable skill references.

## Agentsdk Native Appconfig

Agentsdk supports `agentsdk.app.yaml`, `agentsdk.app.yml`, and
`agentsdk.app.json` as native appconfig entry files. This is an agentsdk-specific
composition format, not an external standard. It intentionally points at
resource roots and appconfig documents rather than inventing new
agent/command/skill file formats.

Current appconfig keys:

```json
{
  "default_agent": "coder",
  "discovery": {
    "include_global_user_resources": false,
    "include_external_ecosystems": false,
    "allow_remote": false,
    "trust_store_dir": ".agentsdk"
  },
  "model_policy": {
    "use_case": "agentic_coding",
    "source_api": "auto",
    "approved_only": false,
    "allow_degraded": false,
    "allow_untested": false,
    "evidence_path": ".agentsdk/compatibility/agentic_coding.json"
  },
  "sources": [
    ".agents",
    "resources/shared.yaml",
    "resources/*.yaml"
  ]
}
```

`sources` entries load additional `.agents`, `.claude`, plugin-root resource
layouts, or appconfig documents from local paths or glob patterns. Local
`.agents` and `.claude` directories next to the entry file are loaded by default.

`model_policy` is an agentsdk runtime policy, not a resource format. It can ask
agentsdk to evaluate or enforce llmadapter compatibility evidence for a use case
such as `agentic_coding`. `source_api: "auto"` allows route selection across
supported source APIs; explicit source APIs restrict both selection and runtime
routing. Relative `evidence_path` values are resolved relative to the appconfig entry
file directory.

## Agentsdk Native Declarative Resources

Agentsdk discovers datasource, workflow, action, trigger, and structured command
YAML resources from native `.agents` layouts:

```text
.agents/datasources/*.yaml
.agents/workflows/*.yaml
.agents/actions/*.yaml
.agents/triggers/*.yaml
.agents/commands/*.yaml
```

When an appconfig source points directly at a resource root such as `.agents`, the
same resources are loaded from the corresponding plugin-root layout:

```text
datasources/*.yaml
workflows/*.yaml
actions/*.yaml
triggers/*.yaml
commands/*.yaml
```

These YAML files are agentsdk-specific discovery resources. Workflow YAML is
converted into executable workflow definitions when the referenced actions are
available. Workflows can expose command and trigger projections with `expose`.
Action YAML is declarative metadata for host/plugin-provided actions; it does
not create executable actions by itself. Trigger YAML is converted by
`agentsdk serve` into in-process interval trigger jobs and may declare inline
workflow targets. Structured command YAML lives alongside Markdown commands and
can target a workflow, action, prompt, or inline workflow. Datasource expansion
remains deferred until daemon/triggers and a concrete datasource case study are
ready.

Initial datasource metadata keys:

```yaml
name: docs
description: Documentation corpus
kind: corpus
config:
  path: docs
metadata:
  owner: example
```

`kind` may also be written as `type` for compatibility with common YAML naming.

Initial workflow metadata keys:

```yaml
name: sync_docs
description: Sync documentation
version: v1
steps:
  - id: fetch
    action: docs.fetch
    depends_on: [prepare]
    input_map:
      query: input.query
    retry:
      max_attempts: 2
      backoff: 1s
    timeout: 30s
    error_policy: continue
    idempotency_key: sync-docs-fetch
```

Initial action metadata keys:

```yaml
name: host.echo
description: Host-provided echo action
kind: host
config:
  namespace: examples
metadata:
  owner: example
```

Initial trigger metadata keys:

```yaml
id: hourly-summary
description: Periodically summarize a session
source:
  interval: 1h
  immediate: false
target:
  workflow: session_summary
  input: Summarize the current session.
session:
  mode: trigger_owned
policy:
  overlap: skip_if_running
```

Initial structured command metadata keys:

```yaml
name: session-summary
description: Run a workflow from a command path
path: [session, summary]
input_schema:
  type: object
  properties:
    input:
      type: string
target:
  workflow: session_summary
  input: "{{ .input }}"
policy:
  user_callable: true
  agent_callable: true
```

## Project Instructions

- AGENTS.md: https://agents.md/

`AGENTS.md` is a project instruction convention. It is not an agent, command,
skill, or plugin bundle format.
