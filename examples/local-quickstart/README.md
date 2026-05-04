# Local quickstart example

A minimal resource-only app for the recommended `agentsdk run` path. It keeps all behavior in `.agents` resources and requires no Go code.

Try it from the repository root:

```bash
go run ./cmd/agentsdk discover --local examples/local-quickstart
go run ./cmd/agentsdk run examples/local-quickstart /session info
```

The agent intentionally uses no explicit tools so it is safe to load with `--no-default-plugins` while learning the resource layout:

```bash
go run ./cmd/agentsdk run examples/local-quickstart --no-default-plugins /workflow list
```
