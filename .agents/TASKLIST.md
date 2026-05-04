# 04 — Remaining Work Tasklist

Use this as the living checklist for the post-refactor path. Keep items checked only when the work is implemented, verified, and documented where needed.

## 0. Immediate checkpoint / dogfood readiness

- [x] Run `agentsdk run .` manually against a local app/default local CLI path.
- [x] Run `agentsdk run . /session info` and verify output renders.
- [x] Run `agentsdk run . /workflow list` and verify output renders.
- [x] Run `agentsdk run . /workflow start <name> [input]` and capture a run ID.
- [x] Run `agentsdk run . /workflow runs` and verify the started run appears.
- [x] Run `agentsdk run . /workflow run <run-id>` and verify detail output.
- [x] Verify default local CLI plugin loads.
- [x] Verify `--no-default-plugins` disables default local CLI composition.
- [x] Verify explicit plugin refs still work.
- [x] Verify `session_command` is visible to an agent turn.
- [x] Verify agent command catalog context is present in agent context.
- [x] Verify agent-callable commands work through `session_command`.
- [x] Verify non-agent-callable commands are rejected through `session_command`.
- [x] Verify app manifest plugin refs load.
- [x] Verify resource-backed agent specs load.
- [x] Verify resource-backed workflows load.
- [x] Decide whether the current refactored state is stable enough for daily dogfood use.

## 1. Documentation / user-facing readiness

- [x] Update README with the current recommended path: local CLI plugin, no hidden standard tools, harness session, command tree, workflow run UX.
- [x] Add a refactored SDK quickstart doc.
- [x] Add a minimal Go app example using `app.New(...)` and named plugins.
- [x] Add a harness session example using `harness.LoadSession(...)` or `harness.NewService(...)`.
- [x] Add a `session.Send(...)` example.
- [x] Add a `session.ExecuteCommand(...)` example.
- [x] Add a `session.ExecuteWorkflow(...)` example.
- [x] Add CLI examples for `/session info`.
- [x] Add CLI examples for `/workflow list`.
- [x] Add CLI examples for `/workflow start`.
- [x] Add CLI examples for `/workflow runs` and `/workflow run <id>`.
- [x] Document current pre-1.0 unstable seams: `agent.Instance`, writer output, synchronous workflow lifecycle, renderer/displayable design.
- [x] Update architecture diagrams if desired after the current shape stabilizes.

## 2. End-to-end tests

- [x] Add an end-to-end test for local CLI plugin + harness load.
- [x] Add an end-to-end test for one-shot terminal command rendering.
- [x] Add an end-to-end test for `/workflow start` then `/workflow runs`.
- [x] Add an end-to-end test for default command projection attachment.
- [x] Add an end-to-end test for `session_command` execution from an agent/tool path.
- [x] Add an end-to-end test for non-agent-callable command rejection.
- [x] Add an end-to-end test for app manifest plugin refs.
- [x] Add an end-to-end test for `--no-default-plugins`.
- [x] Add an end-to-end test for resource-defined workflows using explicit actions.
- [x] Add an end-to-end test for resumed sessions preserving workflow run lookup.

## 3. Agent façade cleanup

- [x] Inspect `agent.Instance` responsibilities before changing code.
- [x] Classify model routing/policy responsibility.
- [x] Classify runtime construction responsibility.
- [x] Classify session/thread setup responsibility.
- [x] Classify skill repository/state responsibility.
- [x] Classify context provider setup responsibility.
- [x] Classify capability registry/session setup responsibility.
- [x] Classify usage tracking responsibility.
- [x] Classify writer output responsibility.
- [x] Classify event handling responsibility.
- [x] Move session/thread opening behind a narrower seam only if it deletes duplicated paths.
- [x] Move context provider lifecycle behind a helper only if it reduces ownership drift.
- [x] Move capability session setup closer to runtime/thread lifecycle if a clean seam appears.
- [x] Reduce direct JSONL store knowledge inside `agent` if a better owner is identified.
- [x] Keep `agent.Instance` as a façade over clearer internals rather than replacing it abruptly.
- [x] Re-check public `agent.Instance` accessors and delete only clearly stale surfaces.

## 4. Harness/session lifecycle

