# Builder product

Section 24 turns the builder from a placeholder into the first real first-party dogfood application.

## Product boundary

`agentsdk build` is a thin launcher. It does not parse product-specific `new`, `refine`, or `verify` flags. Instead, it starts the embedded builder app and lets the app's own agent, skills, tools, commands, and workflows route the product conversation.

```bash
cd /path/to/project-under-construction
agentsdk build
```

The builder app is initialized from `apps/builder/resources`, while the current working directory is treated as the project under construction.

## Runtime layout

```text
apps/builder/
  app.go
  README.md
  resources/
    agentsdk.app.json
    .agents/
      agents/builder.md
      skills/...
      commands/...
      workflows/...
```

Session storage is project-local:

```text
<cwd>/.agentsdk/builder/sessions
<cwd>/.agentsdk/builder/target-sessions
```

The first path stores the builder's own conversations. The second path stores isolated sessions for apps the builder tests.

## Builder as tester

The builder is the controller/tester. The cwd app is the system under test. Helper actions keep those boundaries explicit:

- `builder_inspect_project`
- `builder_discover_target`
- `builder_run_target_smoke`
- `builder_scaffold_resource_app`
- `builder_write_project_file`
- `tools_list`, `tools_activate`, `tools_deactivate`
- `web_fetch`, `web_search`

`builder_run_target_smoke` loads the cwd project through the normal CLI/harness load path and executes non-destructive command checks such as `/session info` and `/workflow list`. It does not send arbitrary prompts or execute risky target tools by default.

## Safety conventions

- Builder resources are embedded and separate from cwd resources.
- File writes are rejected when paths escape the project directory.
- Existing files require explicit overwrite/force input.
- Target app sessions are isolated from builder sessions.
- Deployment remains guidance-only in v1 unless the user explicitly asks for files to be generated.
- Web access is available for documentation lookups. `web_fetch` works without external configuration; `web_search` uses `TAVILY_API_KEY` when configured and otherwise reports a clear configuration error.

## Dogfood commands

The builder resources include declarative command metadata that points users toward workflows, plus guaranteed built-in workflow commands:

```text
/workflow list
/workflow start new_app
/workflow start refine_requirements
/workflow start verify_app
/workflow start test_target_agent
```

The app is also inspectable like any other resource app:

```bash
agentsdk discover --local apps/builder/resources
agentsdk discover --local --json apps/builder/resources
```
