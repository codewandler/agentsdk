# Review and improvements

This file is the single place for architecture review notes, package-boundary findings, and improvement backlog items. The rest of `docs/architecture/` should describe the current and desired infrastructure directly: owners, rules, APIs, and package responsibilities. Do not add more review-log files; aggregate review output here.

## How to continue improvements

1. Read [`../README.md`](../README.md), [`README.md`](README.md), [`overview.md`](overview.md), and [`package-boundaries.md`](package-boundaries.md).
2. Use this file as the backlog for review findings and cleanup candidates.
3. Pick one small cleanup that deletes coupling or removes an obsolete path. Do not add compatibility shims or parallel runtimes.
4. **No legacy fallbacks or deprecated shims.** We are the only users of this SDK. When an API is replaced, delete the old one immediately. Do not retain no-op stubs, `Deprecated:` wrappers, or backward-compatibility adapters.
5. Keep production docs focused on current infrastructure and rules. If new review findings appear, update this file instead of adding another review document.
6. Keep temporary task tracking in [`.agents/TASKLIST.md`](../../.agents/TASKLIST.md), not under `docs/`.
7. Run focused tests for touched code and `./scripts/ci-check.sh` before reporting or committing.
8. Leave datasource runtime expansion deferred until at least one agent/session ownership cleanup slice has been dogfooded.

## Current package boundary summary

The current package graph does not show blocking low-level-to-host dependency violations. The important lower layers do not import terminal, daemon, channel, harness, app, or cmd packages.

Current high-level dependency direction:

1. **Foundation:** `internal/*`, `markdown`, `websearch`, small data packages.
2. **Policy/observability:** `safety`, `usage`.
3. **Persistence:** `conversation`, `thread`, `thread/jsonlstore`.
4. **State/context:** `agentcontext`, `capability`, `capabilities/*`, `skill`.
5. **Tooling projection:** `tool`, `toolactivation`, `toolmw`, `tools/*`.
6. **Execution/orchestration primitives:** `action`, `actionmw`, `command`, `workflow`.
7. **Turn runtime:** `runtime`, `runner`.
8. **Agent façade:** `agent`.
9. **Composition:** `app`, `resource`, `agentdir`, `plugins/*`.
10. **Harness/session:** `harness`, `trigger`.
11. **Hosts/products/channels:** `cmd/agentsdk`, `terminal/*`, `daemon`, `channel/*`, `apps/*`, `examples/*`.

This is not yet a strict acyclic architecture. Some edges are accepted while the SDK is pre-1.0 and while `action`, `workflow`, command resources, capabilities, and agent/session ownership are still settling. The cleanup rule is: do not add compatibility shims or parallel runtimes; move ownership only when doing so deletes coupling. We are the only consumers — when an API is replaced, delete the old one in the same change.

## Boundary findings

### App, resource, agentdir, plugins

Current status:

- `resource` is the declarative metadata model loaded from files/manifests/plugin roots.
- `agentdir` is a filesystem/resource loader that emits `resource.ContributionBundle`.
- `app.App` is the Go composition root for reusable definitions and registries.
- `plugins/*` are named app-level contribution bundles.
- `plugins/localcli` is intentionally high fan-out because it is the named local terminal environment bundle.

Rules:

- `resource` must stay metadata-only. It must not import `app`, `harness`, `runtime`, `terminal`, `daemon`, `tool`, or concrete plugins.
- `agentdir` must not instantiate apps, sessions, or runtimes.
- `app` must not import `harness`, terminal, daemon, channel, or cmd packages.
- First-party plugins remain named contribution bundles. Do not recreate hidden `standard` bundles.
- Resource command contributions are metadata for harness/session/channel binding, not executable app commands.

Improvement backlog:

- ~~`app.App` currently caches live `*agent.Instance` values.~~ Done: `agents` map, `DefaultAgent()`, and `agent()` removed. `InstantiateAgent` returns the instance without caching. All session creation goes through `harness.OpenSession`.
- `resource` and `agentdir` currently depend on `agent` for declarative agent config. If `agent` splits spec/config from live runtime ownership, keep loaders on the spec/config side only.
- Keep trigger execution host-owned: declarative triggers can live in resources/app metadata, but scheduler/process behavior remains daemon/harness-owned.
- Datasource runtime expansion remains deferred until agent/session ownership is cleaner.

### Harness, daemon, channels, terminal, cmd

Current status:

- `harness` is the live session execution boundary.
- `daemon` is a thin service/process wrapper over `harness.Service` plus trigger ownership.
- `channel/httpapi` is a native HTTP/SSE adapter over harness/session APIs.
- `terminal/cli` owns terminal host policy, local CLI fallback policy, flags, and one-shot/REPL selection.
- `terminal/repl` owns the interactive terminal loop.
- `terminal/ui` owns terminal rendering.
- `cmd/agentsdk` is the product executable entrypoint and top-level command tree.

Rules:

- Lower layers must not import terminal, daemon, channel, or cmd packages.
- HTTP/SSE must use structured command paths and session/workflow APIs, not terminal slash strings.
- AG-UI/A2UI compatibility belongs at the channel boundary, not in `harness` core.
- `daemon` must not become a second app/runtime/plugin/session system.
- Terminal slash parsing remains terminal-local.

Improvement backlog:

