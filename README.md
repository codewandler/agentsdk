# codewandler/agentsdk

Portable tool definitions, conversation/runtime helpers, markdown utilities, and instruction file loading for LLM agents.

**agentsdk** is a small foundation for agentic CLIs and applications. It owns reusable agent mechanics such as tools, conversation state, runtime turns, usage accounting, and markdown parsing, while consumers keep product policy, terminal rendering, prompts, and storage locations.

## Features

- **Tool system**: Define and execute LLM agent tools with schema validation.
- **Standard tools**: Filesystem, shell, git, web, notifications, todo, and tool activation management.
- **Runtime facade**: Run model/tool turns over `llmadapter/unified.Client`.
- **Terminal app shell**: Build Cobra-based terminal agents and run filesystem
  agent bundles with `agentsdk run`.
- **Resource discovery**: Load `.agents` and `.claude` agents, commands, and
  skills from local directories or declarative git sources.
- **Runtime skill activation**: Discover skills at runtime, activate skills and
  exact `references/` paths, and persist activation state across resumed
  sessions.
- **Model compatibility policy**: Inspect or require llmadapter use-case
  compatibility evidence for agentic workloads.
- **Conversation state**: Session IDs, conversation IDs, branches, replay projection, JSONL persistence, and provider continuation metadata.
- **Usage tracking**: Aggregate `llmadapter/unified` token and cost records with runner usage helpers.
- **Markdown + frontmatter**: Parse and load structured instruction files.
- **Instruction loading**: AGENTS.md and CLAUDE.md pattern support.

## Use Cases

- Build custom agent tools without a full application framework.
- Build CLI/UI agents while keeping terminal rendering and product policy in the consumer.
- Share a tool vocabulary across projects.
- Persist and resume conversation sessions.
- Load configuration from markdown instruction files.
- Ship slim filesystem-described agents without writing a full Go app.

## Recommended Runtime Stack

Use `runtime` as the high-level turn loop, `tools/standard` for the default tool bundle, and `llmadapter` auto mux for provider selection.

```go
import (
    "context"

    "github.com/codewandler/agentsdk/activation"
    "github.com/codewandler/agentsdk/runner"
    "github.com/codewandler/agentsdk/runtime"
    "github.com/codewandler/agentsdk/tool"
    "github.com/codewandler/agentsdk/tools/standard"
    "github.com/codewandler/llmadapter/adapt"
    "github.com/codewandler/llmadapter/unified"
)

model := "codex/gpt-5.5"
sourceAPI := adapt.ApiOpenAIResponses

auto, err := runtime.AutoMuxClient(model, sourceAPI, nil)
if err != nil {
    return err
}
identity, _, _ := runtime.RouteIdentity(auto, sourceAPI, model)

toolActivation := activation.New(standard.DefaultTools()...)
agent, err := runtime.New(auto.Client,
    runtime.WithProviderIdentity(identity),
    runtime.WithModel(model),
    runtime.WithSystem("You are a concise coding assistant."),
    runtime.WithTools(toolActivation.ActiveTools()),
    runtime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
    runtime.WithCachePolicy(unified.CachePolicyOn),
    runtime.WithCacheKey("session-id"),
    runtime.WithMaxSteps(8),
    runtime.WithToolContextFactory(func(ctx context.Context) tool.Ctx {
        return runtime.NewToolContext(ctx,
            runtime.WithToolWorkDir("."),
            runtime.WithToolSessionID("session-id"),
            runtime.WithToolActivation(toolActivation),
            // Add runtime.WithToolSkillActivation(state) when your app wires
            // mutable skill activation state into the model tool context.
        )
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

### Durable Thread Runtime

Use the thread runtime helpers for durable agents. A thread stores conversation
events, capability attachment/state events, context render records, branch
metadata, and lifecycle metadata in one append-only event log. Callers choose
the storage location; the runtime wires the conversation session and context
manager for you.

```go
import (
    "time"

    "github.com/codewandler/agentsdk/agentcontext/contextproviders"
    "github.com/codewandler/agentsdk/capabilities/planner"
    "github.com/codewandler/agentsdk/capability"
    "github.com/codewandler/agentsdk/runtime"
    "github.com/codewandler/agentsdk/thread"
    threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
    "github.com/codewandler/llmadapter/unified"
)

