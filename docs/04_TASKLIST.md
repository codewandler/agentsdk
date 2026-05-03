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

- [ ] Audit remaining `command.Text(...)` usage.
- [ ] Classify simple message usages.
- [ ] Classify structured notice candidates.
- [ ] Classify typed payload candidates.
- [ ] Classify error candidates.
- [ ] Reduce one-off payload structs where generic payloads are enough.
- [ ] Add generic message payload only if repeated.
- [ ] Add generic validation detail payload only if repeated.
- [ ] Add generic table/list payload only if repeated.
- [ ] Keep rendering at presentation boundaries.
- [ ] Remove any remaining inline formatting from harness command handlers where practical.
- [ ] Improve JSON rendering coverage for structured command payloads.
- [ ] Add snapshot/golden tests if output stabilizes.

## 8. Workflow lifecycle

- [ ] Add asynchronous workflow start.
- [ ] Add queued run status.
- [ ] Add running run status lifecycle handling.
- [ ] Add canceled run status.
- [ ] Add workflow cancellation.
- [ ] Persist session ID metadata for workflow runs.
- [ ] Persist agent name metadata for workflow runs.
- [ ] Persist thread ID / branch ID metadata for workflow runs.
- [ ] Persist trigger/source metadata for workflow runs.
- [ ] Persist invoking command path metadata for workflow runs.
- [ ] Persist workflow input references.
- [ ] Add chronological run ordering.
- [ ] Add pagination for run lists.
- [ ] Add `/workflow rerun <id>`.
- [ ] Add `/workflow events <id>`.
- [ ] Add `/workflow cancel <id>`.
- [ ] Add richer `/workflow show` metadata.
- [ ] Add workflow input validation.
- [ ] Add workflow output validation.
- [ ] Add workflow definition hash/versioning.
- [ ] Add action identity/version metadata where needed.
- [ ] Decide whether thread-backed run storage remains enough.

## 9. Workflow execution semantics

- [ ] Keep sequential pipeline semantics stable before parallel execution.
- [ ] Improve DAG dependency resolution.
- [ ] Add fan-out/fan-in semantics.
- [ ] Add independent parallel steps.
- [ ] Add concurrency limits.
- [ ] Add retry policy.
- [ ] Add timeout policy.
- [ ] Add step-level error policy.
- [ ] Add conditional execution.
- [ ] Add structured dataflow mapping.
- [ ] Add step input templating/mapping.
- [ ] Add typed action input/output checking.
- [ ] Add redaction/external references for large outputs.
- [ ] Add resumability semantics.
- [ ] Add idempotency keys where needed.
- [ ] Add deterministic replay constraints where feasible.

## 10. Action/tool convergence

- [ ] Keep `action.Action` as typed execution primitive.
- [ ] Adapt existing tools to actions where useful.
- [ ] Add action-backed constructors for first-party tools where useful.
- [ ] Keep `tool.Tool` as LLM-facing projection.
- [ ] Avoid duplicating action concepts in tool APIs.
- [ ] Decide how far `tool.Ctx` should alias/adapt to action concepts.
- [ ] Decide how far `tool.Result` should alias/adapt to action concepts.
- [ ] Decide how far `tool.Intent` should alias/adapt to action concepts.
- [ ] Improve action intent model.
- [ ] Improve action middleware model.
- [ ] Add action result contracts.
- [ ] Add action JSON schema projection where needed.
- [ ] Add action-to-command adapters only where they reduce boilerplate.
- [ ] Add action-to-tool adapters only where they are safe and explicit.

## 11. Datasource work

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
- [ ] Add datasource discovery in `agentdir`.
- [ ] Add datasource plugin facets.
- [ ] Add filesystem corpus datasource example.
- [ ] Add git repository datasource example.
- [ ] Add web/API datasource example.
- [ ] Connect datasources cleanly to workflows via actions.

## 12. Resource/app manifests

- [ ] Extend resource bundles for workflows where still incomplete.
- [ ] Extend resource bundles for datasources.
- [ ] Extend resource bundles for actions.
- [ ] Extend resource bundles for plugin refs where still incomplete.
- [ ] Extend resource bundles for command tree descriptors if needed.
- [ ] Improve app manifest plugin config.
- [ ] Add validation for manifest plugin refs.
- [ ] Improve diagnostics for invalid resources.
- [ ] Add discover output for workflows.
- [ ] Add discover output for datasources.
- [ ] Add discover output for actions.
- [ ] Add discover output for plugins.
- [ ] Add discover output for command descriptors.
- [ ] Add resource-only app example.
- [ ] Add hybrid app example.
- [ ] Keep compatibility roots only where intentionally supported.

