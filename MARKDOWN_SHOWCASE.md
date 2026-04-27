# Markdown Showcase for agentsdk

A comprehensive demonstration of markdown capabilities in the context of the agentsdk project.

---

## Table of Contents

1. [Text Formatting](#text-formatting)
2. [Code Examples](#code-examples)
3. [Lists and Nesting](#lists-and-nesting)
4. [Blockquotes and Callouts](#blockquotes-and-callouts)
5. [Tables](#tables)
6. [Links and References](#links-and-references)
7. [Images and Diagrams](#images-and-diagrams)
8. [Advanced Features](#advanced-features)

---

## Text Formatting

### Basic Emphasis

This text demonstrates **bold text** for important concepts, *italic text* for emphasis, and ***bold italic*** for maximum impact. You can also use `inline code` for technical terms.

### Strikethrough and Subscript

The old `flai` naming convention is ~~deprecated~~ maintained for compatibility. The formula H₂O shows subscript capability.

### Headings Hierarchy

# Heading 1 (H1)
## Heading 2 (H2)
### Heading 3 (H3)
#### Heading 4 (H4)
##### Heading 5 (H5)
###### Heading 6 (H6)

---

## Code Examples

### Inline Code

Run `go test ./...` to execute the full test suite, or `go test ./runtime/...` for focused testing.

### Code Blocks with Syntax Highlighting

#### Go Code Block

```go
package main

import (
	"fmt"
	"github.com/codewandler-ai/agentsdk/tool"
)

func main() {
	// Initialize the tool registry
	registry := tool.NewRegistry()
	
	// Register custom tools
	registry.Register("devops-cli", &DevOpsTool{})
	
	fmt.Println("agentsdk initialized successfully")
}
```

#### Bash Commands

```bash
#!/bin/bash
# Dependency update process for agentsdk

# Step 1: Verify llmadapter release
llmadapter resolve claude-3-5-sonnet

# Step 2: Update agentsdk
go get -u github.com/codewandler-ai/llmadapter@v1.2.3

# Step 3: Run tests
go test ./...

# Step 4: Commit and tag
git tag -a v2.1.0 -m "Release v2.1.0"
git push origin v2.1.0
```

#### YAML Configuration

```yaml
# agentsdk configuration example
runtime:
  version: "2.1.0"
  timeout: 30s
  max_retries: 3

tools:
  - name: devops-cli
    enabled: true
    config:
      log_level: debug
  
  - name: research-desk
    enabled: true
    resources:
      - type: bundle
        path: ./resources/research

skills:
  - name: architecture
    status: active
  - name: golang-pro
    status: active
```

#### JSON Example

```json
{
  "agentsdk": {
    "version": "2.1.0",
    "branding": {
      "legacy": "flai",
      "current": "agentsdk",
      "compatibility": "maintained"
    },
    "examples": [
      {
        "name": "devops-cli",
        "type": "CLI agent",
        "status": "active"
      },
      {
        "name": "research-desk",
        "type": "Multi-source research",
        "status": "active"
      },
      {
        "name": "release-notes-agent",
        "type": "Release notes generation",
        "status": "planned"
      }
    ]
  }
}
```

---

## Lists and Nesting

### Unordered Lists

- **Core Components**
  - Runtime stack
  - Tool management
  - Conversation handling
  - Skill registry
    - Architecture skill
    - DevOps skill
    - Code review skill
    - Golang-pro skill

- **Examples Directory**
  - `examples/devops-cli/` — CLI agent with custom tool wiring
  - `examples/research-desk/` — Multi-source research agent
  - `examples/release-notes-agent/` — Planned feature
  - `examples/repo-maintainer/` — Planned feature

### Ordered Lists

1. **Dependency Update Process**
   1. Verify or cut the `llmadapter` release
   2. Update `agentsdk` to that released version
   3. Run `go test ./...` in `agentsdk`
   4. Commit, tag, and push the `agentsdk` release
   5. Update consumers like `../miniagent`
      1. Update `go.mod` with new versions
      2. Run consumer tests
      3. Reinstall compiled binary with `task install`
   6. Verify installed-binary behavior

### Mixed Lists

- **Testing Strategy**
  1. Run full suite: `go test ./...`
  2. Run focused tests:
     - `go test ./runtime/...`
     - `go test ./conversation/...`
     - `go test ./tool/...`
  3. Check review notes in `.agents/reviews/`

---

## Blockquotes and Callouts

### Standard Blockquote

> The agentsdk project maintains backward compatibility with the predecessor `flai` naming convention through public constants like `tools/toolmgmt.KeyActivationState` and `skill.RegistryKey`. However, all new code should use `agentsdk` naming exclusively.

### Nested Blockquotes

> **Important Consideration**
>
> > When upgrading `llmadapter`, the dependency chain must be updated deliberately and in order. This ensures that all consumers receive compatible versions and that the compiled `miniagent` binary is properly rebuilt.

### Callout Blocks (using blockquote styling)

> ⚠️ **Warning**: The `miniagent` binary is compiled Go code. Simply updating `go.mod` does not update the installed binary. Always run `task install` after dependency updates.

> ✅ **Best Practice**: Before committing changes, run the full test suite with `go test ./...` to catch regressions early.

> 💡 **Tip**: Use focused test runs during development to speed up iteration cycles.

> 🔗 **Reference**: See `.agents/reviews/` for detailed architecture and implementation notes.

---

## Tables

### Project Structure

| Directory | Purpose | Status |
|-----------|---------|--------|
| `examples/devops-cli/` | CLI agent with custom tools | Active |
| `examples/research-desk/` | Multi-source research agent | Active |
| `examples/release-notes-agent/` | Release notes generation | Planned |
| `examples/repo-maintainer/` | Repository maintenance | Planned |
| `docs/` | Documentation and references | Active |
| `.agents/reviews/` | Architecture review notes | Active |

### Skills Availability

| Skill | Description | Status | References |
|-------|-------------|--------|-----------|
| `architecture` | Software architecture evaluation | Inactive | — |
| `axon` | Axon CLI operations | Inactive | 2 |
| `babelforce` | Babelforce engineering knowledge | Inactive | 3 |
| `code-review` | Code review analysis | Inactive | — |
| `devops` | CI/CD and infrastructure | Inactive | — |
| `dex` | Kubernetes and platform ops | Inactive | 11 |
| `golang-pro` | Go concurrency and microservices | Inactive | 5 |

### Dependency Chain Update Checklist

| Step | Action | Verification | Status |
|------|--------|--------------|--------|
| 1 | Verify `llmadapter` release | `llmadapter resolve <model>` | ✓ |
| 2 | Update `agentsdk` dependency | `go get -u` | ✓ |
| 3 | Run agentsdk tests | `go test ./...` | ✓ |
| 4 | Tag and push agentsdk | `git tag && git push` | ✓ |
| 5 | Update consumer `go.mod` | `go get -u` | ⏳ |
| 6 | Run consumer tests | `go test ./...` | ⏳ |
| 7 | Rebuild binary | `task install` | ⏳ |
| 8 | Smoke test | Manual verification | ⏳ |

---

## Links and References

### External Links

- [Anthropic Claude Documentation](https://docs.anthropic.com)
- [Go Official Website](https://golang.org)
- [GitHub - agentsdk Repository](https://github.com/codewandler-ai/agentsdk)

### Internal Cross-References

- See [README.md](./README.md) for public API overview
- Check [CHANGELOG.md](./CHANGELOG.md) for release history
- Review [docs/RESOURCES.md](./docs/RESOURCES.md) for format references
- Consult [.agents/reviews/](./agents/reviews/) for detailed notes
- Read [AGENTS.md](./AGENTS.md) for developer guidelines

### Reference-Style Links

This project follows the [established Go conventions][go-conventions] and maintains [backward compatibility][compat-note] with legacy naming.

[go-conventions]: https://golang.org/doc/effective_go
[compat-note]: #branding-flai--agentsdk

---

## Images and Diagrams

### ASCII Diagram: Dependency Chain

```
┌─────────────────────────────────────────────────────────────┐
│                    Dependency Chain Flow                     │
└─────────────────────────────────────────────────────────────┘

    ┌──────────────┐
    │ llmadapter   │  (Release new version)
    │   v1.2.3     │
    └──────┬───────┘
           │
           ▼
    ┌──────────────┐
    │  agentsdk    │  (Update & test)
    │   v2.1.0     │
    └──────┬───────┘
           │
           ├─────────────────┬──────────────────┐
           ▼                 ▼                  ▼
    ┌──────────────┐  ┌──────────────┐  ┌──────────────┐
    │  miniagent   │  │ other-client │  │ another-app  │
    │   (rebuild)  │  │   (update)   │  │   (update)   │
    └──────────────┘  └──────────────┘  └──────────────┘
```

### ASCII Diagram: Project Structure

```
agentsdk/
├── examples/
│   ├── devops-cli/          ← CLI agent with custom tools
│   ├── research-desk/       ← Multi-source research
│   ├── release-notes-agent/ ← Planned
│   └── repo-maintainer/     ← Planned
├── runtime/                 ← Runtime stack
├── conversation/            ← Conversation handling
├── tool/                    ← Tool management
├── skill/                   ← Skill registry
├── docs/
│   └── RESOURCES.md         ← Format references
├── .agents/
│   └── reviews/             ← Architecture notes
├── README.md                ← Public API overview
├── CHANGELOG.md             ← Release history
├── AGENTS.md                ← Developer guidelines
└── go.mod                   ← Dependency manifest
```

---

## Advanced Features

### Footnotes and Definitions

The agentsdk project[^1] maintains compatibility with the legacy `flai` naming[^2] while encouraging new code to use `agentsdk` exclusively.

[^1]: agentsdk is the current branding for the agent SDK framework.
[^2]: The `flai` prefix is retained in public constants like `tools/toolmgmt.KeyActivationState` for downstream compatibility.

### Definition Lists

Term
: Definition of the term in the context of agentsdk.

Tool Registry
: Central component for managing and registering custom tools in the agentsdk runtime.

Skill
: Reusable capability that agents can activate to perform specialized tasks (e.g., architecture, devops, code-review).

Resource Bundle
: Collection of external resources (files, APIs, data) that agents can access during execution.

### Horizontal Rules

---

### Escaped Characters

You can escape special markdown characters like \*, \_, \[, \], and \# when needed.

### HTML Passthrough

<div style="background-color: #f0f0f0; padding: 10px; border-radius: 5px;">
  <strong>Note:</strong> Some markdown renderers support inline HTML for advanced styling and layout control.
</div>

### Task Lists

- [x] Implement core runtime stack
- [x] Create tool management system
- [x] Build conversation handler
- [ ] Complete release-notes-agent example
- [ ] Complete repo-maintainer example
- [ ] Add comprehensive API documentation
- [ ] Implement advanced skill composition

### Collapsible Sections

<details>
<summary><strong>Click to expand: Detailed Dependency Update Process</strong></summary>

1. **Verify llmadapter Release**
   ```bash
   llmadapter resolve claude-3-5-sonnet
   ```

2. **Update agentsdk**
   ```bash
   go get -u github.com/codewandler-ai/llmadapter@v1.2.3
   go test ./...
   ```

3. **Commit and Tag**
   ```bash
   git commit -m "chore: update llmadapter to v1.2.3"
   git tag -a v2.1.0 -m "Release v2.1.0"
   git push origin v2.1.0
   ```

4. **Update Consumers**
   ```bash
   cd ../miniagent
   go get -u github.com/codewandler-ai/agentsdk@v2.1.0
   task install
   ```

5. **Verify Installation**
   ```bash
   miniagent -m claude-3-5-sonnet --help
   ```

</details>

---

## Summary

This markdown showcase demonstrates:

✨ **Text Formatting** — Bold, italic, strikethrough, and inline code  
📝 **Code Blocks** — Go, Bash, YAML, and JSON with syntax highlighting  
📋 **Lists** — Ordered, unordered, nested, and mixed list structures  
💬 **Blockquotes** — Standard and nested blockquotes with callout styling  
📊 **Tables** — Multi-column tables with alignment and content  
🔗 **Links** — External links, internal references, and reference-style links  
📐 **Diagrams** — ASCII art for architecture and flow visualization  
✅ **Advanced** — Footnotes, task lists, collapsible sections, and HTML passthrough  

---

**Last Updated:** 2026-04-27  
**Project:** agentsdk  
**Status:** Documentation Complete
