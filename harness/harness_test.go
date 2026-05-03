package harness

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/runnertest"
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

func TestSessionInfoCommandReportsHarnessMetadata(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	info := session.Info()
	require.NotEmpty(t, info.SessionID)
	require.Equal(t, "coder", info.AgentName)
	require.True(t, info.ThreadBacked)
	require.NotEmpty(t, info.ThreadID)
	threadID, ok := session.ThreadID()
	require.True(t, ok)
	require.Equal(t, info.ThreadID, threadID)
	require.Equal(t, "coder", session.AgentName())

	result, err := session.Send(context.Background(), "/session info")
	require.NoError(t, err)
	text := renderCommandResult(t, result)
	require.Contains(t, text, "session:")
	require.Contains(t, text, "id: "+info.SessionID)
	require.Contains(t, text, "agent: coder")
	require.Contains(t, text, "thread: "+string(info.ThreadID))
	require.Contains(t, text, "model: model: test/model")

	result, err = session.Send(context.Background(), "/session nope")
	require.NoError(t, err)
	text = renderCommandResult(t, result)
	require.Contains(t, text, "unknown subcommand \"nope\"")
	require.Contains(t, text, "usage: /session <info>")
	require.Contains(t, text, "/session info")
}

func TestSessionCommandDescriptorsAndStructuredExecute(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(workflow.Definition{Name: "ask_flow", Description: "Ask the agent", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}}}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	descriptors := session.CommandDescriptors()
	require.Len(t, descriptors, 2)
	require.Equal(t, "workflow", descriptors[0].Name)
	require.Equal(t, []string{"workflow"}, descriptors[0].Path)
	require.Equal(t, []string{"workflow", "list"}, descriptors[0].Subcommands[0].Path)
	require.Equal(t, "session", descriptors[1].Name)
	require.Equal(t, []string{"session", "info"}, descriptors[1].Subcommands[0].Path)
	require.Equal(t, command.InputTypeString, descriptors[0].Subcommands[2].Input.Fields[0].Type)
	require.Equal(t, command.InputTypeArray, descriptors[0].Subcommands[2].Input.Fields[1].Type)
	require.Equal(t, command.InputTypeString, descriptors[0].Subcommands[3].Input.Fields[0].Type)
	require.Equal(t, command.InputTypeString, descriptors[0].Subcommands[3].Input.Fields[1].Type)

	result, err := session.ExecuteCommand(context.Background(), []string{"/workflow", "list"}, nil)
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, result), "- ask_flow: Ask the agent")

	result, err = session.ExecuteCommand(context.Background(), []string{"session", "info"}, nil)
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, result), "agent: coder")

	result, err = session.ExecuteCommand(context.Background(), []string{"workflow", "runs"}, map[string]any{"status": "nope"})
	require.NoError(t, err)
	payload, ok := result.Payload.(command.HelpPayload)
	require.True(t, ok)
	require.NotNil(t, payload.Error)
	require.Equal(t, command.ValidationInvalidFlagValue, payload.Error.Code)
	require.Equal(t, "status", payload.Error.Field)
}

func TestSessionCommandCatalogIncludesExecutableCommandsWithSchemas(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(workflow.Definition{Name: "ask_flow", Description: "Ask the agent", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}}}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	catalog := session.CommandCatalog()
	require.Len(t, catalog, 6)
	requireCatalogPath(t, catalog, "workflow", "list")
	requireCatalogPath(t, catalog, "workflow", "show")
	start := requireCatalogPath(t, catalog, "workflow", "start")
	runs := requireCatalogPath(t, catalog, "workflow", "runs")
	requireCatalogPath(t, catalog, "workflow", "run")
	requireCatalogPath(t, catalog, "session", "info")

	require.Equal(t, "object", start.InputSchema.Type)
	require.Equal(t, "string", start.InputSchema.Properties["name"].Type)
	require.Equal(t, "array", start.InputSchema.Properties["input"].Type)
	require.Equal(t, []string{"name"}, start.InputSchema.Required)
	require.Equal(t, []string{"running", "succeeded", "failed"}, runs.InputSchema.Properties["status"].Enum)

	text, err := command.Render(command.Display(catalog), command.DisplayJSON)
	require.NoError(t, err)
	require.Contains(t, text, `"inputSchema"`)
}

