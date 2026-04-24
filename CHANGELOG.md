# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

Versions below are backfilled from the repository's implementation milestones. Tags
match these entries as the project starts publishing releases.

## [Unreleased]

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
