# agentsdk Builder

The builder is the first-party dogfood app for designing, scaffolding, refining, and testing agentsdk applications.

Run it from the project-under-construction directory:

```bash
agentsdk build
```

`agentsdk build` loads the embedded builder resources from this app. The current working directory is treated as the target project, not as the builder's own agentdir.

## Tools

The builder agent has full access to filesystem, shell, git, web, and vision tools — the same power tools available to the engineer app. This lets the builder explore the project, run external CLIs, read documentation, and understand integrations the target app needs.

| Tool | Purpose |
|------|---------|
| `bash` | Run shell commands, external CLIs, build tools |
| `file_read` | Read files with line numbers |
| `file_write` | Create or overwrite files |
| `file_edit` | Precise edits — replace, insert, remove, patch |
| `file_stat` | File metadata (size, permissions, modification time) |
| `file_delete` | Delete files |
| `grep` | Regex search across files |
| `glob` | Find files by pattern |
| `dir_tree` | Recursive directory tree |
| `dir_list` | Directory listing with metadata |
| `git_status` | Working tree status |
| `git_diff` | Diff of staged or unstaged changes |
| `git_add` | Stage explicit paths |
| `git_commit` | Commit staged changes |
| `web_fetch` | Fetch a URL and extract content |
| `web_search` | Search the web (requires Tavily API key) |
| `vision` | Analyze images and screenshots |
| `skill` | Activate skills and references at runtime |
| `tools_*` | List, activate, and deactivate tools at runtime |

### Builder-specific tools

| Tool | Purpose |
|------|---------|
| `builder_inspect_project` | Inspect the project directory for agentsdk app files |
| `builder_discover_target` | Discover the project as an isolated target agentsdk app |
| `builder_run_target_smoke` | Run non-destructive smoke checks on the target app |
| `builder_scaffold_resource_app` | Scaffold a minimal resource-only app |
| `builder_write_project_file` | Write a file under the project directory with path-safety checks |

## Capabilities

| Capability | Description |
|------------|-------------|
| `planner` | Structured task plans for multi-step work |

## Workflows

| Workflow | Command | Description |
|----------|---------|-------------|
| `new_app` | `/new-app` | Inspect and scaffold a minimal resource app |
| `refine_requirements` | `/refine-requirements` | Turn a vague idea into app resources and next steps |
| `verify_app` | `/verify-app` | Inspect and smoke-test the current project |
| `test_target_agent` | `/test-target-agent` | Load the project as an isolated target app and report findings |

## Bundled skills

| Skill | Description |
|-------|-------------|
| requirements | Requirements refinement for agentsdk apps |
| app-architecture | App architecture guidance and boundary decisions |
| sdk-conventions | agentsdk naming, resources, and hybrid app conventions |
| scaffolding | Resource-only and hybrid app scaffolding guidance |
| testing | Target app discovery and smoke testing guidance |
| deployment | Deployment guidance for generated apps |

## Runtime conventions

- Builder sessions: `.agentsdk/builder/sessions`
- Isolated target test sessions: `.agentsdk/builder/target-sessions`
- Builder resources: `apps/builder/resources` (embedded)

## Example usage

```text
# Explore an external CLI before integrating it
which dex && dex --help

# Scaffold a new app
/new-app

# Refine requirements interactively
/refine-requirements

# Verify the current project
/verify-app

# Run isolated target smoke checks
/test-target-agent
```

The builder can run any shell command to explore external tools, read their documentation, and understand their interfaces before designing the agent resources that wrap them.
