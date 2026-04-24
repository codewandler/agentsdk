# codewandler/agentsdk

Portable tool definitions, conversation/runtime helpers, markdown utilities, and instruction file loading for LLM agents.

**agentsdk** provides a dependency-light foundation for agent tool systems and small agent runtimes, reusable across projects without taking over application-specific CLI or UI concerns.

## Features

- **Tool System**: Define and execute LLM agent tools with schema validation
- **Standard Tools**: Filesystem, shell, git, web, notifications, todo
- **Runtime Facade**: Run conversation turns over `llmadapter/unified.Client`
- **Conversation State**: Session IDs, conversation IDs, branches, and replayable message projection
- **Usage Tracking**: Aggregate `llmadapter/unified` token and cost records
- **Markdown + Frontmatter**: Parse and load structured instruction files
- **Instruction Loading**: AGENTS.md, CLAUDE.md pattern support

## Use Cases

- Build custom agent tools without the full flai SDK
- Integrate tools into lightweight systems
- Define shared tool vocabulary across projects
- Load configuration from markdown instruction files
- Build CLI/UI agents while keeping terminal rendering and product policy in the consumer

## Quick Start

```go
import (
    "github.com/codewandler/agentsdk/tool"
    "github.com/codewandler/agentsdk/tools/filesystem"
    "github.com/codewandler/agentsdk/tools/todo"
    "github.com/codewandler/agentsdk/markdown"
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

## Runtime

The `runtime` package is the preferred entry point for consumers that want a ready model/tool turn loop without hand-wiring `conversation.Session` and `runner.RunTurn`.

```go
import (
    "context"

    "github.com/codewandler/agentsdk/runner"
    "github.com/codewandler/agentsdk/runtime"
    "github.com/codewandler/agentsdk/tool"
    "github.com/codewandler/agentsdk/tools/standard"
    "github.com/codewandler/llmadapter/adapterconfig"
    "github.com/codewandler/llmadapter/adapt"
    "github.com/codewandler/llmadapter/unified"
)

auto, err := adapterconfig.AutoMuxClient(adapterconfig.AutoOptions{
    EnableEnv: true,
    EnableLocalClaude: true,
    SourceAPI: adapt.ApiOpenAIResponses,
    Intents: []adapterconfig.AutoIntent{{Name: "default"}},
})
if err != nil {
    return err
}

tools := standard.Tools(standard.Options{IncludeToolManagement: true})
workDir := "."
agent, err := runtime.New(auto.Client,
    runtime.WithModel("default"),
    runtime.WithSystem("You are a concise coding assistant."),
    runtime.WithTools(tools),
    runtime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
    runtime.WithMaxSteps(8),
    runtime.WithToolContextFactory(func(ctx context.Context) tool.Ctx {
        return runtime.NewToolContext(ctx, runtime.WithToolWorkDir(workDir))
    }),
    runtime.WithEventHandler(func(event runner.Event) {
        // Render text/tool/usage events in your application.
    }),
)
if err != nil {
    return err
}

_, err = agent.RunTurn(context.Background(), "inspect this repo")
```

Use `runtime.NewToolContext` when tools need a work directory, session ID, or extra app state:

```go
toolCtx := runtime.NewToolContext(ctx,
    runtime.WithToolWorkDir(workDir),
    runtime.WithToolSessionID(sessionID),
)
```

## Web tools

The `tools/web` package provides:

- `web_fetch` — always available when you register the web tools
- `web_search` — available when you pass a search provider

Agentcore also includes a Tavily provider at `github.com/codewandler/agentsdk/tools/web/tavily` and a small env-based selector:

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
