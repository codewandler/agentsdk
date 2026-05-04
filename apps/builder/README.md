# agentsdk Builder

The builder is the first-party dogfood app for designing, scaffolding, refining, and testing agentsdk applications.

Run it from the project-under-construction directory:

```bash
agentsdk build
```

`agentsdk build` loads the embedded builder resources from this app. The current working directory is treated as the target project, not as the builder's own agentdir.

Runtime conventions:

- Builder sessions: `.agentsdk/builder/sessions`
- Isolated target test sessions: `.agentsdk/builder/target-sessions`
- Builder resources: `apps/builder/resources`

Useful builder workflows:

```text
/workflow start new_app
/workflow start refine_requirements
/workflow start verify_app
/workflow start test_target_agent
```

The v1 helper actions are intentionally constrained: inspect the project, discover the target app, run non-destructive target smoke checks, scaffold a minimal resource app, and write explicitly requested files under the project directory.

The builder also has web access for current documentation lookups: `web_fetch` is available by default, and `web_search` uses `TAVILY_API_KEY` when configured. Tool-management tools (`tools_list`, `tools_activate`, `tools_deactivate`) let the builder inspect and activate available tools as needed.