func requireCatalogPath(t *testing.T, catalog []CommandCatalogEntry, path ...string) CommandCatalogEntry {
	t.Helper()
	for _, entry := range catalog {
		if strings.Join(entry.Descriptor.Path, "/") == strings.Join(path, "/") {
			return entry
		}
	}
	require.Failf(t, "missing catalog entry", "path %q not found", strings.Join(path, " "))
	return CommandCatalogEntry{}
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
	_, err = application.InstantiateAgent("coder",
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
	var handled []action.Event
	result := session.ExecuteWorkflow(ctx, "ask_flow", "answer through harness",
		app.WithWorkflowRunID("run_harness"),
		app.WithWorkflowEventHandler(func(_ action.Ctx, event action.Event) {
			handled = append(handled, event)
		}),
	)

	require.NoError(t, result.Error)
	require.Equal(t, "workflow answer", result.Data.(workflow.Result).Data)
	require.NotEmpty(t, handled)
	requireHarnessRequestContainsText(t, client.RequestAt(0), "answer through harness")

	state, ok, err := session.WorkflowRunState(ctx, "run_harness")
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, workflow.RunSucceeded, state.Status)
	require.Equal(t, workflow.InlineValue("workflow answer"), state.Output)

	summaries, ok, err := session.WorkflowRuns(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, summaries, 1)
	require.Equal(t, workflow.RunSummary{ID: "run_harness", WorkflowName: "ask_flow", Status: workflow.RunSucceeded, StartedAt: summaries[0].StartedAt, CompletedAt: summaries[0].CompletedAt, Duration: summaries[0].Duration}, summaries[0])
	require.False(t, summaries[0].StartedAt.IsZero())
	require.False(t, summaries[0].CompletedAt.IsZero())
	require.GreaterOrEqual(t, summaries[0].Duration, time.Duration(0))

	cmdResult, err := session.Send(ctx, "/workflow run run_harness")
	require.NoError(t, err)
	require.Equal(t, command.ResultDisplay, cmdResult.Kind)
	require.Contains(t, renderCommandResult(t, cmdResult), "workflow run: run_harness")
	require.Contains(t, renderCommandResult(t, cmdResult), "workflow: ask_flow")
	require.Contains(t, renderCommandResult(t, cmdResult), "status: succeeded")
	require.Contains(t, renderCommandResult(t, cmdResult), "output: workflow answer")
	require.Contains(t, renderCommandResult(t, cmdResult), "- ask")
	require.Contains(t, renderCommandResult(t, cmdResult), "action: ask_agent")
	require.Contains(t, renderCommandResult(t, cmdResult), "status: succeeded")
	require.Contains(t, renderCommandResult(t, cmdResult), "attempt: 1")
	require.Contains(t, renderCommandResult(t, cmdResult), "output: workflow answer")

	runsResult, err := session.Send(ctx, "/workflow runs")
	require.NoError(t, err)
	require.Equal(t, command.ResultDisplay, runsResult.Kind)
	require.Contains(t, renderCommandResult(t, runsResult), "Workflow runs:")
	require.Contains(t, renderCommandResult(t, runsResult), "RUN ID")
	require.Contains(t, renderCommandResult(t, runsResult), "WORKFLOW")
	require.Contains(t, renderCommandResult(t, runsResult), "STATUS")
	require.Contains(t, renderCommandResult(t, runsResult), "STARTED")
	require.Contains(t, renderCommandResult(t, runsResult), "DURATION")
	require.Contains(t, renderCommandResult(t, runsResult), "run_harness")
	require.Contains(t, renderCommandResult(t, runsResult), "ask_flow")
	require.Contains(t, renderCommandResult(t, runsResult), "succeeded")
}

func TestSessionWorkflowRunStateMissingLiveThread(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)

	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)
	store, ok := session.WorkflowRunStore()
	require.False(t, ok)
	require.Nil(t, store)

	state, ok, err := session.WorkflowRunState(context.Background(), "missing")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, workflow.RunState{}, state)

	summaries, ok, err := session.WorkflowRuns(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, summaries)

	result, err := session.Send(context.Background(), "/workflow run missing")
	require.NoError(t, err)
	require.Equal(t, "workflow runs require a thread-backed session", renderCommandResult(t, result))

	result, err = session.Send(context.Background(), "/workflow runs")
	require.NoError(t, err)
	require.Equal(t, "workflow runs require a thread-backed session", renderCommandResult(t, result))
}

