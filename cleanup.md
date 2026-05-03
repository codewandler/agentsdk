# Cleanup / Restructuring Plan

- **Goal**
  - Stop feature work while cleanup is active.
  - Remove ownership drift instead of labeling it transitional.
  - Reduce boilerplate and hidden defaults instead of adding more seams.
  - Keep code aligned with:
    - `toolactivation.Manager` = mutable tool registry / activation state
    - `agent.Instance` = lifecycle façade, not a growing god object
    - `harness.Session` = session/channel boundary
    - `command.Result` = structured result, rendered at boundaries
    - future user-visible output should be structured displayable events/publications rendered by channels; current `io.Writer` plumbing is a transitional compatibility detail and not an immediate cleanup target

- **Phase 1 — Fix tool ownership drift** ✅
  - Completed in:
    - `a56d9ca Move agent tool registry out of standard toolset`
    - `1e54f70 Rename tool activation package`
    - `30f0ac9 Make agent standard tools explicit`
  - Current state:
    - `agent.Instance` owns `toolActivation *toolactivation.Manager`.
    - `agent.WithTools(...)` initializes `toolactivation.New(tools...)` directly.
    - `tools/standard` has since been deleted; named plugins own composition.
    - `agent.New` no longer imports or silently installs generic tool bundles.
    - Hosts pass tools explicitly.

- **Phase 2 — Re-evaluate late-registration APIs after ownership fix** ✅
  - Completed in:
    - `2e66493 Narrow agent projection registration seams`
  - Current state:
    - `agent.Instance.RegisterTools(...)` remains, backed by `toolactivation.Manager`.
    - `agent.Instance.RegisterContextProviders(...)` remains for session projection attachment.
    - `runtime.Engine.RegisterTools(...)` was removed.
    - `runtime.Engine.RegisterContextProviders(...)` remains because runtime owns the active context manager for future turns.
    - Projection attachment has no generic standard-bundle knowledge.

- **Phase 3 — Reduce command/rendering boilerplate** ✅
  - Completed in:
    - `de839ec Add structured command notice payloads`
  - Current state:
    - repeated workflow command notices use structured `command.Notice`, `command.NotFound`, and `command.Unavailable` payloads.
    - Rendering remains centralized through `command.Render(...)` / payload display behavior.

- **Phase 4 — Fix one-shot terminal result discard** ✅
  - Completed in:
    - `a625066 Render one-shot harness command results`
  - Current state:
    - `terminal/cli/run.go` renders returned `command.Result` values for one-shot slash commands.
    - Normal streamed agent-turn behavior remains unchanged.

- **Phase 5 — Decide auto-attachment policy for command projection** ✅
  - Completed in:
    - `01d8409 Attach command projection to harness sessions`
  - Current state:
    - default harness sessions attach the session command projection.
    - attachment remains explicit/idempotent at the session seam.
    - agent-callable command policy still filters unsafe commands.

- **Phase 6 — Clean docs to match actual architecture** ✅
  - Completed in:
    - `7f2425f Document cleaned ownership boundaries`
  - Current state:
    - docs capture ownership guardrails for `toolactivation`, named plugin composition, command rendering, projections, and plugin/contribution invariants.

- **Optional deeper cleanup completed so far**
  - Removed concrete `tools/skills` and `tools/toolmgmt` imports from `runtime`.
  - Renamed generic `activation` package to explicit `toolactivation`.
  - Moved terminal event rendering out of `agent` and into `terminal/ui`.
  - Removed hidden standard-tool defaults from `app.New` and `agent.New`.
  - Deleted generic `tools/standard` and `plugins/standard`; local terminal composition now lives in `plugins/localcli`.
  - Added context-aware `app.PluginFactory` so hosts can resolve named plugin refs from `context.Context` plus config without introducing a separate profile system.
  - Moved reusable terminal session loading into harness:
    - `harness.LoadSession` owns app/default-agent/service/session construction and applies grouped app/agent/session load settings, including source API, model policy, resume-session paths, and loaded plugin application.
    - `harness.ResolveAgentLoadConfig` owns manifest/CLI model-policy overlay and decoded source API application.
    - `harness.ResolvePlugins` owns generic plugin-ref mechanics: ordered defaults/manifest/explicit refs, trimming, dedupe, config forwarding, and factory invocation.
    - `harness.EnsureFallbackAgent` owns fallback-agent injection mechanics while the local CLI plugin still owns the fallback spec.
    - `harness.PrepareResolvedAgent` owns generic default-agent selection plus agent-spec overrides.
    - `terminal/cli.Load` remains the compatibility/channel wrapper for CLI-specific policy: CLI flag/config decoding, local CLI default plugin policy, terminal UI adapters, debug-message output, risk-log presentation, and fallback spec selection.
  - Moved workflow thread recording out of `app.App` and into `harness.Session.ExecuteWorkflow`; app workflow helpers now stay registry/executor-focused while session live-thread persistence is owned by harness.
  - Removed legacy app-level workflow command shims (`RegisterWorkflowCommand` / `WorkflowCommand`); workflow slash commands now live on the harness command tree.

- **Remaining cleanup candidates**
  - Revisit `agent.Instance` responsibilities and move outward only where the slice deletes or simplifies more than it adds:
    - session lifecycle
    - context provider lifecycle
    - capability registry/session ownership
  - Revisit `terminal/cli.Load` only when a slice deletes or clearly collapses remaining setup paths:
    - most generic app/agent/session/plugin load mechanics now live in harness
    - keep terminal as the channel boundary for CLI flags, terminal fallback policy, terminal UI adapters, debug output, and experiments
  - Revisit payload display / output publication design later, as a designed channel/displayable model rather than opportunistic cleanup:
    - model user-visible output as structured displayable events/publications
    - let channels/frontends render displayables differently for terminal, TUI, HTTP/SSE, JSON, and LLM-facing modes
    - consider renderer registry only if it reduces code
    - do not add a registry if payload `Display(...)` is currently simpler
    - leave risk logging alone for now; it is experimental and needs separate design before migration
  - Revisit remaining `app.App` helper seams:
    - keep app as registry/composition host
    - collapse any transitional helper paths once newer harness/session wiring makes them redundant

- **Guardrails for any next slice**
  - No new harness plugin system beside `app.Plugin`.
  - No new command switch namespaces; use declarative `command.Tree`.
  - No generic `tools/standard` or `plugins/standard` default composition packages.
  - No separate profile system for plugin composition; named composition is still `app.Plugin` plus `app.PluginFactory`.
  - No hidden default tool bundles in `app.New` or `agent.New`.
  - No command output discarded at terminal/channel boundaries.
  - Do not expand writer-shaped output in harness/runtime; long-term output should become structured displayable events/publications, but do not churn existing writer seams without a designed replacement.
  - New seams should delete or collapse an old path.
  - Commit only after focused and full verification pass.
