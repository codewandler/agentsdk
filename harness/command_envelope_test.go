package harness

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/stretchr/testify/require"
)

func TestCommandEnvelopeSchemaUsesGenericEnvelope(t *testing.T) {
	schema := CommandEnvelopeSchema()

	require.Equal(t, "object", schema.Type)
	require.Equal(t, []string{"path"}, schema.Required)
	require.Equal(t, "array", schema.Properties["path"].Type)
	require.Equal(t, &command.JSONSchema{Type: "string"}, schema.Properties["path"].Items)
	require.Equal(t, "object", schema.Properties["input"].Type)

	text, err := command.Render(command.Display(schema), command.DisplayJSON)
	require.NoError(t, err)
	require.Contains(t, text, `"path"`)
	require.NotContains(t, text, `"oneOf"`)
}

func TestSessionAgentCommandCatalogExposesReadOnlyWorkflowCommands(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)

	catalog := session.CommandCatalog(CommandCatalogAgentCallable())

	require.Equal(t, []string{"list", "show"}, catalogNames(catalog))
	show := requireCatalogPath(t, catalog, "workflow", "show")
	require.Equal(t, []string{"name"}, show.InputSchema.Required)
	require.Equal(t, "string", show.InputSchema.Properties["name"].Type)
}

func TestSessionExecuteAgentCommandEnvelopeRunsAgentCallableCommand(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)

	result, err := session.ExecuteAgentCommandEnvelope(context.Background(), CommandEnvelope{Path: []string{"workflow", "show"}, Input: map[string]any{"name": "ask_flow"}})

	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, result), "ask_flow")
}

func TestSessionExecuteAgentCommandEnvelopeRejectsMissingPathAndNonAgentCallableCommand(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)

	_, err := session.ExecuteAgentCommandEnvelope(context.Background(), CommandEnvelope{})
	var validation command.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, command.ValidationInvalidSpec, validation.Code)

	_, err = session.ExecuteAgentCommandEnvelope(context.Background(), CommandEnvelope{Path: []string{"workflow", "start"}, Input: map[string]any{"name": "ask_flow"}})
	var notCallable command.ErrNotCallable
	require.ErrorAs(t, err, &notCallable)
	require.Equal(t, "agent", notCallable.Caller)
	require.Equal(t, "workflow start", notCallable.Name)
}

func TestSessionExecuteCommandEnvelopeRunsTrustedNonAgentCallableCommand(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)

	result, err := session.ExecuteCommandEnvelope(context.Background(), CommandEnvelope{Path: []string{"session", "info"}})

	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, result), "agent: coder")

	_, err = session.ExecuteAgentCommandEnvelope(context.Background(), CommandEnvelope{Path: []string{"session", "info"}})
	var notCallable command.ErrNotCallable
	require.ErrorAs(t, err, &notCallable)
	require.Equal(t, "agent", notCallable.Caller)
	require.Equal(t, "session info", notCallable.Name)
}

func TestSessionExecuteAgentCommandEnvelopeUsesCommandValidation(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)

	result, err := session.ExecuteAgentCommandEnvelope(context.Background(), CommandEnvelope{Path: []string{"workflow", "show"}})

	require.NoError(t, err)
	payload, ok := result.Payload.(command.HelpPayload)
	require.True(t, ok)
	require.NotNil(t, payload.Error)
	require.Equal(t, command.ValidationMissingArg, payload.Error.Code)
	require.Equal(t, "name", payload.Error.Field)
}

func newCommandEnvelopeTestSession(t *testing.T) *Session {
	t.Helper()
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(workflow.Definition{Name: "ask_flow", Description: "Ask the agent", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}}}}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)
	return session
}