func TestSessionWorkflowRunStateNilSessionAndAgent(t *testing.T) {
	store, ok := (*Session)(nil).WorkflowRunStore()
	require.False(t, ok)
	require.Nil(t, store)

	state, ok, err := (*Session)(nil).WorkflowRunState(context.Background(), "run")
	require.NoError(t, err)
	require.False(t, ok)
	require.Equal(t, workflow.RunState{}, state)

	summaries, ok, err := (*Session)(nil).WorkflowRuns(context.Background())
	require.NoError(t, err)
	require.False(t, ok)
	require.Nil(t, summaries)

	store, ok = (&Session{}).WorkflowRunStore()
	require.False(t, ok)
	require.Nil(t, store)

	application, err := app.New(app.WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)
	store, ok = (&Session{App: application}).WorkflowRunStore()
	require.False(t, ok)
	require.Nil(t, store)
}

func TestSessionWorkflowCommandUsageAndNotFound(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	result, err := session.Send(context.Background(), "/workflow")
	require.NoError(t, err)
	text := renderCommandResult(t, result)
	require.Contains(t, text, "usage: /workflow <list|show|start|runs|run>")
	require.Contains(t, text, "/workflow list")
	require.Contains(t, text, "/workflow show <name>")
	require.Contains(t, text, "/workflow start <name> [input...]")
	require.Contains(t, text, "/workflow runs [--workflow <workflow>] [--status <running|succeeded|failed>]")
	require.Contains(t, text, "/workflow run <run-id>")

	result, err = session.Send(context.Background(), "/workflow run missing")
	require.NoError(t, err)
	require.Equal(t, "workflow run \"missing\" not found", renderCommandResult(t, result))
}

func TestSessionWorkflowListAndShowCommands(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(
			workflow.Definition{
				Name:        "ask_flow",
				Description: "Ask the default agent",
				Steps: []workflow.Step{
					{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}},
					{ID: "summarize", Action: workflow.ActionRef{Name: "summarize"}, DependsOn: []string{"ask"}},
				},
			},
			workflow.Definition{Name: "release_notes", Steps: []workflow.Step{{ID: "write", Action: workflow.ActionRef{Name: "write_notes"}}}},
		),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	result, err := session.Send(context.Background(), "/workflow list")
	require.NoError(t, err)
	require.Equal(t, command.ResultDisplay, result.Kind)
	require.Contains(t, renderCommandResult(t, result), "Workflows:")
	require.Contains(t, renderCommandResult(t, result), "- ask_flow: Ask the default agent")
	require.Contains(t, renderCommandResult(t, result), "- release_notes")

	result, err = session.Send(context.Background(), "/workflow show ask_flow")
	require.NoError(t, err)
	require.Equal(t, command.ResultDisplay, result.Kind)
	require.Contains(t, renderCommandResult(t, result), "workflow: ask_flow")
	require.Contains(t, renderCommandResult(t, result), "description: Ask the default agent")
	require.Contains(t, renderCommandResult(t, result), "- ask: ask_agent")
	require.Contains(t, renderCommandResult(t, result), "- summarize: summarize depends_on=ask")

	result, err = session.Send(context.Background(), "/workflow show missing")
	require.NoError(t, err)
	require.Equal(t, "workflow \"missing\" not found", renderCommandResult(t, result))
}

