package app

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/datasource"
	"github.com/codewandler/agentsdk/plugins/plannerplugin"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/thread"
	threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/git"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
	"github.com/codewandler/agentsdk/tools/web"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestAppRegistersBundleResources(t *testing.T) {
	bundle := resource.ContributionBundle{
		AgentSpecs: []agent.Spec{{Name: "coder", System: "You code.", Commands: []string{"review"}}},
		Commands: []command.Command{
			command.New(command.Descriptor{Name: "review"}, func(context.Context, command.Params) (command.Result, error) {
				return command.Text("review"), nil
			}),
		},
		SkillSources: []skill.Source{{ID: "test", Root: ".agents/skills"}},
	}
	app, err := New(WithResourceBundle(bundle))
	require.NoError(t, err)
	require.Equal(t, []skill.Source{{ID: "test", Root: ".agents/skills"}}, app.SkillSources())
	spec, ok := app.AgentSpec("coder")
	require.True(t, ok)
	require.Equal(t, "You code.", spec.System)
	require.Equal(t, []string{"review"}, spec.Commands)
	_, ok = app.Commands().Get("review")
	require.True(t, ok)
}

func TestAppRegistersDatasourceWorkflowActionResources(t *testing.T) {
	actionDef := action.New(action.Spec{Name: "docs.search"}, func(action.Ctx, any) action.Result {
		return action.Result{Data: "ok"}
	})
	app, err := New(
		WithActions(actionDef),
		WithDataSources(datasource.Definition{
			Name:    "docs",
			Kind:    datasource.KindCorpus,
			Actions: datasource.Actions{Search: action.Ref{Name: "docs.search"}},
		}),
		WithWorkflows(workflow.Definition{
			Name:  "search_docs",
			Steps: []workflow.Step{{ID: "search", Action: workflow.ActionRef{Name: "docs.search"}}},
		}),
	)
	require.NoError(t, err)
	require.Equal(t, []action.Action{actionDef}, app.Actions())

	ds, ok := app.DataSource("docs")
	require.True(t, ok)
	require.Equal(t, datasource.KindCorpus, ds.Kind)
	require.Equal(t, []datasource.Definition{ds}, app.DataSources())

	wf, ok := app.Workflow("search_docs")
	require.True(t, ok)
	require.Equal(t, []workflow.Definition{wf}, app.Workflows())
}

func TestAppRegistersBundleDatasourceAndWorkflowContributions(t *testing.T) {
	bundle := resource.ContributionBundle{
		DataSources: []resource.DataSourceContribution{{
			Name:        "docs",
			Description: "Documentation corpus",
			Kind:        "corpus",
			Metadata:    map[string]any{"owner": "docs-team"},
		}},
		Workflows: []resource.WorkflowContribution{{
			Name:        "sync_docs",
			Description: "Sync documentation",
		}},
	}
	app, err := New(WithResourceBundle(bundle))
	require.NoError(t, err)

	ds, ok := app.DataSource("docs")
	require.True(t, ok)
	require.Equal(t, datasource.KindCorpus, ds.Kind)
	require.Equal(t, "docs-team", ds.Metadata["owner"])

	wf, ok := app.Workflow("sync_docs")
	require.True(t, ok)
	require.Equal(t, "Sync documentation", wf.Description)
}

func TestAppExecutesRegisteredWorkflow(t *testing.T) {
	app, err := New(
		WithActions(
			action.New(action.Spec{Name: "upper"}, func(_ action.Ctx, input any) action.Result {
				return action.Result{Data: strings.ToUpper(input.(string))}
			}),
			action.New(action.Spec{Name: "suffix"}, func(_ action.Ctx, input any) action.Result {
				return action.Result{Data: input.(string) + "!"}
			}),
		),
		WithWorkflows(workflow.Definition{Name: "shout", Steps: []workflow.Step{
			{ID: "upper", Action: workflow.ActionRef{Name: "upper"}},
			{ID: "suffix", Action: workflow.ActionRef{Name: "suffix"}, DependsOn: []string{"upper"}},
		}}),
	)
	require.NoError(t, err)

	result := app.ExecuteWorkflow(context.Background(), "shout", "hello")
	require.NoError(t, result.Error)
	require.Equal(t, "HELLO!", result.Data.(workflow.Result).Data)
}