- [x] Decide long-term shape of `harness.Service`.
- [x] Decide whether `harness.Session` should own more session open/resume behavior.
- [x] Decide whether `harness.Session` should own more thread lifecycle behavior.
- [x] Decide whether `harness.Session` should own more agent lifecycle behavior.
- [x] Decide whether `harness.Session` should own workflow lifecycle beyond synchronous starts.
- [x] Decide whether `harness.Session` should own channel event publication.
- [x] Add stable harness API for opening named sessions.
- [x] Add stable harness API for listing active sessions.
- [x] Add stable harness API for resuming sessions by ID/path.
- [x] Add stable harness API for closing/shutdown.
- [x] Add stable harness API for event subscriptions.
- [x] Move any remaining generic lifecycle out of terminal when it deletes duplication.

## 5. Writer/output replacement design

- [x] Design structured output/event model before replacing writer fields.
- [x] Define displayable payload shape.
- [x] Define notices publication shape.
- [x] Define command result publication shape.
- [x] Define workflow event publication shape.
- [x] Define usage record publication shape.
- [x] Define diagnostics publication shape.
- [x] Define debug event publication shape.
- [x] Define risk/safety event publication shape.
- [x] Define terminal renderer contract.
- [x] Define TUI renderer contract.
- [x] Define HTTP/SSE renderer contract.
- [x] Define JSON/machine-readable renderer contract.
- [x] Define LLM-facing summary renderer contract.
- [x] Decide whether payload `Display(mode)` remains sufficient.
- [x] Decide whether a renderer registry reduces code.
- [x] Replace `harness.SessionLoadConfig.App.Output` after design exists.
- [x] Replace `agent.WithOutput` after design exists.
- [x] Replace terminal event handler writer paths after design exists.
- [x] Replace debug-message output after design exists.
- [x] Replace auto-compaction output after design exists.
- [x] Replace usage persistence error output after design exists.
- [x] Keep risk logging out of this slice until separately designed.

## 6. Command system follow-ups

- [x] Keep all new broad commands on `command.Tree`.
- [x] Add output payload metadata to command descriptors.
- [x] Add richer command result descriptors if useful for API/LLM clients.
- [x] Decide whether current typed input binding is enough.
- [x] Add typed output binding only if it reduces boilerplate.
- [x] Improve help generation from descriptors.
- [x] Improve JSON schema generation from descriptors.
- [x] Add command descriptor export for HTTP/OpenAPI-like channels.
- [x] Add user-callable policy variant if current policy is insufficient.
- [x] Add internal/trusted policy variant if current policy is insufficient.
- [x] Add workflow-callable policy variant only if needed.
- [x] Consider richer LLM-safe schemas only if generic envelope + catalog context is insufficient.
- [x] Keep `session_command` as the preferred agent command projection.
- [x] Decide whether to remove/deprecate older `command_run`.

## 7. Command result/rendering cleanup

- [x] Audit remaining `command.Text(...)` usage.
- [x] Classify simple message usages.
- [x] Classify structured notice candidates.
- [x] Classify typed payload candidates.
- [x] Classify error candidates.
- [x] Reduce one-off payload structs where generic payloads are enough.
- [x] Add generic message payload only if repeated.
- [x] Add generic validation detail payload only if repeated.
- [x] Add generic table/list payload only if repeated.
- [x] Keep rendering at presentation boundaries.
- [x] Remove any remaining inline formatting from harness command handlers where practical.
- [x] Improve JSON rendering coverage for structured command payloads.
- [x] Add snapshot/golden tests if output stabilizes.

## 8. Workflow lifecycle

- [x] Add asynchronous workflow start.
- [x] Add queued run status.
- [x] Add running run status lifecycle handling.
- [x] Add canceled run status.
- [x] Add workflow cancellation.
- [x] Persist session ID metadata for workflow runs.
- [x] Persist agent name metadata for workflow runs.
- [x] Persist thread ID / branch ID metadata for workflow runs.
- [x] Persist trigger/source metadata for workflow runs.
- [x] Persist invoking command path metadata for workflow runs.
- [x] Persist workflow input references.
- [x] Add chronological run ordering.
- [x] Add pagination for run lists.
- [x] Add `/workflow rerun <id>`.
- [x] Add `/workflow events <id>`.
- [x] Add `/workflow cancel <id>`.
- [x] Add richer `/workflow show` metadata.
- [x] Add workflow input validation.
- [x] Add workflow output validation.
- [x] Add workflow definition hash/versioning.
- [x] Add action identity/version metadata where needed.
- [x] Decide whether thread-backed run storage remains enough.

## 9. Workflow execution semantics

