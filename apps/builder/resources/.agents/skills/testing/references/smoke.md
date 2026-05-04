# Target app testing reference

## Builder testing model

The builder is the tester/controller. The current project is the system under test.
Target app test sessions are isolated under `.agentsdk/builder/target-sessions`.

## Discovery checks

`builder_discover_target` resolves the target project's resource bundle and reports:

- Sources found (`.agents`, manifest entries)
- Agent specs (names)
- Commands, workflows, actions, triggers, datasources
- Diagnostics (parse errors, missing references, etc.)

A clean discovery with no diagnostics is the first gate.

## Smoke checks

`builder_run_target_smoke` performs non-destructive checks:

1. **Discover target app** — resolves resources (same as `builder_discover_target`).
2. **Load target harness** — creates an isolated session and loads the app.
3. **Target `/session info`** — verifies the session command works.
4. **Target `/workflow list`** — verifies workflow registration.

All checks report `passed` or `failed` with details.

## Manual verification steps

After automated smoke checks pass, verify manually:

```bash
# Discover resources
agentsdk discover --local .
agentsdk discover --local --json .

# Run the app interactively
agentsdk run .

# Test specific workflows
# (inside the running app)
/workflow list
/workflow start <workflow_name>
```

## Common failure patterns

| Symptom | Likely cause |
|---------|-------------|
| No agents found | Missing `.agents/agents/*.md` or bad frontmatter |
| Discovery diagnostics | YAML parse errors, missing referenced actions |
| Harness load fails | Invalid manifest, missing sources directory |
| Workflow list empty | No `.agents/workflows/*.yaml` files, or YAML errors |
| Tool not found | Agent spec `tools:` references a tool not in the catalog |

## Testing external integrations

When the target app integrates external CLIs or APIs:

1. Verify the binary exists: `which <tool>` or `<tool> --version`.
2. Run a safe read-only command to confirm it works.
3. Check that the agent's system prompt documents the tool's interface.
4. If the tool needs credentials, verify environment variables are documented.
