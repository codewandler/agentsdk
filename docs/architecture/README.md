# Architecture

This directory documents current agentsdk infrastructure, ownership rules, and desired boundaries. Review notes and improvement backlog items are consolidated in [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md).

## Read in this order

1. [`overview.md`](overview.md) — current architecture summary and ownership map.
2. [`package-boundaries.md`](package-boundaries.md) — dependency layers and hard package rules.
3. [`app-resource-plugin.md`](app-resource-plugin.md) — app composition, resources, and plugin contribution model.
4. [`harness-session-channel.md`](harness-session-channel.md) — harness sessions, terminal, and HTTP/SSE channels.
5. [`agent-runtime.md`](agent-runtime.md) — `agent.Instance`, runtime, and runner responsibilities.
6. [`execution-primitives.md`](execution-primitives.md) — actions, tools, commands, and rendering.
7. [`workflows-triggers-daemon.md`](workflows-triggers-daemon.md) — workflow lifecycle/execution, triggers, and daemon mode.
8. [`persistence-events-output.md`](persistence-events-output.md) — thread persistence, output events, context, capabilities, and skills.
9. [`safety-compaction-usage.md`](safety-compaction-usage.md) — safety policy, compaction, and usage conventions.
10. [`99_REVIEW_AND_IMPROVEMENTS.md`](99_REVIEW_AND_IMPROVEMENTS.md) — consolidated review findings and next improvements.