- [x] Keep sequential pipeline semantics stable before parallel execution.
- [x] Improve DAG dependency resolution.
- [x] Add fan-out/fan-in semantics.
- [x] Add independent parallel steps.
- [x] Add concurrency limits.
- [x] Add retry policy.
- [x] Add timeout policy.
- [x] Add step-level error policy.
- [x] Add conditional execution.
- [x] Add structured dataflow mapping.
- [x] Add step input templating/mapping.
- [x] Add typed action input/output checking.
- [x] Add redaction/external references for large outputs.
- [x] Add resumability semantics.
- [x] Add idempotency keys where needed.
- [x] Add deterministic replay constraints where feasible.

## 10. Action/tool convergence

- [x] Keep `action.Action` as typed execution primitive.
- [x] Adapt existing tools to actions where useful.
- [x] Add action-backed constructors for first-party tools where useful.
- [x] Keep `tool.Tool` as LLM-facing projection.
- [x] Avoid duplicating action concepts in tool APIs.
- [x] Decide how far `tool.Ctx` should alias/adapt to action concepts.
- [x] Decide how far `tool.Result` should alias/adapt to action concepts.
- [x] Decide how far `tool.Intent` should alias/adapt to action concepts.
- [x] Improve action intent model.
- [x] Improve action middleware model.
- [x] Add action result contracts.
- [x] Add action JSON schema projection where needed.
- [x] Add action-to-command adapters only where they reduce boilerplate.
- [x] Add action-to-tool adapters only where they are safe and explicit.

## 11. Harness daemon/service mode

- [x] Use `agentsdk serve` as the daemon/service CLI command shape.
- [x] Define daemon as a harness deployment mode, not a separate product concept.
- [x] Add a slim daemon package wrapper above `harness.Service` for process/config/trigger ownership.
- [x] Keep `harness.Service` as the runtime/session owner; do not create a second app/runtime/plugin system.
- [x] Add stable service lifecycle API for long-running hosts.
- [x] Add session registry behavior suitable for daemon mode.
- [x] Add persisted session open/resume behavior for daemon-owned sessions.
- [x] Add graceful shutdown semantics.
- [x] Add health/status inspection API.
- [x] Add daemon/session storage path conventions.
- [x] Add daemon-readable config/resource loading conventions.
- [x] Add CLI smoke coverage for service-like harness lifecycle without starting an interactive REPL.
- [x] Document daemon/service conventions.

## 12. Triggers and scheduling

- [x] Define `trigger` package boundary.
- [x] Define trigger source interface.
- [x] Define trigger/job sink interface targeting harness sessions/workflows.
- [x] Define config model for trigger targets, session mode, interval, and input/prompt.
- [x] Define trigger event metadata: trigger ID, type, source, schedule, target session, session mode, target agent/workflow/action, input/prompt.
- [x] Implement interval trigger as first proof.
- [x] Support configurable session modes: shared, trigger-owned, ephemeral, resume-or-create.
- [x] Support scheduled agent prompt through a workflow or direct prompt target.
- [x] Support scheduled workflow start as the preferred target model.
- [x] Allow direct action target only where policy/context are explicit; prefer workflow target first.
- [x] Enforce one active run per trigger by default; skip overlapping fires with no overlap-policy config initially.
- [x] Persist trigger-caused run/session metadata.
- [x] Publish trigger/job events through session/harness subscriptions.
- [x] Add cancellation/disable semantics for running triggers/jobs.
- [x] Add daemon API for listing active triggers/jobs and inspecting last fire/error.
- [x] Add REPL commands such as `/triggers` or `/jobs` so normal `agentsdk run` can start/manage repeating work in the current harness/session.
- [x] Add CLI smoke coverage for interval-triggered prompt/workflow execution.
- [x] Document trigger/scheduling conventions.

## 13. Resource/app manifests

- [x] Extend resource bundles for workflows where still incomplete.
- [x] Extend resource bundles for actions.
- [x] Extend resource bundles for plugin refs where still incomplete.
- [x] Extend resource bundles for structured command YAML resources in `commands/`.
- [x] Improve app manifest plugin config.
- [x] Add validation for manifest plugin refs.
- [x] Improve diagnostics for invalid resources.
- [x] Add discover output for workflows.
- [x] Add discover output for actions.
- [x] Add discover output for plugins.
- [x] Add discover output for structured command resources.
- [x] Add resource-only app example.
- [x] Add hybrid app example.
- [x] Keep compatibility roots only where intentionally supported.
- [x] Leave datasource resource expansion deferred until daemon/triggers and a concrete datasource case study are ready.

