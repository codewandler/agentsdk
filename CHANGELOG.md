# Changelog

All notable changes to codewandler/agentcore are documented in this file.

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
- **Filesystem** (8 tools): `file_read`, `file_write`, `file_edit`, `glob`, `grep`, `dir_list`, `dir_tree`, `file_stat`, `file_delete`
- **Shell**: `bash` execution tool
- **Git**: `git_status`, `git_diff` 
- **Web**: `web_fetch`, `web_search` (with pluggable provider interface)
- **Notifications**: System notifications, audio alerts, TTS support
- **Tool Management**: `tools_list`, `tools_activate`, `tools_deactivate`
- **Utilities**: Turn/session helpers

#### Skill System
- Skill metadata parsing and discovery
- Directory-based skill loading
- Skill reference resolution

#### Portable Interfaces (NEW)
- `interfaces/ActivationState` - Tool activation/deactivation contract
- `interfaces/WebSearchProvider` - Web search provider interface
- `interfaces/SearchOptions` & `interfaces/Result` - Web search types
- Enables agentcore tools to work with any runtime implementation

#### Internal Utilities
- Humanize: Number and size formatting
- Diff: Change diff utilities

### Architecture

**Zero external runtime dependencies**: No imports from flai, llm adapters, or agent orchestration layers.

**Clean interface contracts**: Tools depend on portable interfaces, not concrete implementations. Flai and other systems implement these interfaces.

**Fully reusable**: Can be integrated into any agent system, not just flai.

### Key Design Decisions

1. **Removed**: `plugin/plugin.go` - Plugin system is SDK concern, moved to flai
2. **Created**: `interfaces/` package - Portable contracts for runtime features
3. **Decoupled**: All tools use interfaces instead of importing flai packages

### Commit History

- `2bbc7c5` - refactor: Remove all flai dependencies, create portable interfaces
- `0a3d220` - Fix: Update to correct module path github.com/codewandler/agentcore
- `f7ab5ff` - Fix: Update module name to github.com/codewandler/agentcore
- `0250bfb` - Initial commit: Extract core tool system from flai

### Dependencies

**Runtime** (5 packages):
- `gopkg.in/yaml.v3` - YAML frontmatter parsing
- `github.com/invopop/jsonschema` - Tool schema generation
- `github.com/bmatcuk/doublestar/v4` - Glob pattern matching
- `github.com/sabhiram/go-gitignore` - .gitignore handling

**Testing** (1 package):
- `github.com/stretchr/testify` - Assertion and testing utilities

### Testing

- All 60+ tests passing
- Build verified: `go build ./...` ✓
- Tests verified: `go test ./...` ✓

### Status

🚀 **Production Ready** - Fully functional, tested, and ready for integration.

### Migration to flai

This package is designed to be integrated into flai, which will:
1. Implement `interfaces/ActivationState` in its runtime
2. Implement `interfaces/WebSearchProvider` in its adapters
3. Provide implementations via `ctx.Extra()` to tools
4. Result: Tools work seamlessly with flai's orchestration

See `MIGRATION_PLAN.md` in flai repository for integration steps.

---

## Future Releases

- v0.2.0: Add instruction file loader, examples
- v0.3.0: Additional tool categories, plugin framework improvements
- v1.0.0: Stable API, flai integration complete

[0.2.0]: https://github.com/codewandler/agentcore/releases/tag/v0.2.0
[0.1.0]: https://github.com/codewandler/agentcore/releases/tag/v0.1.0
