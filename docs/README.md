# agentsdk documentation

This is the documentation entrypoint for agentsdk. The docs directory is intended to be publishable: public guides and references live here; temporary planning and task tracking live under `.agents/`.

## Start here

- [`05_QUICKSTART.md`](05_QUICKSTART.md) — build and run the current app/harness/CLI path.
- [`../examples/README.md`](../examples/README.md) — runnable examples and dogfood app conventions.
- [`01_VISION.md`](01_VISION.md) — product direction and long-term boundaries.
- [`03_ROADMAP.md`](03_ROADMAP.md) — public roadmap and sequencing.

## Guides

- [`guides/builder.md`](guides/builder.md) — first-party builder dogfood app and `agentsdk build`.

## Reference

- [`reference/command-tree.md`](reference/command-tree.md) — command tree reference.
- [`reference/resources.md`](reference/resources.md) — `.agents` resource layout and external compatibility notes.
- [`reference/discover.md`](reference/discover.md) — discover output and machine-readable introspection.

## Architecture

Architecture docs describe current infrastructure, ownership rules, and desired boundaries. Start with [`architecture/README.md`](architecture/README.md).

Key architecture entrypoints:

- [`architecture/overview.md`](architecture/overview.md) — current architecture summary and ownership map.
- [`architecture/package-boundaries.md`](architecture/package-boundaries.md) — dependency layers and hard package rules.
- [`architecture/99_REVIEW_AND_IMPROVEMENTS.md`](architecture/99_REVIEW_AND_IMPROVEMENTS.md) — consolidated review findings and improvement backlog.

## Internal notes

Temporary task tracking, release-readiness notes, and usage-readiness gates are intentionally kept outside publishable docs under `.agents/`.

## Documentation rules

- Root `README.md` is end-user facing only: short product description, highlights, quick start, and a link here.
- `docs/` is publishable. Do not put temporary tasklists, planning logs, release gates, or agent handoff notes here.
- Internal planning and temporary tracking belong under `.agents/`.
- Architecture docs describe current infrastructure, ownership rules, and desired boundaries. They are not refactor logs.
- Review findings and improvement backlog items belong only in `architecture/99_REVIEW_AND_IMPROVEMENTS.md`.
- Prefer consolidating or renaming existing docs over adding new files. Add a new architecture doc only when it makes the index clearer.
- Keep links stable and update this index whenever docs move.