func TestSessionWorkflowStartCommandExecutesAndRecordsRun(t *testing.T) {
	ctx := context.Background()
	client := runnertest.NewClient(runnertest.TextStream("started answer"))
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(workflow.Definition{Name: "ask_flow", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}}}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	turnAction, err := application.DefaultAgentTurnAction(action.Spec{Name: "ask_agent"})
	require.NoError(t, err)
	require.NoError(t, application.RegisterActions(turnAction))
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	result, err := session.Send(ctx, "/workflow start ask_flow hello from start")
	require.NoError(t, err)
	require.Equal(t, command.ResultDisplay, result.Kind)
	require.Contains(t, renderCommandResult(t, result), "workflow completed: ask_flow")
	require.Contains(t, renderCommandResult(t, result), "run: run_")
	require.Contains(t, renderCommandResult(t, result), "status: succeeded")
	require.Contains(t, renderCommandResult(t, result), "output: started answer")
	requireHarnessRequestContainsText(t, client.RequestAt(0), "hello from start")

	summaries, ok, err := session.WorkflowRuns(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, summaries, 1)
	require.Equal(t, "ask_flow", summaries[0].WorkflowName)
	require.Equal(t, workflow.RunSucceeded, summaries[0].Status)
	require.False(t, summaries[0].StartedAt.IsZero())
	require.False(t, summaries[0].CompletedAt.IsZero())

	runsResult, err := session.Send(ctx, "/workflow runs")
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, runsResult), "RUN ID")
	require.Contains(t, renderCommandResult(t, runsResult), string(summaries[0].ID))
	require.Contains(t, renderCommandResult(t, runsResult), "ask_flow")
	require.Contains(t, renderCommandResult(t, runsResult), "succeeded")
	require.Contains(t, renderCommandResult(t, runsResult), "DURATION")

	runResult, err := session.Send(ctx, "/workflow run "+string(summaries[0].ID))
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, runResult), "workflow run: "+string(summaries[0].ID))
	require.Contains(t, renderCommandResult(t, runResult), "workflow: ask_flow")
	require.Contains(t, renderCommandResult(t, runResult), "status: succeeded")
	require.Contains(t, renderCommandResult(t, runResult), "started:")
	require.Contains(t, renderCommandResult(t, runResult), "completed:")
	require.Contains(t, renderCommandResult(t, runResult), "output: started answer")
}

func TestSessionWorkflowStartCommandUsageAndMissingWorkflow(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	result, err := session.Send(context.Background(), "/workflow start")
	require.NoError(t, err)
	text := renderCommandResult(t, result)
	require.Contains(t, text, "missing required argument \"name\"")
	require.Contains(t, text, "usage: /workflow start <name> [input...]")

	result, err = session.Send(context.Background(), "/workflow start missing")
	require.NoError(t, err)
	require.Equal(t, "workflow \"missing\" not found", renderCommandResult(t, result))
}

func TestSessionWorkflowStartCommandFailureIncludesRunStatusAndIsQueryable(t *testing.T) {
	ctx := context.Background()
	boom := "boom"
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithActions(action.New(action.Spec{Name: "fail"}, func(action.Ctx, any) action.Result {
			return action.Result{Error: errors.New(boom)}
		})),
		app.WithWorkflows(workflow.Definition{Name: "failflow", Steps: []workflow.Step{{ID: "fail", Action: workflow.ActionRef{Name: "fail"}}}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	result, err := session.Send(ctx, "/workflow start failflow")
	require.NoError(t, err)
	require.Equal(t, command.ResultDisplay, result.Kind)
	require.Contains(t, renderCommandResult(t, result), "workflow failed: failflow")
	require.Contains(t, renderCommandResult(t, result), "run: run_")
	require.Contains(t, renderCommandResult(t, result), "status: failed")
	require.Contains(t, renderCommandResult(t, result), "error: workflow \"failflow\" step \"fail\" failed: boom")

	summaries, ok, err := session.WorkflowRuns(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, summaries, 1)
	require.Equal(t, "failflow", summaries[0].WorkflowName)
	require.Equal(t, workflow.RunFailed, summaries[0].Status)
	require.False(t, summaries[0].StartedAt.IsZero())
	require.False(t, summaries[0].CompletedAt.IsZero())
	require.Contains(t, summaries[0].Error, boom)

	runsResult, err := session.Send(ctx, "/workflow runs")
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, runsResult), string(summaries[0].ID))
	require.Contains(t, renderCommandResult(t, runsResult), "failflow")
	require.Contains(t, renderCommandResult(t, runsResult), "failed")
	require.Contains(t, renderCommandResult(t, runsResult), "error=workflow \"failflow\" step \"fail\" failed: boom")

	runResult, err := session.Send(ctx, "/workflow run "+string(summaries[0].ID))
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, runResult), "workflow run: "+string(summaries[0].ID))
	require.Contains(t, renderCommandResult(t, runResult), "workflow: failflow")
	require.Contains(t, renderCommandResult(t, runResult), "status: failed")
	require.Contains(t, renderCommandResult(t, runResult), "error: workflow \"failflow\" step \"fail\" failed: boom")
	require.Contains(t, renderCommandResult(t, runResult), "- fail")
	require.Contains(t, renderCommandResult(t, runResult), "status: failed")
}