## 13. Plugin/contribution model

- [ ] Keep one conceptual plugin/contribution model.
- [ ] Do not add `harness.Plugin`.
- [ ] Keep session projections as projections, not plugins.
- [ ] Decide later whether app/session plugin facets should unify.
- [ ] Add app-level contribution facet only if needed.
- [ ] Add session-level contribution facet only if needed.
- [ ] Add channel contribution facet only if needed.
- [ ] Add trigger contribution facet only if needed.
- [ ] Keep first-party plugins purpose-named.
- [ ] Add new named plugins only for real use cases/environments.

## 14. Terminal CLI polish

- [ ] Keep terminal as channel/presentation/CLI-policy boundary.
- [ ] Reduce `terminal/cli.Load` further only if it deletes duplication.
- [ ] Keep local CLI default plugin policy in terminal.
- [ ] Keep debug/risk-log presentation in terminal for now.
- [ ] Add CLI docs for `--plugin`.
- [ ] Add CLI docs for `--no-default-plugins`.
- [ ] Add CLI docs for manifest plugin refs.
- [ ] Add CLI docs for source API/model policy flags.
- [ ] Improve one-shot result rendering if dogfood finds gaps.
- [ ] Improve REPL result rendering if dogfood finds gaps.
- [ ] Add command help from descriptors.
- [ ] Add discover/inspect UX for command descriptors.
- [ ] Add workflow UX polish after async lifecycle exists.

## 15. HTTP/SSE / other channels

- [ ] Define channel API over harness/session.
- [ ] Use `Session.ExecuteCommand` for command execution.
- [ ] Use structured `command.Result` rendering per channel.
- [ ] Use structured event/displayable publication model once designed.
- [ ] Add HTTP/SSE channel prototype.
- [ ] Add JSON/machine-readable command execution endpoint.
- [ ] Add session open/resume endpoints.
- [ ] Add workflow start/status endpoints.
- [ ] Add event stream endpoint.
- [ ] Avoid duplicating terminal slash parsing as canonical API.

## 16. Context system follow-ups

- [ ] Keep `agentcontext.Manager` as context render/replay model.
- [ ] Clarify app-level context provider lifecycle.
- [ ] Clarify plugin context provider lifecycle.
- [ ] Clarify session projection context provider lifecycle.
- [ ] Clarify agent-local context provider lifecycle.
- [ ] Reduce `agent.Instance` context setup responsibility if a clear seam emerges.
- [ ] Add better context state inspection.
- [ ] Add context diff/debug output for non-terminal channels.
- [ ] Add context provider descriptors if needed.
- [ ] Ensure cache policies are explicit and tested.

## 17. Capability system follow-ups

- [ ] Keep planner as dogfood capability.
- [ ] Clarify capability registry ownership.
- [ ] Keep registry explicit; no hidden planner default.
- [ ] Add capability action facet where useful.
- [ ] Add capability tool projection where safe.
- [ ] Add capability context provider facet where useful.
- [ ] Improve capability replay tests.
- [ ] Decide how capabilities attach through harness/session lifecycle.

## 18. Skill system follow-ups

- [ ] Keep skill repository construction in agent for now.
- [ ] Revisit whether skill repository/state should move outward when session lifecycle clarifies.
- [ ] Improve skill activation event persistence.
- [ ] Improve skill reference activation UX.
- [ ] Add command-tree-based skill commands if current control command code still has boilerplate.
- [ ] Add discover output for skills/references.
- [ ] Improve skill context rendering metadata.

## 19. Safety / risk policy

- [ ] Do not migrate risk logging opportunistically.
- [ ] Design safety policy model separately.
- [ ] Generalize safety beyond tool-only risk.
- [ ] Define approval gates at harness/session boundary.
- [ ] Define risk events/publications.
- [ ] Define channel-specific approval UX.
- [ ] Integrate shell/tool risk analyzer through named plugins/config.
- [ ] Add tests for approval policy enforcement.
- [ ] Add audit trail for approved/rejected operations.

