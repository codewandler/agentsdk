package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
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

func TestAppExplicitSpecCanSelectOptionalStandardTools(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(WithAgentSpec(agent.Spec{
		Name:      "coder",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		Tools:     []string{"git_status", "web_search"},
	}), WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)

	_, err = app.InstantiateAgent("coder",
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
}

func TestAppDefaultSpecUsesDefaultToolsetNotFullCatalog(t *testing.T) {
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
	_, err = app.Send(context.Background(), "hello")
	require.NoError(t, err)

	var names []string
	for _, tool := range client.RequestAt(0).Tools {
		names = append(names, tool.Name)
	}
	require.Contains(t, names, "tools_list")
	require.NotContains(t, names, "git_status")
	require.True(t, slices.Contains(names, "web_fetch"))
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

func TestAppContextBuiltinExplainsWhenDefaultAgentHasNoRenderState(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(WithAgentSpec(agent.Spec{
		Name:      "main",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
	}), WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)

	_, err = app.InstantiateDefaultAgent(
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)

	result, err := app.Commands().Execute(context.Background(), "/context")
	require.NoError(t, err)
	require.Equal(t, command.ResultText, result.Kind)
	require.Contains(t, result.Text, "context: no render state yet for agent \"main\"")
	require.Contains(t, result.Text, "run a turn first to capture provider context")
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

func TestAppSkillBuiltinReportsAlreadyActiveDynamicSkill(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	app, err := New(WithAgentSpec(agent.Spec{
		Name:      "main",
		System:    "You code.",
		Inference: agent.InferenceOptions{Model: "test/model", MaxTokens: 1000},
		SkillSources: []skill.Source{skill.FSSource("skills", "skills", fstest.MapFS{
			"skills/architecture/SKILL.md": {Data: []byte("---\nname: architecture\ndescription: Architecture\n---\n# Architecture")},
		}, "skills", skill.SourceEmbedded, 0)},
	}), WithOutput(&bytes.Buffer{}))
	require.NoError(t, err)

	inst, err := app.InstantiateDefaultAgent(
		agent.WithClient(client),
		agent.WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
	_, err = inst.ActivateSkill("architecture")
	require.NoError(t, err)

	result, err := app.Commands().Execute(context.Background(), "/skill architecture")
	require.NoError(t, err)
	require.Contains(t, result.Text, "already active (dynamic)")
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
		WithOutput(&bytes.Buffer{}),
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
		WithOutput(&bytes.Buffer{}),
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
		WithOutput(&bytes.Buffer{}),
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
	result, err := app.Commands().Execute(context.Background(), "/context")
	require.NoError(t, err)
	require.Contains(t, result.Text, "plugin_git")
}

func TestPluginWithoutContextProvidersInterfaceIgnored(t *testing.T) {
	// A plugin that only implements CommandsPlugin should not contribute
	// context providers.
	app, err := New(
		WithPlugin(testCommandsPlugin{name: "cmds"}),
		WithOutput(&bytes.Buffer{}),
	)
	require.NoError(t, err)
	require.Empty(t, app.ContextProviders())
}

// ── Multi-facet plugin integration test ───────────────────────────────────

// testMultiFacetPlugin implements ToolsPlugin + ContextProvidersPlugin.
type testMultiFacetPlugin struct {
	name      string
	tools     []tool.Tool
	providers []agentcontext.Provider
}

func (p testMultiFacetPlugin) Name() string                                  { return p.name }
func (p testMultiFacetPlugin) Tools() []tool.Tool                            { return p.tools }
func (p testMultiFacetPlugin) ContextProviders() []agentcontext.Provider     { return p.providers }

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
		WithOutput(&bytes.Buffer{}),
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
		WithOutput(&bytes.Buffer{}),
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

	result, err := app.Commands().Execute(context.Background(), "/context")
	require.NoError(t, err)
	require.Contains(t, result.Text, "test_skills")
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
		WithOutput(&bytes.Buffer{}),
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

	result, err := app.Commands().Execute(context.Background(), "/context")
	require.NoError(t, err)
	require.Contains(t, result.Text, "test_skills")
}
