# Resource-only app scaffolding reference

## Minimal resource-only app

```text
project/
  agentsdk.app.json
  README.md
  .agents/
    agents/main.md
```

This is the smallest valid app. It has one agent with a system prompt.

## Standard resource-only app

```text
project/
  agentsdk.app.json
  README.md
  .agents/
    agents/main.md
    skills/<name>/SKILL.md
    skills/<name>/references/*.md
    workflows/*.yaml
    commands/*.yaml
```

## Agent spec template

```markdown
---
name: main
description: <what this agent does>
tools:
  - bash
  - file_read
  - file_write
  - file_edit
  - file_stat
  - file_delete
  - grep
  - glob
  - dir_tree
  - dir_list
  - git_status
  - git_diff
  - git_add
  - git_commit
  - web_fetch
  - web_search
  - skill
  - tools_*
capabilities: [planner]
max-steps: 100
---
System prompt goes here.
```

## Appconfig template

```json
{
  "default_agent": "main",
  "discovery": {
    "include_global_user_resources": true,
    "include_external_ecosystems": false,
    "allow_remote": false,
    "trust_store_dir": ".agentsdk"
  },
  "model_policy": {
    "use_case": "agentic_coding",
    "source_api": "auto"
  },
  "sources": [".agents"]
}
```

## Scaffolding workflow

1. Gather requirements (use `refine_requirements` workflow).
2. Create `agentsdk.app.json` with appropriate appconfig settings.
3. Create `.agents/agents/main.md` with tools, skills, and system prompt.
4. Add skills under `.agents/skills/` for domain knowledge.
5. Add workflows under `.agents/workflows/` for repeatable processes.
6. Add commands under `.agents/commands/` for user-facing entry points.
7. Verify with `builder_discover_target` and `builder_run_target_smoke`.
8. Write `README.md` documenting the app.

## Tool selection guidance

- **Always include** `tools_*` so the agent can self-serve tool activation at runtime.
- **Include `bash`** if the agent needs to run external commands or CLIs.
- **Include `file_*` + `grep` + `glob` + `dir_*`** for filesystem exploration.
- **Include `git_*`** if the agent works with git repositories.
- **Include `web_fetch` + `web_search`** if the agent needs web access.
- **Include `skill`** if the agent has skills that can be activated at runtime.
- **Include `vision`** if the agent needs to analyze images or screenshots.