func TestSessionWorkflowRunsCommandFiltersByWorkflowAndStatus(t *testing.T) {
	ctx := context.Background()
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithActions(
			action.New(action.Spec{Name: "echo"}, func(action.Ctx, any) action.Result {
				return action.Result{Data: "ok"}
			}),
			action.New(action.Spec{Name: "fail"}, func(action.Ctx, any) action.Result {
				return action.Result{Error: errors.New("boom")}
			}),
		),
		app.WithWorkflows(
			workflow.Definition{Name: "okflow", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo"}}}},
			workflow.Definition{Name: "failflow", Steps: []workflow.Step{{ID: "fail", Action: workflow.ActionRef{Name: "fail"}}}},
		),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	_, err = session.Send(ctx, "/workflow start okflow")
	require.NoError(t, err)
	_, err = session.Send(ctx, "/workflow start failflow")
	require.NoError(t, err)

	result, err := session.Send(ctx, "/workflow runs --status succeeded")
	require.NoError(t, err)
	text := renderCommandResult(t, result)
	require.Contains(t, text, "filters:")
	require.Contains(t, text, "status=succeeded")
	require.Contains(t, text, "okflow")
	require.NotContains(t, text, "failflow")

	result, err = session.Send(ctx, "/workflow runs --workflow failflow --status failed")
	require.NoError(t, err)
	text = renderCommandResult(t, result)
	require.Contains(t, text, "workflow=failflow")
	require.Contains(t, text, "status=failed")
	require.Contains(t, text, "failflow")
	require.NotContains(t, text, "okflow")

	result, err = session.Send(ctx, "/workflow runs --workflow missing")
	require.NoError(t, err)
	require.Equal(t, "No workflow runs matched filters.", renderCommandResult(t, result))

	result, err = session.Send(ctx, "/workflow runs --status nope")
	require.NoError(t, err)
	text = renderCommandResult(t, result)
	require.Contains(t, text, "invalid value \"nope\" for --status")
	require.Contains(t, text, "usage: /workflow runs [--workflow <workflow>] [--status <running|succeeded|failed>]")

	result, err = session.Send(ctx, "/workflow runs --workflow")
	require.NoError(t, err)
	text = renderCommandResult(t, result)
	require.Contains(t, text, "missing value for --workflow")
	require.Contains(t, text, "usage: /workflow runs [--workflow <workflow>] [--status <running|succeeded|failed>]")
}

func TestSessionWorkflowListNoWorkflows(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	result, err := session.Send(context.Background(), "/workflow list")
	require.NoError(t, err)
	require.Equal(t, "No workflows registered.", renderCommandResult(t, result))
}

func TestSessionWorkflowRunsNoRecordedRuns(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()), agent.WithSessionStoreDir(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	summaries, ok, err := session.WorkflowRuns(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, summaries)

	result, err := session.Send(context.Background(), "/workflow runs")
	require.NoError(t, err)
	require.Equal(t, "No workflow runs recorded.", renderCommandResult(t, result))
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

func TestSessionExecuteCommandReportsInvalidPathAndUnknownRoot(t *testing.T) {
	application, err := app.New(app.WithAgentSpec(agent.Spec{Name: "coder"}), app.WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)
	_, err = application.InstantiateAgent("coder", agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	session, err := NewService(application).DefaultSession()
	require.NoError(t, err)

	_, err = session.ExecuteCommand(context.Background(), nil, nil)
	var validation command.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, command.ValidationInvalidSpec, validation.Code)

	_, err = session.ExecuteCommand(context.Background(), []string{"missing"}, nil)
	var unknown command.ErrUnknown
	require.ErrorAs(t, err, &unknown)
	require.Equal(t, "missing", unknown.Name)
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

func renderCommandResult(t *testing.T, result command.Result) string {
	t.Helper()
	text, err := command.Render(result, command.DisplayTerminal)
	require.NoError(t, err)
	return text
}
