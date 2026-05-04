package harness

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/plugins/plannerplugin"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/thread"
	threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

// withTestStore returns agent options that open a JSONL thread store at dir.
// Test code uses this instead of bare WithSessionStoreDir now that the agent
// no longer self-opens stores.
func withTestStore(dir string) []agent.Option {
	return []agent.Option{
		agent.WithThreadStore(threadjsonlstore.Open(dir)),
		agent.WithSessionStoreDir(dir),
	}
}

// openTestSession is the standard test helper for creating a harness session.
// It replaces the old InstantiateAgent + DefaultSession pattern.
func openTestSession(t *testing.T, application *app.App, opts ...agent.Option) (*Service, *Session) {
	t.Helper()
	service := NewService(application)
	session, err := service.OpenSession(context.Background(), SessionOpenRequest{AgentOptions: opts})
	require.NoError(t, err)
	return service, session
}

func TestOpenSessionSendDelegatesToDefaultAgent(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	result, err := session.Send(context.Background(), "hello")

	require.NoError(t, err)
	require.Equal(t, command.ResultHandled, result.Kind)
	require.Len(t, client.Requests(), 1)
	requireHarnessRequestContainsText(t, client.RequestAt(0), "hello")
}

func TestSessionInfoCommandReportsHarnessMetadata(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

	info := session.Info()
	require.NotEmpty(t, info.SessionID)
	require.Equal(t, "coder", info.AgentName)
	require.True(t, info.ThreadBacked)
	require.NotEmpty(t, info.ThreadID)

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

func TestSessionControlCommands(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{
			Name:        "coder",
			Description: "Writes code",
			Inference:   agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		}),
		app.WithDefaultAgent("coder"),
	)
	require.NoError(t, err)
	inst, err := application.InstantiateDefaultAgent(agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	session := &Session{App: application, Agent: inst}

	result, err := session.Send(context.Background(), "/help")
	require.NoError(t, err)
	text := renderCommandResult(t, result)
	require.Contains(t, text, "/agents - Show available agents")
	require.Contains(t, text, "/workflow list - List workflows")
	require.Contains(t, text, "/session info - Show session metadata")

	result, err = session.Send(context.Background(), "/agents")
	require.NoError(t, err)
	text = renderCommandResult(t, result)
	require.Contains(t, text, "Agents:")
	require.Contains(t, text, "* coder - Writes code")

	result, err = session.Send(context.Background(), "/context")
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, result), "context: no render state yet for agent \"coder\"")

	result, err = session.Send(context.Background(), "/turn hello")
	require.NoError(t, err)
	require.Equal(t, command.ResultHandled, result.Kind)
	require.Len(t, client.Requests(), 1)
}

