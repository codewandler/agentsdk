# agentsdk docs

This directory is the documentation map for the current pre-1.0 agentsdk architecture and dogfood checkpoint.

The project currently has one real consumer: us. These docs should describe the clean current path and the intended architecture, not preserve obsolete compatibility stories. Datasource expansion remains postponed until the existing runtime, harness, builder, and app boundaries are clean enough to justify it.

## Start here

- [`01_VISION.md`](01_VISION.md) — product direction and long-term boundaries.
- [`03_ROADMAP.md`](03_ROADMAP.md) — milestone roadmap and current sequencing.
- [`04_TASKLIST.md`](04_TASKLIST.md) — living task checklist and completed batches.
- [`05_QUICKSTART.md`](05_QUICKSTART.md) — current app/harness/CLI quickstart.
- [`architecture/01_OVERVIEW.md`](architecture/01_OVERVIEW.md) — architecture overview and subsystem ownership map.

## Architecture aspects

- [`architecture/02_AGENT_INSTANCE.md`](architecture/02_AGENT_INSTANCE.md) — `agent` package / `agent.Instance` responsibilities, current problems, and cleanup path.
- [`architecture/03_HARNESS_SESSION_LIFECYCLE.md`](architecture/03_HARNESS_SESSION_LIFECYCLE.md) — harness service/session lifecycle APIs and event subscriptions.
- [`architecture/04_OUTPUT_EVENT_MODEL.md`](architecture/04_OUTPUT_EVENT_MODEL.md) — structured output/event/displayable model and renderer contracts.
- [`architecture/05_COMMAND_SYSTEM.md`](architecture/05_COMMAND_SYSTEM.md) — command descriptors, schema/export, policy, and session command projection.
- [`architecture/06_COMMAND_RENDERING.md`](architecture/06_COMMAND_RENDERING.md) — command result/rendering cleanup and payload conventions.
- [`architecture/07_WORKFLOW_LIFECYCLE.md`](architecture/07_WORKFLOW_LIFECYCLE.md) — workflow run lifecycle, metadata, validation, and command UX.
- [`architecture/08_WORKFLOW_EXECUTION.md`](architecture/08_WORKFLOW_EXECUTION.md) — workflow DAG, concurrency, retry/timeout/error policy, mapping, and replay semantics.
- [`architecture/09_ACTION_TOOL_CONVERGENCE.md`](architecture/09_ACTION_TOOL_CONVERGENCE.md) — `action.Action` and `tool.Tool` boundaries and adapters.
- [`architecture/10_DAEMON_TRIGGER_SCHEDULING.md`](architecture/10_DAEMON_TRIGGER_SCHEDULING.md) — daemon/trigger scheduling design checkpoint.
- [`architecture/11_DAEMON_SERVICE_MODE.md`](architecture/11_DAEMON_SERVICE_MODE.md) — `agentsdk serve`, daemon lifecycle, status, and storage conventions.
- [`architecture/12_TRIGGERS_SCHEDULING.md`](architecture/12_TRIGGERS_SCHEDULING.md) — event-source/matcher/executor trigger model.
- [`architecture/13_RESOURCE_APP_MANIFESTS.md`](architecture/13_RESOURCE_APP_MANIFESTS.md) — resource bundle/app manifest conventions.
- [`architecture/14_PLUGIN_CONTRIBUTION_MODEL.md`](architecture/14_PLUGIN_CONTRIBUTION_MODEL.md) — plugin/contribution boundaries and session projection rules.
- [`architecture/15_TERMINAL_CLI.md`](architecture/15_TERMINAL_CLI.md) — terminal CLI host/channel conventions.
- [`architecture/16_HTTP_SSE_AGUI.md`](architecture/16_HTTP_SSE_AGUI.md) — native HTTP/SSE channel and AG-UI compatibility boundary.
- [`architecture/17_CONTEXT_SYSTEM.md`](architecture/17_CONTEXT_SYSTEM.md) — context provider lifecycle and inspection surfaces.
- [`architecture/18_CAPABILITY_SYSTEM.md`](architecture/18_CAPABILITY_SYSTEM.md) — capability registry ownership and projection facets.
- [`architecture/19_SKILL_SYSTEM.md`](architecture/19_SKILL_SYSTEM.md) — skill ownership, activation, references, and context metadata.
- [`architecture/20_SAFETY_RISK_POLICY.md`](architecture/20_SAFETY_RISK_POLICY.md) — safety/risk ownership, approval boundary, audit trail, and tool bridge.
- [`architecture/21_PERSISTENCE_THREAD_MODEL.md`](architecture/21_PERSISTENCE_THREAD_MODEL.md) — durable thread model, schema versions, replay, and inspection.
- [`architecture/22_COMPACTION_MEMORY.md`](architecture/22_COMPACTION_MEMORY.md) — default-on compaction policy, visibility, streaming, and persistence.
- [`architecture/23_DISCOVER_INTROSPECTION.md`](architecture/23_DISCOVER_INTROSPECTION.md) — discover output and machine-readable introspection.
- [`architecture/24_BUILDER_PRODUCT.md`](architecture/24_BUILDER_PRODUCT.md) — first-party builder dogfood app and `agentsdk build`.
- [`architecture/25_RELEASE_READINESS.md`](architecture/25_RELEASE_READINESS.md) — internal checkpoint boundaries and CI gates.
- [`architecture/26_USAGE_READINESS_GATES.md`](architecture/26_USAGE_READINESS_GATES.md) — internal dogfood readiness decision and deferred public-facing gates.
- [`architecture/27_PACKAGE_BOUNDARY_ANALYSIS.md`](architecture/27_PACKAGE_BOUNDARY_ANALYSIS.md) — package-level import graph review, intended dependency layers, boundary findings, and cleanup candidates.
- [`architecture/28_APP_RESOURCE_PLUGIN_BOUNDARY.md`](architecture/28_APP_RESOURCE_PLUGIN_BOUNDARY.md) — app/resource/plugin composition boundary review and cleanup candidates.
- [`architecture/29_HARNESS_CHANNEL_BOUNDARY.md`](architecture/29_HARNESS_CHANNEL_BOUNDARY.md) — harness/session/channel host boundary review and cleanup candidates.
- [`architecture/30_AGENT_RUNTIME_BOUNDARY.md`](architecture/30_AGENT_RUNTIME_BOUNDARY.md) — agent/runtime/runner boundary review and cleanup sequence.

