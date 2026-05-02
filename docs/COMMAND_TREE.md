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

Handlers receive validated structured invocation data rather than raw slash-parser leftovers:

```go
type TreeHandler func(context.Context, command.Invocation) (command.Result, error)

func workflowRunsHandler(ctx context.Context, inv command.Invocation) (command.Result, error) {
    workflowName := inv.Flag("workflow")
    status := inv.Flag("status")
    // no manual flag parsing
}
```

The tree still implements the existing flat command contract:

```go
var _ command.Command = (*command.Tree)(nil)
```

`command.Parse` remains the terminal slash tokenizer; the tree owns subcommand dispatch, declared args/flags, validation, descriptors, and generated usage.

## Typed input direction

The current tree slice stops at named invocation values. A later slice should add typed command inputs, similar to `action.NewTyped`:

```go
type WorkflowRunsInput struct {
    Workflow string             `json:"workflow,omitempty" command:"flag=workflow"`
    Status   workflow.RunStatus `json:"status,omitempty" command:"flag=status,enum=running|succeeded|failed"`
}

command.Typed("runs", func(ctx context.Context, in WorkflowRunsInput) (WorkflowRunsPayload, error) {
    // no manual flag parsing
})
```

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
    Args         []ArgDescriptor
    Flags        []FlagDescriptor
    Subcommands  []Descriptor
}

func (t *Tree) Descriptor() Descriptor
func (s *harness.Session) CommandDescriptors() []command.Descriptor
```

Example descriptor for `/workflow runs`:

```json
{
  "name": "workflow.runs",
  "path": ["workflow", "runs"],
  "description": "List workflow runs",
  "input": {
    "type": "object",
    "properties": {
      "workflow": { "type": "string" },
      "status": {
        "type": "string",
        "enum": ["running", "succeeded", "failed"]
      }
    }
  }
}
```

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
4. Typed command input binding: future work.

Recommended commit sequence:

```text
Add declarative command trees
Use command trees for harness commands
Expose command tree descriptors
Add typed command input binding
```

During migration, keep terminal behavior stable where behavior is intentional, but do not preserve dirty parsing patterns. The current `command.Parse` tokenizer remains the terminal slash syntax parser; command validation and command metadata now live in the declarative tree.
