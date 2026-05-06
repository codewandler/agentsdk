# SDK resource conventions

## File naming

- Agent specs: `.agents/agents/<name>.md` — name from frontmatter `name:` field.
- Skills: `.agents/skills/<name>/SKILL.md` — directory name is the skill name.
- Workflows: `.agents/workflows/<name>.yaml` — `name:` field inside YAML.
- Commands: `.agents/commands/<name>.yaml` or `.md` — YAML for structured, MD for prompt.
- Actions: `.agents/actions/<name>.yaml` — declarative metadata for host-provided actions.
- Triggers: `.agents/triggers/<name>.yaml` — interval or event-based trigger definitions.
- Datasources: `.agents/datasources/<name>.yaml` — metadata for data sources.

## Discovery

```bash
agentsdk discover --local .          # discover resources in current directory
agentsdk discover --local --json .   # JSON output for programmatic use
```

Discovery reads `agentsdk.app.yaml`, `agentsdk.app.yml`, or `agentsdk.app.json`,
default local `.agents`/`.claude` directories, and explicit appconfig `sources`
entries. Each source can be a local resource directory, config document, or glob.

## Tool selection in agent specs

The `tools:` field in agent frontmatter selects from the registered tool catalog:

```yaml
tools:
  - bash           # exact name
  - file_*         # glob pattern — matches file_read, file_write, file_edit, etc.
  - builder_*      # all builder tools
  - tools_*        # tool management (list, activate, deactivate)
```

If `tools:` is omitted entirely, the agent gets `DefaultTools` from all plugins.
If `tools:` is present but empty, the agent gets no tools.

## Capability attachment

```yaml
capabilities: [planner]    # shorthand — uses capability name as instance ID
capabilities:
  - name: planner           # explicit form
    instance_id: my_planner
```

## Branding

Use `agentsdk` naming in all new code and resources. Do not introduce new `flai` references.
The predecessor project name appears in some existing constants for downstream compatibility.

## Appconfig keys reference

```json
{
  "default_agent": "main",
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
  "sources": [".agents"]
}
```
