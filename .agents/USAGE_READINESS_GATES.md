# Usage readiness gates

This checkpoint records whether agentsdk is ready for our own daily pre-1.0 use. It is not an external release promise and does not add legacy compatibility obligations.

## Current readiness decision

agentsdk is ready for local/internal dogfooding through the current blessed paths:

- `agentsdk run` for terminal sessions.
- `agentsdk discover` for resource/app inspection.
- `harness.LoadSession` and `harness.Service` for programmatic session hosting.
- `apps/engineer` and `apps/builder` as first-party dogfood apps.
- `examples/*` as small instructional checks, not compatibility fixtures.

More stable public-facing usage stays deferred. The project is still pre-1.0 and we are the only consumer, so cleanup should continue to prefer deleting stale paths over supporting fallback behavior.

## Manual local dogfood pass

Confirmed on 2026-05-04 from the repository root:

```bash
go run ./cmd/agentsdk run . /session info
go run ./cmd/agentsdk run . /workflow list
go run ./cmd/agentsdk discover --local apps/engineer
go run ./cmd/agentsdk discover --local examples/local-quickstart
```

Observed results:

- `agentsdk run . /session info` opened the default terminal-local session and printed session ID, agent, thread, branch, and resolved model metadata.
- `agentsdk run . /workflow list` executed through the harness command path and reported that no workflows are registered for the repository-root default app.
- `agentsdk discover --local apps/engineer` discovered the first-party engineer app, its main agent, planner capability, slash commands, and skills.
- `agentsdk discover --local examples/local-quickstart` discovered the minimal resource-only quickstart agent and command.

## Automated coverage gate

The default internal SDK path is covered by terminal/harness end-to-end tests, especially:

- `terminal/cli/e2e_test.go` `TestE2ELocalCLIPluginHarnessLoadAndSessionCommandProjection`
- `terminal/cli/e2e_test.go` `TestE2EOneShotCommandRenderingAndNoDefaultPlugins`
- `harness/load_test.go` `TestLoadSessionCreatesDefaultHarnessSession`
- `harness/projection_test.go` `TestDefaultSessionAttachesAgentCommandProjection`

These verify that the terminal default local CLI plugin path opens a harness-backed session, attaches the session command projection, renders one-shot command output, and rejects the no-default-plugin path when no app agents are present.

## Documentation gate

Broader pre-1.0 usage is documented through:

- [`docs/05_QUICKSTART.md`](../docs/05_QUICKSTART.md)
- [`examples/README.md`](../examples/README.md)
- [`RELEASE_READINESS.md`](RELEASE_READINESS.md)
- first-party dogfood app docs under [`apps/engineer`](../apps/engineer) and [`apps/builder`](../apps/builder)

## Deferred public-facing readiness

Do not present this as a stable external release yet. Public-facing readiness waits for more daily dogfood of:

- structured display/output design across terminal, HTTP/SSE, and future UI channels;
- async workflow lifecycle ergonomics in real use;
- builder-generated app quality;
- daemon/triggers behavior over longer-running sessions;
- examples proving the smallest blessed path without obsolete compatibility notes.

Until then, use internal checkpoint tags and keep breaking stale APIs when that makes the current path cleaner.
