# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Versions below are backfilled from the repository's implementation milestones. Tags
match these entries as the project starts publishing releases.

## [Unreleased]

### Added

- **`app.Spec`** — new type that declares a first-party application's identity
  (name, description), pre-construction settings (embedded resources, plugin
  defaults), and a deferred `Options` factory that returns `[]app.Option`.
- **`cli.Mount`** — registers one or more `app.Spec` values as cobra
  subcommands with the full standard flag surface. Prompt is derived
  automatically as `$name> `.
- **`apps/runapp`** — new package exporting `Spec()` for the default `run`
  application, completing the three-app model (`run`, `dev`, `build`).
- `apps/engineer.Spec()` and `apps/builder.Spec()` — each first-party app now
  owns its own `app.Spec`, including embedded resources, default agent, plugin
  wiring, and CLI metadata.

### Changed

- **`agentsdk build` prompt** changed from `builder> ` to `build> ` to match
  the command name consistently.
- First-party app wiring (`devCmd`, `buildCmd`, `devAppOptions`,
  `builderAppOptions`) moved out of `cmd/agentsdk/main.go` into the respective
  app packages. `main.go` now uses `cli.Mount` with three `app.Spec` values.

- **`plugins/browserplugin`** — Browser automation via Chrome DevTools Protocol.
  Single `browser` tool with batched operations (oneOf discriminated union):
  open, navigate, click, type, select, read, screenshot, evaluate, wait, scroll,
  hover, pdf, back, forward, close. Includes context provider that injects
  interactable page elements into agent context. Core operations are registered
  as `action.Action` for reuse in YAML workflows.
- **`agentsdk dev` auto-loads browser plugin** — no `--plugin browser` flag
  needed. The browser tool is always available to the engineer agent.
- **`agentsdk dev` subcommand** — runs the embedded engineer agent with the
  current working directory as a discovery root. Supports all standard flags
  (model, session, runtime, inference, etc.).
- **`-d/--discover` flag** on `run`, `dev`, and `build` — repeatable flag to
  specify multiple discovery root directories. Defaults to CWD when omitted.
- `MultiDirResources` and `EmbeddedWithDirResources` in `terminal/cli` for
  resolving multiple discovery roots and merging embedded + directory sources.
- `apps/engineer` is now an embedded Go package with `resources.go` and
  `//go:embed` directive, mirroring the `apps/builder` pattern.
- Added `agentsdk models --thinking` to filter compatibility-backed model
  listings to rows with live reasoning evidence.
- Added release-readiness checkpoint documentation for the pre-1.0 internal
  dogfood boundary, intentional breakage notes, CI guards, and deferred external
  release cadence.

### Fixed

- **`agentsdk build` no longer loads the target project's `.agents` directory**
  as its own resources. The builder now uses only its embedded resources,
  preventing the target's agents/skills/workflows from polluting the builder's
  runtime. Added `EmbeddedOnly` option to `cli.CommandConfig` to support this
  isolation.

### Changed

- **Breaking:** `agentsdk run` no longer takes a positional directory argument.
  Use `-d <path>` instead. All positional arguments are now treated as the task.
  Old: `agentsdk run ./myapp "do something"`. New: `agentsdk run -d ./myapp do something`.
- **`agentsdk build`** now uses the standard `cli.NewCommand` pattern and
  supports all flags available to `run` and `dev` (model, session, runtime,
  debug, etc.).
- Engineer agent (`apps/engineer`) now has an explicit persona (direct, concise,
  no sycophancy, ownership mentality) and strict git/commit rules (conventional
  commits, atomic commits, mandatory CHANGELOG updates).
- Updated llmadapter to `v1.0.0-rc.32` and modeldb to `v0.15.0`, refreshing
  embedded agentic-coding evidence with Qwen, expanded OpenRouter, and Bedrock
  Converse rows.
