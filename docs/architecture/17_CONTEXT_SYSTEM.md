# Context system follow-ups

Section 17 keeps `agentcontext.Manager` as the render/replay owner and tightens the
inspection/lifecycle seams around it. The goal is not to move context rendering
into channels or plugins; channels inspect context state and plugins contribute
providers.

## Ownership model

| Layer | Responsibility |
|-------|----------------|
| `agentcontext` | Provider interfaces, render records, diffs, replayable snapshots, cache policy metadata, and descriptors. |
| `agentcontext/contextproviders` | Built-in provider implementations for environment, time, model, tools, skills, files, git, commands, and project inventory. |
| `agent` | Builds the baseline agent-local provider set and registers providers on the runtime context manager. |
| `app` | Collects app/plugin context contributions and forwards them when an agent is instantiated. |
| `harness` | Projects session-scoped providers, such as the agent-callable command catalog, and exposes context inspection APIs. |
| `channel/*` | Presents context state over protocol-specific surfaces without rendering or mutating providers. |

`agentcontext.Manager` remains the single render/replay model. It owns provider
ordering, duplicate-key validation, diff calculation, render records, and the
machine-readable `StateSnapshot` used by harness/channel inspection.

## Provider lifecycle

Context providers now fall into explicit lifecycle categories:

- **app-level providers** come from `app.WithAgentOptions(...)` or app/plugin
  configuration and are registered for every instantiated agent;
- **plugin app-scoped providers** implement `app.ContextProvidersPlugin` and must
  be stateless/config-only because the plugin instance is registered once;
- **plugin agent-local providers** implement `app.AgentContextPlugin` and are
  created during agent instantiation after skill/tool state is available;
- **session projection providers** are attached by `harness.Session`, for example
  the `agent_command_catalog` provider used by the `session_command` tool;
- **agent-local baseline providers** are built by `agent.Instance` for current
  environment, time, model, tools, skills, and instruction files.

This keeps app/plugin contribution separate from session projection. A plugin can
contribute context, but a harness session decides which session-scoped providers
are projected into an agent.

## Agent façade responsibility

`agent.Instance` still assembles the baseline provider list because it already
owns the active tool set, skill state, model identity, workspace, and instruction
paths. This section does not introduce a new context composition service because
that would add indirection without deleting duplicated code. The narrower seam is
inspection: `agent.Instance` exposes descriptors and snapshots from the runtime
instead of forcing callers to parse human text.

## Inspection APIs

New inspection surfaces:

- `agentcontext.ProviderDescriptor` and `agentcontext.DescribedProvider` publish
  side-effect-free provider metadata;
- `agentcontext.Manager.Descriptors()` lists registered providers without
  rendering them;
- `agentcontext.Manager.StateSnapshot()` returns provider descriptors plus the
  last committed render records in a JSON-friendly shape;
- runtime, agent, and harness wrappers expose those snapshots without moving
  ownership out of the manager;
- `/context` now returns a structured display payload; terminal mode remains
  human-readable, JSON/machine channels can consume the payload;
- HTTP exposes `GET /api/agentsdk/v1/sessions/{session}/context`.

The snapshot includes rendered fragment content because that content is already
model-visible. Channel adapters should redact or authorize the endpoint before
exposing it to untrusted clients.

## Cache policy conventions

Provider descriptors carry the default cache policy when it is known. Built-in
providers use explicit cache scopes:

- environment and instruction-file context are stable thread-scoped context;
- time and git/project inventory are turn-scoped or time-bucketed context;
- model/tool context is turn-scoped because active model/tool state can change;
- skill inventory is thread-scoped and changes through activation state.

The tests cover descriptor export, state snapshots, HTTP context inspection, and
existing manager replay/diff behavior.
