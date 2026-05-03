# Resource-only app example

This example is intentionally configuration-only. It demonstrates the native `.agents` resource layout that `agentsdk discover`, `agentsdk run`, and `agentsdk serve` can load without Go code.

Try:

```bash
agentsdk discover --local examples/resource-only-app
agentsdk run examples/resource-only-app /workflow list
agentsdk serve examples/resource-only-app --status
```

The trigger in `.agents/triggers/hourly-summary.yaml` is declarative and daemon-loadable. It targets the `session_summary` workflow, which uses the built-in session-scoped `agent.turn` action when a live model/client is configured.
