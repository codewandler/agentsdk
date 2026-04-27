---
name: architecture
description: Evaluate and design software architecture with clear trade-off analysis.
---
# Architecture Skill

Use this skill when the user asks about system design, component boundaries,
data flow, or technology selection.

When designing or evaluating architecture:

1. **Start with constraints** — Scale targets, latency budgets, team size,
   deployment model, compliance requirements. If the user hasn't stated them,
   ask or make explicit assumptions.
2. **Draw component boundaries** — Identify services, modules, or layers and
   their communication contracts (sync/async, API shape, data ownership).
3. **Prefer boring patterns** — Well-understood patterns (request/response,
   pub/sub, CQRS, event sourcing) over novel ones. Every abstraction must
   justify its cost with a concrete scenario.
4. **Name trade-offs explicitly** — Consistency vs. availability, coupling vs.
   duplication, flexibility vs. complexity, build cost vs. run cost. Present
   options with pros/cons before recommending one.
5. **Separate state from logic** — Make state boundaries visible. Identify what
   is stateless, what is cached, and what is durable. Call out the consistency
   model for each stateful component.
6. **Design for observability** — Every component should expose health checks,
   structured logs, and key metrics. Specify what to alert on (symptoms, not
   causes).
7. **Mark what is deferred** — Distinguish intentional simplicity from missing
   design. List decisions that are explicitly not made yet and what would
   trigger revisiting them.

Output format for design documents:

- Problem statement and constraints
- Component diagram (ASCII or description)
- Data flow between components
- Key interfaces or contracts
- Trade-offs considered and rationale
- What is explicitly out of scope
- Risks and open questions
- Verification plan
