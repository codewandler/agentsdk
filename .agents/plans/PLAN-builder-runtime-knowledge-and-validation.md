# Builder Refinement Plan — Runtime Knowledge and Validation

## Status: in progress

## What we already did (committed)

- **Full tool access**: Builder plugin now registers filesystem, shell, git, vision, skills, planner in the catalog. The builder agent can run `bash`, read/write/edit files, use git, search the web, and activate tools at runtime.
- **Enriched manifest**: `default_agent: "builder"`, `include_global_user_resources: true`.
- **Rewritten agent prompt**: Tool guidance, SDK format reference inline, "explore first, run things" approach.
- **Real skill references**: All six placeholder stubs replaced with actual content.
- **Planner capability**: Builder can create structured plans for multi-step work.

## What's still wrong — the builder doesn't understand agentsdk

Evidence from the babelforce/agency session:

1. Generated a manifest with no `sources`, no `default_agent`, no `discovery` config.
2. Created an agent (`troubleshooter`) with no YAML frontmatter — no tools, no capabilities.
3. Wrote "load skills by default" as prose instead of using `skills:` in frontmatter.
4. Never set `include_global_user_resources: true` — global skills at `~/.claude/skills/` invisible to the runtime.
5. Created local copies of global skills (twice, across two sessions).
6. Left scaffold junk (`echo` action, `verify` workflow) in the final output.
7. Used prompt-only Markdown commands instead of structured YAML commands.

**Root cause**: The builder has format-level knowledge ("here's what YAML keys exist") but no behavior-level knowledge ("here's what the runtime does with them").

## What needs to happen next

### 1. Teach the builder how agentsdk actually works — a runtime-behavior skill

Not more format examples. Actual runtime semantics:

- **Discovery chain**: manifest → `sources` → directory scan → resource loading → bundle. Global user resources only participate when `include_global_user_resources: true`. Without `sources`, discovery falls back to scanning `.agents`/`.claude` — which silently works but means the manifest is broken.
- **Agent instantiation**: frontmatter is parsed → `tools:` patterns select from catalog → `skills:` pre-activates skills → `capabilities:` attaches capabilities → context providers injected. No frontmatter = no tools, no skills, no capabilities.
- **Skill lifecycle**: discovery finds them (local + global if enabled) → `skills:` in frontmatter pre-activates → `skill` tool allows runtime activation → activated content injected into context. Don't recreate global skills locally.
- **Command/workflow composition**: structured YAML commands target workflows/actions/prompts → workflows chain action steps → actions are the execution unit. Prompt-only `.md` commands work but can't be composed into workflows or triggered.

### 2. Teach the builder to validate its own work using the CLI

The builder has `bash`. `agentsdk discover --local --json .` already exists and returns rich structured output: agents, commands, skills, workflows, actions, triggers, diagnostics, capabilities, sources, manifest details.

The builder should:

- Run `agentsdk discover --local --json .` after every scaffold/write as a validation step.
- Read the diagnostics and self-correct.
- Use the JSON output to verify: does the manifest have sources? Does each agent have frontmatter with tools? Are skills discoverable? Are there diagnostics?

This is better than building new custom validation tools because:

- It's always in sync with actual runtime behavior (it *is* the runtime).
- It's more dogfood-correct — if the builder finds `discover` output lacking, that's a signal to improve the CLI.
- No parallel tooling to maintain.

### 3. Clean up the existing custom builder tools

`builder_discover_target` and `builder_run_target_smoke` are Go-level reimplementations that miss details the CLI reports. Options:

- Keep them as quick convenience wrappers but teach the builder to prefer `agentsdk discover` for detailed diagnostics.
- Or enrich them to report manifest details, global skill availability, agent frontmatter completeness.

### 4. Improve scaffold quality

`builder_scaffold_resource_app` currently produces a minimal skeleton with a broken manifest (no `default_agent`, no `discovery`) and placeholder junk (`echo` action, `verify` workflow). The scaffold should produce a valid, functional starting point that passes `agentsdk discover` with zero diagnostics.

### 5. Builder eval harness — automated output quality testing

We need a repeatable way to give the builder a task, let it build, and then evaluate the output. This validates that builder improvements (skills, prompt, scaffold) actually produce better apps.

#### Approach: task-driven eval with `agentsdk discover` as the judge

**Eval structure:**

```text
apps/builder/eval/
  cases/
    babelforce-agency.yaml      # eval case definition
    minimal-resource-app.yaml
    cli-wrapper-agent.yaml
  runner.go                     # or shell script
  judge.go                      # discovery-based evaluation
```

**Eval case definition:**

```yaml
name: babelforce-agency
description: Build a babelforce agency app with Slack awareness using global dex/babelforce skills
task: |
  Create a babelforce agency app. It needs two agents: a main communications
  awareness agent and a troubleshooter for operational diagnostics. Both agents
  should use the global skills 'babelforce' and 'dex' from ~/.claude/skills/.
  The main agent should have commands for daily briefing, my mentions, and
  incident radar using read-only dex slack commands.
expect:
  manifest:
    has_sources: true
    has_default_agent: true
    global_user_resources: true
  agents:
    - name: main
      has_frontmatter: true
      has_tools: true
      has_skills: [babelforce, dex]
      has_capabilities: [planner]
    - name: troubleshooter
      has_frontmatter: true
      has_tools: true
  commands_min: 3
  diagnostics_max: 0
  no_placeholder_actions: true    # no leftover echo/scaffold junk
  no_local_skill_copies:          # skills that must NOT exist locally
    - babelforce
    - dex
```

**Eval flow:**

1. Create a temp directory.
2. Run `agentsdk build` in one-shot/task mode with the eval case task text.
3. Wait for the builder to finish (max step limit or turn limit).
4. Run `agentsdk discover --local --json .` on the output directory.
5. Evaluate the discovery JSON against the `expect` assertions.
6. Optionally: parse individual agent `.md` files to check frontmatter completeness.
7. Report pass/fail per assertion, overall score.

**Key design decisions:**

- **`agentsdk discover` is the primary judge.** It's the source of truth for whether the app is valid. If discover reports zero diagnostics and the expected agents/commands/skills are present, the app works.
- **Determinism is not the goal.** The builder is an LLM — outputs will vary. The eval checks structural correctness (valid manifest, frontmatter present, skills discoverable) not exact file content.
- **Eval cases are additive.** Start with the babelforce-agency case (we have ground truth from the real session). Add more cases as we find new failure modes.
- **Run after every builder change.** The eval suite becomes the builder's own CI gate.

**Implementation options:**

- **Option A: Go test.** Each eval case is a `TestBuilderEval_*` function that runs the builder in one-shot mode via `cli.Run`, then runs discovery on the output. Fits existing test infrastructure.
- **Option B: Shell script.** Simpler, uses `agentsdk build` and `agentsdk discover` as black boxes. Easier to run manually but harder to assert on structured JSON.
- **Option C: Hybrid.** Shell script drives the builder, Go test evaluates the output. Separates generation from evaluation.

Recommendation: **Option A (Go test)** for CI integration, with the eval case YAML files as test fixtures. The builder's `cli.Run` with `Task` field already supports one-shot mode.

## Session references

- Builder sessions: `~/babelforce/agency/.agentsdk/builder/sessions/`
  - `hwfvAPQa.jsonl` — first session (pre-tool-access fix)
  - `2ZsvFYAv.jsonl` — second session (post-tool-access fix, Slack exploration)
- Target output: `~/babelforce/agency/`
