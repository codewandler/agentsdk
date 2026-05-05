# DESIGN: Resource Identity and Resolution

Status: draft
Date: 2026-05-05

## Problem

Resources (agents, commands, workflows, skills, actions) from multiple sources
are dumped into a flat namespace. Name collisions are handled by first-wins
(agents) or hard error (workflows). There is no way to:

- Override an embedded resource with a local one
- Reference a specific resource when multiple sources define the same name
- Compose resources from multiple origins without collision anxiety
- Alias or rewire resource references in app configuration

## Design

### Canonical ID

Every resource gets a canonical ID encoding its origin:

```
<origin>:<namespace>:<kind>:<name>
```

Segments:

| Segment | Examples | Source |
|---|---|---|
| origin | `agentsdk`, `local`, `github`, `user` | Derived from SourceRef ecosystem/scope |
| namespace | `engineer`, `my-app`, `user/repo:skills` | Derived from app name, project dir, repo path |
| kind | `command`, `agent`, `workflow`, `skill`, `action` | Resource type |
| name | `commit`, `main`, `deploy` | Resource name |

Examples:

```
agentsdk:engineer:command:commit       ← embedded in engineer app
local:my-app:command:commit            ← .agents/commands/commit.md in project
github:acme/tools:command:lint         ← remote repo contribution
user:global:agent:reviewer             ← ~/.agents/agents/reviewer.md
```

### Resolution

User-facing references omit `kind` (it's implied by context — `/commit` is
always a command, agent selection is always an agent).

Resolution path: **suffix match** on `origin:namespace:name`, walking from
right (name) to left (origin).

```
/commit                → all commands named "commit" across all origins
/engineer:commit       → commands named "commit" in namespace "engineer"
/agentsdk:engineer:commit → fully qualified
/local:commit          → commands named "commit" from local scope
```

Resolution rules:

1. Collect all resources matching the suffix
2. If exactly one → use it
3. If multiple and no scope precedence configured → error with qualified suggestions
4. If scope precedence is configured → apply precedence (see below)

### Scope Precedence

Default precedence (closest wins):

```
local > user > remote > embedded
```

When `/commit` matches both `local:my-app:command:commit` and
`agentsdk:engineer:command:commit`, the local one wins silently.

This can be overridden per-app via aliases (see below).

### Aliases

App configuration can define aliases that rewrite resolution:

```yaml
aliases:
  # Short name → canonical ID prefix
  commit: local:commit                    # always prefer local commit
  deploy: github:ops-team/deploy:deploy   # pin to specific origin
  gc: agentsdk:engineer:commit            # short alias for long path
  
  # Wildcard: all unqualified commands prefer local
  "*": local:*
```

Aliases are checked before suffix resolution. An alias maps a user-facing
name to a canonical ID prefix, bypassing the normal resolution walk.

### Override Semantics

With canonical IDs + precedence, override is natural:

- User defines `.agents/commands/commit.md` → gets ID `local:my-app:command:commit`
- Engineer app has `agentsdk:engineer:command:commit`
- `/commit` resolves to local (precedence: local > embedded)
- `/agentsdk:engineer:commit` still reaches the original
- Workflow steps can reference either by canonical ID

### Extension

A local command can reference the base command it extends:

```markdown
---
name: commit
extends: agentsdk:engineer:command:commit
---
Additional commit rules on top of the base.
```

The `extends` field is a canonical ID reference. The runtime merges the
extended resource's content/config with the local overrides. Exact merge
semantics are resource-type-specific (e.g. system prompt concatenation for
agents, step prepend/append for workflows).

## Implementation Layers

### 1. `resource.QualifiedName`

```go
type QualifiedName struct {
    Origin    string // "agentsdk", "local", "github", "user"
    Namespace string // "engineer", "my-app", "user/repo"
    Kind      string // "command", "agent", "workflow", etc.
    Name      string // "commit", "main", "deploy"
}

func (q QualifiedName) String() string // "agentsdk:engineer:command:commit"
func (q QualifiedName) MatchesSuffix(suffix string) bool
```

### 2. `resource.ContributionBundle` changes

Each contribution type gains a `QualifiedName` field alongside the existing
`Name string`. The `Name` field remains for backward compatibility; the
`QualifiedName` is populated during loading from `SourceRef` context.

### 3. `app.App` registration

Registration uses `QualifiedName` as the primary key. The existing `Name`
becomes a shortcut index for unqualified lookup. Duplicate short names are
allowed (stored as a list); resolution picks based on precedence or errors
on ambiguity.

### 4. Resolution service

```go
type Resolver struct {
    precedence []string           // ["local", "user", "remote", "embedded"]
    aliases    map[string]string   // user-facing name → canonical prefix
}

func (r *Resolver) Resolve(kind, ref string) (QualifiedName, error)
```

### 5. SourceRef → QualifiedName derivation

`SourceRef` already has `Ecosystem`, `Scope`, `Root`, `Path`. The mapping:

| SourceRef field | QualifiedName segment |
|---|---|
| Scope=embedded, app name known | origin="agentsdk", namespace=app name |
| Scope=project | origin="local", namespace=project dir basename |
| Scope=user | origin="user", namespace="global" |
| Ecosystem="github" | origin="github", namespace=repo path |

### 6. App configuration

```yaml
# In agentsdk.app.json or app config YAML
resolution:
  precedence: [local, user, remote, embedded]
  aliases:
    commit: local:commit
    deploy: github:ops-team/deploy:deploy
```

## Migration

- Existing apps with no collisions: zero change, unqualified names resolve as before
- Existing apps with collisions: behavior changes from first-wins to precedence-based
- New `QualifiedName` fields are additive; `Name` string stays for backward compat
- Aliases are opt-in configuration

## Open Questions

- Should `extends` be a first-class concept or handled per-resource-type?
- Should wildcard aliases (`"*": local:*`) be supported or is explicit-only safer?
- How does this interact with plugin-contributed resources? Plugin name as namespace?
- Should the resolution service be on `app.App` or a standalone component?
