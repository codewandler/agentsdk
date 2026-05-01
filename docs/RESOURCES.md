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

## Agentsdk Native App Manifests

Agentsdk supports `app.manifest.json` and `agentsdk.app.json` as native app
manifests. These are agentsdk-specific composition files, not an external
standard. They intentionally point at external resource formats rather than
inventing new agent/command/skill file formats.

Current manifest keys:

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
    "file:///absolute/path/to/plugin",
    "git+https://github.com/codewandler/agentplugins.git#main",
    "git+ssh://git@github.com/codewandler/agentplugins.git#main"
  ]
}
```

`sources` entries load the same `.agents`, `.claude`, or plugin-root resource
layouts from local directories or git materialized directories.

`model_policy` is an agentsdk runtime policy, not a resource format. It can ask
agentsdk to evaluate or enforce llmadapter compatibility evidence for a use case
such as `agentic_coding`. `source_api: "auto"` allows route selection across
supported source APIs; explicit source APIs restrict both selection and runtime
routing. Relative `evidence_path` values are resolved relative to the manifest
directory.

## Agentsdk Native Datasource and Workflow Resources

Agentsdk discovers datasource and workflow YAML resources from native `.agents`
layouts:

```text
.agents/datasources/*.yaml
.agents/workflows/*.yaml
```

When a manifest source points directly at a resource root such as `.agents`, the
same resources are loaded from the corresponding plugin-root layout:

```text
datasources/*.yaml
workflows/*.yaml
```

These datasource and workflow YAML files are agentsdk-specific discovery
resources. They are deliberately declarative-only at first: `agentsdk discover`
loads and reports their name, description, kind/source metadata, and provenance,
but workflow execution and datasource runtime behavior are wired in later
milestones through the action/workflow/app registries.

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
steps:
  - id: fetch
    action: docs.fetch
```

The loader preserves the full workflow YAML mapping as a declarative definition
for future validation/execution layers.

## Project Instructions

- AGENTS.md: https://agents.md/

`AGENTS.md` is a project instruction convention. It is not an agent, command,
skill, or plugin bundle format.
