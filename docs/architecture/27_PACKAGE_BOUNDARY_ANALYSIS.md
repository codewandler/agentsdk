# Package boundary analysis

This is a docs-only architecture review of package-level imports. It validates whether the current implementation matches the intended high-to-low boundaries before any further cleanup work. No code changes are proposed in this batch.

Generated on 2026-05-04 with:

```bash
go list -f '{{.ImportPath}} {{join .Imports " "}}' ./...
```

Only imports within `github.com/codewandler/agentsdk` are considered for the boundary analysis.

## Summary

Current package graph size:

- Go packages: 61
- Internal direct import edges: 198

High fan-out packages:

| Internal imports | Package | Interpretation |
| ---: | --- | --- |
| 19 | `plugins/localcli` | Expected host/plugin aggregation; watch that it remains terminal-local and named. |
| 15 | `harness` | Expected live session coordinator; should own more lifecycle over time. |
| 13 | `agent` | Main architecture smell; still owns too many runtime/session/state concerns. |
| 11 | `cmd/agentsdk` | Expected product entrypoint aggregation. |
| 11 | `terminal/cli` | Expected channel host, but should not own generic lifecycle. |
| 10 | `app` | Expected composition hub. |
| 9 | `apps/builder` | Expected first-party dogfood app aggregation. |
| 9 | `runtime` | Expected runtime composition. |

High fan-in packages:

| Importers | Package | Interpretation |
| ---: | --- | --- |
| 31 | `tool` | Tool remains the broad LLM-facing projection API. |
| 16 | `action` | Action is becoming the central execution primitive. |
| 11 | `agentcontext` | Context providers are widely reused. |
| 10 | `skill` | Skill metadata/state has several consumers. |
| 9 | `thread` | Thread is the durable persistence foundation. |
| 9 | `command` | Command descriptors/results are shared by channels/harness. |
| 8 | `capability` | Capability projection/state is broadly reused. |
| 8 | `agent` | Too many packages still construct or reach through `agent.Instance`. |

## Intended dependency layers

The current intended layers, from low-level to high-level, are:

1. **Foundation:** `internal/*`, `markdown`, `websearch`, small data packages.
2. **Policy/observability:** `safety`, `usage`.
3. **Persistence:** `conversation`, `thread`, `thread/jsonlstore`.
4. **State/context:** `agentcontext`, `capability`, `capabilities/*`, `skill`.
5. **Tooling projection:** `tool`, `toolactivation`, `toolmw`, `tools/*`.
6. **Execution/orchestration primitives:** `action`, `actionmw`, `command`, `workflow`.
7. **Turn runtime:** `runtime`, `runner`.
8. **Agent façade:** `agent`.
9. **Composition:** `app`, `resource`, `agentdir`, `plugins/*`.
10. **Harness/session:** `harness`, `trigger`.
11. **Hosts/products/channels:** `cmd/agentsdk`, `terminal/*`, `daemon`, `channel/*`, `apps/*`, `examples/*`.

This is not a strict acyclic architecture yet. Some lower-level packages intentionally import concepts that are moving upward or sideways, especially while `action` is being promoted. The point of this analysis is to classify those edges explicitly.

## Layer edge observations

Most edges flow from higher-level packages down into reusable packages. The expected aggregators are visible:

- `cmd/agentsdk`, `terminal/cli`, `daemon`, `channel/httpapi`, `apps/*`, and `examples/*` import composition/harness/runtime primitives.
- `harness` imports `app`, `agent`, `command`, `workflow`, `thread`, and state/context packages to run live sessions.
- `app`, `resource`, `agentdir`, and `plugins/*` import definitions and registries rather than channels.
- `runtime` imports persistence/context/tooling because it is the turn execution composition point.

The graph validates the broad architecture direction: low-level packages do not import terminal, daemon, channel, app, harness, or cmd packages. That is the most important boundary.

## Notable edges into `agent`

Current packages importing `agent` directly:

- `agentdir`
- `app`
- `cmd/agentsdk`
- `harness`
- `plugins/localcli`
- `resource`
- `terminal/cli`
- `terminal/ui`

Classification:

| Edge | Status | Notes |
| --- | --- | --- |
| `app` → `agent` | OK for now | `app` composes `agent.Spec` and instantiates agents. Long term, keep spec/options in `agent`, not live session ownership. |
| `resource` / `agentdir` → `agent` | OK for now | Resource loading emits `agent.Spec`. Keep this declarative. Do not let resource loading depend on live `agent.Instance`. |
| `harness` → `agent` | Cleanup candidate | Harness should eventually own session lifecycle and create runtime state without treating `agent.Instance` as the session owner. |
| `terminal/cli` → `agent` | Cleanup candidate | Terminal should mostly configure host policy and call harness/session APIs; direct `agent` option wiring should shrink. |
| `terminal/ui` → `agent` | Watch | UI rendering should avoid depending on the full agent façade if it only needs display metadata. |
| `cmd/agentsdk` → `agent` | Watch | CLI entrypoint may pass model/options, but should avoid owning runtime/session details. |
| `plugins/localcli` → `agent` | Watch | Local CLI plugin needs a fallback `agent.Spec`; keep it named and terminal-local. |

## `agent` fan-out

`agent` currently imports:

