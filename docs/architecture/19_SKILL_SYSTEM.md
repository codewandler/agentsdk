# Skill system follow-ups

Section 19 keeps skill ownership deliberately boring while tightening the runtime and CLI surfaces around it.

## Ownership boundary

Skill repository construction stays in `agent.Instance` for now:

- `app` and plugins contribute `skill.Source` values.
- `agent.New` scans those sources into a `skill.Repository` and creates session-scoped `skill.ActivationState`.
- `harness` exposes inspection and activation commands over the current session agent.
- Model tools receive both the mutable state and a session-aware activator in `tool.Ctx.Extra()`.

This avoids introducing a separate skill service before the daemon/session lifecycle has more production pressure. Revisit moving skill state outward only if multiple live agents need to share one mutable skill activation projection.

## Persisted activation

Thread-backed agents persist dynamic skill activation as thread events:

- `skill.EventSkillActivated`
- `skill.EventSkillReferenceActivated`

The model-facing `skill` tool now prefers `skill.ActivatorContextKey` when present. `agent.Instance` installs itself as that activator in the default tool context, so tool-driven skill/reference activations use `agent.ActivateSkill(...)` and `agent.ActivateSkillReferences(...)` instead of mutating `skill.ActivationState` directly. That keeps resumed sessions consistent with user-driven harness commands.

Fallback behavior remains: direct `ActivationState` mutation still works for tests or custom hosts that only provide `skill.ContextKey`.

## Harness commands

Skill commands are command-tree based:

```text
/skills
/skill activate <name>
/skill refs <name>
/skill ref <name> <path>
```

`/skills` lists discovered skills, status, source identity, active references, and replay diagnostics. `/skill refs` lists exact reference paths and trigger metadata. `/skill ref` activates one exact reference and persists it through the agent activator.

## Discover output

`agentsdk discover` now reports discovered skill references under the `Skills:` section when skill sources are available:

```text
Skills:
  go  Go skill  .agents/skills/go
  References:
    go/references/testing.md  triggers=tests
```

This is intentionally an inventory surface, not an activation surface. Runtime activation still belongs to harness/session commands and the model tool.

## Context metadata

The skill inventory context provider now includes richer metadata for each catalog entry:

- source label and source ID
- skill directory
- activation status
- domain, role, risk, compatibility
- allowed tools
- discovered reference count and exact reference paths

Active skill bodies and active reference bodies continue to be materialized only when activated.