func TestSessionCapabilitiesCommandReportsCapabilityProjections(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{
			Name:      "planner_agent",
			Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
			Capabilities: []capability.AttachSpec{{
				CapabilityName: planner.CapabilityName,
				InstanceID:     "default",
			}},
		}),
		app.WithPlugin(plannerplugin.New()),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, agent.WithClient(runnertest.NewClient(runnertest.TextStream("ok"))), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, session.Agent.RunTurn(context.Background(), 1, "attach planner"))

	state := session.CapabilityState()
	require.Len(t, state.Capabilities, 1)
	require.Equal(t, "planner", state.Capabilities[0].Name)
	require.Equal(t, "default", state.Capabilities[0].InstanceID)
	require.Contains(t, state.Capabilities[0].Tools, "plan")
	require.Contains(t, state.Capabilities[0].Actions, planner.ActionApplyActions)
	require.Equal(t, "planner/default", string(state.Capabilities[0].Context.Key))

	result, err := session.Send(context.Background(), "/capabilities")
	require.NoError(t, err)
	text := renderCommandResult(t, result)
	require.Contains(t, text, "capabilities:")
	require.Contains(t, text, "instance: default (planner)")
	require.Contains(t, text, "tools: plan")
	require.Contains(t, text, "actions: planner.apply_actions")
}
func TestSessionSkillCommandReportsAlreadyActiveDynamicSkill(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	application, err := app.New(app.WithAgentSpec(agent.Spec{
		Name:      "coder",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		SkillSources: []skill.Source{skill.FSSource("skills", "skills", fstest.MapFS{
			"skills/architecture/SKILL.md":                {Data: []byte("---\nname: architecture\ndescription: Architecture\n---\n# Architecture")},
			"skills/architecture/references/tradeoffs.md": {Data: []byte("---\ntrigger: tradeoffs\n---\nTradeoffs reference")},
		}, "skills", skill.SourceEmbedded, 0)},
	}))
	require.NoError(t, err)
	inst, err := application.InstantiateDefaultAgent(agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	_, err = inst.ActivateSkill("architecture")
	require.NoError(t, err)
	session := &Session{App: application, Agent: inst}

	result, err := session.Send(context.Background(), "/skill activate architecture")
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, result), "already active (dynamic)")

	result, err = session.Send(context.Background(), "/skills")
	require.NoError(t, err)
	skillsText := renderCommandResult(t, result)
	require.Contains(t, skillsText, "architecture (dynamic)")
	require.Contains(t, skillsText, "source=skills")

	result, err = session.Send(context.Background(), "/skill refs architecture")
	require.NoError(t, err)
	refsText := renderCommandResult(t, result)
	require.Contains(t, refsText, "Skill references for architecture")
	require.Contains(t, refsText, "references/tradeoffs.md")
	require.Contains(t, refsText, "triggers=tradeoffs")

	result, err = session.Send(context.Background(), "/skill ref architecture references/tradeoffs.md")
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, result), `skill ref: activated "references/tradeoffs.md" for "architecture"`)
}

func TestSessionCompactCommand(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream("resp3"),
		runnertest.TextStream("Summary."),
	)
	application, err := app.New(app.WithAgentSpec(agent.Spec{
		Name:      "coder",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}))
	require.NoError(t, err)
	inst, err := application.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	session := &Session{App: application, Agent: inst}

	ctx := context.Background()
	_, err = session.Send(ctx, "old1")
	require.NoError(t, err)
	_, err = session.Send(ctx, "old2")
	require.NoError(t, err)
	_, err = session.Send(ctx, "recent")
	require.NoError(t, err)

	result, err := session.Send(ctx, "/compact")
	require.NoError(t, err)
	require.Equal(t, command.ResultDisplay, result.Kind)
	require.Contains(t, renderCommandResult(t, result), "Compacted")
}

func TestSessionPublishesCompactionEvents(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream("resp3"),
		runnertest.TextStream("Visible summary."),
	)
	application, err := app.New(app.WithAgentSpec(agent.Spec{
		Name:      "coder",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}))
	require.NoError(t, err)
	_, session := openTestSession(t, application, agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	events, cancel := session.Subscribe(16)
	defer cancel()

	ctx := context.Background()
	_, err = session.Send(ctx, "old1")
	require.NoError(t, err)
	_, err = session.Send(ctx, "old2")
	require.NoError(t, err)
	_, err = session.Send(ctx, "recent")
	require.NoError(t, err)
	_, err = session.Send(ctx, "/compact")
	require.NoError(t, err)

	var sawDelta, sawCommitted bool
	for i := 0; i < 16; i++ {
		select {
		case event := <-events:
			if event.Type != SessionEventCompaction {
				continue
			}
			if event.CompactionEvent.Type == agent.CompactionEventSummaryDelta && event.CompactionEvent.SummaryDelta != "" {
				sawDelta = true
			}
			if event.CompactionEvent.Type == agent.CompactionEventCommitted && event.CompactionEvent.Summary == "Visible summary." {
				sawCommitted = true
			}
		default:
			i = 16
		}
	}
	require.True(t, sawDelta)
	require.True(t, sawCommitted)
	policy := session.CompactionPolicy()
	require.True(t, policy.Enabled)
	require.Equal(t, 0.85, policy.ContextWindowRatio)
}
func TestSessionCompactCommandTooShort(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("resp"))
	application, err := app.New(app.WithAgentSpec(agent.Spec{
		Name:      "coder",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}))
	require.NoError(t, err)
	inst, err := application.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	session := &Session{App: application, Agent: inst}

	_, err = session.Send(context.Background(), "hello")
	require.NoError(t, err)
	result, err := session.Send(context.Background(), "/compact")
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, result), "too short")
}

