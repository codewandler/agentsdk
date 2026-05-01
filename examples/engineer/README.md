# Engineer example compatibility

The engineer agent has been promoted to a first-party dogfood app at
[`apps/engineer`](../../apps/engineer/).

This directory remains as a compatibility copy during the transition. Prefer the
new app path for docs, dogfood work, and smoke tests:

```bash
go run ./cmd/agentsdk run apps/engineer
```