store := threadjsonlstore.Open("/path/to/threads")
registry, err := capability.NewRegistry(planner.Factory{})
if err != nil {
    return err
}

agent, stored, err := runtime.OpenThreadEngine(ctx,
    store,
    thread.CreateParams{
        ID:     "thread_session-id",
        Source: thread.EventSource{Type: "cli", SessionID: "session-id"},
        Metadata: map[string]string{"title": "repo inspection"},
    },
    auto.Client,
    registry,
    runtime.WithProviderIdentity(identity),
    runtime.WithModel(model),
    runtime.WithSystem("You are a concise coding assistant."),
    runtime.WithTools(toolActivation.ActiveTools()),
    runtime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
    runtime.WithCapabilities(capability.AttachSpec{
        CapabilityName: planner.CapabilityName,
        InstanceID:     "planner_1",
    }),
    runtime.WithContextProviders(
        contextproviders.Environment(contextproviders.WithWorkDir(".")),
        contextproviders.Time(time.Minute),
    ),
    runtime.WithCachePolicy(unified.CachePolicyOn),
    runtime.WithCacheKey("session-id"),
)
if err != nil {
    return err
}
_ = stored.ID

_, err = agent.RunTurn(ctx, "inspect this repo")
```

Use `CreateThreadEngine` when creating a brand-new thread must fail if the ID
already exists. Use `ResumeThreadEngine` when a missing thread should fail. Use
`OpenThreadEngine` for the usual create-or-resume path.

Thread-backed engines restore:

- committed conversation messages and provider continuation metadata
- attached capabilities and their event-sourced state, such as planner steps
- active tools exported by attached capabilities
- context render records used for full replay and native-continuation diffs

Applications do not need to manually create `conversation.ThreadEventStore`.
For explicit semantic compaction, call `agent.Compact(ctx, summary, nodeIDs...)`;
the runtime appends the compaction event and refreshes persisted context render
records.

## CLI Resource Bundles

The built-in CLI can run an agent described by files on disk:

```bash
go run ./cmd/agentsdk run [path] [task]
```

`path` defaults to the current working directory. When no explicit agent is
found, agentsdk uses a small built-in general-purpose terminal agent so
`agentsdk run` still opens a usable REPL.

Resource roots can be shaped like either project compatibility directories or
plugin roots:

```text
.agents/
  agents/
  commands/
  skills/

.claude/
  agents/
  commands/
  skills/

plugin-root/
  agents/
  commands/
  skills/
