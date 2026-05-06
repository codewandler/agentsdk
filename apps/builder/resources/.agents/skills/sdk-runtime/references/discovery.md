# Discovery deep dive

## Appconfig resolution order

1. `agentsdk.app.yaml`
2. `agentsdk.app.yml`
3. `agentsdk.app.json`
4. No appconfig entry → fallback to `.agents/` and `.claude/` directory scan

## Appconfig `sources` field

Each entry in `sources` is resolved as a local resource directory, config
document, or glob pattern:

```json
{
  "sources": [
    ".agents",
    "resources/shared.yaml",
    "resources/*.yaml"
  ]
}
```

Discovery also scans `.agents/` and `.claude/` in the app directory by default.
Use `sources` for additional resource directories or config documents.

## Global user resources

Controlled by `discovery.include_global_user_resources` in appconfig:

```json
{
  "discovery": {
    "include_global_user_resources": true
  }
}
```

When enabled, these directories are scanned for skills:
- `~/.agents/skills/`
- `~/.claude/skills/`

When disabled (the default), global skills are invisible to the app.

## Discovery policy fields

| Field | Default | Effect |
|-------|---------|--------|
| `include_global_user_resources` | `false` | Scan `~/.agents/skills/` and `~/.claude/skills/` |
| `include_external_ecosystems` | `false` | Include non-agents ecosystem resources |
| `allow_remote` | `false` | Allow `git+https://` and `git+ssh://` sources |
| `trust_store_dir` | `""` | Directory for trust/verification data |

## What discovery produces

A `ContributionBundle` containing:
- Agent specs (from `.agents/agents/*.md`)
- Skills (from `.agents/skills/*/SKILL.md`)
- Workflows (from `.agents/workflows/*.yaml`)
- Actions (from `.agents/actions/*.yaml`)
- Commands (from `.agents/commands/*.yaml` and `.md`)
- Triggers (from `.agents/triggers/*.yaml`)
- Datasources (from `.agents/datasources/*.yaml`)
- Diagnostics (parse errors, missing references, etc.)

## Verifying discovery

```bash
agentsdk discover --local .          # human-readable
agentsdk discover --local --json .   # machine-readable JSON
agentsdk validate .                  # structural validation with checks
agentsdk validate --json .           # machine-readable validation
```

Or from the builder: `builder_validate_target` and `builder_discover_target`.
