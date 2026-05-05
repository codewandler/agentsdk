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

### Resource ID

Every resource has a structured identity:

```go
type ResourceID struct {
    Kind      string   // "command", "agent", "workflow", "skill", "action"
    Origin    string   // opaque origin token assigned by the loader
    Namespace []string // origin-local namespace path
    Name      string   // leaf resource name
}
```

`Kind` is metadata — it scopes resolution but is not part of the user-facing
address. Users never type the kind; it's inferred from context (`/commit` is
a command, `--agent main` is an agent).

The **canonical address** (what users can type to disambiguate) is:

```
<origin>:<namespace[0]>:<namespace[1]>:...:<name>
```

Examples:

```
agentsdk:engineer:commit       ← embedded in engineer app
local:my-app:commit            ← .agents/commands/commit.md in project
github.com:acme/tools:lint     ← remote repo contribution
user:global:reviewer           ← ~/.agents/agents/reviewer.md
```

### Origin

Origin is an opaque token assigned by the loader that produced the resource.
It identifies *how* the resource was loaded, not where it lives.

| Load mechanism | Origin |
|---|---|
| Embedded in app binary | app name (e.g. `agentsdk`) |
| Local filesystem `.agents/` | `local` |
| `~/.agents/` or `~/.claude/` | `user` |
| Explicit `--resource` flag or config ref | loader-assigned (e.g. `github.com`) |
| Custom protocol | loader-assigned (e.g. `tcp+myproto`) |

Origin is always a single token (no `:` inside it). The boundary between
origin and namespace is unambiguous.

### Namespace

`Namespace []string` is the origin-local path that scopes the resource.
What populates it depends on the origin:

| Origin | Namespace source | Example |
|---|---|---|
| `agentsdk` | App name from `app.Spec.Name` | `["engineer"]` |
| `local` | Project directory basename | `["my-app"]` |
| `user` | Fixed | `["global"]` |
| `github.com` | Repo path | `["acme/tools"]` |
| Plugin | Plugin name | `["gitplugin"]` |

### Resolution

Resolution is always scoped by kind (you never resolve across kinds).
Input: `(kind, ref)` where `ref` is what the user typed.

Resolution path: **suffix match** on the canonical address.

```
/commit                → all commands named "commit" across all origins
/engineer:commit       → commands where namespace+name suffix matches
/agentsdk:engineer:commit → fully qualified
/local:commit          → commands from origin "local" named "commit"
```

Resolution rules:

1. Check aliases → if match, rewrite ref, continue
2. Collect all resources of matching kind whose canonical address ends with ref
3. If exactly one → use it
4. If multiple → apply resolver policy

### Resolver Policy

Resolution of ambiguous names is controlled by a pluggable `ResolverPolicy`:

```go
type ResolverPolicy interface {
    Resolve(kind string, ref string, candidates []ResourceID) (ResourceID, error)
}
```

Built-in policies:

| Policy | Behavior |
|---|---|
| `PrecedencePolicy` | Pick by origin precedence order; default |
| `ErrorPolicy` | Always error on ambiguity with candidate list |
| `AskPolicy` | Prompt the user to choose; optionally persist choice as alias |

#### Default Precedence

The default `PrecedencePolicy` uses explicitness as the axis — more
specific/explicit wins:

```
alias > explicit-load > local > user > embedded
```

- **alias**: configured rewrite, always wins
- **explicit-load**: `--resource` flag or config file reference
- **local**: project `.agents/` directory
- **user**: `~/.agents/` or `~/.claude/`
- **embedded**: shipped with the app binary

When `/commit` matches both `local:my-app:commit` and
`agentsdk:engineer:commit`, the local one wins because `local` has higher
precedence than `embedded`. The original is still reachable via
`/agentsdk:engineer:commit`.

Precedence order is configurable per-app.

#### Ask Policy

Interactive hosts (REPL, TUI) can use `AskPolicy`:

```
? Multiple commands match "commit":
  1. local:my-app:commit
  2. agentsdk:engineer:commit
  Choose [1-2], or 'r' to remember choice: _
```

Choosing 'r' persists the selection as an alias in the app's resolution
config, so the question is never asked again.

### Aliases