## 14. Plugin/contribution model

- [x] Keep one conceptual plugin/contribution model.
- [x] Do not add `harness.Plugin`.
- [x] Keep session projections as projections, not plugins.
- [x] Decide later whether app/session plugin facets should unify.
- [x] Add app-level contribution facet only if needed.
- [x] Add session-level contribution facet only if needed.
- [x] Add channel contribution facet only if needed.
- [x] Add trigger contribution facet only if needed.
- [x] Keep first-party plugins purpose-named.
- [x] Add new named plugins only for real use cases/environments.

## 15. Terminal CLI polish

- [x] Keep terminal as channel/presentation/CLI-policy boundary.
- [x] Reduce `terminal/cli.Load` further only if it deletes duplication.
- [x] Keep local CLI default plugin policy in terminal.
- [x] Keep debug/risk-log presentation in terminal for now.
- [x] Add CLI docs for `--plugin`.
- [x] Add CLI docs for `--no-default-plugins`.
- [x] Add CLI docs for manifest plugin refs.
- [x] Add CLI docs for source API/model policy flags.
- [x] Improve one-shot result rendering if dogfood finds gaps.
- [x] Improve REPL result rendering if dogfood finds gaps.
- [x] Add command help from executable command catalog descriptors.
- [x] Add discover/inspect UX for structured command resources and executable command descriptors.
- [x] Add workflow UX polish after async lifecycle exists.

## 16. HTTP/SSE / other channels

- [x] Define channel API over harness/session.
- [x] Use `Session.ExecuteCommand` for command execution.
- [x] Use structured `command.Result` rendering per channel.
- [x] Use structured event/displayable publication model once designed.
- [x] Add HTTP/SSE channel prototype.
- [x] Add JSON/machine-readable command execution endpoint.
- [x] Add session open/resume endpoints.
- [x] Add workflow start/status endpoints.
- [x] Add event stream endpoint.
- [x] Avoid duplicating terminal slash parsing as canonical API.

## 17. Context system follow-ups

- [x] Keep `agentcontext.Manager` as context render/replay model.
- [x] Clarify app-level context provider lifecycle.
- [x] Clarify plugin context provider lifecycle.
- [x] Clarify session projection context provider lifecycle.
- [x] Clarify agent-local context provider lifecycle.
- [x] Reduce `agent.Instance` context setup responsibility if a clear seam emerges.
- [x] Add better context state inspection.
- [x] Add context diff/debug output for non-terminal channels.
- [x] Add context provider descriptors if needed.
- [x] Ensure cache policies are explicit and tested.

## 18. Capability system follow-ups

- [x] Keep planner as dogfood capability.
- [x] Clarify capability registry ownership.
- [x] Keep registry explicit; no hidden planner default.
- [x] Add capability action facet where useful.
- [x] Add capability tool projection where safe.
- [x] Add capability context provider facet where useful.
- [x] Improve capability replay tests.
- [x] Decide how capabilities attach through harness/session lifecycle.

## 19. Skill system follow-ups

- [x] Keep skill repository construction in agent for now.
- [x] Revisit whether skill repository/state should move outward when session lifecycle clarifies.
- [x] Improve skill activation event persistence.
- [x] Improve skill reference activation UX.
- [x] Add command-tree-based skill commands if current control command code still has boilerplate.
- [x] Add discover output for skills/references.
- [x] Improve skill context rendering metadata.

## 20. Safety / risk policy

- [x] Do not migrate risk logging opportunistically.
- [x] Design safety policy model separately.
- [x] Generalize safety beyond tool-only risk.
- [x] Define approval gates at harness/session boundary.
- [x] Define risk events/publications.
- [x] Define channel-specific approval UX.
- [x] Integrate shell/tool risk analyzer through named plugins/config.
- [x] Add tests for approval policy enforcement.
- [x] Add audit trail for approved/rejected operations.

## 21. Persistence / thread model

- [x] Keep thread as durable event foundation.
- [x] Improve workflow event persistence indexing only if needed.
- [x] Add session/thread lifecycle APIs in harness.
- [x] Add thread store abstraction choices beyond JSONL when needed.
- [x] Add migration/versioning for thread event schemas.
- [x] Add compaction event/read model improvements.
- [x] Add usage event attribution.
- [x] Add event replay tests for workflow runs.
- [x] Add event replay tests for context renders.
- [x] Add event replay tests for capabilities.
- [x] Add event replay tests for skills.
- [x] Add event replay tests for usage.