- Added local modeldb aliases for OpenRouter `qwen3-coder` and
  `qwen3-coder-next` so the refreshed compatibility evidence can resolve them
  in auto model listings.

### Removed

- Deleted stale standard-bundle assumptions from current docs/checkpoint guidance.
  `tools/standard` and `plugins/standard` remain intentionally removed; use
  named plugins and resource-selected tools instead.

## [0.33.0] - 2026-05-03

### Added

- Added declarative datasource and workflow resource discovery for
  `.agents/datasources/*.yaml` and `.agents/workflows/*.yaml`, including
  `agentsdk discover` output.
- Promoted the engineer dogfood agent to `apps/engineer` and reserved
  `apps/builder` for the planned `agentsdk build` app.

### Changed

- Updated app/example documentation to distinguish first-party dogfood apps from
  small instructional examples.
- Updated llmadapter to `v1.0.0-rc.29`.

### Fixed

- Preserved assistant message phase metadata and kept `commentary` responses in
  the active runner transcript instead of treating them as final no-tool turns.

## [0.32.0] - 2026-04-30

### Added

- Added a project inventory context provider that summarizes repository root,
  language counts, package-manager files, key directories, test patterns, and
  entrypoints.
- Added project inventory support for the then-current default plugin set. This historical default plugin set has since been deleted in favor of named plugins.

## [0.31.0] - 2026-04-30

### Added

- Added `json_query` for querying JSON files with jq-like field, index, and
  wildcard selectors without shelling out to Python or `jq`.

## [0.30.0] - 2026-04-30

### Added

- Added `dir_create`, `file_copy`, and `file_move` filesystem tools for native
  directory creation plus conservative copy/move operations without shelling out.

## [0.29.0] - 2026-04-30

### Added

- Added `git_add` and `git_commit` tools to the git tool bundle for explicit staging and commit creation without shelling out to `bash`.
- Added `summary` mode to the git context provider with changed-file, staged,
  unstaged, and untracked counts.
- Added `show_lines` support to `dir_tree` so directory exploration can include
  compact per-file line counts in tree and flat modes.
- Added a consolidated implementation plan for context-provider and tooling
  improvements derived from `.agents/reviews/`.

### Changed

- Enabled git tools in the then-current default toolset and engineer resource bundle. The current blessed dogfood app is `apps/engineer`, and generic standard bundles have been deleted.
- Improved planner single-plan lifecycle errors and guidance when `create_plan`
  is called after a plan already exists.
- Improved `bash` multi-command output with a compact success/failure summary.
- Improved `file_edit` guidance and `old_string not found` diagnostics.

## [0.28.0] - 2026-04-30

### Added

- Added `phone` tool (`tools/phone`) for SIP call origination via
  diago/sipgo. Supports `dial`, `hangup`, and `status` operations using
  the `oneOf` discriminated-union parameter pattern. Each dial creates an
  independent SIP user agent so multiple concurrent calls are fully
  isolated. Includes a `Dialer` interface for testability.
- Expanded phone dialing with per-call `sip_endpoint`, `protocol`,
  `caller_id`, custom SIP headers, `username:password` credentials, audio
  modes (`none`, `echo`, `device` with app-provided `AudioDevice`), and
  `debug` SIP response diagnostics.
- Added phone tool configuration for the then-current standard bundle model. The current path is explicit named plugin/tool composition; generic standard bundles have been deleted.
- Added live terminal Markdown rendering using
  `github.com/codewandler/markdown` v0.41.0 with GFM autolinks, OSC8
  clickable URLs, live table redraws, and Markdown rendering for reasoning
  output.
- Added `Behaviors` field to `tool.Intent` for high-level semantic labels
  (e.g. `filesystem_read`, `network_fetch`) shared with cmdrisk vocabulary.
- Added `CmdRiskAssessor` in `toolmw` — unified risk assessor that bridges
  cmdrisk scoring, policy, and allowances into the agentsdk middleware layer.
  Supports four assessment strategies: pre-computed assessment reuse,
  structured intent operations, command string parsing, and opaque fallback.