func TestSessionCommandsExposeDescriptorsAndStructuredExecute(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithWorkflows(workflow.Definition{Name: "ask_flow", Description: "Ask the agent", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}}}}),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

	commands, err := session.Commands()
	require.NoError(t, err)
	descriptors := commands.Descriptors()
	workflowDescriptor := requireDescriptorPath(t, descriptors, "workflow")
	sessionDescriptor := requireDescriptorPath(t, descriptors, "session")
	require.Equal(t, []string{"workflow"}, workflowDescriptor.Path)
	require.Equal(t, []string{"workflow", "list"}, workflowDescriptor.Subcommands[0].Path)
	require.Equal(t, []string{"session", "info"}, sessionDescriptor.Subcommands[0].Path)
	require.Equal(t, command.InputTypeString, workflowDescriptor.Subcommands[2].Input.Fields[0].Type)
	require.Equal(t, command.InputTypeArray, workflowDescriptor.Subcommands[2].Input.Fields[1].Type)
	require.Equal(t, command.InputTypeString, workflowDescriptor.Subcommands[3].Input.Fields[0].Type)
	require.Equal(t, command.InputTypeString, workflowDescriptor.Subcommands[3].Input.Fields[1].Type)

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
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

	catalog := session.CommandCatalog()
	require.Len(t, catalog, 12)
	requireCatalogPath(t, catalog, "skill", "activate")
	requireCatalogPath(t, catalog, "skill", "refs")
	requireCatalogPath(t, catalog, "skill", "ref")
	requireCatalogPath(t, catalog, "workflow", "list")
	requireCatalogPath(t, catalog, "workflow", "show")
	start := requireCatalogPath(t, catalog, "workflow", "start")
	runs := requireCatalogPath(t, catalog, "workflow", "runs")
	requireCatalogPath(t, catalog, "workflow", "run")
	requireCatalogPath(t, catalog, "workflow", "rerun")
	requireCatalogPath(t, catalog, "workflow", "events")
	requireCatalogPath(t, catalog, "workflow", "cancel")
	requireCatalogPath(t, catalog, "session", "info")

	require.Equal(t, "object", start.InputSchema.Type)
	require.Equal(t, "string", start.InputSchema.Properties["name"].Type)
	require.Equal(t, "array", start.InputSchema.Properties["input"].Type)
	require.Equal(t, []string{"name"}, start.InputSchema.Required)
	require.Equal(t, []string{"queued", "running", "succeeded", "failed", "canceled"}, runs.InputSchema.Properties["status"].Enum)

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
	)
	require.NoError(t, err)
	inst, err := application.InstantiateAgent("coder",
		append(withTestStore(t.TempDir()), agent.WithClient(client), agent.WithWorkspace(t.TempDir()))...,
	)
	require.NoError(t, err)
	turnAction := agent.TurnAction(inst, action.Spec{Name: "ask_agent"})
	require.NoError(t, application.RegisterActions(turnAction))

	service := NewService(application)
	session, err := service.OpenSession(context.Background(), SessionOpenRequest{AgentOptions: []agent.Option{}})
	require.NoError(t, err)
	// Re-attach inst from the pre-created agent for TurnAction compatibility.
	session.Agent = inst
	var handled []action.Event
	result := session.ExecuteWorkflow(ctx, "ask_flow", "answer through harness",
		workflow.WithRunID("run_harness"),
		workflow.WithEventHandler(func(_ action.Ctx, event action.Event) {
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
	require.Equal(t, "run_harness", string(summaries[0].ID))
	require.Equal(t, "ask_flow", summaries[0].WorkflowName)
	require.Equal(t, workflow.RunSucceeded, summaries[0].Status)
	require.Equal(t, "coder", summaries[0].Metadata.AgentName)
	require.Equal(t, "harness", summaries[0].Metadata.Trigger)
	require.Equal(t, []string{"workflow", "start"}, summaries[0].Metadata.CommandPath)
	require.Equal(t, workflow.InlineValue("answer through harness"), summaries[0].Input)
	require.NotEmpty(t, summaries[0].DefinitionHash)
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
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))
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

	application, err := app.New()
	require.NoError(t, err)
	store, ok = (&Session{App: application}).WorkflowRunStore()
	require.False(t, ok)
	require.Nil(t, store)
}

