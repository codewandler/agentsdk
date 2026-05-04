# Harness/session/channel boundary review

This is a docs-only review of the host execution layer: `harness`, `daemon`, `channel/httpapi`, `terminal/*`, and `cmd/agentsdk`. It follows the package-level import analysis and the app/resource/plugin boundary review. No code changes are part of this batch.

## Review command

```bash
go list -f '{{.ImportPath}} {{join .Imports " "}}' ./harness ./daemon ./channel/... ./terminal/... ./cmd/agentsdk
```

Internal imports observed:

```text
harness
  -> action
  -> agent
  -> agentcontext
  -> agentdir
  -> app
  -> capability
  -> command
  -> resource
  -> skill
  -> thread
  -> thread/jsonlstore
  -> tool
  -> trigger
  -> usage
  -> workflow

daemon
  -> harness
  -> trigger

channel/httpapi
  -> action
  -> command
  -> harness

terminal/cli
  -> agent
  -> agentdir
  -> app
  -> command
  -> harness
  -> plugins/localcli
  -> resource
  -> runner
  -> terminal/repl
  -> terminal/ui
  -> tool

terminal/repl
  -> command
  -> terminal/ui
  -> usage

terminal/ui
  -> agent
  -> runner
  -> usage

cmd/agentsdk
  -> agent
  -> agentdir
  -> app
  -> apps/builder
  -> command
  -> daemon
  -> resource
  -> runtime
  -> skill
  -> terminal/cli
  -> trigger
```

## Intended boundary

| Package/group | Job | Must not own |
| --- | --- | --- |
| `harness` | Live session lifecycle and session-bound execution. | Product CLI parsing, terminal rendering, HTTP wire protocol, app/resource discovery policy not needed by all hosts. |
| `daemon` | Long-running process/service conventions over `harness.Service`. | A second app/runtime/plugin/session system. |
| `channel/httpapi` | Native HTTP/SSE and compatibility adapter surface over harness/session APIs. | Core harness semantics or protocol-specific types inside `harness`. |
| `terminal/cli` | Terminal host policy, local CLI fallback policy, flags, one-shot/REPL selection. | Canonical runtime/session semantics. |
| `terminal/repl` | Interactive loop and command/turn dispatch UX. | App loading, plugin/resource composition, generic session lifecycle. |
| `terminal/ui` | Terminal rendering of runner/usage/command/session output. | Runtime state ownership or direct agent mutation. |
| `cmd/agentsdk` | Product executable command tree and top-level wiring. | Reusable SDK runtime behavior. |

The boundary is broadly correct: host/channel packages depend on harness/composition/runtime packages, while lower layers do not depend back on terminal/daemon/channel/cmd.

## Current state assessment

### `harness`

`harness` is now the main live-session aggregation point. Its fan-out is expected because it coordinates app definitions, agent instances, command trees, workflows, trigger metadata, thread persistence inspection, session events, and projections.

Import assessment:

- `harness -> app` is correct: sessions are opened from app definitions and registries.
- `harness -> agent` is currently necessary but remains the primary cleanup pressure. Harness still receives live `agent.Instance` values from app/agent construction rather than owning runtime/session construction directly.
- `harness -> agentdir` / `resource` is acceptable for `LoadSession`, but this is a host-loading convenience. Long term, keep generic `harness.Service` independent from filesystem discovery policy.
- `harness -> thread/jsonlstore` is a cleanup candidate. Since session/thread lifecycle is moving toward harness, this may become correct if centralized there; avoid duplicate store selection in both harness and agent.
- `harness -> command` / `workflow` / `trigger` is correct for session-bound execution and metadata publication.

Watch point: `harness` should continue to absorb generic lifecycle from terminal and agent only when doing so deletes duplication. It should not absorb product CLI policy.

### `daemon`

`daemon` imports only `harness` and `trigger`, which matches the intended design. It is a slim process/service wrapper, not a second runtime.

This boundary is healthy. Future daemon growth should stay in these categories:

- process lifecycle;
- service status/health;
- storage defaults;
- trigger/job process ownership;
- graceful shutdown.

Do not add daemon-owned app/plugin/agent construction beyond delegating to existing load paths.

### `channel/httpapi`

`channel/httpapi` imports `harness`, `command`, and `action`.

Assessment:

