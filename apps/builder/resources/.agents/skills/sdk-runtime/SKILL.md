---
name: sdk-runtime
description: How the agentsdk runtime actually works — discovery, agent instantiation, skills, commands, and validation
---
# agentsdk runtime behavior

This skill teaches how agentsdk processes resources at runtime. Understanding
these mechanics prevents structural mistakes that silently break apps.

## Discovery chain

When `agentsdk run <dir>` starts:

1. Look for `agentsdk.app.yaml`, `agentsdk.app.yml`, or `agentsdk.app.json` in the directory.
2. If found, load appconfig `sources` entries and default local `.agents/` and
   `.claude/` directories.
3. If `discovery.include_global_user_resources` is `true`, also scan
   `~/.agents/skills/` and `~/.claude/skills/` for global skills.
4. All discovered resources are merged into a single ContributionBundle.

**Critical**: without `include_global_user_resources: true`, global skills are invisible.

## Agent instantiation

When an agent is instantiated from a spec (`.agents/agents/<name>.md`):

1. The file is parsed for YAML frontmatter between `---` delimiters.
2. **No frontmatter = no configuration.** The agent gets only a name (from
   filename) and system prompt (the Markdown body). No tools, no skills, no
   capabilities, no commands.
3. From frontmatter:
   - `tools:` patterns select tools from the registered catalog. Patterns
     support globs (`file_*`, `builder_*`). If `tools:` is omitted entirely,
     the agent gets default tools. If present but empty, it gets none.
   - `skills:` pre-activates named skills at session start. The skills must
     be discoverable (local or global).
   - `capabilities:` attaches runtime capabilities (e.g. `planner`).
   - `commands:` lists which commands this agent exposes.
   - `max-steps:` limits the agent's turn count (default 30).
4. The system prompt is the Markdown content after the frontmatter.

**Every agent file must have YAML frontmatter** with at least `name:`,
`description:`, and `tools:`.

## Skill lifecycle

Skills are directories containing `SKILL.md` with optional `references/*.md`.

1. **Discovery**: skills are found in local `.agents/skills/` directories
   and optionally in `~/.agents/skills/` and
   `~/.claude/skills/` (when `include_global_user_resources: true`).
2. **Pre-activation**: `skills:` in agent frontmatter activates skills at
   session start. The skill content is injected into the agent context.
3. **Runtime activation**: the `skill` tool allows activating additional
   skills and their references during a session.
4. **References**: `references/*.md` files provide deep-dive content that
   can be activated separately via the `skill` tool.

**Never recreate global skills locally.** If a skill exists at
`~/.claude/skills/dex`, reference it via `skills: [dex]` in the agent
frontmatter and `include_global_user_resources: true` in appconfig.
Creating a local copy at `.agents/skills/dex` shadows the global one and
causes confusion.

## Command composition

There are two command formats:

- **Prompt commands** (`.md`): inject prompt text into the conversation.
  Simple but not composable — they can't be chained, triggered, or targeted
  by workflows.
- **Structured commands** (`.yaml`): target a workflow, action, or prompt.
  Composable, triggerable, and can accept input schemas.

Prefer structured YAML commands when the command has a clear execution
pattern. Use prompt commands only for open-ended guidance.

## Workflow and action model

- **Actions** (`.agents/actions/*.yaml`): declarative metadata for execution
  units. Actions can be host-provided (Go-native) or declared as metadata.
- **Workflows** (`.agents/workflows/*.yaml`): chain action steps with
  dependencies, input mapping, retry, and error policies.
- **Commands → Workflows**: structured commands can target workflows,
  creating a user-facing entry point for automated processes.

## Validation

After writing or modifying any project files, always validate:

1. Run `builder_validate_target` to check structural correctness.
2. Fix all errors before continuing.
3. Pay attention to warnings — they indicate missing configuration that
   will cause runtime problems (e.g. global skills available but not included).

Common validation errors and their fixes:

| Error | Fix |
|-------|-----|
| appconfig has no `default_agent` | Add `"default_agent": "<name>"` to appconfig |
| agent has no YAML frontmatter | Add `---` delimited frontmatter with name, description, tools |
| agent has no tools: field | Add `tools:` with appropriate tool patterns |
| skill exists globally but not enabled | Add `"discovery": {"include_global_user_resources": true}` to appconfig |
| skill not discoverable | Check spelling, verify skill directory exists locally or globally |
| workflow step references undeclared action | Add action YAML to `.agents/actions/` or fix the reference |