## 22. Compaction / memory

- [x] Keep current `/compact`, `agent.Compact`, and runtime compaction APIs stable enough for dogfood.
- [x] Keep auto-compaction enabled by default for normal agent sessions.
- [x] Add an explicit opt-out path for auto-compaction.
- [x] Replace absolute auto-compaction token threshold configuration with context-window percentage configuration.
- [x] Default auto-compaction to trigger at 85% of the resolved model context window.
- [x] Source max context window from resolved model metadata/modeldb when available.
- [x] Add documented fallback behavior when model context window metadata is unavailable.
- [x] Remove or deprecate absolute `TokenThreshold` as a public configuration path.
- [x] Clamp/validate percentage configuration so invalid values cannot silently disable protection.
- [x] Preserve compaction floor behavior and resume behavior after compaction.
- [x] Improve compaction result payloads with reason, trigger, threshold percentage, context window, estimated tokens, replaced count, saved tokens, summary, and compaction node ID.
- [x] Expose current compaction policy/state through agent/session/harness inspection surfaces.
- [x] Publish compaction lifecycle events for started, summary delta, summary completed, committed, skipped, and failed.
- [x] Stream generated compaction summary deltas to terminal users while `/compact` or auto-compaction is running.
- [x] Bridge compaction lifecycle/summary events into `harness.Session.Subscribe` for HTTP/SSE and future channels.
- [x] Keep the final committed `conversation.compaction` event as the authoritative persisted summary.
- [x] Do not persist every live summary delta by default; persist the final summary once.
- [x] Add visible terminal output showing why compaction happened and what summary will be used going forward.
- [x] Make auto-compaction output clearly distinct from normal assistant output.
- [x] Record compaction summary generation usage as compaction usage, not as a normal assistant turn.
- [x] Add request/provider visibility for compaction summary generation where existing observers support it.
- [x] Add tests for default-enabled auto-compaction at 85% of model context window.
- [x] Add tests for explicit opt-out.
- [x] Add tests that percentage config is used instead of absolute thresholds.
- [x] Add tests for fallback behavior when context window metadata is unavailable.
- [x] Add tests for auto-compaction with session/thread persistence and resume.
- [x] Add tests for compaction lifecycle events and final persisted summary.
- [x] Add tests for live summary streaming to harness/session subscribers.
- [x] Add or update CLI/terminal smoke coverage for visible manual and auto compaction output.
- [x] Document compaction policy, percentage configuration, opt-out, event visibility, streaming, and persistence conventions.

## 23. Discover / introspection

- [x] Keep `agentsdk discover` as debugging surface.
- [x] Add command descriptors to discover output.
- [x] Add workflow descriptors to discover output.
- [x] Add datasource descriptors to discover output.
- [x] Add action descriptors to discover output.
- [x] Add plugin refs/config to discover output.
- [x] Add capability info if needed.
- [x] Add machine-readable discover output.

## 24. Builder product

- [x] Redesign builder as first-party dogfood app under `apps/builder`.
- [x] Add importable builder app package with embedded builder resources.
- [x] Add builder `agentsdk.app.json` and `.agents/` resource tree.
- [x] Add main builder agent initialized from builder resources, not cwd resources.
- [x] Make `agentsdk build` launch the embedded builder app from the current working directory.
- [x] Keep current working directory as project-under-construction context.
- [x] Add builder context provider exposing project dir, builder sessions dir, and target sessions dir.
- [x] Add builder session storage conventions under `.agentsdk/builder/sessions`.
- [x] Add isolated target test session storage under `.agentsdk/builder/target-sessions`.
- [x] Add markdown skills for requirements, app architecture, SDK conventions, scaffolding, testing, and deployment guidance.
- [x] Add constrained builder helper actions/tools for target project inspection, target discovery, scoped scaffolding, scoped file writes, and verification.
- [x] Add target-app tester helper that loads the cwd app as isolated system under test.
- [x] Keep builder session/runtime separate from target app test sessions.
- [x] Add declarative workflows for new app scaffolding, requirements refinement, app verification, and target-agent smoke testing.
- [x] Add declarative command resources that point users toward builder workflows.
- [x] Keep product routing inside the builder app, not CLI flags/subcommands.
- [x] Ensure `agentsdk discover apps/builder/resources` and `agentsdk discover --json apps/builder/resources` expose builder resources.
- [x] Add smoke coverage for builder resource discovery, `agentsdk build` command wiring, and target-app tester initialization.
- [x] Document builder architecture, cwd/project context, tester role, safety boundaries, and dogfood conventions.