## 20. Persistence / thread model

- [ ] Keep thread as durable event foundation.
- [ ] Improve workflow event persistence indexing only if needed.
- [ ] Add session/thread lifecycle APIs in harness.
- [ ] Add thread store abstraction choices beyond JSONL when needed.
- [ ] Add migration/versioning for thread event schemas.
- [ ] Add compaction event/read model improvements.
- [ ] Add usage event attribution.
- [ ] Add event replay tests for workflow runs.
- [ ] Add event replay tests for context renders.
- [ ] Add event replay tests for capabilities.
- [ ] Add event replay tests for skills.
- [ ] Add event replay tests for usage.

## 21. Compaction / memory

- [ ] Keep current compaction APIs stable enough for dogfood.
- [ ] Reduce writer output from compaction after displayable/event model exists.
- [ ] Add compaction command payload improvements if needed.
- [ ] Add better compaction policy config.
- [ ] Add tests for auto-compaction with session/thread persistence.
- [ ] Expose compaction state through session/harness if needed.

## 22. Discover / introspection

- [ ] Keep `agentsdk discover` as debugging surface.
- [ ] Add command descriptors to discover output.
- [ ] Add workflow descriptors to discover output.
- [ ] Add datasource descriptors to discover output.
- [ ] Add action descriptors to discover output.
- [ ] Add plugin refs/config to discover output.
- [ ] Add capability info if needed.
- [ ] Add machine-readable discover output.

## 23. Builder product

- [ ] Design `agentsdk build`.
- [ ] Make builder dogfood agentsdk itself.
- [ ] Builder interviews users about use cases.
- [ ] Builder generates agent specs.
- [ ] Builder generates workflows.
- [ ] Builder generates actions.
- [ ] Builder generates plugins.
- [ ] Builder generates datasources/connectors.
- [ ] Builder generates tests.
- [ ] Builder generates manifests.
- [ ] Builder generates deployment assets.
- [ ] Add resource-only app template.
- [ ] Add hybrid app template.
- [ ] Add full Go app template.
- [ ] Add deployment-ready app template.
- [ ] Add validation loop: generate, test, inspect diagnostics, refine.
- [ ] Use workflow engine for builder steps once workflow lifecycle is stronger.

## 24. Examples / dogfood apps

- [ ] Update local quickstart example.
- [ ] Add workflow example.
- [ ] Add command tree example.
- [ ] Add plugin composition example.
- [ ] Add datasource example.
- [ ] Add action/tool adapter example.
- [ ] Update `apps/engineer` to use blessed paths.
- [ ] Decide what remains in `examples/*` vs `apps/*`.
- [ ] Remove stale examples referencing old APIs.
- [ ] Ensure every example passes `go test ./...`.

## 25. Release readiness

- [ ] Decide pre-1.0 public API boundaries.
- [ ] Mark unstable packages/docs clearly.
- [ ] Remove stale changelog references to deleted APIs or add migration notes.
- [ ] Add migration guide for no standard tools.
- [ ] Add migration guide for local CLI plugin.
- [ ] Add migration guide for app vs harness responsibilities.
- [ ] Add migration guide for workflow options moved to `workflow`.
- [ ] Add migration guide for command descriptors through registry/catalog.
- [ ] Add migration guide for agent projection path.
- [ ] Add CI check for `go test ./...`.
- [ ] Add CI guard for no `tools/standard`.
- [ ] Add CI guard for no `plugins/standard`.
- [ ] Add CI guard for no ignored command results at terminal boundary if feasible.
- [ ] Tag an internal dogfood checkpoint.
- [ ] Decide external release cadence.

## 26. Usage readiness gates

- [ ] Local/internal dogfooding through `agentsdk run` is confirmed manually.
- [ ] Default internal SDK path is confirmed by one end-to-end harness/local CLI test.
- [ ] Broader pre-1.0 usage is documented with quickstart and examples.
- [ ] More stable public-facing use waits for displayable/output design, async workflow lifecycle, and better examples.

## Recommended next sequence

- [ ] Manual dogfood pass.
- [ ] Add one end-to-end local CLI/harness test.
- [ ] Add/update quickstart.
- [ ] Update examples/apps to use blessed paths.
- [ ] Resume deeper cleanup only where dogfooding exposes friction.