- `action`
- `agentcontext`
- `agentcontext/contextproviders`
- `capability`
- `conversation`
- `runner`
- `runtime`
- `skill`
- `thread`
- `thread/jsonlstore`
- `tool`
- `toolactivation`
- `usage`

This confirms the documented problem in [`02_AGENT_INSTANCE.md`](02_AGENT_INSTANCE.md): `agent.Instance` is both a configured agent façade and a live runtime/session holder.

Classification:

| Import group | Status | Cleanup direction |
| --- | --- | --- |
| `runtime`, `runner`, `tool`, `toolactivation`, `action` | OK/watch | Agent must construct turn runtime today. Later split spec/options normalization from live runtime ownership. |
| `conversation`, `thread`, `thread/jsonlstore` | Cleanup candidate | Store selection/open/resume belongs behind harness/session lifecycle now that harness APIs exist. |
| `agentcontext`, `capability`, `skill` | Cleanup candidate | Live activation/projection state should become session-owned where possible; definitions can remain app/agent metadata. |
| `usage` | Cleanup candidate | Usage should flow through structured events and thread persistence rather than agent writer/event side paths. |

## Other suspicious or inverted edges

These are not necessarily bugs, but they should be reviewed before deeper cleanup:

| Edge | Classification | Reason |
| --- | --- | --- |
| `datasource` → `action` | Watch | Datasource is postponed. Existing action refs are acceptable metadata, but do not expand datasource abstractions until core ownership cleanup is done. |
| `safety` → `action` | OK/watch | Safety policy needs surface-neutral action metadata. Keep it primitive-level, not harness/channel-specific. |
| `usage` → `runner` / `conversation` | Cleanup candidate | Usage conversion from runner events is useful, but `usage` as policy/observability now depends upward on runtime/persistence concepts. Consider moving adapters closer to runtime or agent when cleaning usage paths. |
| `agentcontext/contextproviders` → `tool` | Watch | Some context providers summarize active tools. Acceptable today, but context provider core should not require model-tool projection long term. |
| `capability` / `capabilities/planner` → `action` / `tool` | OK/watch | Capability projection facets can expose actions/tools. Keep projection explicit and avoid hidden default tool composition. |
| `tool` → `action` and `toolmw` → `actionmw` | OK | Intentional action/tool convergence; `action` is the execution primitive, `tool` is the LLM-facing projection. |
| `tools/jsonquery` → `action` | OK | First-party action-backed tool constructor example. |

## Boundary findings

### OK / intended

- No low-level package imports `terminal`, `cmd/agentsdk`, `daemon`, `channel`, `harness`, or `app`.
- `command`, `workflow`, `action`, and `tool` remain independent of terminal/channel hosts.
- `harness` is the primary live-session aggregation point.
- `plugins/*` are named contribution bundles rather than a generic standard plugin system.
- `thread` remains a low-level persistence foundation with broad but downward-facing use.

### Watch

- `plugins/localcli` has the highest fan-out. This is acceptable only because it is an explicit named terminal-local plugin.
- `terminal/cli` still imports `agent`, `app`, `agentdir`, `harness`, and runtime-facing options. Continue moving generic lifecycle behind harness when it deletes coupling.
- Context/capability/skill packages expose useful projections, but their live mutable state should not sprawl across app, agent, and harness.

### Cleanup candidates

1. **`agent.Instance` session/thread ownership:** move JSONL store selection and open/resume details behind harness/session APIs.
2. **`agent.Instance` output/event ownership:** route diagnostics, usage, compaction, and notices through structured session/channel events.
3. **Terminal direct `agent` dependency:** reduce terminal to host policy + harness/session calls where possible.
4. **Usage adapter placement:** review whether runner/conversation adapters belong in `usage` or closer to runtime/agent event handling.
5. **Context provider tool dependency:** keep active-tool context as an adapter/provider, not a core context-system dependency.

### No blocking violations found

The package graph does not show hard architectural violations such as:

- persistence importing app/harness/terminal;
- command/workflow importing terminal/channel hosts;
- action importing tool/harness/app;
- app importing harness or terminal;
- runtime importing app/harness/terminal.

The main issue is not an import cycle or a forbidden high-level dependency. The main issue is **fan-in/fan-out concentration around `agent.Instance` and host packages**, which matches the current architecture docs.

## Next review order

Use this order for future docs/code review passes:

1. Product and docs surface: `README.md`, `docs/README.md`, vision, roadmap, tasklist.
2. App/resource/plugin composition: `app`, `resource`, `agentdir`, `plugins/*` ([`28_APP_RESOURCE_PLUGIN_BOUNDARY.md`](28_APP_RESOURCE_PLUGIN_BOUNDARY.md)).
3. Harness/session/channel hosts: `harness`, `daemon`, `channel/*`, `terminal/*`.
4. Agent/runtime boundary: `agent`, `runtime`, `runner` ([`30_AGENT_RUNTIME_BOUNDARY.md`](30_AGENT_RUNTIME_BOUNDARY.md)).
5. Execution primitives: `action`, `tool`, `command`, `workflow`.
6. Persistence/state/context: `thread`, `conversation`, `agentcontext`, `capability`, `skill`.
7. Policy/observability/memory: `safety`, `usage`, compaction paths.

Do not start datasource expansion until at least the first `agent.Instance` ownership cleanup slice has been completed and dogfooded.
