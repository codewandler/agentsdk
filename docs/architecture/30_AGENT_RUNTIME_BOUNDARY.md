# Agent/runtime boundary review

This is a docs-only review of `agent`, `runtime`, `runner`, and `runnertest`. It follows the package boundary, app/resource/plugin, and harness/channel reviews. No code changes are part of this batch.

## Review command

```bash
go list -f '{{.ImportPath}} {{join .Imports " "}}' ./agent ./runtime ./runner ./runnertest
```

Internal imports observed:

```text
agent
  -> action
  -> agentcontext
  -> agentcontext/contextproviders
  -> capability
  -> conversation
  -> runner
  -> runtime
  -> skill
  -> thread
  -> thread/jsonlstore
  -> tool
  -> toolactivation
  -> usage

runtime
  -> action
  -> agentcontext
  -> capability
  -> conversation
  -> runner
  -> skill
  -> thread
  -> tool
  -> toolactivation

runner
  -> conversation
  -> thread
  -> tool

runnertest
  -> no internal packages
```

## Intended boundary

| Package | Job | Must not own |
| --- | --- | --- |
| `runner` | Low-level model/tool turn loop and event stream over provider clients/tools. | App composition, harness sessions, persistence store selection, terminal rendering. |
| `runtime` | Higher-level turn engine over runner, conversation/history projection, context/capability/thread runtime composition. | Product app/plugin loading, session registry, channel lifecycle. |
| `agent` | Agent spec/options normalization and current façade for model policy, runtime construction inputs, and dogfood execution APIs. | Long-term live session ownership, channel rendering, generic store/session lifecycle. |
| `runnertest` | Provider test doubles. | Production runtime behavior. |

The boundary is partly healthy: `runner` and `runtime` do not import `agent`, `app`, `harness`, terminal, daemon, or channel packages. The problem is concentrated in `agent`, which imports both lower runtime primitives and session/state/persistence concerns.

## Current state assessment

### `runner`

`runner` imports `conversation`, `thread`, and `tool`.

Assessment:

- `runner -> tool` is expected because the runner executes LLM-facing tools.
- `runner -> conversation` is expected for model-message/history projection helpers.
- `runner -> thread` is acceptable for event/session metadata types, but should remain narrow.
- `runner` does not import `agent`, `runtime`, `app`, `harness`, terminal, daemon, or channel packages. This is the strongest boundary in this layer.

Decision: `runner` is appropriately low-level. Do not move product/session behavior into it.

### `runtime`

`runtime` imports `action`, `agentcontext`, `capability`, `conversation`, `runner`, `skill`, `thread`, `tool`, and `toolactivation`.

Assessment:

- `runtime -> runner` is correct: runtime wraps the low-level turn loop.
- `runtime -> conversation` / `thread` is correct for durable thread-backed engines and history projection.
- `runtime -> agentcontext` / `capability` / `skill` is acceptable because runtime composes context/capability/session replay for turns.
- `runtime -> tool` / `toolactivation` is expected while tools remain the LLM-facing projection.
- `runtime -> action` is acceptable for action-backed turn/runtime integration.
- `runtime` does not import `agent`, `app`, `harness`, terminal, daemon, or channel packages. That validates runtime as reusable execution infrastructure.

Watch point: runtime already carries a lot of thread/context/capability composition. That is acceptable because it is still execution-infrastructure, but harness/session should own multi-session lifecycle and channel-facing event publication.

### `agent`

`agent` imports nearly every lower subsystem needed for one live agent: runtime, runner, conversation, thread/jsonlstore, context providers, capability, skill, action, tool/toolactivation, and usage.

This confirms the main architecture issue. `agent.Instance` currently combines:

- spec/options normalization;
- model/source API policy;
- runtime construction;
- session ID and store path handling;
- JSONL store selection/open/resume;
- skill repository and activation state;
- context provider composition;
- capability registry/session setup;
- usage tracking and persistence;
- compaction policy and execution;
- event/output/writer hooks;
- turn execution façade APIs.

Some of that belongs in `agent` long term, but not all of it belongs in one live `Instance` type.

Keep in `agent`:

- `agent.Spec` and agent frontmatter-compatible config shape;
- model/source API policy and normalized inference defaults;
- options that describe how to construct one agent runtime;
- action-backed agent turn helpers if they remain thin;
- stable dogfood APIs until replacement seams are proven.

Move out over time:

- session/thread store selection and open/resume lifecycle to `harness.Service` / `harness.Session`;
- live session registry/cache ownership out of `app.App` and `agent.Instance`;
- output/writer diagnostics to structured session/channel events;
- usage persistence to thread/session event handling rather than agent-owned side paths;
- session-owned skill/capability/context activation/projection state where harness can own it cleanly.

### `runnertest`

`runnertest` imports no internal packages. This is healthy. It stays as provider/client test support and should not become a testing-only runtime abstraction.

## Boundary findings

### OK / intended

- `runner` is low-level and does not import higher layers.
- `runtime` is reusable execution infrastructure and does not import `agent`, `app`, `harness`, terminal, daemon, or channel packages.
- `agent` imports lower layers rather than the reverse; the graph has no runtime/agent import cycle.
- `runnertest` remains isolated.

### Watch

- `runtime` has broad context/capability/thread composition. Keep it execution-focused; do not let it become session registry or channel host.
- `agent` still imports `thread/jsonlstore`, which hardwires store selection into the agent façade.
- `agent` still imports both `runner` and `runtime`, which is fine today but shows that it is translating options and also acting as execution façade.
- `agent` usage and output hooks still bridge model events to diagnostics/persistence directly.

### Cleanup candidates

1. **Split spec/config from live instance.** Keep `agent.Spec` and construction options in `agent`, but make live session/runtime ownership harness-owned over time.
2. **Move store selection out of `agent`.** JSONL store opening should be centralized in harness/session lifecycle when that can serve terminal, daemon, and HTTP without duplication.
3. **Split runtime construction from session ownership.** A narrower builder/helper can translate `agent.Spec` + options into runtime options without also owning session lifecycle.
4. **Route output and usage through events.** Replace writer/usage side paths with structured session/channel events and thread persistence hooks.
5. **Make skill/capability/context state session-aware.** Definitions remain app/agent metadata; live activation/projection state should be session-owned when possible.

## Proposed cleanup sequence after review batches

When docs-only reviews are complete, the first code cleanup slice should be small and high-confidence:

1. Add or identify a harness-owned session/thread opening helper that centralizes store directory, resume ID/path, and live thread inspection.
2. Move one duplicated or leaky `agent.Instance` session/thread responsibility into that helper.
3. Keep tests green with existing dogfood paths.
4. Delete the old path rather than keeping compatibility shims.

Do not begin datasource expansion before at least one such `agent.Instance` ownership slice is complete and dogfooded.

## Decision

The agent/runtime boundary has no blocking import violations: low-level runtime packages are clean. The architecture issue is `agent.Instance` breadth and its role as both configured agent façade and live runtime/session owner. That should be the first real code cleanup area after the docs/review pass sequence.