func TestAppExecuteWorkflowAcceptsExecutionOptions(t *testing.T) {
	app, err := New(
		WithActions(action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
			return action.Result{Data: input}
		})),
		WithWorkflows(workflow.Definition{Name: "echo_flow", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo"}}}}),
	)
	require.NoError(t, err)

	var events []action.Event
	result := app.ExecuteWorkflow(context.Background(), "echo_flow", "hi",
		WithWorkflowRunID("run_fixed"),
		WithWorkflowEventHandler(func(_ action.Ctx, event action.Event) {
			events = append(events, event)
		}),
	)
	require.NoError(t, result.Error)
	require.Equal(t, workflow.RunID("run_fixed"), result.Data.(workflow.Result).RunID)
	require.Len(t, events, 4)
	started := events[0].(workflow.Started)
	require.Equal(t, workflow.RunID("run_fixed"), started.RunID)
	require.Equal(t, "echo_flow", started.WorkflowName)
	require.False(t, started.At.IsZero())
	stepStarted := events[1].(workflow.StepStarted)
	require.Equal(t, workflow.RunID("run_fixed"), stepStarted.RunID)
	require.Equal(t, "echo", stepStarted.StepID)
	require.False(t, stepStarted.At.IsZero())
	stepCompleted := events[2].(workflow.StepCompleted)
	require.Equal(t, workflow.RunID("run_fixed"), stepCompleted.RunID)
	require.Equal(t, workflow.InlineValue("hi"), stepCompleted.Output)
	require.False(t, stepCompleted.At.IsZero())
	completed := events[3].(workflow.Completed)
	require.Equal(t, workflow.RunID("run_fixed"), completed.RunID)
	require.Equal(t, workflow.InlineValue("hi"), completed.Output)
	require.False(t, completed.At.IsZero())
}

func TestAppWorkflowActionAcceptsExecutionOptions(t *testing.T) {
	app, err := New(
		WithActions(action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
			return action.Result{Data: input}
		})),
		WithWorkflows(workflow.Definition{Name: "echo_flow", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo"}}}}),
	)
	require.NoError(t, err)

	wfAction, ok := app.WorkflowAction("echo_flow", WithWorkflowRunID("run_action"))
	require.True(t, ok)
	result := wfAction.Execute(context.Background(), "hi")
	require.NoError(t, result.Error)
	require.Equal(t, workflow.RunID("run_action"), result.Data.(workflow.Result).RunID)
}

func TestAppAgentTurnActionCanBackWorkflow(t *testing.T) {
	ctx := context.Background()
	client := runnertest.NewClient(runnertest.TextStream("model says hi"))
	app, err := New(
		WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		WithWorkflows(workflow.Definition{Name: "ask_flow", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}}}}),
	)
	require.NoError(t, err)
	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
	turnAction, err := app.DefaultAgentTurnAction(action.Spec{Name: "ask_agent", Description: "Ask the default agent"})
	require.NoError(t, err)
	require.NoError(t, app.RegisterActions(turnAction))

	result := app.ExecuteWorkflow(ctx, "ask_flow", "say hi")

	require.NoError(t, result.Error)
	require.Equal(t, "model says hi", result.Data.(workflow.Result).Data)
	require.Len(t, client.Requests(), 1)
	requireAppRequestContainsText(t, client.RequestAt(0), "say hi")
}
func TestWorkflowResourceCanUseDefaultAgentTurnAction(t *testing.T) {
	ctx := context.Background()
	bundle, err := agentdir.LoadDir("../testdata/workflow-app")
	require.NoError(t, err)
	client := runnertest.NewClient(runnertest.TextStream("resource workflow response"))
	app, err := New(
		WithResourceBundle(bundle),
		WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
		agent.WithSessionStoreDir(t.TempDir()),
	)
	require.NoError(t, err)
	turnAction, err := app.DefaultAgentTurnAction(action.Spec{Name: "ask_agent"})
	require.NoError(t, err)
	require.NoError(t, app.RegisterActions(turnAction))

	def, ok := app.Workflow("ask_agent_flow")
	require.True(t, ok)
	require.Len(t, def.Steps, 1)
	require.Equal(t, "ask", def.Steps[0].ID)
	require.Equal(t, "ask_agent", def.Steps[0].Action.Name)

	result := app.ExecuteWorkflow(ctx, "ask_agent_flow", "answer from resource workflow", WithWorkflowRunID("run_resource_workflow"))
	require.NoError(t, result.Error)
	require.Equal(t, "resource workflow response", result.Data.(workflow.Result).Data)
	require.Len(t, client.Requests(), 1)
	requireAppRequestContainsText(t, client.RequestAt(0), "answer from resource workflow")
}

