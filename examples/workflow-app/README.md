# Workflow app example

A resource-only workflow example. The workflow uses the built-in session-scoped `agent.turn` action, so it works in a loaded harness/session without custom Go code.

Try:

```bash
go run ./cmd/agentsdk discover --local examples/workflow-app
go run ./cmd/agentsdk run examples/workflow-app /workflow list
go run ./cmd/agentsdk run examples/workflow-app /workflow show summarize_topic
```

Starting the workflow requires a configured model client because the step prompts the active agent:

```bash
go run ./cmd/agentsdk run examples/workflow-app /workflow start summarize_topic "Summarize agentsdk workflows"
```
