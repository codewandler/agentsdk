package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestAppRegistersBundleResources(t *testing.T) {
	bundle := resource.ContributionBundle{
		AgentSpecs: []agent.Spec{{Name: "coder", System: "You code.", Commands: []string{"review"}}},
		Commands: []command.Command{
			command.New(command.Spec{Name: "review"}, func(context.Context, command.Params) (command.Result, error) {
				return command.Text("review"), nil
			}),
		},
		SkillSources: []skill.Source{{ID: "test", Root: ".agents/skills"}},
	}
	app, err := New(WithResourceBundle(bundle), WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)
	require.Equal(t, []skill.Source{{ID: "test", Root: ".agents/skills"}}, app.SkillSources())
	spec, ok := app.AgentSpec("coder")
	require.True(t, ok)
	require.Equal(t, "You code.", spec.System)
	require.Equal(t, []string{"review"}, app.AgentCommandNames("coder"))
	_, ok = app.Commands().Get("review")
	require.True(t, ok)
}

func TestAppResourceBundleDuplicateAgentFirstWinsWithDiagnostic(t *testing.T) {
	app, err := New(
		WithResourceBundle(resource.ContributionBundle{AgentSpecs: []agent.Spec{{Name: "reviewer", System: "one"}}}),
		WithResourceBundle(resource.ContributionBundle{AgentSpecs: []agent.Spec{{Name: "reviewer", System: "two"}}}),
		WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	spec, ok := app.AgentSpec("reviewer")
	require.True(t, ok)
	require.Equal(t, "one", spec.System)
	require.Len(t, app.Diagnostics(), 1)
}

func TestPluginDuplicateCommandFirstWinsWithDiagnostic(t *testing.T) {
	app, err := New(
		WithCommand(command.New(command.Spec{Name: "review"}, func(context.Context, command.Params) (command.Result, error) {
			return command.Text("first"), nil
		})),
		WithPlugin(testCommandsPlugin{name: "plugin", commands: []command.Command{
			command.New(command.Spec{Name: "review"}, func(context.Context, command.Params) (command.Result, error) {
				return command.Text("second"), nil
			}),
		}}),
		WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	result, err := app.Commands().Execute(context.Background(), "/review")
	require.NoError(t, err)
	require.Equal(t, "first", result.Text)
	require.Len(t, app.Diagnostics(), 1)
}

func TestAppOwnsMarkdownCommandDispatch(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/commands/review.md": {Data: []byte("---\ndescription: Review\n---\nReview {{.Query}}")},
	}
	bundle, err := agentdir.LoadFS(fsys, ".")
	require.NoError(t, err)
	app, err := New(WithResourceBundle(bundle), WithoutBuiltins())
	require.NoError(t, err)

	result, err := app.Commands().Execute(context.Background(), "/review security")
	require.NoError(t, err)
	require.Equal(t, command.ResultAgentTurn, result.Kind)
	require.Equal(t, "Review security", result.Input)
}

func TestAppInstantiateAndSendRoutesToDefaultAgent(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(WithAgentSpec(agent.Spec{
		Name:      "coder",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}), WithOutput(&bytes.Buffer{}))
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

	contextResult, err := app.Commands().Execute(context.Background(), "/context")
	require.NoError(t, err)
	require.Contains(t, contextResult.Text, "provider: environment")
	require.Contains(t, contextResult.Text, "provider: time")
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
	}), WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)
	_, err = app.InstantiateAgent("coder", agent.WithClient(client), agent.WithWorkspace(t.TempDir()))
	require.NoError(t, err)

	_, err = app.Send(context.Background(), "first")
	require.NoError(t, err)
	_, err = app.Send(context.Background(), "second")
	require.NoError(t, err)

	require.Equal(t, 1, app.Tracker().AggregateTurn("1").Usage.Tokens.Count(unified.TokenKindInputNew))
	require.Equal(t, 2, app.Tracker().AggregateTurn("2").Usage.Tokens.Count(unified.TokenKindInputNew))
}

