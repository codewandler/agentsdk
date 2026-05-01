package workflow

import (
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/stretchr/testify/require"
)

func TestValidateRejectsInvalidWorkflowDefinitions(t *testing.T) {
	require.ErrorContains(t, Validate(Definition{Steps: []Step{{Action: ActionRef{Name: "echo"}}}}), "step id")
	require.ErrorContains(t, Validate(Definition{Steps: []Step{{ID: "echo"}}}), "action name")
	require.ErrorContains(t, Validate(Definition{Steps: []Step{{ID: "dup", Action: ActionRef{Name: "a"}}, {ID: "dup", Action: ActionRef{Name: "b"}}}}), "duplicate")
	require.ErrorContains(t, Validate(Definition{Steps: []Step{{ID: "a", Action: ActionRef{Name: "a"}, DependsOn: []string{"missing"}}}}), "unknown step")
	require.ErrorContains(t, Validate(Definition{Steps: []Step{{ID: "a", Action: ActionRef{Name: "a"}, DependsOn: []string{"b"}}, {ID: "b", Action: ActionRef{Name: "b"}, DependsOn: []string{"a"}}}}), "cycle")
}

func TestValidateActionsChecksRegistryResolution(t *testing.T) {
	reg := action.NewRegistry()
	require.NoError(t, reg.Register(action.New(action.Spec{Name: "echo"}, func(action.Ctx, any) action.Result { return action.Result{} })))

	valid := Definition{Name: "valid", Steps: []Step{{ID: "echo", Action: ActionRef{Name: "echo"}}}}
	require.NoError(t, ValidateActions(valid, reg))

	missing := Definition{Name: "missing", Steps: []Step{{ID: "missing", Action: ActionRef{Name: "missing"}}}}
	require.ErrorContains(t, ValidateActions(missing, reg), "action \"missing\" not found")
}