func TestAppExecuteWorkflowDoesNotRecordToDefaultAgentLiveThread(t *testing.T) {
	ctx := context.Background()
	app, err := New(
		WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		WithActions(action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
			return action.Result{Data: input}
		})),
		WithWorkflows(workflow.Definition{Name: "echo_flow", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo"}}}}),
	)
	require.NoError(t, err)
	inst, err := app.InstantiateAgent("coder",
		agent.WithClient(runnertest.NewClient(runnertest.TextStream("ok"))),
		agent.WithWorkspace(t.TempDir()),
		agent.WithSessionStoreDir(t.TempDir()),
	)
	require.NoError(t, err)
	require.NotNil(t, inst.LiveThread())

	var handled []action.Event
	result := app.ExecuteWorkflow(ctx, "echo_flow", "hi",
		WithWorkflowRunID("run_thread"),
		WithWorkflowEventHandler(func(_ action.Ctx, event action.Event) {
			handled = append(handled, event)
		}),
	)
	require.NoError(t, result.Error)
	require.NotEmpty(t, handled)

	store := threadjsonlstore.Open(filepath.Dir(inst.SessionStorePath()))
	_, ok, err := (workflow.ThreadRunStore{Store: store, ThreadID: thread.ID(inst.SessionID())}).State(ctx, "run_thread")
	require.NoError(t, err)
	require.False(t, ok)
}

func TestAppWorkflowActionExposesRegisteredWorkflow(t *testing.T) {
	app, err := New(
		WithActions(action.New(action.Spec{Name: "echo"}, func(_ action.Ctx, input any) action.Result {
			return action.Result{Data: input}
		})),
		WithWorkflows(workflow.Definition{Name: "echo_flow", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo"}}}}),
	)
	require.NoError(t, err)

	wfAction, ok := app.WorkflowAction("echo_flow")
	require.True(t, ok)
	require.Equal(t, "echo_flow", wfAction.Spec().Name)
	result := wfAction.Execute(context.Background(), "hi")
	require.NoError(t, result.Error)
	require.Equal(t, "hi", result.Data.(workflow.Result).Data)
}

func TestAppWorkflowExecutionReportsMissingWorkflowAndAction(t *testing.T) {
	app, err := New(
		WithWorkflows(workflow.Definition{Name: "missing_action", Steps: []workflow.Step{{ID: "missing", Action: workflow.ActionRef{Name: "missing"}}}}),
	)
	require.NoError(t, err)

	require.ErrorContains(t, app.ExecuteWorkflow(context.Background(), "nope", nil).Error, "workflow \"nope\" not found")
	require.ErrorContains(t, app.ExecuteWorkflow(context.Background(), "missing_action", nil).Error, "action \"missing\" not found")
	_, ok := app.WorkflowAction("nope")
	require.False(t, ok)
}

func TestAppResourceBundleDuplicateAgentFirstWinsWithDiagnostic(t *testing.T) {
	app, err := New(
		WithResourceBundle(resource.ContributionBundle{AgentSpecs: []agent.Spec{{Name: "reviewer", System: "one"}}}),
		WithResourceBundle(resource.ContributionBundle{AgentSpecs: []agent.Spec{{Name: "reviewer", System: "two"}}}),
	)
	require.NoError(t, err)
	spec, ok := app.AgentSpec("reviewer")
	require.True(t, ok)
	require.Equal(t, "one", spec.System)
	require.Len(t, app.Diagnostics(), 1)
}

func TestPluginDuplicateCommandFirstWinsWithDiagnostic(t *testing.T) {
	app, err := New(
		WithCommand(command.New(command.Descriptor{Name: "review"}, func(context.Context, command.Params) (command.Result, error) {
			return command.Text("first"), nil
		})),
		WithPlugin(testCommandsPlugin{name: "plugin", commands: []command.Command{
			command.New(command.Descriptor{Name: "review"}, func(context.Context, command.Params) (command.Result, error) {
				return command.Text("second"), nil
			}),
		}}),
	)
	require.NoError(t, err)
	result, err := app.Commands().Execute(context.Background(), "/review")
	require.NoError(t, err)
	require.Equal(t, "first", renderCommandResult(t, result))
	require.Len(t, app.Diagnostics(), 1)
}

func TestAppOwnsMarkdownCommandDispatch(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/commands/review.md": {Data: []byte("---\ndescription: Review\n---\nReview {{.Query}}")},
	}
	bundle, err := agentdir.LoadFS(fsys, ".")
	require.NoError(t, err)
	app, err := New(WithResourceBundle(bundle))
	require.NoError(t, err)

	result, err := app.Commands().Execute(context.Background(), "/review security")
	require.NoError(t, err)
	require.Equal(t, command.ResultAgentTurn, result.Kind)
	require.Equal(t, "Review security", agentTurnInput(t, result))
}

func TestAppInstantiateAndSendRoutesToDefaultAgent(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(WithAgentSpec(agent.Spec{
		Name:      "coder",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}))
	require.NoError(t, err)
	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)

	result, err := app.Send(context.Background(), "hello")
	require.NoError(t, err)
	require.Equal(t, command.ResultHandled, result.Kind)
	require.Len(t, client.Requests(), 1)

	inst, ok := app.DefaultAgent()
	require.True(t, ok)
	require.Contains(t, inst.ContextState(), "provider: environment")
	require.Contains(t, inst.ContextState(), "provider: time")
}