## 25. Examples / dogfood apps

- [x] Update local quickstart example.
- [x] Add workflow example.
- [x] Add command tree example.
- [x] Add plugin composition example.
- [x] Add datasource example.
- [x] Add action/tool adapter example.
- [x] Update `apps/engineer` to use blessed paths.
- [x] Decide what remains in `examples/*` vs `apps/*`.
- [x] Remove stale examples referencing old APIs.
- [x] Ensure every example passes `go test ./...`.

## 26. Release readiness

- [x] Decide pre-1.0 public API boundaries.
- [x] Mark unstable packages/docs clearly.
- [x] Remove stale changelog references to deleted APIs or add migration notes.
- [x] Add migration guide for no standard tools.
- [x] Add migration guide for local CLI plugin.
- [x] Add migration guide for app vs harness responsibilities.
- [x] Add migration guide for workflow options moved to `workflow`.
- [x] Add migration guide for command descriptors through registry/catalog.
- [x] Add migration guide for agent projection path.
- [x] Add CI check for `go test ./...`.
- [x] Add CI guard for no `tools/standard`.
- [x] Add CI guard for no `plugins/standard`.
- [x] Add CI guard for no ignored command results at terminal boundary if feasible.
- [x] Document the internal dogfood checkpoint tag command.
- [x] Decide external release cadence.

## 27. Usage readiness gates

- [x] Local/internal dogfooding through `agentsdk run` is confirmed manually.
- [x] Default internal SDK path is confirmed by one end-to-end harness/local CLI test.
- [x] Broader pre-1.0 usage is documented with quickstart and examples.
- [x] More stable public-facing use waits for displayable/output design, async workflow lifecycle, and better examples.

## 28. Architecture docs and agent cleanup checkpoint

- [x] Add `docs/README.md` as the docs index.
- [x] Move detailed architecture aspect docs under `docs/architecture/`.
- [x] Keep vision and roadmap top-level in `docs/`.
- [x] Add `docs/architecture/overview.md` as the architecture overview.
- [x] Review and document the remaining `agent` package / `agent.Instance` architecture problem.
- [x] Keep datasource work postponed until core ownership cleanup is clearer.

## 29. Architecture review and improvement consolidation

- [x] Keep architecture docs focused on current infrastructure, rules, and desired ownership.
- [x] Keep package boundary rules in one package-boundary document.
- [x] Consolidate review notes and improvement backlog into one `99_REVIEW_AND_IMPROVEMENTS.md` file.
- [x] Remove separate per-area review files from the architecture index.
- [x] Preserve package-boundary analysis findings in the consolidated review/improvement file.
- [x] Keep this batch docs/review-only with no code changes.
- [x] Keep datasource work postponed until core ownership cleanup is clearer.

## 30. Datasource work — deferred

- [ ] Revisit datasource work after daemon/service mode, trigger scheduling, and agent ownership cleanup are proven.
- [ ] Pick one concrete datasource case study before expanding abstractions.
- [ ] Flesh out datasource resource format.
- [ ] Add datasource config schemas.
- [ ] Add datasource record/item schemas.
- [ ] Add datasource sync semantics.
- [ ] Add datasource checkpoint semantics.
- [ ] Add standard datasource action refs: fetch.
- [ ] Add standard datasource action refs: list.
- [ ] Add standard datasource action refs: search.
- [ ] Add standard datasource action refs: sync.
- [ ] Add standard datasource action refs: map.
- [ ] Add standard datasource action refs: transform.
- [ ] Add datasource event model.
- [ ] Add datasource thread/checkpoint persistence where useful.
- [ ] Add datasource discovery in `agentdir` only where current discovery is insufficient.
- [ ] Add datasource plugin facets only if current plugin facets are insufficient.
- [ ] Add filesystem corpus datasource example.
- [ ] Add git repository datasource example.
- [ ] Add web/API datasource example.
- [ ] Connect datasources cleanly to workflows via actions.

## Recommended next sequence

- [ ] Manual dogfood pass.
- [ ] Add one end-to-end local CLI/harness test.
- [ ] Add/update quickstart.
- [ ] Update examples/apps to use blessed paths.
- [ ] Resume deeper cleanup only where dogfooding exposes friction.