func TestAppProtectedBuiltinsCannotBeOverridden(t *testing.T) {
	_, err := New(WithCommand(command.New(command.Spec{Name: "help"}, nil)))
	require.Error(t, err)
	require.Contains(t, err.Error(), "already registered")
}

func TestAppHelpListsAgentsCommand(t *testing.T) {
	app, err := New(WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)

	result, err := app.Commands().Execute(context.Background(), "/help")
	require.NoError(t, err)
	require.Contains(t, result.Text, "/agents")
	require.Contains(t, result.Text, "Show available agents")
	require.Contains(t, result.Text, "/context")
	require.Contains(t, result.Text, "Show last context render state")
}

func TestAppContextBuiltinHandlesNoDefaultAgent(t *testing.T) {
	app, err := New(WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)

	result, err := app.Commands().Execute(context.Background(), "/context")
	require.NoError(t, err)
	require.Equal(t, command.ResultText, result.Kind)
	require.Equal(t, "context: no default agent", result.Text)
}

func TestAppAgentsBuiltinListsRegisteredAgents(t *testing.T) {
	app, err := New(
		WithAgentSpec(agent.Spec{Name: "reviewer", Description: "Reviews changes"}),
		WithAgentSpec(agent.Spec{Name: "main", Description: "Default assistant"}),
		WithDefaultAgent("main"),
		WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)

	result, err := app.Commands().Execute(context.Background(), "/agents")
	require.NoError(t, err)
	require.Equal(t, command.ResultText, result.Kind)
	require.Contains(t, result.Text, "Agents:")
	require.Contains(t, result.Text, "* main - Default assistant")
	require.Contains(t, result.Text, "  reviewer - Reviews changes")
}

func TestAppAgentsBuiltinHandlesNoAgents(t *testing.T) {
	app, err := New(WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)

	result, err := app.Commands().Execute(context.Background(), "/agents")
	require.NoError(t, err)
	require.Equal(t, "No agents registered.", result.Text)
}

func TestAppResourceBundleCannotOverrideProtectedBuiltins(t *testing.T) {
	_, err := New(
		WithResourceBundle(resource.ContributionBundle{
			Commands: []command.Command{command.New(command.Spec{Name: "help"}, nil)},
		}),
		WithOutput(&bytes.Buffer{}),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already registered")
}

func TestAppPluginCannotOverrideProtectedBuiltins(t *testing.T) {
	_, err := New(
		WithPlugin(testCommandsPlugin{name: "plugin", commands: []command.Command{
			command.New(command.Spec{Name: "exit"}, nil),
		}}),
		WithOutput(&bytes.Buffer{}),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "already registered")
}

func TestAppCommandResultAgentTurnRoutesToDefaultAgent(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(
		WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000}}),
		WithCommand(command.New(command.Spec{Name: "ask"}, func(context.Context, command.Params) (command.Result, error) {
			return command.AgentTurn("expanded"), nil
		})),
		WithOutput(&bytes.Buffer{}),
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
		WithCommand(command.New(command.Spec{
			Name:   "agent_only",
			Policy: command.Policy{AgentCallable: true},
		}, func(context.Context, command.Params) (command.Result, error) {
			return command.Text("no"), nil
		})),
		WithOutput(&bytes.Buffer{}),
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
		WithCommand(command.New(command.Spec{
			Name:   "review",
			Policy: command.Policy{AgentCallable: true},
		}, nil)),
		WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	require.Empty(t, app.AgentCommandView("coder").AgentCommands())

	app, err = New(
		WithAgentSpec(agent.Spec{Name: "coder", Commands: []string{"review"}}),
		WithCommand(command.New(command.Spec{
			Name:   "review",
			Policy: command.Policy{AgentCallable: true},
		}, nil)),
		WithOutput(&bytes.Buffer{}),
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
		WithOutput(&bytes.Buffer{}),
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
