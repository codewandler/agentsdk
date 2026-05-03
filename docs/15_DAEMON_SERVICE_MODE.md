# 15 — Harness daemon/service mode

Section 11 establishes daemon mode as a deployment shape for the existing harness runtime, not a second SDK runtime.

## Decision summary

- The CLI command shape is `agentsdk serve [path]`.
- Daemon mode is a harness deployment mode: `harness.Service` remains the runtime/session owner.
- The new `daemon` package is intentionally slim. It wraps `harness.Service` for process-level conventions: storage paths, status snapshots, and graceful shutdown.
- Resource, app, plugin, agent, command, and workflow loading continue to use the same `cli.Load(...)` / `harness.LoadSession(...)` path used by `agentsdk run`.
- The first smoke-testable CLI path is `agentsdk serve [path] --status`, which opens the service stack, prints status, and exits without starting an interactive REPL.

## Ownership model

```text
agentsdk serve
  terminal/cmd CLI glue
    daemon.Host
      harness.Service
        harness.Session
          app.App
          agent.Instance
          command.Tree
          workflow.Registry/RunStore
```

`daemon.Host` must not grow its own app, runtime, plugin, command, or workflow system. If behavior belongs to sessions or runtime execution, it belongs in `harness.Service` / `harness.Session` or below.

## Public APIs

Harness service APIs for long-running hosts:

```go
service := harness.NewService(application)
session, err := service.OpenSession(ctx, harness.SessionOpenRequest{
    Name:      "daily",
    AgentName: "coder",
    StoreDir:  ".agentsdk/sessions",
})
status := service.Status()
_ = service.Close()
```

Daemon wrapper APIs:

```go
host, err := daemon.New(daemon.Config{
    Service:     service,
    SessionsDir: ".agentsdk/sessions",
    ConfigPath:  "agentsdk.app.json",
})
status := host.Status()
_ = host.Shutdown(ctx)
```

`daemon.Host.OpenSession(...)` and `daemon.Host.ResumeSession(...)` default the request `StoreDir` to the daemon sessions directory when the caller does not provide one.

## CLI conventions

Status smoke path:

```bash
agentsdk serve . --status
```

Expected output includes:

```text
agentsdk service
mode: harness.service
health: ok
sessions: .agentsdk/sessions
active_sessions: 1
- <session-name> id=<session-id> agent=<agent> thread_backed=true
```

Long-running mode:

```bash
agentsdk serve .
```

This starts the same harness stack and waits for interrupt. Future HTTP/SSE, trigger, and scheduler control planes should attach to this host shape instead of adding a parallel runtime.

Useful flags:

```bash
agentsdk serve . --sessions-dir ./var/sessions
agentsdk serve . --agent coder
agentsdk serve . --plugin local_cli
agentsdk serve . --no-default-plugins
```

## Storage conventions

Default service session storage is:

```text
<resource-path>/.agentsdk/sessions
```

Use `--sessions-dir` for explicit deployments. Daemon-owned sessions should be thread-backed by default so workflow runs, future trigger fires, and resume/continue operations have a durable correlation point.

## Config/resource conventions

`agentsdk serve` uses the same resource path and manifest loading behavior as `agentsdk run`:

- resource path argument defaults to `.`;
- app manifests such as `agentsdk.app.json` are resolved by the existing agentdir/resource loader;
- manifest plugin refs and explicit `--plugin` refs flow through the existing plugin factory path;
- `--no-default-plugins` disables only the built-in local CLI fallback plugin, not manifest or explicit plugin refs.

## Verification

Covered by tests for:

- harness service status and registry lookup;
- daemon host lifecycle, persisted-session defaults, status, and shutdown;
- `agentsdk serve --status` CLI smoke output without entering an interactive REPL.

## Follow-up boundary

Section 12 should add triggers/scheduling on top of this shape. Trigger loops should target `daemon.Host` / `harness.Service` APIs and publish trigger-caused session/workflow metadata rather than creating a separate scheduler runtime.
