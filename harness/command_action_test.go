package harness

import (
	"context"

	"github.com/codewandler/agentsdk/action"
	"testing"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/stretchr/testify/require"
)

func TestSessionCommandActionExecutesCommandEnvelope(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)
	a := session.CommandAction()
	require.Equal(t, CommandActionName, a.Spec().Name)
	require.False(t, a.Spec().Input.IsZero())

	result := a.Execute(action.NewCtx(context.Background()), map[string]any{"path": []any{"session", "info"}})

	require.NoError(t, result.Error)
	cmdResult, ok := result.Data.(command.Result)
	require.True(t, ok)
	require.Contains(t, renderCommandResult(t, cmdResult), "agent: coder")
}

func TestSessionCommandActionReportsInvalidEnvelope(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)
	a := session.CommandAction()

	result := a.Execute(action.NewCtx(context.Background()), map[string]any{})

	var validation command.ValidationError
	require.ErrorAs(t, result.Error, &validation)
	require.Equal(t, command.ValidationInvalidSpec, validation.Code)
}

func TestWorkflowCanExecuteCommandAction(t *testing.T) {
	application, session := newCommandActionWorkflowTestSession(t)
	require.NoError(t, application.RegisterActions(session.CommandAction()))

	result := session.ExecuteWorkflow(action.NewCtx(context.Background()), "session_info_flow", nil)

	require.NoError(t, result.Error)
	wfResult := result.Data.(workflow.Result)
	cmdResult, ok := wfResult.Data.(command.Result)
	require.True(t, ok)
	require.Contains(t, renderCommandResult(t, cmdResult), "agent: coder")
}

func newCommandActionWorkflowTestSession(t *testing.T) (*app.App, *Session) {
	t.Helper()
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(workflow.Definition{Name: "session_info_flow", Steps: []workflow.Step{{
			ID:     "session_info",
			Action: workflow.ActionRef{Name: CommandActionName},
			Input:  map[string]any{"path": []any{"session", "info"}},
		}}}),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)
	return application, session
}