App configuration can define aliases that rewrite resolution:

```yaml
resolution:
  aliases:
    commit: local:commit                    # always prefer local
    deploy: github.com:ops-team/deploy:deploy
    gc: agentsdk:engineer:commit            # short alias
```

Aliases are checked first, before suffix matching. An alias maps a
user-facing name to a canonical address prefix.

### Override

With canonical IDs + precedence, override is natural:

- User defines `.agents/commands/commit.md` → ID `local:my-app:commit`
- Engineer app has `agentsdk:engineer:commit`
- `/commit` resolves to local (precedence)
- `/agentsdk:engineer:commit` still reaches the original

### Discovery Tree

`agentsdk discover` renders the resolution as a tree grouped by origin:

```
agentsdk:engineer (embedded)
├── agents
│   └── main
├── commands
│   ├── commit
│   └── review
└── skills
    ├── architecture
    └── code-review

local:my-project (.agents)
├── agents
│   └── main  ⚠ shadows agentsdk:engineer:main
├── commands
│   └── deploy
└── skills
    └── go

user:global (~/.agents)
└── commands
    └── scratch

Resolution:
  main     → local:my-project:main (local > embedded)
  commit   → agentsdk:engineer:commit
  deploy   → local:my-project:deploy
  review   → agentsdk:engineer:review
  scratch  → user:global:scratch
```

## Implementation Layers

### 1. `resource.ResourceID`

```go
type ResourceID struct {
    Kind      string   // "command", "agent", "workflow", "skill", "action"
    Origin    string   // opaque loader token, no ":" allowed
    Namespace []string // origin-local namespace path
    Name      string   // leaf name
}

func (r ResourceID) Address() string    // "agentsdk:engineer:commit"
func (r ResourceID) MatchesSuffix(ref string) bool
```

### 2. `resource.ContributionBundle` changes

Each contribution type gains a `ResourceID` field alongside the existing
`Name string`. The `Name` field remains for backward compatibility; the
`ResourceID` is populated during loading from `SourceRef` context.

### 3. `app.App` registration

Registration uses `ResourceID` as the primary key. The existing `Name`
becomes a shortcut index for unqualified lookup. Duplicate short names are
allowed (stored as a list); resolution picks via policy.

### 4. Resolver

```go
type Resolver struct {
    policy  ResolverPolicy
    aliases map[string]string  // user-facing name → canonical address prefix
}

func (r *Resolver) Resolve(kind, ref string, all []ResourceID) (ResourceID, error)
```

Hosts inject their preferred policy. CLI uses `PrecedencePolicy` by default,
REPL can use `AskPolicy`.

### 5. SourceRef → ResourceID derivation

`SourceRef` already has `Ecosystem`, `Scope`, `Root`, `Path`. The mapping:

| SourceRef field | ResourceID field |
|---|---|
| Scope=embedded + app name | Origin=app name, Namespace=[app spec name] |
| Scope=project | Origin="local", Namespace=[dir basename] |
| Scope=user | Origin="user", Namespace=["global"] |
| Ecosystem="github" | Origin="github.com", Namespace=[repo path] |
| Plugin contribution | Origin=plugin origin, Namespace=[plugin name] |

### 6. App configuration

```yaml
resolution:
  precedence: [alias, explicit, local, user, embedded]
  aliases:
    commit: local:commit
    deploy: github.com:ops-team/deploy:deploy
```

## Migration

- Existing apps with no collisions: zero change, unqualified names resolve
  as before (single candidate, no policy needed)
- Existing apps with collisions: behavior changes from first-wins/error to
  policy-based resolution (default: precedence)
- New `ResourceID` fields are additive; `Name` string stays for compat
- Aliases are opt-in configuration
- Custom resolver policies are opt-in for library consumers

## Open Questions

- Should wildcard aliases (`"*": local:*`) be supported or is explicit-only safer?
- How does this interact with plugin-contributed resources? Plugin name as
  namespace segment, but what origin? The app that loaded the plugin?
- Should the resolver be on `app.App` or a standalone component injected
  via `app.Option`?
- How are persisted alias choices stored? In `agentsdk.app.json`? In a
  separate `.agentsdk/resolution.yaml`?
