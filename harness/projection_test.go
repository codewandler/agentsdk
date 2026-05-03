package harness

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestSessionAgentCommandProjectionShape(t *testing.T) {
	session, _ := newProjectionTestSession(t)

	projection := session.AgentCommandProjection()

	require.Len(t, projection.Tools, 1)
	require.Equal(t, AgentCommandToolName, projection.Tools[0].Name())
	require.Len(t, projection.ContextProviders, 1)
	require.Equal(t, AgentCommandCatalogProviderKey, projection.ContextProviders[0].Key())
}

func TestDefaultSessionAttachesAgentCommandProjection(t *testing.T) {
	session, client := newProjectionTestSession(t)

	require.NoError(t, session.Agent.RunTurn(context.Background(), 1, "hello"))

	request := client.RequestAt(0)
	require.Contains(t, requestToolNames(request.Tools), AgentCommandToolName)
	require.Contains(t, session.Agent.ContextState(), string(AgentCommandCatalogProviderKey))
}

func TestSessionAttachAgentProjectionIsIdempotent(t *testing.T) {
	session, client := newProjectionTestSession(t)
	projection := session.AgentCommandProjection()

	// DefaultSession already attached the command projection; explicit attachment
	// remains idempotent for callers that attach projections manually.
	require.NoError(t, session.AttachAgentProjection(projection))
	require.NoError(t, session.AttachAgentProjection(projection))
	require.NoError(t, session.Agent.RunTurn(context.Background(), 1, "hello"))

	require.Equal(t, 1, countStrings(requestToolNames(client.RequestAt(0).Tools), AgentCommandToolName))
}

func TestSessionAttachAgentProjectionRequiresSessionAndAgent(t *testing.T) {
	var nilSession *Session
	require.ErrorContains(t, nilSession.AttachAgentProjection(AgentProjection{}), "session is nil")
	require.ErrorContains(t, (&Session{}).AttachAgentProjection(AgentProjection{}), "agent is required")
}

func newProjectionTestSession(t *testing.T) (*Session, *runnertest.Client) {
	t.Helper()
	client := runnertest.NewClient(runnertest.TextStream("ok"), runnertest.TextStream("ok"))
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(workflow.Definition{Name: "ask_flow", Description: "Ask the agent", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}}}}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)
	return session, client
}

func requestToolNames(tools []unified.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func countStrings(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}
