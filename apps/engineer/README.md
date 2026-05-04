# Engineer

A first-party dogfood agent for agentsdk development. It acts as a senior software
engineer with skills in architecture, code review, and DevOps. It does not
contain Go code — it is a pure resource bundle.

Run it from the agentsdk repository root:

```bash
go run ./cmd/agentsdk run apps/engineer
```

The bundled `main` agent is configured with `max-steps: 100` so longer multi-step coding and review sessions do not stop at the SDK default of 30.

This app also includes an `agentsdk.app.json` manifest that enables global user skill discovery, so `~/.agents/skills` and `~/.claude/skills` participate in `/skills` without requiring `--include-global`.
## Tools

The agent has access to filesystem, shell, git, and web tools:

| Tool | Purpose |
|------|---------|
| `bash` | Run shell commands |
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
| `git_add` | Stage explicit paths in the git index |
| `git_commit` | Commit staged changes, optionally staging explicit paths first |
| `web_fetch` | Fetch a URL and extract content |
| `web_search` | Search the web (requires Tavily API key) |

## Capabilities

| Capability | Description |
|------------|-------------|
| `planner` | Structured task plans for multi-step work |

## Commands

| Command   | Description                                                  |
| --------- | ------------------------------------------------------------ |
| `/review` | Review code changes for correctness, clarity, maintainability |
| `/design` | Produce a lightweight architecture design for a feature       |
| `/deploy` | Create a deployment checklist for a service or change         |

## Bundled skills

These skills are bundled with the app and discoverable through `/skills`, but they are not activated automatically. Activate them during a session with `/skill activate <name>` or the model-side `skill` tool.

| Skill          | Description                                              |
| -------------- | -------------------------------------------------------- |
| architecture   | System design, component boundaries, trade-off analysis  |
| code-review    | Structured code review with actionable feedback          |
| devops         | CI/CD pipelines, deployment strategies, infrastructure   |

## Example prompts

```text
/review the changes in src/auth/session.go — focus on error handling
```

```text
/design a rate-limiting layer in front of the public API
```

```text
/deploy the billing service v2.4.0 to production
```

```text
What are the trade-offs between an event-driven and a request-driven architecture for our notification system?
```

```text
Look up the latest Go 1.24 changes to the net/http package and summarize what affects our HTTP client code.
```

If you have an installed `agentsdk` binary:

```bash
agentsdk run apps/engineer
```

## Runtime skill activation

The engineer app supports runtime skill discovery and activation.

List discovered skills and their current state:

```text
/skills
```

Activate a discovered skill on the current agent session:

```text
/skill activate architecture
```

If the `skill` tool is available to the model, it can also activate skills and exact references under `references/` with batched actions.