func TestSessionWorkflowCommandUsageAndNotFound(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

	result, err := session.Send(context.Background(), "/workflow")
	require.NoError(t, err)
	text := renderCommandResult(t, result)
	require.Contains(t, text, "usage: /workflow <list|show|start|runs|run|rerun|events|cancel>")
	require.Contains(t, text, "/workflow list")
	require.Contains(t, text, "/workflow show <name>")
	require.Contains(t, text, "/workflow start <name> [input...]")
	require.Contains(t, text, "/workflow runs [--workflow <workflow>] [--status <queued|running|succeeded|failed|canceled>] [--limit <limit>] [--offset <offset>]")
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
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))

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
	)
	require.NoError(t, err)
	service, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(client), agent.WithWorkspace(t.TempDir()))...)
	inst := session.Agent
	turnAction := agent.TurnAction(inst, action.Spec{Name: "ask_agent"})
	require.NoError(t, application.RegisterActions(turnAction))
	_ = service

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
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

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
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

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
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

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
	require.Contains(t, text, "usage: /workflow runs [--workflow <workflow>] [--status <queued|running|succeeded|failed|canceled>] [--limit <limit>] [--offset <offset>]")

	result, err = session.Send(ctx, "/workflow runs --workflow")
	require.NoError(t, err)
	text = renderCommandResult(t, result)
	require.Contains(t, text, "missing value for --workflow")
	require.Contains(t, text, "usage: /workflow runs [--workflow <workflow>] [--status <queued|running|succeeded|failed|canceled>] [--limit <limit>] [--offset <offset>]")
}

func TestSessionWorkflowAsyncRerunEventsCancelAndPagination(t *testing.T) {
	ctx := context.Background()
	started := make(chan struct{})
	release := make(chan struct{})
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		app.WithActions(
			action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
				return action.Result{Data: input}
			}),
			action.New(action.Spec{Name: "block"}, func(ctx action.Ctx, input any) action.Result {
				select {
				case <-started:
				default:
					close(started)
				}
				select {
				case <-ctx.Done():
					return action.Result{Error: ctx.Err()}
				case <-release:
					return action.Result{Data: input}
				}
			}),
		),
		app.WithWorkflows(
			workflow.Definition{Name: "echo_flow", Version: "v1", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo", Version: "v1"}}}},
			workflow.Definition{Name: "block_flow", Steps: []workflow.Step{{ID: "block", Action: workflow.ActionRef{Name: "block"}}}},
		),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

	first, err := session.Send(ctx, "/workflow start echo_flow first")
	require.NoError(t, err)
	firstRun := first.Payload.(WorkflowStartPayload).RunID

	async, err := session.ExecuteCommand(ctx, []string{"workflow", "start"}, map[string]any{"name": "block_flow", "input": "wait", "async": true})
	require.NoError(t, err)
	asyncPayload := async.Payload.(WorkflowStartPayload)
	require.Equal(t, workflow.RunQueued, asyncPayload.Status)
	<-started

	eventsResult, err := session.Send(ctx, "/workflow events "+string(firstRun))
	require.NoError(t, err)
	eventsText := renderCommandResult(t, eventsResult)
	require.Contains(t, eventsText, "workflow events: "+string(firstRun))
	require.Contains(t, eventsText, string(workflow.EventStarted))
	require.Contains(t, eventsText, string(workflow.EventCompleted))

	rerun, err := session.Send(ctx, "/workflow rerun "+string(firstRun))
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, rerun), "workflow completed: echo_flow")

	cancelResult, err := session.Send(ctx, "/workflow cancel "+string(asyncPayload.RunID)+" no longer needed")
	require.NoError(t, err)
	require.Contains(t, renderCommandResult(t, cancelResult), "workflow canceled: "+string(asyncPayload.RunID))

	require.Eventually(t, func() bool {
		state, ok, err := session.WorkflowRunState(ctx, asyncPayload.RunID)
		return err == nil && ok && state.Status == workflow.RunCanceled
	}, time.Second, 10*time.Millisecond)

	runs, err := session.Send(ctx, "/workflow runs --limit 1 --offset 1")
	require.NoError(t, err)
	text := renderCommandResult(t, runs)
	require.Contains(t, text, "limit=1")
	require.Contains(t, text, "offset=1")
}

