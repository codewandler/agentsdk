---
name: sdk-conventions
description: agentsdk naming, resources, and hybrid app conventions
---
# SDK conventions

Use `agentsdk` naming. Keep app resources under `.agents/`. Keep first-party plugins under `plugins/` and plugin interfaces under `app/plugin.go`.

Resource apps should be inspectable with:

```bash
agentsdk discover --local .
agentsdk discover --local --json .
```

Hybrid apps should keep Go helpers small and expose behavior through actions/tools/workflows rather than bespoke CLI parsing.
