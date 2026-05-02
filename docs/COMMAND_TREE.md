# Command Tree Direction

## Problem

The original command implementation was intentionally simple, but it was not the long-term SDK surface. Handwritten harness command namespaces such as `/workflow` hid subcommands, positional arguments, flags, and validation inside switch statements and ad hoc parsing. That creates several problems:

- subcommands are invisible to the type system;
- command help, docs, and API schemas cannot be generated reliably;
- flags and positional arguments are repeatedly hand-parsed;
- terminal slash commands, HTTP command APIs, and LLM-callable command projections would each need bespoke mapping;
- every new command namespace increases later migration cost.

The first command-tree implementation now lives in `command.Tree`, and current harness `/workflow` and `/session` commands are tree-backed. Avoid adding more broad command namespaces outside this tree model.

## Direction

Commands now have a declarative, channel-neutral command tree in the existing `command` package: similar in spirit to Cobra, but smaller and SDK-native. The command package knows about:

- root command names such as `workflow`, `session`, `agent`, or `thread`;
- nested subcommands such as `workflow runs`, `workflow show`, `agent list`;
- positional arguments;
- flags;
- enum/required/default constraints;
- structured input/output metadata;
- descriptors that can later become JSON Schema or OpenAPI-style command APIs.

Terminal slash syntax should be one input projection over this tree, not the canonical command model.

## Target shape

The current API uses a fluent builder and keeps `Tree` compatible with the existing `command.Command` interface:

```go
workflowTree, err := command.NewTree("workflow",
    command.Description("Inspect and run workflows"),
).
    Sub("list", workflowListHandler,
        command.Description("List workflows"),
    ).
    Sub("show", workflowShowHandler,
        command.Description("Show workflow"),
        command.Arg("name").Required(),
    ).
    Sub("start", workflowStartHandler,
        command.Description("Start workflow"),
        command.Arg("name").Required(),
        command.Arg("input").Variadic(),
    ).
    Sub("runs", workflowRunsHandler,
        command.Description("List workflow runs"),
        command.Flag("workflow").Describe("Workflow name"),
        command.Flag("status").Enum("running", "succeeded", "failed"),
    ).
    Build()
```

Construction errors are accumulated while building the tree and returned from `Build()`. There is no parallel legacy `AddSub`/`Spec` tree-construction API.

Handlers can receive validated typed inputs rather than raw slash-parser leftovers:

```go
type WorkflowRunsInput struct {
    Workflow string             `command:"flag=workflow"`
    Status   workflow.RunStatus `command:"flag=status"`
}

func workflowRunsHandler(ctx context.Context, input WorkflowRunsInput) (command.Result, error) {
    // input.Workflow and input.Status were bound from the validated invocation
}

workflowTree, err := command.NewTree("workflow").
    Sub("runs", command.Typed(workflowRunsHandler),
        command.Flag("workflow"),
        command.Flag("status").Enum("running", "succeeded", "failed"),
    ).
    Build()
```

Untyped `TreeHandler` functions that accept `command.Invocation` remain available for commands that need direct access to invocation metadata.

The tree still implements the existing flat command contract:

```go
var _ command.Command = (*command.Tree)(nil)
```

`command.Parse` remains the terminal slash tokenizer; the tree owns subcommand dispatch, declared args/flags, validation, descriptors, and generated usage.

## Typed input binding

`command.Typed` adapts typed handlers to tree handlers. The input type must be a struct, or a pointer to a struct, with exported fields tagged as command args or flags:

```go
type WorkflowStartInput struct {
    Name  string `command:"arg=name"`
    Input string `command:"arg=input"`
}

type WorkflowRunsInput struct {
    Workflow string             `json:"workflow,omitempty" command:"flag=workflow"`
    Status   workflow.RunStatus `json:"status,omitempty" command:"flag=status"`
}
```

Supported scalar targets currently include strings, named string types, bools, ints, uints, floats, pointers to supported scalar types, slices of supported scalar types, and `encoding.TextUnmarshaler` fields. Optional values remain zero-valued when omitted.

Typed inputs can also enrich command input descriptors with primitive type hints:

