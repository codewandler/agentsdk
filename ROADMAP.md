# Roadmap

## Plugin Architecture

The plugin architecture is implemented. First-party plugins bundle tools,
context providers, commands, and skill sources behind the `app.Plugin`
interface.

- `app/plugin.go` — `Plugin`, `ContextProvidersPlugin`, `AgentContextPlugin`,
  and other facet interfaces.
- `plugins/gitplugin` — git tools + git context provider.
- `plugins/skillplugin` — skill tool + skill source discovery + skill inventory
  context provider.
- `plugins/toolmgmtplugin` — tool management tools + active-tools context
  provider.
- `plugins/standard` — pre-assembled plugin sets (`DefaultPlugins`,
  `Plugins(Options)`).

Follow-up work:

- Migrate `examples/devops-cli` to use `plugins/standard.DefaultPlugins()`.
- Plugin lifecycle hooks (init, shutdown) if needed.
- Remote/external plugin support.

## Skills

The phase-1 runtime skill feature is complete. The next follow-up work for skills is:

- **Human-facing reference activation**
  - Add a CLI command for activating exact reference paths under `references/`.
  - Example direction: `/skillref <skill> <references/path.md>`.

- **Replay diagnostics improvements**
  - Expand observability around replay mismatches.
  - Consider surfacing warnings through additional diagnostics sinks beyond `/skills`.

- **Recommendation layer before auto-activation**
  - Use `when`, `triggers`, and `WhenEntry.Refs` metadata to suggest relevant skills/references.
  - Keep operator/model control rather than silently auto-loading by default.

- **Unload / deactivate support**
  - Add explicit deactivation actions for dynamically activated skills and references.
  - Decide how baseline/spec skills should behave under deactivation requests.

- **Reference activation UX expansion**
  - Extend user-facing commands once the reference workflow settles.
  - Keep exact relative path semantics under `references/`.

- **Heuristic / metadata-driven activation (phase 2+)**
  - `WhenEntry.Refs`
  - trigger-driven reference recommendations or activation
  - detector-driven startup activation from `SkillMetadata.When` and `RefMetadata.When`

- **Remote-skill workflow integration**
  - Tie future search/install flows into runtime activation once the local activation model is stable.

- **Additional edge-case testing**
  - Missing replayed skills/references
  - nested `references/...` paths
  - malformed reference frontmatter
  - repeated resume cycles with dynamic skill/reference state
