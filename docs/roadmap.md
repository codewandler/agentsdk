# Roadmap

This roadmap is the public sequencing view. It intentionally omits task-level history; detailed internal planning lives under `.agents/`, and architecture cleanup backlog lives in [`architecture/99_REVIEW_AND_IMPROVEMENTS.md`](architecture/99_REVIEW_AND_IMPROVEMENTS.md).

## Guiding rules

1. Keep `go test ./...` and `./scripts/ci-check.sh` green.
2. Reuse current packages before creating parallel systems.
3. Prefer explicit named composition over hidden defaults.
4. Because the project is pre-1.0 with no external legacy users, delete stale paths instead of adding compatibility shims.
5. Let dogfood apps validate abstractions before broadening them.
6. Keep `docs/` publishable; internal tracking belongs under `.agents/`.

## Current foundation

Already present and actively dogfooded:

- `runtime` / `runner` for model-tool turns;
- `thread` / `conversation` for durable state;
- `app`, `resource`, `agentdir`, and named `plugins/*` for composition and discovery;
- `harness.Service` / `harness.Session` for live session hosting;
- terminal, daemon, and HTTP/SSE channels;
- command trees, workflow execution, triggers, skills, capabilities, context providers, compaction, usage, and safety primitives;
- examples plus first-party `apps/engineer` and `apps/builder` dogfood apps.

## Current priorities

### 1. Continue dogfood-driven ownership cleanup

`agent.Instance` has been reduced from 53 to 32 fields through successive extractions (model routing, spec embed, baseline providers, JSONL store ownership, live instance cache, output/diagnostics, config types). The remaining fields are genuinely runtime-coupled; further field extraction has diminishing returns.

Completed cleanup:

1. ~~centralize harness/session thread opening and resume behavior;~~ Done.
2. ~~move session/thread responsibility to harness-owned path;~~ Done.
3. ~~remove old paths instead of keeping compatibility fallbacks;~~ Done.
4. ~~reduce terminal direct `agent` dependencies;~~ Done: `terminal/ui` dropped `agent`; `terminal/cli` uses `agentconfig` for config types.
5. ~~move output, usage, compaction, and diagnostics toward structured session/channel events;~~ Done.
6. ~~re-check app live instance caching;~~ Done: cache removed.
7. ~~split spec/config types from agent runtime;~~ Done: `agentconfig` package.

Next candidates (diminishing returns — pick only when dogfood finds friction):

- Define a narrower harness→agent interface to reduce coupling surface.
- Make skill/capability/context activation state session-aware.
- Revisit datasource runtime expansion after concrete case study.

### 2. Keep docs publishable

The docs structure is now:

- root `README.md` for end users;
- `docs/README.md` as the published docs index;
- `docs/architecture/` as stable infrastructure/rules documentation;
- `.agents/` for temporary tasklists, readiness gates, detailed historical notes, and planning.

Do not add new review-log files under `docs/architecture/`. Update [`architecture/99_REVIEW_AND_IMPROVEMENTS.md`](architecture/99_REVIEW_AND_IMPROVEMENTS.md) instead.

### 3. Strengthen builder and examples through real use

`apps/builder` should remain the first-party product that proves agentsdk can scaffold useful apps. Improve it only through concrete dogfood loops:

- inspect an existing project;
- discover target resources;
- scaffold or update a resource app;
- run smoke checks;
- report gaps with sources and concrete commands.

Examples should stay small, current, and runnable. Durable first-party apps belong under `apps/`, not `examples/`.

### 4. Keep daemon/triggers practical

Daemon mode and triggers are near-term because they prove long-running harness/session ownership:

- interval/scheduled agent prompts;
- scheduled workflow starts;
- shared or trigger-owned sessions;
- observable trigger/run metadata;
- graceful shutdown and status inspection.

Do not turn triggers into a generic rules engine prematurely. Keep public config simple while preserving the internal event → matcher → target/executor model.

### 5. Defer datasource runtime expansion

Datasource work remains deferred. Do not expand datasource resources, sync semantics, or datasource plugin facets until at least one agent/session ownership cleanup slice has been implemented and dogfooded.

When datasource work resumes, pick one concrete case study first. Datasources should be data boundaries accessed by actions, not a second execution system.

## Completed foundation milestones

The following foundations are already in place:

- no hidden standard tool/plugin bundle;
- local CLI plugin as explicit terminal composition;
- resource-backed agents, commands, workflows, triggers, skills, and manifests;
- app/resource/plugin contribution model;
- harness service/session lifecycle APIs;
- terminal CLI, daemon mode, and HTTP/SSE channel prototype;
- command tree descriptors/results/export;
- action/tool convergence primitives;
- workflow lifecycle and execution semantics;
- trigger scheduling primitives;
- capability and skill follow-ups;
- safety/risk policy primitives;
- thread schema versioning and replay/read-model coverage;
- default-on visible compaction policy;
- discover/introspection output;
- builder dogfood app;
- examples/dogfood app refresh;
- publishable docs restructuring.

## Next concrete work

Use [`architecture/99_REVIEW_AND_IMPROVEMENTS.md`](architecture/99_REVIEW_AND_IMPROVEMENTS.md) as the active improvement backlog. The agent ownership cleanup sequence is largely complete; further improvements should be dogfood-driven.

Recommended next slices (pick when friction appears):

- Define a narrower harness→agent interface to formalize the 15+ accessor methods harness uses today.
- Make skill/capability/context activation state session-aware where dogfood finds gaps.
- Pick one concrete datasource case study when daemon/triggers are stable.

## Deferred work

- Broad datasource runtime expansion.
- Generic rules-engine trigger configuration.
- Public API stability promises.
- Compatibility layers for old internal paths.
