# Roadmap

This file is the short, root-level backlog for contributors. The canonical
architecture roadmap lives in [`docs/roadmap.md`](docs/roadmap.md).

Keep this file focused on near-term, concrete follow-ups. Move broader product
or architecture sequencing into `docs/roadmap.md` so the two roadmaps do not
drift.

## Near-term cleanup

- ~~Move session/thread store ownership from `agent.Instance` to `harness.Session`~~ ✅ Done.
  - `agent` no longer imports `thread/jsonlstore`. Harness opens the store and
    passes it via `agent.WithThreadStore`. `LoadSession` routes through `OpenSession`.
  - `app.App` no longer caches live `*agent.Instance` values; `DefaultAgent()` and
    `DefaultSession()` removed. All session creation goes through `OpenSession`.
  - `terminal/ui` no longer imports `agent`; event handler factory uses
    `runner.EventHandlerContext` instead of `*agent.Instance`.

- **Continue shrinking `agent.Instance` only when a slice deletes code**
  - Candidate areas: context provider lifecycle, capability registry/session
    ownership, workflow recording.
  - Do not add new façade methods unless they remove older ownership paths.

- **Route diagnostics, usage, compaction, and notices through structured session/channel events**
  - Replace `agent.WithOutput` and terminal event handler writer paths with
    structured event publication through harness/session subscriptions.

## Terminal rendering

- **Clickable issue references**
  - Add configurable issue-link rules for terminal Markdown rendering.
  - Initial target: Jira-style references such as `#DEV-1234` should render as
    OSC8 clickable links.
  - Store rules in SDK/app configuration as pattern-to-URL-template mappings,
    for example `#DEV-1234` -> `https://jira.example.com/browse/DEV-1234`.
  - Keep URL resolution configurable; agentsdk should not hardcode a Jira host
    or project key.

## Plugin architecture follow-ups

The plugin architecture is the single contribution model. First-party plugins
bundle tools, context providers, commands, capability factories, and skill
sources behind `app.Plugin` facets. `app.PluginFactory` resolves named plugin
references with `context.Context` plus config.

Current first-party plugins include:

- `plugins/gitplugin` — git tools + git context provider.
- `plugins/skillplugin` — skill tool + skill source discovery + skill inventory
  context provider.
- `plugins/toolmgmtplugin` — tool management tools + active-tools context
  provider.
- `plugins/plannerplugin` — planner capability factory.
- `plugins/visionplugin` — vision tool for image understanding.
- `plugins/localcli` — local terminal plugin composition.

Follow-up work:

- Plugin lifecycle hooks (`init`, `shutdown`) only if a concrete plugin needs
  them.
- Remote/external plugin support later, with trust and distribution policy.
- Split `plugins/localcli` only if doing so removes use-case ambiguity rather
  than adding indirection.

## Skills

The phase-1 runtime skill feature is complete. The next follow-up work for
skills is:

- **Human-facing reference activation**
  - Add a CLI command for activating exact reference paths under `references/`.
  - Example direction: `/skillref <skill> <references/path.md>`.

- **Replay diagnostics improvements**
  - Expand observability around replay mismatches.
  - Consider surfacing warnings through additional diagnostics sinks beyond
    `/skills`.

- **Recommendation layer before auto-activation**
  - Use `when`, `triggers`, and `WhenEntry.Refs` metadata to suggest relevant
    skills/references.
  - Keep operator/model control rather than silently auto-loading by default.

- **Unload / deactivate support**
  - Add explicit deactivation actions for dynamically activated skills and
    references.
  - Decide how baseline/spec skills should behave under deactivation requests.

- **Remote-skill workflow integration**
  - Tie future search/install flows into runtime activation once the local
    activation model is stable.

- **Additional edge-case testing**
  - Missing replayed skills/references.
  - Nested `references/...` paths.
  - Malformed reference frontmatter.
  - Repeated resume cycles with dynamic skill/reference state.
