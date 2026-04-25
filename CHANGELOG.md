# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Versions below are backfilled from the repository's implementation milestones. Tags
match these entries as the project starts publishing releases.

## [Unreleased]

## [0.14.0] - 2026-04-25

### Added

- Added approximate token-aware projection budgeting with protected recent messages and tool-boundary repair.
- Added projection-time compaction summaries that replace omitted history in the request without mutating the conversation tree.
- Added `conversation.WithTokenBudget`, `runtime.WithTokenBudget`, `conversation.NewTokenBudgetProjectionPolicy`, and exported message token estimation.

## [0.13.0] - 2026-04-25

### Added

- Added `conversation.ProjectionPolicy` with the default replay/native-continuation projection moved behind an explicit policy hook.
- Added `conversation.WithProjectionPolicy`, `conversation.WithMessageBudget`, `runtime.WithProjectionPolicy`, and `runtime.WithMessageBudget`.
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
- Added `runtime.WithRequestDefaults` for setting reusable request defaults in one option.

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
