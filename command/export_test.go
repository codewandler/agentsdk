package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExportDescriptorsFlattensExecutableCommandsWithSchemasAndPolicy(t *testing.T) {
	tree, err := NewTree("workflow").
		Sub("list", func(context.Context, Invocation) (Result, error) { return Handled(), nil },
			WithPolicy(AgentPolicy()),
			Output(OutputDescriptor{
				Kind:        "workflow.list",
				Description: "Workflow list payload",
				Schema:      JSONSchema{Type: "object"},
			}),
		).
		Sub("show", func(context.Context, Invocation) (Result, error) { return Handled(), nil },
			Arg("name").Required(),
		).
		Build()
	require.NoError(t, err)

	exports := ExportDescriptors([]Descriptor{tree.Descriptor()})

	require.Len(t, exports, 2)
	require.Equal(t, []string{"workflow", "list"}, exports[0].Descriptor.Path)
	require.Equal(t, AgentPolicy(), exports[0].Policy)
	require.Equal(t, JSONSchema{Type: "object"}, exports[0].InputSchema)
	require.Equal(t, JSONSchema{Type: "object"}, exports[0].OutputSchema)
	require.Equal(t, []string{"workflow", "show"}, exports[1].Descriptor.Path)
	require.Equal(t, []string{"name"}, exports[1].InputSchema.Required)
}

func TestPolicyConstructorsDescribeCallerVariants(t *testing.T) {
	require.True(t, Descriptor{Policy: UserPolicy()}.UserCallable())
	require.False(t, Descriptor{Policy: UserPolicy()}.AgentCallable())

	require.False(t, Descriptor{Policy: AgentPolicy()}.UserCallable())
	require.True(t, Descriptor{Policy: AgentPolicy()}.AgentCallable())

	require.True(t, Descriptor{Policy: InternalPolicy()}.InternalCallable())
	require.False(t, Descriptor{Policy: TrustedPolicy()}.UserCallable())
	require.False(t, Descriptor{Policy: TrustedPolicy()}.AgentCallable())
}

func TestExportDescriptorsPreservesCommandSafetyPolicy(t *testing.T) {
	tree, err := NewTree("deploy").
		Handle(func(context.Context, Invocation) (Result, error) { return Handled(), nil },
			WithPolicy(Policy{UserCallable: true, SafetyClass: "deployment", RequiresApproval: true}),
		).
		Build()
	require.NoError(t, err)

	exports := ExportDescriptors([]Descriptor{tree.Descriptor()})
	require.Len(t, exports, 1)
	require.Equal(t, "deployment", exports[0].Policy.SafetyClass)
	require.True(t, exports[0].Policy.RequiresApproval)
	require.Equal(t, "deployment", exports[0].Descriptor.Policy.SafetyClass)
}
