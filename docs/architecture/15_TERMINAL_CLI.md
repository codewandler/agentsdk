# Terminal CLI polish

Section 15 keeps the terminal package as a channel and presentation boundary. It
should adapt resources, app composition, harness sessions, and command results to
CLI behavior without becoming the canonical runtime owner.

## Boundary

`terminal/cli` owns CLI policy:

- one-shot versus interactive mode selection;
- terminal help text and grouped flag presentation;
- local terminal fallback plugin policy;
- resource-path argument handling;
- command-line flag parsing;
- terminal rendering and warning output.

It should not own reusable execution semantics:

- `app` composes reusable app definitions and registries;
- `harness.Session` executes session-bound commands, workflows, and agent turns;
- `daemon` owns long-running service/process conventions;
- `command` owns descriptors, policies, schemas, and result rendering contracts.

## One-shot and interactive modes

`agentsdk run [path] [task]` has two modes:

- if `[task]` is non-empty, it is sent once through `harness.Session.Send(...)`;
- if `[task]` is empty, the terminal REPL opens over the loaded session.

Slash commands use the same session execution path in both modes. The terminal
host renders returned `command.Result` values in one-shot mode and the REPL
renders command/turn output interactively.

Examples:

```bash
agentsdk run .
agentsdk run . /session info
agentsdk run . /workflow list
agentsdk run . /workflow start session_summary hello
```

## Plugin flags

The terminal host may activate a named local fallback plugin when resources do
not provide an agent. This is CLI host policy, not `app.New` default behavior.

```bash
agentsdk run . --plugin local_cli
agentsdk run . --plugin git --plugin skill
agentsdk run . --no-default-plugins
```

Conventions:

- `--plugin <name>` activates a named app plugin through the configured
  `app.PluginFactory`; it can be repeated.
- `--no-default-plugins` disables only the terminal host's built-in `local_cli`
  fallback policy.
- `--no-default-plugins` does not disable manifest plugin refs or explicit
  `--plugin` refs.
- Unknown plugin names should fail during load with a plugin-resolution error.

App manifests can declare plugin refs too:

```json
{
  "sources": [".agents"],
  "plugins": [
    "local_cli",
    {"name": "git", "config": {"mode": "read_only"}}
  ]
}
```

Manifest plugin refs are resource/app configuration. CLI `--plugin` refs are
operator overrides. Both flow through the same plugin factory path.

## Model/source API policy flags

The terminal command exposes model/source policy flags as channel policy knobs:

```bash
agentsdk run . --source-api openai.responses
agentsdk run . --model gpt-4.1
agentsdk run . --model-use-case agentic_coding --model-approved-only
agentsdk models --source-api anthropic.messages --thinking
```

Conventions:

- `--source-api` selects the model provider API compatibility path.
- `--model-use-case` and `--model-approved-only` constrain compatibility
  selection when configured.
- `agentsdk models` is the inspection surface for source API and model policy
  decisions; normal `agentsdk run` should only apply them.

## Debug and risk presentation

Debug-message output and risk-log presentation remain terminal concerns for now.
The terminal host currently wires log-only tool risk middleware and writes risk
observations to stderr so the TUI/REPL output remains readable. Do not move this
into `app.Plugin` or `harness.Session` until the safety policy model is designed.

## Command help and inspect surfaces

There are two command concepts exposed in terminal UX:

- command catalog descriptors from executable command trees;
- structured command resources from `.agents/commands/*.yaml`.

Executable command descriptors power `/help`, command catalog context for agents,
`session_command`, and future channel/API exports. Structured command resources
are declarative metadata; they become executable only when a harness/session
projection binds their target.

Current conventions:

```bash
agentsdk run . /help
agentsdk run . /workflow list
agentsdk discover .
```

`agentsdk discover` is the debugging surface for resource/app manifests. It
should distinguish:

- Markdown/app commands (`Commands`);
- structured command resources (`Structured commands`);
- workflows, actions, triggers, datasources, plugin refs, skills, and diagnostics.

The next high-value CLI improvement is executable structured command resources:

```text
resource.CommandContribution -> harness command projection -> Session target execution
```

That should remain a session projection rather than a new terminal runtime.

## Workflow UX

The CLI already renders async workflow starts, workflow run listings, run detail,
workflow events, reruns, and cancellation through structured command payloads.
Further workflow polish should happen only when dogfood finds concrete gaps.

Examples:

```bash
agentsdk run . /workflow start nightly_check --async
agentsdk run . /workflow runs
agentsdk run . /workflow run <run-id>
agentsdk run . /workflow events <run-id>
agentsdk run . /workflow cancel <run-id>
```

## Section 15 decisions

- Keep `terminal/cli.Load` as the shared CLI prelude unless another extraction
  deletes duplicated code.
- Keep local CLI fallback policy in terminal.
- Keep debug/risk presentation in terminal until the safety model is designed.
- Do not make terminal slash parsing the canonical API for future HTTP/SSE
  channels; use `harness.Session.ExecuteCommand` and command descriptors instead.
- Use "structured command resources" for resource YAML commands and "command
  descriptors" for executable catalog metadata. Avoid reviving the removed
  `command-descriptors/` resource directory concept.

## Host boundary review

The boundary review in [`29_HARNESS_CHANNEL_BOUNDARY.md`](29_HARNESS_CHANNEL_BOUNDARY.md) keeps terminal as a host/channel package but flags its direct `agent`, `runner`, and `tool` dependencies as cleanup candidates. Terminal should keep CLI policy, local fallback plugin policy, slash parsing, and rendering; live runtime/session state should move behind harness/session APIs as those APIs become sufficient.
