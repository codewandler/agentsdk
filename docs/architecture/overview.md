# Architecture overview

agentsdk is organized around explicit composition, live harness sessions, structured execution primitives, durable thread state, and thin host/channel adapters.

## Current runtime path

```text
cmd/agentsdk
  -> terminal/cli or daemon/channel host
  -> agentdir/resource discovery
  -> app.App composition
  -> harness.Service / harness.Session
  -> agent/runtime/runner
  -> action/tool/command/workflow/thread subsystems
```

## Ownership rules

| Area | Owns | Must not own |
| --- | --- | --- |
| `resource`, `agentdir` | Declarative metadata and filesystem/resource loading. | Live sessions, runtime state, rendering. |
| `app`, `plugins/*` | Reusable definitions, registries, named contribution bundles. | Harness/session lifecycle, terminal or daemon policy. |
| `harness` | Live session lifecycle, command/workflow dispatch, event fanout. | Product CLI parsing, terminal rendering, protocol wire formats. |
| `daemon`, `terminal/*`, `channel/*`, `cmd/agentsdk` | Host/process/channel policy and presentation. | Core runtime/session semantics. |
| `agent` | Agent spec/config, model policy, current runtime facade. | Long-term generic session/thread ownership. |
| `runtime`, `runner` | Model/tool turn execution. | App loading, harness sessions, channels. |
| `action`, `workflow`, `command`, `tool` | Execution primitive and projection surfaces. | Host/channel ownership. |
| `thread`, `conversation` | Durable event/state foundation. | App or channel policy. |

## Current main cleanup pressure

`agent.Instance` still combines too many live concerns: model policy, runtime construction, thread/session setup, skill activation, context/capability wiring, usage, compaction, and event/output plumbing. The desired direction is to move session/thread lifecycle and channel-facing events toward harness/session without adding compatibility shims or parallel runtimes.

See [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) for the consolidated review backlog.
