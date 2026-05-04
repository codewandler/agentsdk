# Quickstart

Compose behavior explicitly with named plugins, open a harness session,
and send turns, commands, or workflows through that session boundary.

## Recommended shape

- Use `app.New(...)` as the composition root for agent specs, actions,
  workflows, commands, datasources, and named plugins.
- Use `harness.LoadSession(...)` when a host wants the standard app + default
  agent + default harness session wiring.
- Use `harness.Session` as the channel boundary for user turns,
  slash-command execution, and workflow dispatch.
- Use named plugins such as `plugins/localcli` when you want the local terminal
  tool bundle. There are no hidden standard tools in `app.New` or `agent.New`.
- Use `command.Tree` for broad slash-command namespaces; render
  `command.Result` at presentation boundaries.

## Minimal Go app with a named plugin

```go
package main

import (
    "context"
    "fmt"

    "github.com/codewandler/agentsdk/agent"
    "github.com/codewandler/agentsdk/app"
    "github.com/codewandler/agentsdk/plugins/localcli"
    "github.com/codewandler/agentsdk/runnertest"
)

func main() {
    application, err := app.New(
        app.WithAgentSpec(agent.Spec{
            Name:   "coder",
            System: "You are a concise coding assistant.",
        }),
        app.WithDefaultAgent("coder"),
        app.WithPlugin(localcli.New()),
    )
    if err != nil {
        panic(err)
    }

    inst, err := application.InstantiateDefaultAgent(
        // Replace this with your llmadapter-backed client in production.
        agent.WithClient(runnertest.NewClient(runnertest.TextStream("ok"))),
    )
    if err != nil {
        panic(err)
    }

    if err := inst.RunTurn(context.Background(), 1, "Say hello"); err != nil {
        panic(err)
    }
    fmt.Println("turn complete")
}
```

`localcli.New()` contributes the local terminal plugin explicitly. If you omit
that plugin, the app does not receive the local CLI default tools or planner
factory by accident.

## Harness session load

`harness.LoadSession` is the preferred host helper when you want the standard
app/default-agent/session stack without putting generic lifecycle wiring in the
terminal layer.

```go
package main

import (
    "context"

    "github.com/codewandler/agentsdk/agent"
    "github.com/codewandler/agentsdk/app"
    "github.com/codewandler/agentsdk/harness"
    "github.com/codewandler/agentsdk/runnertest"
)

func main() {
    loaded, err := harness.LoadSession(harness.SessionLoadConfig{
        App: harness.AppLoadConfig{DefaultAgent: "coder"},
        AppOptions: []app.Option{
            app.WithAgentSpec(agent.Spec{Name: "coder", System: "system"}),
        },
        AgentOptions: []agent.Option{
            agent.WithClient(runnertest.NewClient(runnertest.TextStream("ok"))),
        },
    })
    if err != nil {
        panic(err)
    }

    _, err = loaded.Session.Send(context.Background(), "hello")
    if err != nil {
        panic(err)
    }
}
```

`LoadSession` also attaches the agent command projection, so model turns can see
and use the `session_command` tool where appropriate.

## `session.Send(...)`

Use `session.Send` for channel-style input. Plain text is an agent turn;
slash-prefixed input is dispatched through the session command registry.

```go
result, err := loaded.Session.Send(ctx, "/session info")
if err != nil {
    return err
}
text, err := command.Render(result, command.DisplayTerminal)
if err != nil {
    return err
}
fmt.Println(text)
```

## `session.ExecuteCommand(...)`

Use `ExecuteCommand` when the caller already has a structured command path and
input map. This avoids reparsing slash text and is the seam used by API-like
callers.

```go
result, err := loaded.Session.ExecuteCommand(ctx,
    []string{"workflow", "show"},
    map[string]any{"name": "release_notes"},
)
```

For generic structured envelopes, use:

```go
result, err := loaded.Session.ExecuteCommandEnvelope(ctx, harness.CommandEnvelope{
    Path:  []string{"session", "info"},
    Input: map[string]any{},
})
```

Agent-facing calls must use `ExecuteAgentCommandEnvelope` or the
`session_command` tool so non-agent-callable commands are rejected.

## `session.ExecuteWorkflow(...)`

Register workflows on `app.New(...)`, then execute them through the harness
session so thread-backed sessions can record run events.

```go
application, err := app.New(
    app.WithAgentSpec(agent.Spec{Name: "coder", System: "system"}),
    app.WithDefaultAgent("coder"),
    app.WithActions(action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
        return action.Result{Data: input}
    })),
    app.WithWorkflows(workflow.Definition{
        Name:  "echo_flow",
        Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo"}}},
    }),
)
```

After loading a session:

```go
result := loaded.Session.ExecuteWorkflow(ctx, "echo_flow", "hello")
if result.Error != nil {
    return result.Error
}
```

## CLI examples

Run a resource-backed app from the current directory:

```bash
agentsdk run .
```

Disable the built-in local CLI fallback plugin:

```bash
agentsdk run --no-default-plugins .
```

Activate an explicit named plugin:

```bash
agentsdk run --plugin local_cli .
```

Inspect the current session:

```bash
agentsdk run . /session info
```

List and inspect workflows:

```bash
agentsdk run . /workflow list
agentsdk run . /workflow show release_notes
```

Start a workflow and capture its run ID:

```bash
agentsdk run --sessions-dir .agentsdk/sessions . /workflow start release_notes "v1.2.3"
```

List and inspect runs from the same persisted session:

```bash
agentsdk run --sessions-dir .agentsdk/sessions --continue . /workflow runs
agentsdk run --sessions-dir .agentsdk/sessions --continue . /workflow run run_abc123
```

## Current pre-1.0 unstable seams

These seams are intentionally still marked unstable while the architecture is
being shaped:

- `agent.Instance` is a lifecycle façade and still owns several transitional
  responsibilities around model policy, runtime construction, thread/session
  opening, context setup, skill state, usage tracking, and writer output.
- Writer output options such as `agent.WithOutput` and
  `harness.SessionLoadConfig.App.Output` are transitional. The intended
  direction is structured event/publication/displayable output, not arbitrary
  writer spillage.
- Workflow execution is currently synchronous. The run store can record and
  query runs from thread-backed sessions, but asynchronous lifecycle, queued and
  canceled states, richer metadata, pagination, and cancellation remain future
  work.
- Renderer/displayable design is still evolving. Keep `command.Result`
  structured and render it at terminal, API, TUI, or model-facing presentation
  boundaries.
- Architecture diagrams are not updated in this slice; the current text docs are
  the source of truth until the shape stabilizes further.
