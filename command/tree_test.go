package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTreeImplementsCommandAndDispatchesSubcommands(t *testing.T) {
	var _ Command = NewTree("workflow")
	var got Invocation
	tree, err := NewTree("workflow", Description("Inspect workflows")).
		Sub("show", func(_ context.Context, inv Invocation) (Result, error) {
			got = inv
			return Text("shown " + inv.Arg("name")), nil
		}, Description("Show workflow"), Arg("name").Required()).
		Build()
	require.NoError(t, err)

	name, params, err := Parse("/workflow show ask_flow")
	require.NoError(t, err)
	require.Equal(t, "workflow", name)
	result, err := tree.Execute(context.Background(), params)

	require.NoError(t, err)
	require.Equal(t, "shown ask_flow", renderCommandResult(t, result))
	require.Equal(t, []string{"workflow", "show"}, got.Path)
	require.Equal(t, "ask_flow", got.Arg("name"))
}

func TestTreePassesVariadicArgsAndFlags(t *testing.T) {
	var got Invocation
	tree, err := NewTree("workflow").
		Sub("start", func(_ context.Context, inv Invocation) (Result, error) {
			got = inv
			return Text(inv.Arg("input")), nil
		}, Arg("name").Required(), Arg("input").Variadic(), Flag("status").Enum("running", "succeeded", "failed")).
		Build()
	require.NoError(t, err)

	_, params, err := Parse("/workflow start ask_flow hello from tree --status succeeded")
	require.NoError(t, err)
	result, err := tree.Execute(context.Background(), params)

	require.NoError(t, err)
	require.Equal(t, "hello from tree", renderCommandResult(t, result))
	require.Equal(t, "ask_flow", got.Arg("name"))
	require.Equal(t, []string{"hello", "from", "tree"}, got.ArgValues("input"))
	require.Equal(t, "succeeded", got.Flag("status"))
}

func TestTreeValidationReturnsStructuredHelpPayload(t *testing.T) {
	tree, err := NewTree("workflow").
		Sub("runs", func(context.Context, Invocation) (Result, error) {
			return Text("unreachable"), nil
		}, Flag("status").Enum("running", "succeeded", "failed")).
		Build()
	require.NoError(t, err)

	_, params, err := Parse("/workflow runs --status nope")
	require.NoError(t, err)
	result, err := tree.Execute(context.Background(), params)

	require.NoError(t, err)
	require.Equal(t, ResultDisplay, result.Kind)
	payload, ok := result.Payload.(HelpPayload)
	require.True(t, ok)
	require.NotNil(t, payload.Error)
	require.Equal(t, ValidationInvalidFlagValue, payload.Error.Code)
	require.Equal(t, "status", payload.Error.Field)
	text := renderCommandResult(t, result)
	require.Contains(t, text, "invalid value \"nope\" for --status")
	require.Contains(t, text, "usage: /workflow runs")
}

func TestTreeValidationCoversMissingArgsTooManyArgsUnknownFlagsAndSubcommands(t *testing.T) {
	tree, err := NewTree("workflow").
		Sub("show", func(context.Context, Invocation) (Result, error) {
			return Text("unreachable"), nil
		}, Arg("name").Required()).
		Build()
	require.NoError(t, err)

	cases := []struct {
		line  string
		code  ValidationErrorCode
		field string
	}{
		{line: "/workflow show", code: ValidationMissingArg, field: "name"},
		{line: "/workflow show one two", code: ValidationTooManyArgs},
		{line: "/workflow show one --unknown value", code: ValidationUnknownFlag, field: "unknown"},
		{line: "/workflow missing", code: ValidationUnknownSubcommand, field: "missing"},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			_, params, err := Parse(tc.line)
			require.NoError(t, err)
			result, err := tree.Execute(context.Background(), params)
			require.NoError(t, err)
			payload, ok := result.Payload.(HelpPayload)
			require.True(t, ok)
			require.NotNil(t, payload.Error)
			require.Equal(t, tc.code, payload.Error.Code)
			require.Equal(t, tc.field, payload.Error.Field)
		})
	}
}