```go
command.NewTree("workflow").
    Sub("runs", command.Typed(workflowRunsHandler),
        command.TypedInput[WorkflowRunsInput](),
        command.Flag("workflow"),
        command.Flag("status").Enum("running", "succeeded", "failed"),
    ).
    Build()
```

`command.TypedInput[T]()` infers descriptor field types from `T`'s tagged fields: strings and `encoding.TextUnmarshaler` become `string`, bools become `bool`, ints/uints become `integer`, floats become `number`, and slices become `array`. Enum constraints, required flags, descriptions, and variadic behavior still belong in the command tree declaration, not in struct tags.

Commands are not a replacement for actions. The intended relationship is:

```text
Action   = executable typed unit
Command  = user/channel invocation surface over typed input
Tool     = model-callable projection over typed input
Workflow = orchestration over actions
```

Some commands may wrap actions. Other commands, such as `/session info`, are channel/session control surfaces and may not be model-callable actions.

## Descriptor and schema direction

A running harness can expose supported command shapes through descriptors:

```go
type Descriptor struct {
    Name         string
    Path         []string
    Description  string
    ArgumentHint string
    Policy       command.Policy
    Args         []ArgDescriptor
    Flags        []FlagDescriptor
    Input        InputDescriptor
    Executable   bool
    Subcommands  []Descriptor
}

type InputDescriptor struct {
    Fields []InputFieldDescriptor
}

type InputFieldDescriptor struct {
    Name        string
    Source      InputSource // "arg" or "flag"
    Type        InputType   // "string", "array", ...
    Description string
    Required    bool
    Variadic    bool
    EnumValues  []string
}

func (t *Tree) Descriptor() Descriptor
func (s *harness.Session) CommandDescriptors() []command.Descriptor
func (s *harness.Session) CommandCatalog() []harness.CommandCatalogEntry
```

Example descriptor input for `/workflow runs`:

```json
{
  "name": "runs",
  "path": ["workflow", "runs"],
  "description": "List workflow runs",
  "input": {
    "fields": [
      {
        "name": "workflow",
        "source": "flag",
        "type": "string",
        "description": "Workflow name"
      },
      {
        "name": "status",
        "source": "flag",
        "type": "string",
        "enumValues": ["running", "succeeded", "failed"]
      }
    ]
  }
}
```

Input descriptors are populated from declared `command.Arg(...)` and `command.Flag(...)` specs. That keeps the command tree declaration canonical for validation, help, typed binding, and non-terminal execution surfaces. Variadic args are exposed as `array`; scalar args and flags default to `string`, and `command.TypedInput[T]()` can enrich those defaults with typed-handler field metadata.

Descriptors also expose `executable` for nodes with handlers. Harness sessions use that metadata to provide a flattened command catalog of executable commands plus their input schemas:

```go
type CommandCatalogEntry struct {
    Descriptor  command.Descriptor
    InputSchema command.JSONSchema
}

catalog := session.CommandCatalog()
agentCatalog := session.CommandCatalog(harness.CommandCatalogAgentCallable())
userCatalog := session.CommandCatalog(harness.CommandCatalogUserCallable())
```

Namespace-only nodes such as `/workflow` are omitted from the catalog unless they become executable. Leaf commands such as `/workflow start`, `/workflow runs`, and `/session info` are included with `inputSchema` generated from their descriptors. Policy filters can project only agent-callable or user-callable commands from the same catalog model.

Structured command callers use a small generic command execution envelope instead of embedding the full command catalog into a formal tool schema:

```go
type CommandEnvelope struct {
    Path  []string       `json:"path"`
    Input map[string]any `json:"input,omitempty"`
}

schema := harness.CommandEnvelopeSchema()
catalog := session.AgentCommandCatalog()
result, err := session.ExecuteAgentCommandEnvelope(ctx, harness.CommandEnvelope{
    Path:  []string{"workflow", "show"},
    Input: map[string]any{"name": "ask_flow"},
})
```

Exact per-command schemas are provided through `AgentCommandCatalog()` as context/discovery metadata, while the command tree remains responsible for execution-time validation. This keeps the future tool schema small and avoids one tool per command while still exposing the known command input schemas to the model.