func TestAppExplicitSpecCanSelectOptionalLocalCLITools(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(
		WithAgentSpec(agent.Spec{
			Name:      "coder",
			System:    "You code.",
			Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
			Tools:     []string{"git_status", "web_search"},
		}),
		WithPlugin(testToolCatalogPlugin{}),
	)
	require.NoError(t, err)

	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
}

func TestAppDefaultSpecUsesConfiguredLocalCLIPluginTools(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(
		WithAgentSpec(agent.Spec{
			Name:      "coder",
			System:    "You code.",
			Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		}),
		WithPlugin(testToolCatalogPlugin{}),
	)
	require.NoError(t, err)

	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
	_, err = app.Send(context.Background(), "hello")
	require.NoError(t, err)

	var names []string
	for _, tool := range client.RequestAt(0).Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "tools_list")
	require.Contains(t, names, "git_status")
	require.Contains(t, names, "git_add")
	require.Contains(t, names, "git_commit")
	require.True(t, slices.Contains(names, "web_fetch"))
}

func TestAppPluginCapabilityFactoriesConfigureAgentCapabilities(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(
		WithAgentSpec(agent.Spec{
			Name:      "planner_agent",
			System:    "Use the plan tool.",
			Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
			Capabilities: []capability.AttachSpec{{
				CapabilityName: planner.CapabilityName,
				InstanceID:     "default",
			}},
		}),
		WithPlugin(plannerplugin.New()),
	)
	require.NoError(t, err)

	_, err = app.InstantiateAgent("planner_agent",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
	_, err = app.Send(context.Background(), "hello")
	require.NoError(t, err)

	var names []string
	for _, tool := range client.RequestAt(0).Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "plan")
}
func TestAppSendAdvancesTurnUsageIDs(t *testing.T) {
	client := runnertest.NewClient(
		[]unified.Event{
			unified.UsageEvent{Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 1}}},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
		},
		[]unified.Event{
			unified.UsageEvent{Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 2}}},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop},
		},
	)
	app, err := New(WithAgentSpec(agent.Spec{
		Name:      "coder",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}))
	require.NoError(t, err)
	_, err = app.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)

	_, err = app.Send(context.Background(), "first")
	require.NoError(t, err)
	_, err = app.Send(context.Background(), "second")
	require.NoError(t, err)

	inst, ok := app.DefaultAgent()
	require.True(t, ok)
	require.Equal(t, 1, inst.Tracker().AggregateTurn("1").Usage.Tokens.Count(unified.TokenKindInputNew))
	require.Equal(t, 2, inst.Tracker().AggregateTurn("2").Usage.Tokens.Count(unified.TokenKindInputNew))
}

func TestAppCommandResultAgentTurnRoutesToDefaultAgent(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(
		WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		WithCommand(command.New(command.Descriptor{Name: "ask"}, func(context.Context, command.Params) (command.Result, error) {
			return command.AgentTurn("expanded"), nil
		})),
	)
	require.NoError(t, err)
	_, err = app.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)

	result, err := app.Send(context.Background(), "/ask")
	require.NoError(t, err)
	require.Equal(t, command.ResultHandled, result.Kind)
	require.Len(t, client.Requests(), 1)
}