func TestTreeExecuteMapUsesStructuredInputWithoutSlashStringification(t *testing.T) {
	var got Invocation
	tree, err := NewTree("workflow").
		Sub("runs", func(_ context.Context, inv Invocation) (Result, error) {
			got = inv
			return Text(inv.Flag("status") + ":" + inv.Flag("workflow")), nil
		}, Flag("workflow"), Flag("status").Enum("running", "succeeded", "failed")).
		Build()
	require.NoError(t, err)

	result, err := tree.ExecuteMap(context.Background(), []string{"workflow", "runs"}, map[string]any{
		"workflow": "ask_flow",
		"status":   "failed",
	})

	require.NoError(t, err)
	require.Equal(t, "failed:ask_flow", renderCommandResult(t, result))
	require.Equal(t, []string{"workflow", "runs"}, got.Path)
	require.Equal(t, "ask_flow", got.Flag("workflow"))
	require.Equal(t, "failed", got.Flag("status"))
}

func TestTreeDescriptorIncludesSubcommandsArgsFlagsAndEnums(t *testing.T) {
	tree, err := NewTree("workflow", Description("Inspect workflows")).
		Sub("runs", nil,
			Description("List runs"),
			Flag("workflow").Describe("Workflow name"),
			Flag("status").Enum("running", "succeeded", "failed"),
		).
		Sub("show", nil,
			Description("Show workflow"),
			Arg("name").Required().Describe("Workflow name"),
		).
		Build()
	require.NoError(t, err)

	desc := tree.Descriptor()
	require.Equal(t, "workflow", desc.Name)
	require.Equal(t, []string{"workflow"}, desc.Path)
	require.Len(t, desc.Subcommands, 2)
	require.Equal(t, "runs", desc.Subcommands[0].Name)
	require.Equal(t, []string{"workflow", "runs"}, desc.Subcommands[0].Path)
	require.Equal(t, []FlagDescriptor{
		{Name: "workflow", Description: "Workflow name"},
		{Name: "status", EnumValues: []string{"running", "succeeded", "failed"}},
	}, desc.Subcommands[0].Flags)
	require.Equal(t, "show", desc.Subcommands[1].Name)
	require.Equal(t, []ArgDescriptor{{Name: "name", Description: "Workflow name", Required: true}}, desc.Subcommands[1].Args)
}

func TestTreeDescriptorMarksExecutableNodes(t *testing.T) {
	tree, err := NewTree("workflow").
		Sub("runs", func(context.Context, Invocation) (Result, error) { return Text("runs"), nil }).
		Sub("help", nil).
		Build()
	require.NoError(t, err)

	desc := tree.Descriptor()
	require.False(t, desc.Executable)
	require.True(t, desc.Subcommands[0].Executable)
	require.False(t, desc.Subcommands[1].Executable)
}

func TestTreeDescriptorIncludesStructuredInputFields(t *testing.T) {
	tree, err := NewTree("workflow", Description("Inspect workflows")).
		Sub("start", nil,
			Description("Start workflow"),
			Arg("name").Required().Describe("Workflow name"),
			Arg("input").Variadic().Describe("Workflow input"),
			Flag("status").Required().Describe("Run status").Enum("running", "succeeded", "failed"),
		).
		Build()
	require.NoError(t, err)

	desc := tree.Descriptor().Subcommands[0]
	require.Equal(t, InputDescriptor{Fields: []InputFieldDescriptor{
		{Name: "name", Source: InputSourceArg, Type: InputTypeString, Description: "Workflow name", Required: true},
		{Name: "input", Source: InputSourceArg, Type: InputTypeArray, Description: "Workflow input", Variadic: true},
		{Name: "status", Source: InputSourceFlag, Type: InputTypeString, Description: "Run status", Required: true, EnumValues: []string{"running", "succeeded", "failed"}},
	}}, desc.Input)
}

func TestTreeDescriptorIncludesOutputMetadataAndDefensiveCopies(t *testing.T) {
	tree, err := NewTree("workflow").
		Sub("list", nil,
			Output(OutputDescriptor{
				Kind:        "workflow.list",
				Description: "Workflow list payload",
				MediaTypes:  []string{"application/json"},
				Schema: JSONSchema{Type: "object", Properties: map[string]JSONSchema{
					"definitions": {Type: "array", Items: &JSONSchema{Type: "object"}},
				}},
			}),
		).
		Build()
	require.NoError(t, err)

	desc := tree.Descriptor().Subcommands[0]
	require.Equal(t, "workflow.list", desc.Output.Kind)
	require.Equal(t, []string{"application/json"}, desc.Output.MediaTypes)
	require.Equal(t, "array", desc.Output.Schema.Properties["definitions"].Type)

	desc.Output.MediaTypes[0] = "mutated"
	desc.Output.Schema.Properties["definitions"] = JSONSchema{Type: "mutated"}
	desc = tree.Descriptor().Subcommands[0]
	require.Equal(t, []string{"application/json"}, desc.Output.MediaTypes)
	require.Equal(t, "array", desc.Output.Schema.Properties["definitions"].Type)
}

