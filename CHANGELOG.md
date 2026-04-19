# Changelog

All notable changes to codewandler/agentcore are documented in this file.

## [0.2.2] - "$date"

### Changed
- Markdown buffer now emits standalone renderable blocks by trimming inter-block trailing blank lines while preserving stable streaming behavior
- Updated markdown buffer tests to assert independently renderable block payloads and stable concatenation semantics

## [0.2.1] - 2026-04-19

### Added
- Tavily web search provider at `tools/web/tavily` with request/response mapping to `interfaces.WebSearchProvider`
- Env-based default provider selector via `web.DefaultSearchProviderFromEnv()`
- Focused tests for Tavily provider behavior and default provider selection

### Changed
- Centralized default web search provider wiring in `tools/web` so consumers can enable a default backend without hardcoding provider construction
- Default provider policy now supports `TAVILY_API_KEY`, `WEBSEARCH_PROVIDER=tavily`, and `WEBSEARCH_PROVIDER=none`

## [0.2.0] - 2026-04-18

### Added
- Streaming markdown buffer in 
- Callback-based stable top-level markdown block emission
- Buffer API: , , , , , and 
- Configurable markdown parser injection via 
- Extensive regression tests for streaming markdown behavior
- Markdown buffer implementation plan in 

### Changed
- Cleaned up the  package to consistently use 
- Renamed frontmatter files to  and 
- Hardened markdown buffer concurrency behavior with serialized callback delivery
- Improved writer semantics so accepted writes return  on internal failure
- Documented the markdown buffer emission contract and streaming behavior

### Fixed
- Preserved whitespace-only buffered tails on 
- Prevented invalid buffer internal state from panicking by returning a proper error
- Added conservative fenced-code handling for partial streaming markdown

## [0.1.0] - 2025-04-16

### Initial Release

This is the first public release of codewandler/agentcore, extracted from flai as a portable, independent tool system.

### Added

#### Core System
- Tool interface and execution framework
- Result type with display formatting
- Schema validation via jsonschema
- StringSliceParam for flexible JSON input handling

#### Markdown & Configuration
- Frontmatter parsing (YAML headers in markdown files)
- Markdown file reading and parsing utilities

#### Standard Tools (7 categories, 8+ tools)
- **Filesystem** (8 tools): , , , , , , , , 
- **Shell**:  execution tool
- **Git**: , 
- **Web**: ,  (with pluggable provider interface)
- **Notifications**: System notifications, audio alerts, TTS support
- **Tool Management**: , , 
- **Utilities**: Turn/session helpers

#### Skill System
- Skill metadata parsing and discovery
- Directory-based skill loading
- Skill reference resolution

#### Portable Interfaces (NEW)
-  - Tool activation/deactivation contract
-  - Web search provider interface
-  &  - Web search types
- Enables agentcore tools to work with any runtime implementation

#### Internal Utilities
- Humanize: Number and size formatting
- Diff: Change diff utilities

### Architecture

**Zero external runtime dependencies**: No imports from flai, llm adapters, or agent orchestration layers.

**Clean interface contracts**: Tools depend on portable interfaces, not concrete implementations. Flai and other systems implement these interfaces.

**Fully reusable**: Can be integrated into any agent system, not just flai.

### Key Design Decisions

1. **Removed**:  - Plugin system is SDK concern, moved to flai
2. **Created**:  package - Portable contracts for runtime features
3. **Decoupled**: All tools use interfaces instead of importing flai packages

### Commit History

-  - refactor: Remove all flai dependencies, create portable interfaces
-  - Fix: Update to correct module path github.com/codewandler/agentcore
-  - Fix: Update module name to github.com/codewandler/agentcore
-  - Initial commit: Extract core tool system from flai

### Dependencies

**Runtime** (5 packages):
-  - YAML frontmatter parsing
-  - Tool schema generation
-  - Glob pattern matching
-  - .gitignore handling

**Testing** (1 package):
-  - Assertion and testing utilities

### Testing

- All 60+ tests passing
- Build verified:  ✓
- Tests verified: ?   	github.com/codewandler/agentcore/interfaces	[no test files]
ok  	github.com/codewandler/agentcore/internal/diff	(cached)
ok  	github.com/codewandler/agentcore/internal/humanize	(cached)
ok  	github.com/codewandler/agentcore/markdown	(cached)
ok  	github.com/codewandler/agentcore/skill	(cached)
ok  	github.com/codewandler/agentcore/tool	(cached)
ok  	github.com/codewandler/agentcore/tools/filesystem	(cached)
ok  	github.com/codewandler/agentcore/tools/git	(cached)
ok  	github.com/codewandler/agentcore/tools/notify	(cached)
ok  	github.com/codewandler/agentcore/tools/shell	(cached)
ok  	github.com/codewandler/agentcore/tools/todo	(cached)
ok  	github.com/codewandler/agentcore/tools/toolmgmt	(cached)
ok  	github.com/codewandler/agentcore/tools/turn	(cached)
ok  	github.com/codewandler/agentcore/tools/web	(cached) ✓

### Status

🚀 **Production Ready** - Fully functional, tested, and ready for integration.

### Migration to flai

This package is designed to be integrated into flai, which will:
1. Implement  in its runtime
2. Implement  in its adapters
3. Provide implementations via  to tools
4. Result: Tools work seamlessly with flai's orchestration

See  in flai repository for integration steps.

---

## Future Releases

- v0.2.0: Add instruction file loader, examples
- v0.3.0: Additional tool categories, plugin framework improvements
- v1.0.0: Stable API, flai integration complete

[0.2.0]: https://github.com/codewandler/agentcore/releases/tag/v0.2.0
[0.1.0]: https://github.com/codewandler/agentcore/releases/tag/v0.1.0