func TestSessionWorkflowListNoWorkflows(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))

	result, err := session.Send(context.Background(), "/workflow list")
	require.NoError(t, err)
	require.Equal(t, "No workflows registered.", renderCommandResult(t, result))
}

func TestSessionWorkflowRunsNoRecordedRuns(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	_, session := openTestSession(t, application, append(withTestStore(t.TempDir()), agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))...)

	summaries, ok, err := session.WorkflowRuns(context.Background())
	require.NoError(t, err)
	require.True(t, ok)
	require.Empty(t, summaries)

	result, err := session.Send(context.Background(), "/workflow runs")
	require.NoError(t, err)
	require.Equal(t, "No workflow runs recorded.", renderCommandResult(t, result))
}

func TestOpenSessionReportsMissingAppAndAgent(t *testing.T) {
	_, err := (*Service)(nil).OpenSession(context.Background(), SessionOpenRequest{})
	require.ErrorContains(t, err, "app is required")

	service := NewService(nil)
	_, err = service.OpenSession(context.Background(), SessionOpenRequest{})
	require.ErrorContains(t, err, "app is required")

	application, err := app.New()
	require.NoError(t, err)
	_, err = NewService(application).OpenSession(context.Background(), SessionOpenRequest{})
	require.ErrorContains(t, err, "no default agent")
}

func TestSessionReportsMissingApp(t *testing.T) {
	_, err := (*Session)(nil).Send(context.Background(), "hello")
	require.ErrorContains(t, err, "app is required")

	result := (*Session)(nil).ExecuteWorkflow(context.Background(), "missing", nil)
	require.ErrorContains(t, result.Error, "app is required")
}

func TestSessionExecuteCommandReportsInvalidPathAndUnknownRoot(t *testing.T) {
	application, err := app.New(app.WithAgentSpec(agent.Spec{Name: "coder"}))
	require.NoError(t, err)
	_, session := openTestSession(t, application, agent.WithClient(runnertest.NewClient()), agent.WithWorkspace(t.TempDir()))

	_, err = session.ExecuteCommand(context.Background(), nil, nil)
	var validation command.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, command.ValidationInvalidSpec, validation.Code)

	_, err = session.ExecuteCommand(context.Background(), []string{"missing"}, nil)
	var unknown command.ErrUnknown
	require.ErrorAs(t, err, &unknown)
	require.Equal(t, "missing", unknown.Name)
}

func TestServiceOpenListResumeAndCloseSessions(t *testing.T) {
	ctx := context.Background()
	storeDir := t.TempDir()
	application, err := app.New(app.WithAgentSpec(agent.Spec{Name: "coder"}), app.WithDefaultAgent("coder"))
	require.NoError(t, err)
	service := NewService(application)

	first, err := service.OpenSession(ctx, SessionOpenRequest{
		Name:         "work",
		StoreDir:     storeDir,
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})
	require.NoError(t, err)
	require.Equal(t, "work", first.Name)
	require.NotEmpty(t, first.SessionID())
	require.True(t, first.Info().ThreadBacked)

	summaries := service.Sessions()
	require.Len(t, summaries, 1)
	require.Equal(t, "work", summaries[0].Name)
	require.Equal(t, first.SessionID(), summaries[0].SessionID)
	require.Equal(t, "coder", summaries[0].AgentName)
	require.True(t, summaries[0].ThreadBacked)

	resumed, err := service.ResumeSession(ctx, SessionOpenRequest{
		Name:         "resumed",
		StoreDir:     storeDir,
		Resume:       first.SessionID(),
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})
	require.NoError(t, err)
	require.Equal(t, first.SessionID(), resumed.SessionID())

	summaries = service.Sessions()
	require.Len(t, summaries, 2)
	require.Equal(t, []string{"resumed", "work"}, []string{summaries[0].Name, summaries[1].Name})

	require.NoError(t, first.Close())
	summaries = service.Sessions()
	require.Len(t, summaries, 1)
	require.Equal(t, "resumed", summaries[0].Name)

	require.NoError(t, service.Close())
	require.Empty(t, service.Sessions())
	_, err = service.OpenSession(ctx, SessionOpenRequest{AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())}})
	require.ErrorContains(t, err, "service is closed")
}