func TestAppSendRejectsAgentOnlyCommandsFromUserInput(t *testing.T) {
	app, err := New(
		WithCommand(command.New(command.Descriptor{
			Name:   "agent_only",
			Policy: command.Policy{AgentCallable: true},
		}, func(context.Context, command.Params) (command.Result, error) {
			return command.Text("no"), nil
		})),
	)
	require.NoError(t, err)

	_, err = app.Send(context.Background(), "/agent_only")
	var notCallable command.ErrNotCallable
	require.ErrorAs(t, err, &notCallable)
	require.Equal(t, "user", notCallable.Caller)
}

func TestAgentCommandViewRequiresExplicitAgentCommandSelection(t *testing.T) {
	app, err := New(
		WithAgentSpec(agent.Spec{Name: "coder"}),
		WithCommand(command.New(command.Descriptor{
			Name:   "review",
			Policy: command.Policy{AgentCallable: true},
		}, nil)),
	)
	require.NoError(t, err)
	require.Empty(t, app.AgentCommandView("coder").AgentCommands())

	app, err = New(
		WithAgentSpec(agent.Spec{Name: "coder", Commands: []string{"review"}}),
		WithCommand(command.New(command.Descriptor{
			Name:   "review",
			Policy: command.Policy{AgentCallable: true},
		}, nil)),
	)
	require.NoError(t, err)
	require.Len(t, app.AgentCommandView("coder").AgentCommands(), 1)
}

func TestAppDiscoversDefaultSkillSources(t *testing.T) {
	workspace := t.TempDir()
	home := t.TempDir()
	writeAppFile(t, filepath.Join(workspace, ".claude", "skills", "project", "SKILL.md"), "---\nname: project-skill\ndescription: Project skill\n---\n# Project")
	writeAppFile(t, filepath.Join(home, ".agents", "skills", "home", "SKILL.md"), "---\nname: home-skill\ndescription: Home skill\n---\n# Home")
	client := runnertest.NewClient(runnertest.TextStream("ok"))

	app, err := New(
		WithAgentSpec(agent.Spec{
			Name:      "coder",
			System:    "Base",
			Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
			Skills:    []string{"project-skill", "home-skill"},
		}),
		WithDefaultSkillSourceDiscovery(SkillSourceDiscovery{WorkspaceDir: workspace, HomeDir: home, IncludeGlobalUserResources: true}),
	)
	require.NoError(t, err)
	inst, err := app.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(workspace))
	require.NoError(t, err)

	require.Equal(t, []string{"project-skill", "home-skill"}, inst.SkillRepository().LoadedNames())
	require.Contains(t, inst.MaterializedSystem(), "# Project")
	require.Contains(t, inst.MaterializedSystem(), "# Home")
	require.Len(t, inst.SkillRepository().Sources(), 4)
}

func TestAgentSpecSkillSourcesStayScopedToAgent(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md": {Data: []byte("---\nname: coder-skill\ndescription: Coder skill\n---\n# Coder")},
	}
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(WithAgentSpec(agent.Spec{
		Name:         "coder",
		Inference:    agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		Skills:       []string{"coder-skill"},
		SkillSources: []skill.Source{skill.FSSource("spec", "spec", fsys, "skills", skill.SourceEmbedded, 0)},
	}))
	require.NoError(t, err)
	require.Empty(t, app.SkillSources())

	inst, err := app.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)
	require.Equal(t, []string{"coder-skill"}, inst.SkillRepository().LoadedNames())
}

func writeAppFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

type testCommandsPlugin struct {
	name     string
	commands []command.Command
}

func (p testCommandsPlugin) Name() string { return p.name }

func (p testCommandsPlugin) Commands() []command.Command {
	return append([]command.Command(nil), p.commands...)
}

// ── ContextProvidersPlugin tests ─────────────────────────────────────────────

type testContextProvidersPlugin struct {
	name      string
	providers []agentcontext.Provider
}

func (p testContextProvidersPlugin) Name() string { return p.name }
func (p testContextProvidersPlugin) ContextProviders() []agentcontext.Provider {
	return append([]agentcontext.Provider(nil), p.providers...)
}

type stubProvider struct {
	key agentcontext.ProviderKey
}