- Added `buildCmdRiskContext` in both `toolmw` and `tools/shell` to construct
  `cmdrisk.Context` from `tool.Ctx.Extra()` metadata (environment, trust,
  allowances, path prefixes) with safe defaults.
- Added `bashIntent` in `tools/shell` — typed `DeclareIntent` for the bash
  tool with per-command cmdrisk analysis, worst-case multi-command merging,
  and pre-computed assessment passthrough via `Intent.Extra`.
- Added cmdrisk analyzer wiring for the then-current standard bundle model. Current risk/safety wiring is explicit through named plugins, tool middleware, and safety primitives.
- Added comprehensive tests for shell intent declaration, risk context
  threading, multi-command assessment merging, and standard toolset wiring
  (`tools/shell/intent_test.go`, former standard-bundle tests,
  `toolmw/cmdrisk_test.go`, `toolmw/riskgate_test.go`).

### Changed

- Reworked `toolmw/cmdrisk.go` from a simple command-string assessor to a
  multi-strategy intent assessor that handles structured operations, command
  strings, and pre-computed assessments.
- Reworked `tools/shell/intent.go` from a minimal stub to a full
  cmdrisk-integrated intent provider with per-command analysis, assessment
  merging, and context-aware risk context construction.
- Updated `tools/vision/vision.go` default model to `anthropic/claude-sonnet-4`.
- Updated `github.com/codewandler/markdown` to v0.41.0.

## [0.27.1] - 2026-04-28

### Fixed

- Fixed `file_edit` tool JSON schema: all `oneOf` operation variants now carry
  `type: object`, preventing LLM dispatch ambiguity.
- Added field-level descriptions to `FileEditParams` (`path`, `dry_run`,
  `allow_partial`, `operations`) so the schema is self-documenting without
  relying on external documentation.
- Stopped adding `examples` inside `oneOf`/`anyOf`/`allOf` branches.
  Examples on discriminated-union variants are noise that some LLMs
  misinterpret as valid values; they now only appear on scalar parameters.
- Fixed `injectRequiredFromTags` to parse jsonschema tag values that contain
  escaped commas (`\,`) correctly, so descriptions with commas in struct tags
  no longer break required-field detection.

## [0.27.0] - 2026-04-28

### Added

- Added conversation compaction with LLM-generated summaries. New
  `agent.Instance.Compact` and `CompactWithOptions` methods replace older
  messages with a summary node, reducing context window pressure.