```

Agent files live in `agents/*.md` and use YAML frontmatter plus a Markdown
system prompt body:

```markdown
---
name: coder
description: General coding agent
tools: [bash, file_read]
skills: [go]
commands: [review]
---
You are a concise coding agent.
```

Command files live in `commands/*.md`; their frontmatter is parsed into slash
command metadata and their body is used as a prompt template. Skills use the
`SKILL.md` directory format under `skills/<name>/SKILL.md`. Skill directories
may also contain optional reference files under `skills/<name>/references/*.md`.
Runtime skill activation recognizes only exact relative paths under
`references/` as activatable skill references.

At runtime, the terminal app exposes:

- `/skills` — list discovered skills and their activation state
- `/skill <name>` — activate a discovered skill on the current agent session

If the `skill` tool is available to the model, it can activate skills and exact
reference paths with batched actions.

To inspect what agentsdk can load without running an agent:

```bash
go run ./cmd/agentsdk discover [path]
go run ./cmd/agentsdk discover --local [path]
```

`discover` is broad by default: it includes the target path, global user
resources, manifest-declared git sources, and disabled suggestions for known
external files such as `AGENTS.md` or `Taskfile.yaml`. `--local` limits
inspection to the target path and disables global and remote sources.

Generic `agentsdk run` does not include global user resources by default. Pass
`--include-global` to include `~/.agents` and `~/.claude`:

```bash
go run ./cmd/agentsdk run . --include-global
```

Use-case compatibility can be inspected or enforced through llmadapter evidence:

```bash
go run ./cmd/agentsdk run . --model-use-case agentic_coding -v
go run ./cmd/agentsdk run . --model-approved-only -v
go run ./cmd/agentsdk models --model-use-case agentic_coding
```

`--model-approved-only` pins runtime routing to one evidence-approved provider
route and disables fallback. Without `--model-approved-only`,
`--model-use-case` only adds diagnostics. `--source-api auto` allows selection
across supported source APIs; explicit values such as `openai.responses` or
`anthropic.messages` restrict selection and routing.

## App Manifests

A directory can contain `app.manifest.json` or `agentsdk.app.json` to declare
the app's default agent, discovery policy, and source list:

```json
{
  "default_agent": "coder",
  "discovery": {
    "include_global_user_resources": false,
    "include_external_ecosystems": false,
    "allow_remote": false,
    "trust_store_dir": ".agentsdk"
  },
  "model_policy": {
    "use_case": "agentic_coding",
    "source_api": "auto",
    "approved_only": false
  },
  "sources": [
    ".agents",
    "file:///absolute/path/to/plugin",
    "git+https://github.com/codewandler/agentplugins.git#main"
  ]
}
```

Source strings are URL-like:

- Bare paths are resolved relative to the manifest directory.
- `file://...` points at an explicit local directory.
- `git+https://...` and `git+ssh://...` materialize a git repository/ref under
  `<workspace>/.agentsdk/cache/git/...` and then load declarative resources
  from it.

Remote sources are declarative only; repository code is not executed just by
loading it.

### Cache And History Rules

By default, applications should use stable per-session cache affinity:

```go
runtime.WithCachePolicy(unified.CachePolicyOn)
runtime.WithCacheKey(sessionID)
```

Provider history must remain immutable. Do not trim, compact, summarize, or otherwise rewrite projected history for cost control. For providers that support native continuation, agentsdk can send the provider continuation handle; otherwise it replays the canonical selected-branch history. Usage response data is for observability, reporting, warnings, or explicit product UX, not automatic SDK history rewriting.

## Tool Context

Use `runtime.NewToolContext` when tools need a work directory, session ID, or extra app state:

```go
toolCtx := runtime.NewToolContext(ctx,
    runtime.WithToolWorkDir(workDir),
    runtime.WithToolSessionID(sessionID),
    runtime.WithToolActivation(toolActivation),
)
```

`runtime.WithToolActivation` wires the state used by `tools_list`, `tools_activate`, and `tools_deactivate`.

## Web Tools

The `tools/web` package provides:

- `web_fetch` — always available when you register the web tools.
- `web_search` — available when you pass a search provider.

Agentsdk includes a Tavily provider at `github.com/codewandler/agentsdk/tools/web/tavily` and a small env-based selector:

```go
provider := web.DefaultSearchProviderFromEnv()
webTools := web.Tools(provider)
```

Environment variables:

- `TAVILY_API_KEY` — enables the default Tavily-backed web search provider.
- `WEBSEARCH_PROVIDER=tavily` — explicitly select Tavily.
- `WEBSEARCH_PROVIDER=none` — disable web search while keeping `web_fetch` available.

## First-party Apps

The `apps/` directory contains larger first-party dogfood applications. These
apps are used to validate agentsdk product architecture and may be more complete
than the small instructional examples.

| App | Description |
|-----|-------------|
| [`engineer`](apps/engineer/) | Resource-only coding, architecture, code-review, and DevOps dogfood agent |
| [`builder`](apps/builder/) | Planned builder app for the future `agentsdk build` experience |

Run the engineer app from the repository root:

```bash
go run ./cmd/agentsdk run apps/engineer
```

## Examples

The `examples/` directory contains small instructional agent applications:

| Example | Description |
|---------|-------------|
| [`devops-cli`](examples/devops-cli/) | CLI agent with custom tool wiring |
| [`research-desk`](examples/research-desk/) | Multi-source research agent with resource bundles |
| [`release-notes-agent`](examples/release-notes-agent/) | Release notes generation agent |
| [`repo-maintainer`](examples/repo-maintainer/) | Repository maintenance agent |

## Package Index

Use these directly when `runtime.Agent` is too high level:

| Package | Purpose |
|---------|---------|
| `activation` | Reusable tool activation manager with glob-based activate/deactivate |
| `agent` | Agent resource definitions, model policy, and evidence evaluation |
| `agentcontext` | Context manager, context providers, render records, and fingerprinting |
| `agentcontext/contextproviders` | Built-in context providers including environment, git, time, file, command, and project inventory |
| `agentdir` | Agent directory loading, external resource resolution, and source discovery |
| `app` | App manifest loading, plugin bundles, and skill wiring |
| `capabilities/planner` | Built-in planner capability for structured task plans |
| `capability` | Capability interface, registry, manager, and event-sourced state |
| `command` | Declarative command trees, slash parsing, structured results, descriptors, and tool/action adapters |
| `conversation` | Branchable conversation tree, internal items, request projection, compaction, and provider continuations |
| `internal/diff` | Internal unified diff helpers |
| `internal/humanize` | Internal human-readable formatting |
| `markdown` | Streaming markdown buffer, frontmatter parsing, and instruction file loading |
| `plugin` | Plugin bundle definitions |
| `resource` | Resource discovery and resource type definitions |
| `runner` | Model/tool loop over `llmadapter/unified.Client` with typed UI events |
| `runnertest` | Fake unified clients, recorded requests, and stream helpers for testing |
| `runtime` | High-level agent runtime, thread engine, auto mux, and tool context |
| `skill` | Skill metadata, directory loading, reference resolution, and search |
| `terminal/cli` | Cobra-based CLI wiring for terminal agents |
| `terminal/repl` | Interactive REPL loop |
| `terminal/ui` | Terminal UI rendering and formatting |
| `thread` | Thread event log, store interface, and memory store |
| `thread/jsonlstore` | Append-only JSONL thread persistence |
| `tool` | Tool definitions, schemas, execution contracts, and unified conversion |
| `tools/filesystem` | File read, write, edit, copy, move, glob, stat, delete, and directory tools |
| `tools/git` | Git status, diff, add, and commit tools |
| `tools/jsonquery` | JSON file query tool with field, index, and wildcard selectors |
| `tools/notify` | Desktop notification and TTS tools |
| `tools/shell` | Bash command execution with streaming and timeout |
| `tools/standard` | Default tool bundle assembly; mutable activation is owned by `activation.Manager` |
| `tools/todo` | Todo/task list tools |
| `tools/toolmgmt` | Tool list, activate, and deactivate management tools |
| `tools/turn` | Turn-done signaling tool |
| `tools/web` | Web fetch and web search tools |
| `tools/web/tavily` | Tavily-backed web search provider |
| `usage` | Token/cost records, aggregation, drift helpers, and runner event conversion |
| `websearch` | Web search provider interface |

## Further Reading

- [`AGENTS.md`](AGENTS.md) — developer and AI agent notes, testing guidance, dependency process.
- [`CHANGELOG.md`](CHANGELOG.md) — release history and migration notes.
- [`docs/RESOURCES.md`](docs/RESOURCES.md) — external format references and compatibility layouts.
- [`.agents/reviews`](.agents/reviews/) — detailed architecture and implementation review notes.

## Status

Under active development, extracted from flai as a portable foundation. The current shape is proven through `miniagent`, which uses `runtime`, `conversation`, `usage`, `tools/standard`, and `llmadapter` auto mux helpers.

## License

MIT