func (p stubProvider) Key() agentcontext.ProviderKey { return p.key }
func (p stubProvider) GetContext(context.Context, agentcontext.Request) (agentcontext.ProviderContext, error) {
	return agentcontext.ProviderContext{}, nil
}

func TestPluginContextProvidersCollected(t *testing.T) {
	prov := stubProvider{key: "test_ctx"}
	app, err := New(
		WithPlugin(testContextProvidersPlugin{
			name:      "test",
			providers: []agentcontext.Provider{prov},
		}),
	)
	require.NoError(t, err)
	require.Len(t, app.ContextProviders(), 1)
	require.Equal(t, agentcontext.ProviderKey("test_ctx"), app.ContextProviders()[0].Key())
}

func TestPluginContextProvidersMultiplePlugins(t *testing.T) {
	app, err := New(
		WithPlugin(testContextProvidersPlugin{
			name:      "alpha",
			providers: []agentcontext.Provider{stubProvider{key: "alpha_ctx"}},
		}),
		WithPlugin(testContextProvidersPlugin{
			name:      "beta",
			providers: []agentcontext.Provider{stubProvider{key: "beta_ctx"}},
		}),
	)
	require.NoError(t, err)
	require.Len(t, app.ContextProviders(), 2)
	keys := make([]agentcontext.ProviderKey, len(app.ContextProviders()))
	for i, p := range app.ContextProviders() {
		keys[i] = p.Key()
	}
	require.Contains(t, keys, agentcontext.ProviderKey("alpha_ctx"))
	require.Contains(t, keys, agentcontext.ProviderKey("beta_ctx"))
}

func TestPluginContextProvidersForwardedToAgent(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	prov := stubProvider{key: "plugin_git"}
	app, err := New(
		WithPlugin(testContextProvidersPlugin{
			name:      "git",
			providers: []agentcontext.Provider{prov},
		}),
		WithAgentSpec(agent.Spec{
			Name:      "coder",
			System:    "You code.",
			Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		}),
	)
	require.NoError(t, err)

	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)

	// Run a turn and verify the plugin provider's context is included.
	_, err = app.Send(context.Background(), "hello")
	require.NoError(t, err)

	// The context state should mention the plugin provider key.
	inst, ok := app.DefaultAgent()
	require.True(t, ok)
	require.Contains(t, inst.ContextState(), "plugin_git")
}

func TestPluginWithoutContextProvidersInterfaceIgnored(t *testing.T) {
	// A plugin that only implements CommandsPlugin should not contribute
	// context providers.
	app, err := New(
		WithPlugin(testCommandsPlugin{name: "cmds"}),
	)
	require.NoError(t, err)
	require.Empty(t, app.ContextProviders())
}

// ── Multi-facet plugin integration test ───────────────────────────────────

type testDWAPlugin struct {
	name        string
	actions     []action.Action
	datasources []datasource.Definition
	workflows   []workflow.Definition
}

func (p testDWAPlugin) Name() string { return p.name }
func (p testDWAPlugin) Actions() []action.Action {
	return append([]action.Action(nil), p.actions...)
}
func (p testDWAPlugin) DataSources() []datasource.Definition {
	return append([]datasource.Definition(nil), p.datasources...)
}
func (p testDWAPlugin) Workflows() []workflow.Definition {
	return append([]workflow.Definition(nil), p.workflows...)
}

func TestPluginRegistersActionsDataSourcesAndWorkflows(t *testing.T) {
	actionDef := action.New(action.Spec{Name: "docs.fetch"}, func(action.Ctx, any) action.Result { return action.Result{} })
	app, err := New(
		WithPlugin(testDWAPlugin{
			name:        "docs",
			actions:     []action.Action{actionDef},
			datasources: []datasource.Definition{{Name: "docs", Kind: datasource.KindCorpus, Actions: datasource.Actions{Fetch: action.Ref{Name: "docs.fetch"}}}},
			workflows:   []workflow.Definition{{Name: "fetch_docs", Steps: []workflow.Step{{ID: "fetch", Action: workflow.ActionRef{Name: "docs.fetch"}}}}},
		}),
	)
	require.NoError(t, err)
	require.Equal(t, []action.Action{actionDef}, app.Actions())
	_, ok := app.DataSource("docs")
	require.True(t, ok)
	_, ok = app.Workflow("fetch_docs")
	require.True(t, ok)
}

