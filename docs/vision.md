# Vision

agentsdk is a Go SDK for building secure, observable, and deployable agentic applications.

The project should make agent apps boring to build: explicit composition, durable sessions, typed execution, visible safety policy, reproducible context, reliable workflows, and channel-neutral hosting.

## Product promise

A developer describes an agentic use case and agentsdk provides the reusable infrastructure to turn it into a working app:

- agent/resource definitions;
- tools, actions, commands, workflows, and triggers;
- durable thread/session state;
- safety/risk policy and approval seams;
- terminal, daemon, and HTTP/SSE hosting paths;
- examples and first-party dogfood apps;
- a builder app that can scaffold the same primitives.

## Product roles

agentsdk has three related roles:

1. **Runtime** — model turns, tools, context, capabilities, skills, events, usage, compaction, safety, and persistence.
2. **Development kit** — Go APIs, resource formats, plugins, examples, and app composition for building agentic apps.
3. **Builder** — an agent-powered product surface (`agentsdk build`) that helps create resources, Go extensions, tests, and deployment assets.

## Evolution thesis

Do not build a second runtime. Evolve the existing one:

```text
runtime + runner + app/resource/plugin system + thread log
  -> clearer harness/session/channel boundaries
  -> workflow/action/trigger orchestration
  -> builder-generated apps that use the same primitives
```

The current code already contains the important foundations: runtime, runner, thread persistence, app/resource/plugin composition, terminal execution, tools, skills, capabilities, context providers, commands, workflows, daemon mode, triggers, safety primitives, and dogfood apps.

## North star

A good agentsdk application has:

- declarative resources where they reduce boilerplate;
- Go-code extension points where behavior is real code;
- typed actions as the execution primitive;
- tools as LLM-facing projections;
- commands as user/channel projections;
- workflows as reliable orchestration;
- explicit side-effect policy and approval gates;
- durable thread/session state;
- observable events and usage;
- multiple channels over the same harness/session model;
- triggers for scheduled or event-driven background work;
- packaging/deployment paths suitable for real services.

## What agentsdk is

- A reusable Go SDK for agent runtimes and apps.
- A resource/app format for declarative agents, commands, workflows, triggers, skills, and manifests.
- A harness/session model for running turns, commands, workflows, events, and persistence.
- A CLI product surface for running, discovering, serving, and building apps.

## What agentsdk is not

- Not a monolithic hosted SaaS product.
- Not a hidden bundle of default tools.
- Not a generic compatibility layer for old internal APIs.
- Not a place for product-specific integrations inside core runtime packages.

Integrations should be adapters, plugins, resources, examples, first-party apps, or generated app code unless they are genuinely universal primitives.

## Product surfaces

| Surface | Purpose |
| --- | --- |
| `agentsdk run` | Run a resource or Go-composed app in the terminal. |
| `agentsdk discover` | Inspect resources and app manifests without running an agent. |
| `agentsdk serve` | Run a daemon/service-style harness host. |
| `agentsdk build` | Use the builder dogfood app to scaffold/refine agent apps. |
| `harness.Service` / `harness.Session` | Programmatic host/session APIs for turns, commands, workflows, events, and persistence. |
| `channel/httpapi` | Native HTTP/SSE adapter over harness/session APIs. |

## Core concepts

| Concept | Role |
| --- | --- |
| Agent | Configured actor with instructions, model policy, tools/actions, skills, capabilities, context, and persistent session state. |
| App | Composition root for agents, plugins, commands, workflows, tools, actions, skills, context providers, and resources. |
| Resource bundle | Declarative contribution set loaded from `.agents`, manifests, plugin roots, or embedded filesystems. |
| Plugin | Named Go-code contribution bundle. Plugins are app-level composition, not a second session/channel plugin system. |
| Action | Surface-neutral typed execution primitive. |
| Tool | LLM-facing projection of executable capability. |
| Command | User/channel-facing invocation surface. |
| Workflow | Reliable orchestration of actions with metadata, policy, events, and run state. |
| Trigger | Time/event source that starts or resumes work through harness/session APIs. |
| Channel | Human/system ingress and event egress around harness sessions. |
| Thread | Durable append-only session event foundation. |
| Capability | Attachable session feature with lifecycle, context, and optional event-sourced state. |
| Skill | Instruction/reference resource with activation state and context projection. |
| Datasource | Deferred data-boundary abstraction accessed by actions; do not expand until agent/session ownership cleanup is proven. |

## Design principles

- Prefer explicit named composition over hidden defaults.
- Prefer deleting stale paths over preserving compatibility shims; there are no external legacy users yet.
- Keep host/channel policy out of lower runtime packages.
- Keep review/improvement backlog in one place: [`architecture/99_REVIEW_AND_IMPROVEMENTS.md`](architecture/99_REVIEW_AND_IMPROVEMENTS.md).
- Keep temporary planning outside publishable docs under `.agents/`.