- `channel/httpapi -> harness` is correct: HTTP/SSE is a channel over harness/session.
- `channel/httpapi -> command` is correct for structured command execution and result envelopes.
- `channel/httpapi -> action` is acceptable for action-result/status translation where HTTP payloads expose workflow/action outcomes.

This boundary is healthy as long as AG-UI/A2UI compatibility remains inside channel packages and does not leak into `harness` or core command/workflow types.

### `terminal/cli`

`terminal/cli` is still a high fan-out host package. This is acceptable for now because it is the terminal product/channel boundary, but it should not own reusable lifecycle semantics.

Current acceptable responsibilities:

- resolving resource path and manifest sources;
- applying local CLI fallback plugin policy;
- handling `--plugin` and `--no-default-plugins`;
- model/source API flags;
- one-shot vs REPL selection;
- terminal output/warning streams.

Cleanup candidates:

- Reduce direct `terminal/cli -> agent` over time. Model/options may remain terminal policy, but live session/runtime details should go through harness/session.
- Reduce direct `terminal/cli -> runner` and `tool` if those are only for rendering or event plumbing that can move to structured session/channel events.
- Keep slash parsing terminal-local. Do not make slash strings the canonical API for HTTP/SSE or future channels.

### `terminal/repl`

`terminal/repl` imports `command`, `terminal/ui`, and `usage`. This is reasonable for an interactive terminal loop.

Watch point: if usage rendering becomes structured channel events, `terminal/repl` should consume channel/session event payloads rather than reaching into usage-specific helpers directly.

### `terminal/ui`

`terminal/ui` imports `agent`, `runner`, and `usage`. This is a watch item.

Rendering runner events is expected, but depending on `agent` is a sign that presentation still reaches into the façade for metadata or event types. As structured output/session events mature, terminal UI should depend less on `agent.Instance` and more on neutral event/result payloads.

### `cmd/agentsdk`

`cmd/agentsdk` is a product entrypoint and naturally aggregates many packages. Current imports are acceptable because `cmd` sits at the top.

Watch points:

- `cmd/agentsdk -> runtime` should remain limited to model inspection/routing commands that are product-level CLI surfaces.
- `cmd/agentsdk -> agent` should shrink if command code can pass policy/config into terminal/daemon loaders instead of constructing agent-facing options directly.
- `cmd/agentsdk` should not become a second implementation of terminal/harness behavior.

## Boundary findings

### OK / intended

- Lower layers do not import `terminal`, `daemon`, `channel`, or `cmd`.
- `daemon` is correctly thin over `harness` and `trigger`.
- `channel/httpapi` is correctly a wrapper over harness/session APIs.
- `terminal/cli` owns host policy and presentation decisions, not app/plugin definitions.
- HTTP/SSE uses structured command paths rather than terminal slash strings.

### Watch

- `harness` imports `agent`, `agentdir`, `resource`, and `thread/jsonlstore`; these are acceptable today, but distinguish generic service/session APIs from filesystem loading convenience.
- `terminal/cli` still has broad imports and should shed generic lifecycle as harness/session APIs mature.
- `terminal/ui` imports `agent`, which should shrink when display/event payloads are more neutral.
- `cmd/agentsdk` imports both product surfaces and lower-level runtime/agent packages; keep it orchestration-only.

### Cleanup candidates

1. **Harness owns more session/thread lifecycle.** Move store selection/open/resume out of `agent.Instance` once a single harness-owned path can serve terminal, daemon, and HTTP.
2. **Split `harness.LoadSession` convenience from core `harness.Service`.** Keep filesystem/resource/plugin loading available, but do not let core service APIs depend on terminal-like policy.
3. **Reduce terminal direct `agent` dependency.** Terminal should configure model/policy and render events; live runtime/session state should be harness-owned.
4. **Move terminal UI off `agent` where possible.** Prefer `runner.Event`, `command.Result`, `harness.SessionEvent`, and future displayable payloads.
5. **Keep daemon and HTTP adapters thin.** If they need behavior, add it to harness/session only when it is channel-neutral.

## Decision

The harness/session/channel boundary is acceptable for the current dogfood checkpoint. No blocking import violations were found. The next code cleanup pressure remains aligned with previous reviews: move live session/thread/runtime ownership out of `agent.Instance` and reduce direct terminal/app reliance on live agent instances, without adding compatibility shims or parallel runtimes.
