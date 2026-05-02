package harness

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/thread"
	threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestDefaultSessionSendDelegatesToAppDefaultAgent(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)

	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)
	result, err := session.Send(context.Background(), "hello")

	require.NoError(t, err)
	require.Equal(t, command.ResultHandled, result.Kind)
	require.Len(t, client.Requests(), 1)
	requireHarnessRequestContainsText(t, client.RequestAt(0), "hello")
}

func TestSessionExecuteWorkflowRecordsThreadBackedRun(t *testing.T) {
	ctx := context.Background()
	client := runnertest.NewClient(runnertest.TextStream("workflow answer"))
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(workflow.Definition{Name: "ask_flow", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}}}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	inst, err := application.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
		agent.WithSessionStoreDir(t.TempDir()),
	)
	require.NoError(t, err)
	turnAction, err := application.DefaultAgentTurnAction(action.Spec{Name: "ask_agent"})
	require.NoError(t, err)
	require.NoError(t, application.RegisterActions(turnAction))

	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)
	result := session.ExecuteWorkflow(ctx, "ask_flow", "answer through harness", app.WithWorkflowRunID("run_harness"))

	require.NoError(t, result.Error)
	require.Equal(t, "workflow answer", result.Data.(workflow.Result).Data)
	requireHarnessRequestContainsText(t, client.RequestAt(0), "answer through harness")

	store := threadjsonlstore.Open(filepath.Dir(inst.SessionStorePath()))
	state, ok, err := (workflow.ThreadRunStore{Store: store, ThreadID: thread.ID(application.SessionID())}).State(ctx, "run_harness")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, workflow.RunSucceeded, state.Status)
	require.Equal(t, workflow.InlineValue("workflow answer"), state.Output)
}

func TestDefaultSessionReportsMissingAppAndAgent(t *testing.T) {
	_, err := (*Service)(nil).DefaultSession()
	require.ErrorContains(t, err, "app is required")

	service := NewService(nil)
	_, err = service.DefaultSession()
	require.ErrorContains(t, err, "app is required")

	application, err := app.New(app.WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)
	_, err = NewService(application).DefaultSession()
	require.ErrorContains(t, err, "no default agent")
}

func TestSessionReportsMissingApp(t *testing.T) {
	_, err := (*Session)(nil).Send(context.Background(), "hello")
	require.ErrorContains(t, err, "app is required")

	result := (*Session)(nil).ExecuteWorkflow(context.Background(), "missing", nil)
	require.ErrorContains(t, result.Error, "app is required")
}

func requireHarnessRequestContainsText(t *testing.T, req unified.Request, want string) {
	t.Helper()
	for _, msg := range req.Messages {
		for _, part := range msg.Content {
			if text, ok := part.(unified.TextPart); ok && strings.Contains(text.Text, want) {
				return
			}
		}
	}
	for _, inst := range req.Instructions {
		for _, part := range inst.Content {
			if text, ok := part.(unified.TextPart); ok && strings.Contains(text.Text, want) {
				return
			}
		}
	}
	t.Fatalf("request does not contain %q", want)
}