// testMultiFacetPlugin implements ToolsPlugin + ContextProvidersPlugin.
type testMultiFacetPlugin struct {
	name      string
	tools     []tool.Tool
	providers []agentcontext.Provider
}

func (p testMultiFacetPlugin) Name() string                              { return p.name }
func (p testMultiFacetPlugin) Tools() []tool.Tool                        { return p.tools }
func (p testMultiFacetPlugin) ContextProviders() []agentcontext.Provider { return p.providers }

func TestMultiFacetPluginRegistersToolsAndContextProviders(t *testing.T) {
	dummyTool := tool.New("multi_tool", "A multi-facet tool", func(tool.Ctx, struct{}) (tool.Result, error) {
		return tool.NewResult().Text("ok").Build(), nil
	})
	prov := stubProvider{key: "multi_ctx"}

	app, err := New(
		WithPlugin(testMultiFacetPlugin{
			name:      "multi",
			tools:     []tool.Tool{dummyTool},
			providers: []agentcontext.Provider{prov},
		}),
	)
	require.NoError(t, err)

	// Context providers should be collected.
	require.Len(t, app.ContextProviders(), 1)
	require.Equal(t, agentcontext.ProviderKey("multi_ctx"), app.ContextProviders()[0].Key())

	// Tool should be registered in the catalog.
	selected, err := app.ToolCatalog().Select([]string{"multi_tool"})
	require.NoError(t, err)
	require.Len(t, selected, 1)
	require.Equal(t, "multi_tool", selected[0].Name())
}

// ── AgentContextPlugin tests ──────────────────────────────────────────────

type testAgentContextPlugin struct {
	name string
	key  agentcontext.ProviderKey
}

func (p testAgentContextPlugin) Name() string { return p.name }
func (p testAgentContextPlugin) AgentContextProviders(info AgentContextInfo) []agentcontext.Provider {
	if info.SkillRepository == nil {
		return nil
	}
	return []agentcontext.Provider{stubProvider{key: p.key}}
}

func TestAgentContextPluginForwardedToAgent(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(
		WithPlugin(testAgentContextPlugin{name: "skill_ctx", key: "test_skills"}),
		WithAgentSpec(agent.Spec{
			Name:      "coder",
			System:    "You code.",
			Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		}),
	)
	require.NoError(t, err)

	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)

	// Run a turn and verify the agent-scoped provider is present.
	_, err = app.Send(context.Background(), "hello")
	require.NoError(t, err)

	inst, ok := app.DefaultAgent()
	require.True(t, ok)
	require.Contains(t, inst.ContextState(), "test_skills")
}

func TestAgentContextPluginSkillRepoAlwaysAvailable(t *testing.T) {
	// Even without explicit skill sources, the agent creates an empty skill
	// repo during initSkills. The factory always receives a non-nil repo.
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(
		WithPlugin(testAgentContextPlugin{name: "skill_ctx", key: "test_skills"}),
		WithAgentSpec(agent.Spec{
			Name:      "coder",
			System:    "You code.",
			Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		}),
	)
	require.NoError(t, err)

	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)

	// Run a turn — the plugin should contribute a provider because the
	// agent always creates a skill repo (even if empty).
	_, err = app.Send(context.Background(), "hello")
	require.NoError(t, err)

	inst, ok := app.DefaultAgent()
	require.True(t, ok)
	require.Contains(t, inst.ContextState(), "test_skills")
}

func requireAppRequestContainsText(t *testing.T, req unified.Request, want string) {
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

func agentTurnInput(t *testing.T, result command.Result) string {
	t.Helper()
	input, ok := command.AgentTurnInput(result)
	require.True(t, ok)
	return input
}

type testToolCatalogPlugin struct{}

func (testToolCatalogPlugin) Name() string { return "test_tools" }

func (testToolCatalogPlugin) DefaultTools() []tool.Tool {
	var tools []tool.Tool
	tools = append(tools, git.Tools()...)
	tools = append(tools, web.Tools(nil)...)
	tools = append(tools, toolmgmt.Tools()...)
	return tools
}

func (testToolCatalogPlugin) CatalogTools() []tool.Tool {
	var tools []tool.Tool
	tools = append(tools, git.Tools()...)
	tools = append(tools, web.SearchTool(nil))
	tools = append(tools, toolmgmt.Tools()...)
	return tools
}