func TestSessionSubscribePublishesCommandWorkflowAndCloseEvents(t *testing.T) {
	ctx := context.Background()
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder"}),
		app.WithDefaultAgent("coder"),
		app.WithWorkflows(workflow.Definition{Name: "noop_flow", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "test.echo"}}}}),
		app.WithActions(action.New(action.Spec{Name: "test.echo"}, func(action.Ctx, any) action.Result { return action.Result{Data: "ok"} })),
	)
	require.NoError(t, err)
	service := NewService(application)
	session, err := service.OpenSession(ctx, SessionOpenRequest{Name: "events", AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())}})
	require.NoError(t, err)
	events, cancel := session.Subscribe(4)
	defer cancel()

	_, err = session.ExecuteCommand(ctx, []string{"session", "info"}, nil)
	require.NoError(t, err)
	cmdEvent := <-events
	require.Equal(t, SessionEventCommand, cmdEvent.Type)
	require.Equal(t, "events", cmdEvent.SessionName)
	require.Equal(t, []string{"session", "info"}, cmdEvent.CommandPath)
	require.NoError(t, cmdEvent.Error)

	result := session.ExecuteWorkflow(ctx, "noop_flow", "hello")
	require.NoError(t, result.Error)
	workflowEvent := <-events
	require.Equal(t, SessionEventWorkflow, workflowEvent.Type)
	require.Equal(t, "noop_flow", workflowEvent.WorkflowName)
	require.NoError(t, workflowEvent.Error)

	require.NoError(t, session.Close())
	closeEvent := <-events
	require.Equal(t, SessionEventClosed, closeEvent.Type)
	_, ok := <-events
	require.False(t, ok)
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

func requireDescriptorPath(t *testing.T, descriptors []command.Descriptor, path ...string) command.Descriptor {
	t.Helper()
	for _, desc := range descriptors {
		if slices.Equal(desc.Path, path) {
			return desc
		}
		if found, ok := findDescriptorPath(desc.Subcommands, path...); ok {
			return found
		}
	}
	t.Fatalf("descriptor path %v not found", path)
	return command.Descriptor{}
}

func findDescriptorPath(descriptors []command.Descriptor, path ...string) (command.Descriptor, bool) {
	for _, desc := range descriptors {
		if slices.Equal(desc.Path, path) {
			return desc, true
		}
		if found, ok := findDescriptorPath(desc.Subcommands, path...); ok {
			return found, true
		}
	}
	return command.Descriptor{}, false
}

func renderCommandResult(t *testing.T, result command.Result) string {
	t.Helper()
	text, err := command.Render(result, command.DisplayTerminal)
	require.NoError(t, err)
	return text
}

func TestSessionThreadEventsExposesPersistedBranchEvents(t *testing.T) {
	ctx := context.Background()
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	service := NewService(application)
	session, err := service.OpenSession(ctx, SessionOpenRequest{Name: "coder", StoreDir: t.TempDir(), AgentOptions: []agent.Option{agent.WithClient(client)}})
	require.NoError(t, err)

	_, err = session.Send(ctx, "hello")
	require.NoError(t, err)
	events, ok, err := session.ThreadEvents(ctx)
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEmpty(t, events)
	require.True(t, slices.ContainsFunc(events, func(event thread.Event) bool {
		return event.Kind == thread.EventThreadCreated && event.SchemaVersion == thread.CurrentEventSchemaVersion
	}))
	require.True(t, slices.ContainsFunc(events, func(event thread.Event) bool {
		return event.Kind == "conversation.user_message" && event.SchemaVersion == thread.CurrentEventSchemaVersion
	}))
}
