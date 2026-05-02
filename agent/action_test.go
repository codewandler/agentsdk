package agent

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/stretchr/testify/require"
)

func TestTurnActionRunsAgentTurnAndReturnsAssistantText(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("hello from model"))
	inst, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)

	act := inst.TurnAction(action.Spec{Name: "ask_agent", Description: "Ask agent"})
	result := act.Execute(context.Background(), "say hello")

	require.NoError(t, result.Error)
	require.Equal(t, "hello from model", result.Data)
	require.Equal(t, "ask_agent", act.Spec().Name)
	require.Equal(t, action.TypeOf[string]().GoType, act.Spec().Input.GoType)
	require.Equal(t, action.TypeOf[string]().GoType, act.Spec().Output.GoType)
	require.Len(t, client.Requests(), 1)
	requireRequestContainsText(t, client.RequestAt(0), "say hello")
}

func TestTurnActionUsesDefaultsAndReportsInvalidInput(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("unused"))
	inst, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)

	act := TurnAction(inst, action.Spec{})
	require.Equal(t, DefaultTurnActionName, act.Spec().Name)

	var invalid action.ErrInvalidInput
	result := act.Execute(context.Background(), []string{"not", "a", "prompt"})
	require.ErrorAs(t, result.Error, &invalid)
	require.Empty(t, client.Requests())
}

func TestTurnActionNilInstanceReturnsError(t *testing.T) {
	act := TurnAction(nil, action.Spec{Name: "missing_agent"})

	result := act.Execute(context.Background(), "hello")

	require.ErrorContains(t, result.Error, "instance is nil")
}
