# Command tree example

Demonstrates both command authoring forms:

- YAML command resources for structured targets, schemas, and policy.
- Markdown command resources for prompt-style shortcuts.

Try:

```bash
go run ./cmd/agentsdk discover --local examples/command-tree
go run ./cmd/agentsdk run examples/command-tree /workflow list
go run ./cmd/agentsdk run examples/command-tree /help
```
