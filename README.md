# codewandler/core

Portable tool definitions, markdown utilities, and instruction file loading for LLM agents.

**core** provides a minimal, dependency-light foundation for agent tool systems—reusable across projects without requiring a full agent runtime.

## Features

- **Tool System**: Define and execute LLM agent tools with schema validation
- **Standard Tools**: Filesystem, shell, git, web, notifications
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
    "github.com/codewandler/core/tool"
    "github.com/codewandler/core/tools/filesystem"
    "github.com/codewandler/core/markdown"
)

// Load instruction files
files, _ := markdown.LoadInstructionFiles(".", markdown.DefaultPatterns)

// Get standard tools
fsTools := filesystem.Tools()

// Use tools in your system
for _, t := range fsTools {
    // Register and execute tools
}
```

## Status

🚧 Under development — extracted from flai as a portable foundation.

## License

MIT
