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
    Kind      string    // "command", "agent", "workflow", "skill", "action"
    Origin    string    // opaque origin token assigned by the loader
    Namespace Namespace // origin-local namespace path
    Name      string    // leaf resource name
}

type Namespace struct {
    segments []string
}

func (n Namespace) String() string          // "engineer" or "user/repo/plugins/foo"
func NewNamespace(segments ...string) Namespace
```

`Kind` is metadata — it scopes resolution but is not part of the user-facing
address. Users never type the kind; it's inferred from context (`/commit` is
a command, `--agent main` is an agent).

The **canonical address** (what users can type to disambiguate) is:

```
<origin>:<namespace>:<name>
```

Namespace renders as `/`-joined segments. The `:` separator delimits origin,
namespace, and name. No `:` inside origin or name; `/` inside namespace
separates segments.

Examples:

```
agentsdk:engineer:commit                  ← embedded in engineer app
local:my-app:commit                       ← .agents/commands/commit.md in project
github.com:acme/tools:lint                ← remote repo contribution
github.com:user/repo/plugins/foo:fruit    ← command from a plugin in a repo
user:global:reviewer                      ← ~/.agents/agents/reviewer.md
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

`Namespace` is the origin-local path that scopes the resource. Internally
stored as `[]string` segments, rendered as `/`-joined for display. The
agentdir loader strips conventional directories (`commands/`, `agents/`,
`skills/`) — those map to `kind`, not namespace. Namespace is the path from
origin root to the agentdir root (the directory that *contains* `commands/`,
`agents/`, etc.).

| Origin | Namespace source | Example |
|---|---|---|
| `agentsdk` | App spec name | `engineer` |
| `local` | Project directory basename | `my-app` |
| `user` | Fixed | `global` |
| `github.com` | Repo path + subdir | `acme/tools` or `user/repo/plugins/foo` |

### Resource Index

Resources are indexed by name for O(1) lookup:

```go
type ResourceIndex struct {
    byName map[string][]ResourceID  // name → all resources with that name
}
```

Lookup for `/commit` (kind=command): map lookup by name `"commit"`, filter
by kind. Candidate list is small (collisions are the exception).

Lookup for `/engineer:commit`: map lookup by name `"commit"`, filter by kind,
filter where namespace suffix matches `engineer`.

Fully qualified `/agentsdk:engineer:commit`: map lookup by name, filter by
kind + origin + namespace. Direct match.

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

1. Check `resolution.aliases` → if match, rewrite ref, continue
2. Check `resolution.resolved` cache → if hit, return
3. Lookup by name in index, filter by kind
4. Filter by ref suffix match on canonical address
5. If exactly one → store in `resolution.resolved`, return
6. If multiple → apply resolver policy

`resolution.resolved` and `resolution.aliases` are separate stores:
- `aliases`: user-configured rewrites (explicit intent)
- `resolved`: cached policy decisions (can be regenerated)

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

Choosing 'r' persists the selection as an alias in `resolution.aliases`,
so the question is never asked again.

### Aliases

App configuration can define aliases that rewrite resolution:

```yaml
resolution:
  aliases:
    commit: local:commit                    # always prefer local
    deploy: github.com:ops-team/deploy:deploy
    gc: agentsdk:engineer:commit            # short alias
```

Aliases are checked first, before index lookup. An alias maps a user-facing
name to a canonical address prefix.

### Plugins and Provenance

A plugin is itself a resource with a canonical ID:

```
kind=plugin  origin=github.com  namespace=user/repo/plugins  name=foo
```

Resources loaded from within a plugin inherit the plugin's origin and get
the plugin name appended to namespace:

```
kind=command  origin=github.com  namespace=user/repo/plugins/foo  name=fruit
```

The loader returns provenance metadata alongside each resource:

```go
type LoadResult struct {
    ID       ResourceID
    ParentID *ResourceID  // e.g. the plugin that contributed this resource
    // ... resource content
}
```

The registration layer can use `ParentID` to create automatic aliases.
For example, if plugin `foo` contributes command `fruit`, the registrar
can add an alias `plugin:foo:fruit → github.com:user/repo/plugins/foo:fruit`
so that `plugin:foo:fruit` resolves without knowing the full origin path.

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

### 1. `resource.ResourceID` and `resource.Namespace`

```go
type Namespace struct {
    segments []string
}

func NewNamespace(segments ...string) Namespace
func (n Namespace) String() string           // "user/repo/plugins/foo"
func (n Namespace) Segments() []string
func (n Namespace) Last() string             // "foo"
func (n Namespace) SuffixMatch(ref []string) bool

type ResourceID struct {
    Kind      string
    Origin    string
    Namespace Namespace
    Name      string
}

func (r ResourceID) Address() string         // "github.com:user/repo/plugins/foo:fruit"
```

### 2. `resource.ResourceIndex`

```go
type ResourceIndex struct {
    byName map[string][]ResourceID
}

func (idx *ResourceIndex) Add(id ResourceID)
func (idx *ResourceIndex) Lookup(kind, name string) []ResourceID
```

### 3. `resource.Resolver`

```go
type Resolver struct {
    index    *ResourceIndex
    policy   ResolverPolicy
    aliases  map[string]string  // resolution.aliases — user-configured
    resolved map[string]string  // resolution.resolved — cached decisions
}

func (r *Resolver) Resolve(kind, ref string) (ResourceID, error)
```

### 4. `resource.ContributionBundle` changes

Each contribution type gains a `ResourceID` field alongside the existing
`Name string`. The `Name` field remains for backward compatibility; the
`ResourceID` is populated during loading from `SourceRef` context.

### 5. `app.App` registration

Registration adds to the `ResourceIndex`. Duplicate short names are allowed.
Resolution picks via policy.

### 6. SourceRef → ResourceID derivation

`SourceRef` already has `Ecosystem`, `Scope`, `Root`, `Path`. The mapping:

| SourceRef field | ResourceID field |
|---|---|
| Scope=embedded + app name | Origin=app name, Namespace from app spec |
| Scope=project | Origin="local", Namespace from dir basename |
| Scope=user | Origin="user", Namespace="global" |
| Ecosystem="github" | Origin="github.com", Namespace from repo path |

### 7. App configuration

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

- Should wildcard aliases be supported or is explicit-only safer?
- Should the resolver be on `app.App` or a standalone component injected
  via `app.Option`?
- How are persisted alias choices (from `AskPolicy`) stored? In
  `agentsdk.app.json`? In a separate `.agentsdk/resolution.yaml`?
- Should `LoadResult.ParentID` auto-generate `plugin:<name>:*` aliases,
  or should that be opt-in per plugin?