- ~~Centralize session/thread store selection and open/resume behavior in harness/session.~~ Done: `LoadSession` routes through `OpenSession`; agent no longer self-opens stores.
- Split `harness.LoadSession` filesystem convenience from core `harness.Service` where useful.
- ~~Reduce `terminal/ui -> agent`.~~ Done: `terminal/ui` no longer imports `agent`; event handler factory uses `runner.EventHandlerContext`.
- ~~Reduce `terminal/cli -> agent` execution coupling.~~ Done: `terminal/cli/run.go` no longer imports `agent`; uses `runner.ErrMaxStepsReached`. Remaining `terminal/cli` → `agent` imports are spec/config types (`InferenceOptions`, `Option`, `ModelPolicy`, etc.) which are acceptable while `agent` owns those types.
- Keep `cmd/agentsdk` orchestration-only; do not duplicate terminal or harness behavior there.

### Agent, runtime, runner

Current status:

- `runner` is the low-level model/tool turn loop.
- `runtime` is reusable execution infrastructure around runner, context, capabilities, skills, thread, tools, and actions.
- `agent` is currently both a configured agent façade and a live runtime/session holder.
- `runnertest` stays isolated provider/client test support.

Rules:

- `runner` must not own app composition, harness sessions, persistence store selection, terminal rendering, or channel behavior.
- `runtime` must not import `agent`, `app`, `harness`, terminal, daemon, channel, or cmd packages.
- `agent` may own `agent.Spec`, model/source API policy, inference defaults, and construction options.
- Long-term live session/thread ownership should not remain concentrated in `agent.Instance`.

The main architecture problem remains `agent.Instance` breadth. It currently combines spec/options normalization, model policy, runtime construction, session IDs, skill repository/state, context providers, capability setup, usage, compaction, output hooks, and turn façade APIs. JSONL store ownership and live instance caching have been moved out.

Improvement backlog:

- Split spec/config from live instance behavior.
- ~~Move JSONL store selection and open/resume lifecycle to harness/session.~~ Done.
- ~~Move live session registry/cache ownership out of `app.App` and `agent.Instance`.~~ Done.
- ~~Route diagnostics, usage, compaction, and notices through structured session/channel events.~~ Done: `agent.WithOutput` deprecated, `compact_render.go` deleted, usage persistence errors routed through `DiagnosticHandler` → `SessionEventDiagnostic`, session owns terminal writer.
- Make skill/capability/context activation and projection state session-aware where possible.

### Execution primitives

Current status:

- `action` is the surface-neutral Go execution primitive.
- `actionmw` is surface-neutral action middleware.
- `tool` is the LLM-facing projection layer.
- `toolactivation` owns mutable LLM tool visibility state.
- `toolmw` is LLM-facing tool middleware plus compatibility bridge to action/safety middleware.
- `tools/*` are first-party concrete tool implementations.
- `command` is the user/channel command descriptor/result/policy layer.
- `command/markdown` is an authoring adapter.
- `workflow` is host-independent action orchestration.

Rules:

- Actions execute.
- Tools project actions or tool-native behavior to LLMs.
- Commands project session/channel operations to users, APIs, and agents through catalog metadata.
- Workflows orchestrate actions and must not own host/session/channel behavior.
- Safety ownership belongs in `safety`/`actionmw` as the primitive policy seam; `toolmw` remains a bridge.
- New agent-facing command execution should use harness `session_command` and command catalog context, not slash-string tool bridges.

Improvement backlog:

- Keep `command -> tool` contained. It exists for legacy/projection support and should shrink once `session_command` covers dogfood use.
- Do not expand `toolmw` into core safety ownership.
- Keep action-backed tool migration incremental and use it only where workflows/commands need reusable execution.
- Keep workflow command/API/CLI behavior in harness/channel layers, not in `workflow`.

## Package graph watch list

High fan-out packages are acceptable when they are top-level hosts or explicit aggregators, but they should not leak ownership downward:

- `plugins/localcli`: expected named terminal-local aggregation; must not become hidden standard composition.
- `harness`: expected live-session coordinator; should own session lifecycle, not product policy.
- `agent`: main architecture smell; still owns too many runtime/session/state concerns.
- `cmd/agentsdk` and `terminal/cli`: expected host/product aggregation; should stay orchestration/policy/presentation only.
- `app`: expected composition hub; should stop caching live instances when harness/session construction can replace it.

High fan-in packages are expected foundations or projections:

- `tool`: broad LLM-facing projection API.
- `action`: central execution primitive.
- `agentcontext`, `skill`, `capability`: shared state/projection systems.
- `thread`: durable persistence foundation.
- `command`: shared command descriptors/results.

## Preferred cleanup sequence

1. ~~Centralize harness/session thread opening and resume behavior.~~ Done.
2. ~~Move one concrete `agent.Instance` session/thread responsibility to that harness-owned path.~~ Done.
3. ~~Remove the old path instead of keeping compatibility fallback.~~ Done: agent no longer imports `thread/jsonlstore`.
4. ~~Reduce terminal direct `agent` dependencies.~~ Done for `terminal/ui`; `terminal/cli` still imports `agent` for options/spec types.
5. ~~Move output, usage, compaction, and diagnostics toward structured session/channel events.~~ Done.
6. ~~Re-check app live instance caching after harness owns enough runtime construction.~~ Done: cache removed.
7. Revisit datasource runtime expansion only after at least one agent/session ownership cleanup slice has been dogfooded.

## Historical review inputs

The findings above consolidate the previous package-boundary and focused boundary reviews for:

- package import boundaries;
- app/resource/plugin composition;
- harness/session/channel hosts;
- agent/runtime/runner;
- action/tool/command/workflow execution primitives.

Do not add new per-area review files. Update this file when review findings or improvement priorities change.
