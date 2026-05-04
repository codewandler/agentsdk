# Package boundaries

## Current package boundary rules

This document states the current intended package layers and dependency rules. Detailed review findings and improvement backlog live in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md).

## Dependency layers

From lower-level to higher-level:

1. **Foundation:** `internal/*`, `markdown`, `websearch`, small data packages.
2. **Policy and observability:** `safety`, `usage`.
3. **Persistence:** `conversation`, `thread`, `thread/jsonlstore`.
4. **State and context:** `agentcontext`, `capability`, `capabilities/*`, `skill`.
5. **Execution primitives and projections:** `action`, `actionmw`, `workflow`, `command`, `tool`, `toolactivation`, `toolmw`, `tools/*`.
6. **Turn runtime:** `runtime`, `runner`.
7. **Agent config:** `agentconfig` (pure spec/config types, no runtime dependencies).
8. **Agent façade:** `agent` (re-exports `agentconfig`, owns runtime/session).
9. **Composition and resource loading:** `app`, `resource`, `agentdir`, `plugins/*`.
10. **Session/daemon orchestration:** `harness`, `trigger`, `daemon`.
11. **Channels/products/examples:** `terminal/*`, `channel/*`, `cmd/agentsdk`, `apps/*`, `examples/*`.

The graph is not perfectly layered yet. That is acceptable pre-1.0 as long as ownership moves toward these layers and old paths are deleted instead of preserved as compatibility shims.

## Hard rules

- Low-level packages must not import terminal, daemon, channel, harness, app, or cmd packages.
- `resource` and `agentdir` describe/load metadata; they must not own runtime or sessions.
- `app` composes reusable definitions and registries; it must not own channels, daemon lifecycle, or terminal rendering.
- `harness` owns live session execution and session-bound command/workflow dispatch.
- `daemon` is only a service/process wrapper over harness plus trigger ownership.
- `channel/*` packages adapt harness/session APIs to protocols. Protocol compatibility must not leak into harness core.
- `terminal/*` owns terminal UX and slash parsing. Slash strings are not the canonical API for other channels.
- `runner` and `runtime` stay below `agent`, `app`, `harness`, terminal, daemon, and channels.
- `workflow` remains host-independent action orchestration.
- Safety policy lives in `safety`/`actionmw`; `toolmw` is only a bridge.

## Known current pressure points

- `agent.Instance` is at 32 fields after successive extractions. The remaining fields are genuinely runtime-coupled; further field extraction has diminishing returns. A narrower harness→agent interface would reduce coupling surface.
- ~~`app.App` still caches live `agent.Instance` values.~~ Done: cache removed.
- ~~`terminal/ui` depends directly on `agent`.~~ Done: uses `runner.EventHandlerContext`. `terminal/cli` uses `agentconfig` for config types; only `agent.Option` requires the `agent` import.
- `command -> tool` remains as an older projection path; prefer harness `session_command` for agent-facing command execution.

See [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) for the consolidated review findings and cleanup backlog.
