---
name: scaffolding
description: Resource-only and hybrid app scaffolding guidance
---
# Scaffolding

Scaffold the smallest useful app first.

Default resource-only shape:

```text
agentsdk.app.json
.agents/agents/main.md
.agents/workflows/*.yaml
.agents/commands/*.yaml
.agents/actions/*.yaml
README.md
```

Use `builder_scaffold_resource_app` for constrained initial scaffolding. Use `builder_write_project_file` only for explicit file writes under the project directory.