The envelope is tool/action neutral. Agent-facing adapters should call `ExecuteAgentCommandEnvelope`, which enforces `AgentCallable` policy. Trusted SDK and API callers can call `ExecuteCommandEnvelope` and apply their own policy boundary.

Harness sessions expose an LLM-facing tool adapter over the same envelope:

```go
tool := session.AgentCommandTool() // tool name: session_command
```

Default harness sessions attach this command projection automatically, so `session_command` and the command catalog context provider are available to the next agent turn without manual registration. The tool schema is the generic `CommandEnvelope` schema. Exact command paths and input shapes still come from `session.AgentCommandCatalog()` and can be rendered as model context through:

```go
context := harness.FormatAgentCommandCatalog(session.AgentCommandCatalog())
provider := session.AgentCommandCatalogContextProvider()
```

The provider emits a stable `agent_command_catalog/session_command` context fragment explaining the `session_command` tool and its currently agent-callable command paths. Tool execution calls `ExecuteAgentCommandEnvelope`, so non-agent-callable commands are rejected before command execution.

Harness sessions also expose a workflow/action adapter:

```go
action := session.CommandAction() // action name: command.execute
err := app.RegisterActions(action)
```

Workflow steps can then call commands through the same envelope:

```go
workflow.Step{
    ID:     "session_info",
    Action: workflow.ActionRef{Name: harness.CommandActionName},
    Input: map[string]any{
        "path": []any{"session", "info"},
    },
}
```

The command action uses trusted `ExecuteCommandEnvelope`; do not run untrusted workflow definitions without an outer policy boundary.

## JSON rendering

Command display payloads can be rendered as machine-readable JSON through the generic command renderer:

```go
text, err := command.Render(command.Display(tree.Descriptor()), command.DisplayJSON)
```

`DisplayJSON` uses the structured payload directly instead of terminal `Display(...)` strings, so descriptor consumers get stable JSON with lower-camel field names such as `input.fields`, `enumValues`, and `subcommands`.

## JSON Schema projection

Command input descriptors can be projected into a small JSON Schema-compatible object schema:

```go
schema := command.CommandInputSchema(tree.Descriptor().Subcommands[0])
text, err := command.Render(command.Display(schema), command.DisplayJSON)
```

Example schema for `/workflow start <name> [input]`:

```json
{
  "type": "object",
  "properties": {
    "name": {"type": "string"},
    "input": {"type": "array", "items": {"type": "string"}}
  },
  "required": ["name"]
}
```

Descriptors remain the source of truth. The schema projection intentionally models only the small subset currently needed for form/API/tool discovery surfaces: object properties, primitive types, arrays, required fields, descriptions, and enums.

This enables future surfaces from the same command model:

- terminal slash commands;
- HTTP command execution APIs;
- web forms;
- generated help/docs;
- LLM-callable command projections where explicitly allowed;
- JSON/machine-readable command invocation.

## Migration plan

Do not keep adding command namespaces with handwritten switch-based subcommand parsing. Current state:

1. Declarative command tree core in `command`: ✅
2. Existing harness command namespaces (`/workflow`, `/session`) migrated onto it: ✅
3. Command descriptors/introspection exposed through harness sessions: ✅
4. Typed command input binding: ✅
5. JSON rendering for structured command payloads/descriptors: ✅
6. Typed input descriptor type hints: ✅
7. JSON Schema projection for command inputs: ✅
8. Harness command catalog with input schemas: ✅
9. Policy-aware command catalog filters: ✅
10. Generic command execution envelope: ✅
11. Command envelope action adapter: ✅
12. Agent command envelope tool adapter: ✅
13. Agent command catalog context renderer/provider: ✅
14. Default harness-session command projection attachment: ✅

Recommended commit sequence:

```text
Add declarative command trees
Use command trees for harness commands
Expose command tree descriptors
Add typed command input binding
Add JSON command payload rendering
Infer command input descriptor types
```

During migration, keep terminal behavior stable where behavior is intentional, but do not preserve dirty parsing patterns. The current `command.Parse` tokenizer remains the terminal slash syntax parser; command validation and command metadata now live in the declarative tree.