## Reference docs

- [`COMMAND_TREE.md`](COMMAND_TREE.md) — command tree reference.
- [`RESOURCES.md`](RESOURCES.md) — `.agents`/resource layout reference and external compatibility notes.

## Current architecture review findings

The detailed aspect docs now cover the architecture in smaller files. Review from high level to low level: product/docs, app/resource/plugin composition, harness/session/channels, agent/runtime, execution primitives, persistence/state/context, and finally policy/observability/memory.

The package-level import analysis in [`architecture/27_PACKAGE_BOUNDARY_ANALYSIS.md`](architecture/27_PACKAGE_BOUNDARY_ANALYSIS.md) found no blocking low-level-to-host dependency violations. The app/resource/plugin review in [`architecture/28_APP_RESOURCE_PLUGIN_BOUNDARY.md`](architecture/28_APP_RESOURCE_PLUGIN_BOUNDARY.md) confirms the composition boundary is acceptable, with one watch item: `app.App` still caches live `agent.Instance` values. The harness/channel review in [`architecture/29_HARNESS_CHANNEL_BOUNDARY.md`](architecture/29_HARNESS_CHANNEL_BOUNDARY.md) confirms daemon and HTTP adapters are thin, while `harness` and terminal still carry lifecycle cleanup pressure. The agent/runtime review in [`architecture/30_AGENT_RUNTIME_BOUNDARY.md`](architecture/30_AGENT_RUNTIME_BOUNDARY.md) confirms `runner` and `runtime` are clean lower layers and the remaining problem is concentrated in `agent.Instance`. It still combines model policy, runtime construction, thread/session setup, skill activation, context/capability wiring, usage, compaction, and event/output plumbing.

The preferred cleanup is not a compatibility shim or a second runtime. The cleanup path is to move ownership to already-proven homes when doing so deletes coupling:

- session/thread lifecycle toward `harness.Session` / `harness.Service`;
- turn execution toward `runtime` / `runner`;
- app composition toward `app`, `resource`, and named `plugins`;
- command/workflow/trigger execution toward `harness` and `workflow`;
- output/event publication toward structured channel-facing events.

Datasource work stays deferred until this cleanup has been dogfooded further.
