# codewandler/agentcore

Portable tool definitions, markdown utilities, and instruction file loading for LLM agents.

**agentcore** provides a minimal, dependency-light foundation for agent tool systems—reusable across projects without requiring a full agent runtime.

## Features

- **Tool System**: Define and execute LLM agent tools with schema validation
- **Standard Tools**: Filesystem, shell, git, web, notifications, todo
- **Markdown + Frontmatter**: Parse and load structured instruction files
- **Instruction Loading**: AGENTS.md, CLAUDE.md pattern support
- **Minimal Dependencies**: ~5 external packages

## Use Cases

- Build custom agent tools without the full flai SDK
- Integrate tools into lightweight systems
- Define shared tool vocabulary across projects
- Load configuration from markdown instruction files

## Quick Start

```go
import (
    "github.com/codewandler/agentcore/tool"
    "github.com/codewandler/agentcore/tools/filesystem"
    "github.com/codewandler/agentcore/tools/todo"
    "github.com/codewandler/agentcore/markdown"
)

// Load instruction files
files, _ := markdown.LoadInstructionFiles(".", markdown.DefaultPatterns)

// Get standard tools
fsTools := filesystem.Tools()
todoTools := todo.Tools()

// Use tools in your system
for _, t := range append(fsTools, todoTools...) {
    // Register and execute tools
}
```

## Web tools

The `tools/web` package provides:

- `web_fetch` — always available when you register the web tools
- `web_search` — available when you pass a search provider

Agentcore also includes a Tavily provider at `github.com/codewandler/agentcore/tools/web/tavily` and a small env-based selector:

```go
provider := web.DefaultSearchProviderFromEnv()
webTools := web.Tools(provider)
```

Environment variables:

- `TAVILY_API_KEY` — enables the default Tavily-backed web search provider
- `WEBSEARCH_PROVIDER=tavily` — explicitly select Tavily
- `WEBSEARCH_PROVIDER=none` — disable web search while keeping `web_fetch` available

This keeps provider selection centralized and makes future provider swaps easy for consumers.

## Status

🚧 Under development — extracted from flai as a portable foundation.

## License

MIT
