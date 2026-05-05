# Agent spec deep dive

## File format

Agent specs live at `.agents/agents/<name>.md`. The file has two parts:

1. YAML frontmatter between `---` delimiters
2. Markdown body = the system prompt

```markdown
---
name: main
description: What this agent does
tools:
  - bash
  - file_*
  - web_fetch
  - skill
  - tools_*
skills:
  - my-skill
commands: [my-command]
capabilities: [planner]
max-steps: 100
---
System prompt content as Markdown.
```

## Frontmatter fields

| Field | Required | Effect |
|-------|----------|--------|
| `name` | yes | Agent identity |
| `description` | yes | Short description |
| `tools` | recommended | Tool selection patterns from catalog |
| `skills` | optional | Skills to pre-activate at session start |
| `commands` | optional | Commands this agent exposes |
| `capabilities` | optional | Runtime capabilities (e.g. `planner`) |
| `max-steps` | optional | Max turns per session (default 30) |

## What happens without frontmatter

If an agent `.md` file has no `---` delimiters:
- The agent gets a name derived from the filename
- The entire file content becomes the system prompt
- **No tools are selected** — the agent cannot use any tools
- **No skills are pre-activated**
- **No capabilities are attached** (no planner, etc.)
- **No commands are exposed**

This is almost always a bug. Every agent file should have frontmatter.

## Tool selection

The `tools:` field selects from the registered tool catalog using exact
names or glob patterns:

```yaml
tools:
  - bash           # exact match
  - file_*         # glob: file_read, file_write, file_edit, file_stat, file_delete
  - git_*          # glob: git_status, git_diff, git_add, git_commit
  - tools_*        # tool management: tools_list, tools_activate, tools_deactivate
  - builder_*      # all builder tools
```

If `tools:` is omitted entirely, the agent gets `DefaultTools` from all
plugins. If `tools:` is present but empty (`tools: []`), the agent gets
no tools at all.

**Always include `tools_*`** so the agent can self-serve tool activation
at runtime.

## Skill pre-activation

```yaml
skills:
  - dex
  - babelforce
```

Skills listed here are activated when the session starts. Their content is
injected into the agent context. The skills must be discoverable — either
locally in `.agents/skills/` or globally in `~/.agents/skills/` /
`~/.claude/skills/` (requires `include_global_user_resources: true`).

## Capability attachment

```yaml
capabilities: [planner]           # shorthand
capabilities:
  - name: planner                 # explicit form
    instance_id: my_planner
```

The `planner` capability gives the agent structured task planning with
the `plan` tool.

## Common mistakes

1. **No frontmatter** → agent has no tools, skills, or capabilities
2. **`skills: [dex]` without global discovery** → skill not found at runtime
3. **Missing `tools_*`** → agent can't activate additional tools
4. **`max-steps` too low** → agent stops mid-task on complex work
