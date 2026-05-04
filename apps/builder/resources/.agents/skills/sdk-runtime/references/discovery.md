# Discovery deep dive

## Manifest resolution order

1. `agentsdk.app.json` (preferred)
2. `app.manifest.json` (legacy)
3. No manifest → fallback to `.agents/` and `.claude/` directory scan

## Manifest `sources` field

Each entry in `sources` is resolved as a resource directory:

```json
{
  "sources": [
    ".agents",                                    // local relative path
    "file:///absolute/path/to/plugin",            // absolute local path
    "git+https://github.com/org/repo.git#main"   // remote git (if allow_remote)
  ]
}
```

Without `sources`, discovery falls back to scanning `.agents/` and `.claude/`
in the app directory. This fallback is silent — the app appears to work but
the manifest is not driving discovery.

## Global user resources

Controlled by `discovery.include_global_user_resources` in the manifest:

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