func TestTreeDescriptorInputEnumValuesAreDefensiveCopies(t *testing.T) {
	tree, err := NewTree("workflow").
		Sub("runs", nil, Flag("status").Enum("running", "succeeded", "failed")).
		Build()
	require.NoError(t, err)

	desc := tree.Descriptor()
	desc.Subcommands[0].Input.Fields[0].EnumValues[0] = "mutated"
	desc.Subcommands[0].Flags[0].EnumValues[0] = "also-mutated"

	desc = tree.Descriptor()
	require.Equal(t, "running", desc.Subcommands[0].Input.Fields[0].EnumValues[0])
	require.Equal(t, "running", desc.Subcommands[0].Flags[0].EnumValues[0])
}

func TestTreeDescriptorRendersAsJSON(t *testing.T) {
	tree, err := NewTree("workflow", Description("Inspect workflows")).
		Sub("runs", nil,
			Description("List runs"),
			Flag("status").Describe("Run status").Enum("running", "succeeded", "failed"),
		).
		Build()
	require.NoError(t, err)

	text, err := Render(Display(tree.Descriptor()), DisplayJSON)

	require.NoError(t, err)
	require.JSONEq(t, `{
		"name": "workflow",
		"path": ["workflow"],
		"description": "Inspect workflows",
		"input": {},
		"subcommands": [{
			"name": "runs",
			"path": ["workflow", "runs"],
			"description": "List runs",
			"flags": [{"name": "status", "description": "Run status", "enumValues": ["running", "succeeded", "failed"]}],
			"input": {"fields": [{"name": "status", "source": "flag", "type": "string", "description": "Run status", "enumValues": ["running", "succeeded", "failed"]}]}
		}]
	}`, text)
}

func TestTreeDescriptorUsesTypedInputHintsForFieldTypes(t *testing.T) {
	tree, err := NewTree("workflow").
		Sub("runs", nil,
			TypedInput[typedCommandHintInput](),
			Arg("name").Required().Describe("Workflow name"),
			Arg("values").Variadic().Describe("Values"),
			Flag("limit").Describe("Result limit"),
			Flag("verbose").Describe("Verbose output"),
			Flag("status").Enum("running", "succeeded", "failed"),
		).
		Build()
	require.NoError(t, err)

	desc := tree.Descriptor().Subcommands[0]
	require.Equal(t, []InputFieldDescriptor{
		{Name: "name", Source: InputSourceArg, Type: InputTypeString, Description: "Workflow name", Required: true},
		{Name: "values", Source: InputSourceArg, Type: InputTypeArray, Description: "Values", Variadic: true},
		{Name: "limit", Source: InputSourceFlag, Type: InputTypeInteger, Description: "Result limit"},
		{Name: "verbose", Source: InputSourceFlag, Type: InputTypeBool, Description: "Verbose output"},
		{Name: "status", Source: InputSourceFlag, Type: InputTypeString, EnumValues: []string{"running", "succeeded", "failed"}},
	}, desc.Input.Fields)
}

func TestTreeRejectsDuplicateAndInvalidSpecs(t *testing.T) {
	_, err := NewTree("workflow").
		Sub("show", nil).
		Sub("show", nil).
		Build()
	var dup ErrDuplicate
	require.ErrorAs(t, err, &dup)

	_, err = NewTree("workflow").
		Sub("bad", nil, Arg("rest").Variadic(), Arg("later")).
		Build()
	var validation ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, ValidationInvalidSpec, validation.Code)
}

func TestTreeBuilderSupportsAliasesAndPolicy(t *testing.T) {
	tree, err := NewTree("workflow", Alias("wf"), WithPolicy(Policy{AgentCallable: true})).Build()
	require.NoError(t, err)
	require.Equal(t, Descriptor{Name: "workflow", Path: []string{"workflow"}, Aliases: []string{"wf"}, Policy: Policy{AgentCallable: true}}, tree.Descriptor())
	require.Equal(t, Policy{AgentCallable: true}, tree.Descriptor().Policy)
	require.True(t, tree.Descriptor().AgentCallable())
}