- Added `/compact` builtin slash command for user-triggered compaction.
- Added auto-compaction between turns via `WithAutoCompaction` option.
  When enabled, the agent checks projected token count after each turn and
  compacts automatically when it exceeds a configurable threshold (default:
  80% of the model's context window).
- Added compaction floor on `conversation.Tree` — `SetFloor`/`Floor` methods
  and `Path()` stop-at-floor behavior. After compaction, the floor bounds
  path traversal to only the active portion of the conversation.
- Added `conversation.ProjectItems` floor-aware placement: when replaced nodes
  are excluded by a floor, the compaction summary is placed at the beginning
  of the projected items.
- Added usage event persistence to the thread event log
  (`harness.usage_recorded`). Usage records now survive session resume.
  Replay deduplicates by request ID.
- Added `contextWindow` field to `agent.Instance`, populated from
  `AutoRouteSummary.ContextWindow` via the llmadapter model resolution
  pipeline. Also populates `contextproviders.ModelInfo.ContextWindow`.

### Changed

- Updated `github.com/codewandler/llmadapter` to `v1.0.0-rc.14`.
  New version carries `Limits` (including `ContextWindow`) through
  `ModelResolutionCandidate` and `AutoRouteSummary`.

## [0.26.0] - 2026-04-28

### Added

- Added `tools/vision` package — image understanding tool that delegates to a
  vision-capable LLM (google/gemini-2.5-flash via OpenRouter). Supports URLs,
  local file paths, and data URIs. Multiple images per action for comparison.
- Added `plugins/visionplugin` — `app.Plugin` wrapper for the vision tool with
  `WithClient` and `WithAPIKey` options and env-based auto-detection
  (`OPENROUTER_API_KEY` / `VISION_OPENROUTER_API_KEY`).
- Added vision tool catalog support for resource specs. Current apps select `vision` through explicit tool/plugin composition.
- Added `vision` tool to the engineer dogfood agent.
- Added `internal/htmlconvert` package to centralize HTML-to-Markdown conversion
  logic, providing a clean integration point for `tools/web` and future HTML
  processing needs.

### Fixed

- Tool executor now preserves partial output on timeout and cancellation.
  Previously, when a tool returned both a result and a context error, the
  result was discarded and replaced with `[Timed out]` or `[Canceled]`. Now
  the partial output is included before the label so the LLM sees what was
  produced before the interruption.
- Tool executor no longer discards valid results when the context expires
  after `Execute` returns. Tools that handle timeouts internally (like bash)
  now always have their results forwarded.

### Changed

- Defaulted request-level cache policy to `On` while removing the public
  cache-key and cache-TTL knobs from `agentsdk`; provider-specific cache
  placement is now handled internally.
- Changed context-provider rendering so the committed transcript carries the
  append-only `<system-context>` diff blocks that are sent to upstream on user
  turns and tool-result follow-ups.
- Updated `github.com/codewandler/llmadapter` to `v1.0.0-rc.12`.
- Refactored `tools/web` to use `internal/htmlconvert` for HTML-to-Markdown
  conversion, decoupling the web tool from direct dependency on external HTML
  conversion library.

## [0.25.0] - 2026-04-27

### Added

- Added `app.ContextProvidersPlugin` interface for plugins that contribute
  app-scoped, stateless context providers at registration time.
- Added `app.AgentContextPlugin` interface and `app.AgentContextInfo` for
  plugins that contribute context providers depending on per-agent runtime
  state (skill repository, activation state, active tools).
- Added `agent.WithContextProviders` option for injecting extra context
  providers into an agent instance with key-set dedup against built-in
  providers.
- Added `agent.WithContextProviderFactories` option and
  `agent.ContextProviderFactory` type for deferred context provider
  construction that runs after skill and tool initialization.
- Added `plugins/gitplugin` — bundles git tools (`git_status`, `git_diff`)
  and the git context provider behind `app.Plugin`.
- Added `plugins/skillplugin` — bundles the skill activation tool, skill
  source discovery, and the skill inventory context provider.
- Added `plugins/toolmgmtplugin` — bundles tool management tools
  (`tools_list`, `tools_activate`, `tools_deactivate`) and a lazy
  active-tools context provider that reflects runtime activation changes.
- Added `plugins/standard` with `Plugins(Options)` and `DefaultPlugins()`
  for pre-assembled plugin sets, as the plugin-oriented counterpart to
  `tools/standard.Tools()`.
- Added `app.App.ContextProviders()` accessor for collected plugin context
  providers.

### Changed

- `app.App.RegisterPlugin` now detects and wires `ContextProvidersPlugin`
  and `AgentContextPlugin` facets alongside the existing command, tool,
  agent spec, and skill source facets.
- `app.App.InstantiateAgent` forwards plugin context providers and
  agent-context factories to the agent via the new agent options.
- `agent.Instance.contextProviders()` skips built-in providers whose keys
  are already contributed by plugin providers (key-set dedup), allowing
  plugins to replace built-in context providers.

### Removed

- Removed the empty `plugin/` directory.

## [0.24.2] - 2026-04-27

### Changed

- Updated `github.com/codewandler/llmadapter` to `v1.0.0-rc.11`, including
  `modeldb v0.13.2` so the default `codex/gpt-5.5` route resolves through
  `codex_responses`.

## [0.24.1] - 2026-04-27

### Added

- Added `Taskfile.yaml` with an `install` task for installing the `agentsdk` binary.
- Added an app manifest to the engineer example so global user skill discovery participates by default.

### Changed

- Changed the engineer example so bundled skills are discoverable but not auto-activated by default.
- Explicitly enabled the `skill` tool in the engineer example.
- Included active tool `Guidance()` text in the prompt-visible tools context provider.
- Improved `skill` tool guidance for activating exact `references/` paths.

## [0.24.0] - 2026-04-27

### Added

- Added runtime skill discovery and activation state with session-persistent replay for skills and exact `references/` paths.
- Added `tools/skills` with batched `skill` actions for model-driven skill activation.
- Added `/skills` and `/skill <name>` builtins to the terminal app.
- Added reference discovery under `skills/<name>/references/*.md` and prompt/context rendering for activated references.
- Added `ROADMAP.md` with the next planned follow-up work for the skill system.

### Changed

- Expanded README and `docs/RESOURCES.md` to document runtime skill activation and skill references.

## [0.23.0] - 2026-04-27

### Added

- Added `agentcontext/contextproviders.Git` with configurable `off`, `minimal`, and `changed_files` modes plus file and byte caps.
- Added `agentcontext/contextproviders.CmdContext` for short command-backed context fragments.
- Added planner capability attachment to the default agent spec.

### Changed

- Removed raw `ProjectionInput.Messages` and `ProjectionInput.PendingMessages`; provider projection now takes normalized conversation items only.
- Expanded README and agent-facing project notes for the completed context/conversation architecture.

### Fixed

- Hardened tool-call normalization for repeated call IDs and documented recovery-only partial transcript commits.

## [0.22.0] - 2026-04-27

### Added

- Added the thread-backed runtime foundation with capability attachment/replay, context render events, and high-level create/open/resume engine helpers.
- Added manager-owned context providers, context fragment diffing, render record persistence, and runtime hooks for custom thread context providers.
- Added baseline environment and time context providers.
- Added conversation item normalization before provider projection, including support for contextual messages and item-level compaction.
- Added thread runtime compaction APIs and durable runtime documentation.
- Added model compatibility policy tooling for provider continuation behavior.

### Changed

- Updated `github.com/codewandler/llmadapter` to `v1.0.0-rc.8`.
- Projected provider requests from normalized conversation items instead of direct message history.
- Polished public runtime/context API documentation and thread-safety notes.
- Renamed remaining runtime-facing `flai` defaults and context keys to `agentsdk`.

### Fixed

- Fixed context render lifecycle handling and branch replay.
- Hardened `StringSliceParam` JSON null handling.

## [0.21.0] - 2026-04-26

### Added

- Added agent resource discovery architecture for agent directories, external resources, and resource resolution.
- Added `.agents` resource layout documentation for agent, command, plugin, and skill compatibility formats.
- Added terminal CLI resource-loading support and regression coverage.

### Changed

- Expanded README guidance for agent resource discovery and runtime wiring.

## [0.20.1] - 2026-04-26

### Added

- Added terminal UI regression tests covering streamed markdown buffering, usage formatting, trimming, and truncation behavior.

## [0.20.0] - 2026-04-26

### Added

- Added app, plugin bundle, command, skill, and agent directory abstractions for loading agents from resource directories.
- Added `agentsdk run` plus reusable `terminal/cli`, `terminal/repl`, and `terminal/ui` packages for slim agent CLIs.
- Added `runtime.Engine` as the execution engine naming boundary.
- Added workspace and home skill source discovery for `.agents/skills` and `.claude/skills`.

## [0.19.2] - 2026-04-25

### Changed

- Updated `github.com/codewandler/llmadapter` to `v0.48.22`.

## [0.19.1] - 2026-04-25

### Added

- Added runtime examples for the preferred SDK stack and durable session option derivation.

### Changed

- Updated README guidance for `runtime.AutoMuxClient`, `standard.Toolset`, durable sessions, cache defaults, and immutable provider history.
- Updated the migration plan to mark provider setup consolidation complete.

## [0.19.0] - 2026-04-25

### Added

- Added runtime helpers for llmadapter auto mux defaults and route summary to provider identity conversion.

## [0.18.0] - 2026-04-25

### Added

- Added `runtime.SessionOptions` to derive conversation session defaults from the same runtime options used to construct a runtime agent.

## [0.17.0] - 2026-04-25

### Added

- Added `usage.FromRunnerEvent`, `usage.RouteState`, and provider/model normalization helpers for runner usage accounting.
- Added `usage.Tracker.AggregateTurn` for turn-scoped usage summaries.

## [0.16.0] - 2026-04-25

### Added

- Added `tools/standard.Toolset` helpers that bundle standard tools with an activation manager.
- Added `runtime.WithToolActivation` for wiring tool-management state into runtime tool contexts without consumers importing `tools/toolmgmt`.

## [0.15.0] - 2026-04-25

### Removed

- Removed automatic message/token budget projection helpers because provider history must remain immutable for caching and continuation.
- Removed `conversation.WithMessageBudget`, `conversation.WithTokenBudget`, `runtime.WithMessageBudget`, `runtime.WithTokenBudget`, and built-in budget projection constructors.

### Changed

- Kept `conversation.ProjectionPolicy` as an explicit advanced override hook and token estimation helpers for observability/application decisions that do not rewrite history.

## [0.14.0] - 2026-04-25

### Added

- Added approximate token-aware projection budgeting with protected recent messages and tool-boundary repair.
- Added projection-time compaction summaries that replace omitted history in the request without mutating the conversation tree.
- Added `conversation.WithTokenBudget`, `runtime.WithTokenBudget`, `conversation.NewTokenBudgetProjectionPolicy`, and exported message token estimation.

## [0.13.0] - 2026-04-25

### Added

- Added `conversation.ProjectionPolicy` with the default replay/native-continuation projection moved behind an explicit policy hook.
- Added `conversation.WithProjectionPolicy` and `conversation.WithMessageBudget`.
- Added optional projection-time message budgeting, disabled by default.
- Added `runnertest` helpers for fake unified clients, recorded requests, text streams, tool-call streams, reasoning streams, route events, error streams, and incomplete streams.

## [0.12.3] - 2026-04-25

### Fixed

- Preserved Anthropic thinking signatures on committed assistant reasoning parts so tool-loop replay can include signed thinking blocks.
- Dropped unsigned assistant reasoning parts from request projection to avoid replaying invalid Anthropic thinking blocks from older sessions.
- Persisted reasoning signatures in JSONL conversation storage.

## [0.12.2] - 2026-04-25

### Changed

- Updated `github.com/codewandler/llmadapter` to `v0.48.10`.

## [0.12.1] - 2026-04-25

### Fixed

- Limited native `previous_response_id` projection to OpenAI Responses-compatible routes so Codex Responses and other unsupported Responses-family providers fall back to replay.

## [0.12.0] - 2026-04-25

### Added

- Added provider-aware conversation projection with native OpenAI Responses `previous_response_id` continuation support.
- Added branch-head continuation lookup so forked branches use the continuation handle attached to their selected head.
- Added request projection tests for matching continuation handles, Responses API aliases, provider/model mismatch replay fallback, branch forks, and runner integration.

### Changed

- Updated `github.com/codewandler/llmadapter` to `v0.48.9`.
- Updated `runner.RunTurn` to pass resolved provider identity into conversation request projection, enabling native continuation when compatible and replay otherwise.

## [0.11.5] - 2026-04-25

### Changed

- Updated `github.com/codewandler/llmadapter` to `v0.48.7`.

## [0.11.4] - 2026-04-25

### Added

- Added conversation and runtime cache policy defaults/options for `llmadapter/unified` cache controls.

### Changed

- Defaulted conversation-built requests to `cache_policy=on`.

## [0.11.3] - 2026-04-25

### Changed

- Updated `github.com/codewandler/llmadapter` to `v0.48.6`.

## [0.11.2] - 2026-04-25

### Fixed

- Stripped OpenAI Responses `resp_...` assistant ids during message projection so older persisted sessions can still be continued.

## [0.11.1] - 2026-04-25

### Fixed

- Prevented OpenAI Responses `resp_...` continuation IDs from being replayed as assistant message IDs in future requests.

## [0.11.0] - 2026-04-25

### Added

- Added payload-bearing conversation event storage with `conversation.WithStore` and `conversation.Resume` for durable session replay.
- Added `conversation/jsonlstore` for append-only JSONL conversation persistence.
- Added JSON-safe payload encoding for unified messages, assistant turns, tool calls, tool results, continuations, usage, and supported content parts.
- Added local `AGENTS.md` notes covering the llmadapter -> agentsdk -> miniagent dependency update process.

## [0.10.1] - 2026-04-25

### Changed

- Updated `github.com/codewandler/llmadapter` to `v0.48.4` so consumers inherit the latest auto-routing, Claude provider identity, and provider capability fixes.

## [0.10.0] - 2026-04-24

### Changed

- Updated `github.com/codewandler/llmadapter` to `v0.46.0` so runtime consumers inherit modeldb-aware auto intent routing and Anthropic reasoning support.

## [0.9.0] - 2026-04-24

### Added

- Added `runtime.WithToolContextFactory` and per-turn `WithTurnToolContextFactory` so consumers can derive tool contexts from each turn context without manually passing `WithTurnToolCtx`.

## [0.8.0] - 2026-04-24

### Added

- Added `runtime.NewToolContext` and related options for reusable tool context construction with work directory, agent ID, session ID, and extra application state.
- Added README guidance for using `runtime` as the preferred consumer-facing model/tool loop entry point.

### Removed

- Removed the deprecated `interfaces` compatibility package. Use `activation.State` and `websearch.Provider` directly.

## [0.7.0] - 2026-04-24

### Added

- Added a `runtime` package that wraps `conversation.Session` and `runner.RunTurn` with reusable request defaults, session reset support, and per-turn overrides for tools, tool context, event handlers, route identity, and max steps.

## [0.6.0] - 2026-04-24

### Added

- Added runner route metadata events sourced from `llmadapter` mux route selection.
- Added selected-route provider identity propagation into runner usage events, step completion events, and provider continuation metadata.

### Changed

- Updated `llmadapter` to `v0.37.0` so agentsdk consumers can rely on mux-selected provider/model identity without local route guessing.

## [0.5.1] - 2026-04-24

### Added

- Added runner support for streamed tool-call argument deltas, warning/raw event passthrough, custom tool executors, and per-tool execution timeouts.
- Added runner step lifecycle events, provider/model dimensions on usage events, and a helper for decoding tool-call arguments for display.
- Added conversation request/default support for reasoning, response formats, sampling controls, stop sequences, user IDs, and safety config.

### Changed

- Hardened runner tool execution so context cancellation emits canceled results for remaining tool calls and incomplete/error streams do not commit conversation history.

## [0.5.0] - 2026-04-24

### Added

- Added a `websearch` package to own web search provider contracts outside the deprecated `interfaces` compatibility package.
- Added conversation provider continuation metadata, structural storage event types, transactional turn fragments, and atomic fragment commits.
- Added a `runner` package for running model/tool loops over `llmadapter/unified.Client` with event emission and safe end-of-turn conversation commits.

### Changed

- Moved web tools, Tavily, and standard tool bundle wiring from `interfaces.WebSearchProvider` to `websearch.Provider`.
- Kept `interfaces` as deprecated compatibility aliases for activation and web search contracts.

## [0.4.0] - 2026-04-24

### Added

- Added an `activation` package with a reusable tool activation manager and glob-based activation/deactivation support.
- Added a replay-only `conversation` package with session IDs, conversation IDs, branches, append-only tree events, message projection, and `llmadapter/unified` request building.
- Added a `usage` package for recording `llmadapter/unified` usage, aggregating token counts, tracking estimates, calculating costs, checking budgets, and reporting estimate drift.
- Added a `tools/standard` package for assembling the default agentsdk tool bundle with optional git, notification, todo, tool-management, and turn-done tools.
- Added `tool.ToUnified` and `tool.UnifiedToolsFrom` helpers to convert agentsdk tools into `llmadapter/unified.Tool` values.
- Added `.agents/plans/agentsdk-conversation-runtime-llmadapter-plan.md` covering the agentsdk architecture direction, llmadapter conversation requirements, miniagent migration, and flai follow-ups.

### Changed

- Replaced the remaining `github.com/codewandler/llm/tool` dependency in tool schema generation with local JSON Schema reflection.
- Updated tool-management wiring to depend on the new `activation.State` boundary while keeping `interfaces.ActivationState` as a compatibility alias.
- Moved agentsdk toward `llmadapter/unified` as the canonical model-facing boundary for tools, requests, and usage data.

## [0.3.0] - 2026-04-21

### Changed

- Renamed the module from `github.com/codewandler/agentcore` to `github.com/codewandler/agentsdk`.
- Updated internal import paths, README content, and documentation for the new module path.

## [0.2.2] - 2026-04-21

### Changed

- Markdown buffer now emits standalone renderable blocks by trimming inter-block trailing blank lines while preserving stable streaming behavior.
- Updated markdown buffer tests to assert independently renderable block payloads and stable concatenation semantics.

## [0.2.1] - 2026-04-19

### Added

- Added a Tavily web search provider at `tools/web/tavily` with request/response mapping to `interfaces.WebSearchProvider`.
- Added environment-based default provider selection via `web.DefaultSearchProviderFromEnv()`.
- Added focused tests for Tavily provider behavior and default provider selection.

### Changed

- Centralized default web search provider wiring in `tools/web` so consumers can enable a default backend without hardcoding provider construction.
- Default provider policy now supports `TAVILY_API_KEY`, `WEBSEARCH_PROVIDER=tavily`, and `WEBSEARCH_PROVIDER=none`.

## [0.2.0] - 2026-04-18

### Added

- Added a streaming markdown buffer with callback-based stable top-level block emission.
- Added markdown buffer APIs for writing, flushing, resetting, closing, and retrieving buffered content.
- Added configurable markdown parser injection.
- Added regression tests for streaming markdown behavior.
- Added a markdown buffer implementation plan.

### Changed

- Cleaned up the markdown package around consistent goldmark usage.
- Renamed frontmatter files for clearer markdown/frontmatter separation.
- Hardened markdown buffer concurrency behavior with serialized callback delivery.
- Improved writer semantics so accepted writes return an error only on internal failure.
- Documented the markdown buffer emission contract and streaming behavior.

### Fixed

- Preserved whitespace-only buffered tails on flush.
- Prevented invalid buffer internal state from panicking by returning an error.
- Added conservative fenced-code handling for partial streaming markdown.

## [0.1.0] - 2025-04-16

### Added

- Added the initial portable tool interface, typed tool execution framework, tool result formatting, JSON Schema generation, and `StringSliceParam`.
- Added markdown frontmatter parsing and markdown file reading helpers.
- Added standard filesystem, shell, git, web, notification, tool-management, todo, and turn/session helper tools.
- Added the skill metadata parser, directory-based skill loading, and skill reference resolution.
- Added portable runtime interfaces for tool activation and web search providers.
- Added internal humanize and diff utilities.

### Changed

- Extracted agentsdk from flai as a portable SDK with no direct flai runtime dependency.
- Moved plugin/runtime concerns out of the SDK boundary and into consuming applications.
