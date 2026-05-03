package harness

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/stretchr/testify/require"
)

func TestLoadSessionCreatesDefaultHarnessSession(t *testing.T) {
	loaded, err := LoadSession(SessionLoadConfig{
		App: AppLoadConfig{DefaultAgent: "test"},
		AppOptions: []app.Option{
			app.WithAgentSpec(agent.Spec{Name: "test", System: "system"}),
		},
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})

	require.NoError(t, err)
	require.NotNil(t, loaded.App)
	require.NotNil(t, loaded.Agent)
	require.NotNil(t, loaded.Service)
	require.NotNil(t, loaded.Session)
	require.Equal(t, loaded.App, loaded.Session.App)
	require.Equal(t, loaded.Agent, loaded.Session.Agent)
}

func TestLoadSessionReturnsAppCreationError(t *testing.T) {
	loaded, err := LoadSession(SessionLoadConfig{})

	require.Nil(t, loaded)
	require.ErrorContains(t, err, "app: no default agent configured")
}
func TestLoadSessionRegistersSessionScopedActions(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("workflow answer"))
	loaded, err := LoadSession(SessionLoadConfig{
		App: AppLoadConfig{DefaultAgent: "test"},
		AppOptions: []app.Option{
			app.WithAgentSpec(agent.Spec{Name: "test", System: "system"}),
			app.WithWorkflows(workflow.Definition{Name: "ask_flow", Steps: []workflow.Step{{ID: "ask", Action: workflow.ActionRef{Name: agent.DefaultTurnActionName}}}}),
		},
		AgentOptions: []agent.Option{agent.WithClient(client)},
	})

	require.NoError(t, err)
	result := loaded.Session.ExecuteWorkflow(t.Context(), "ask_flow", "hello through workflow")

	require.NoError(t, result.Error)
	require.Equal(t, "workflow answer", result.Data.(workflow.Result).Data)
	require.Len(t, client.Requests(), 1)
}

func TestLoadSessionAppliesPlugins(t *testing.T) {
	loaded, err := LoadSession(SessionLoadConfig{
		App: AppLoadConfig{DefaultAgent: "plugin-agent"},
		Plugins: []app.Plugin{
			agentSpecsPlugin{specs: []agent.Spec{{Name: "plugin-agent", System: "from plugin"}}},
		},
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})

	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, "plugin-agent", loaded.Agent.Spec().Name)
}

func TestPrepareResolvedAgentSelectsAndAppliesOverrides(t *testing.T) {
	inference := agent.InferenceOptions{Model: "override/model", MaxTokens: 123}
	resolved := testResolution(agent.Spec{Name: "coder", System: "old"})

	selection, err := PrepareResolvedAgent(&resolved, "", AgentSpecOverrides{
		Inference:      inference,
		ApplyInference: true,
		MaxSteps:       7,
		ApplyMaxSteps:  true,
		System:         "new system",
	})

	require.NoError(t, err)
	require.Equal(t, "coder", selection.Name)
	require.Len(t, resolved.Bundle.AgentSpecs, 1)
	got := resolved.Bundle.AgentSpecs[0]
	require.Equal(t, inference, got.Inference)
	require.Equal(t, 7, got.MaxSteps)
	require.Equal(t, "new system", got.System)
}

func TestPrepareResolvedAgentReturnsSelectionError(t *testing.T) {
	resolved := testResolution(agent.Spec{Name: "coder"})

	selection, err := PrepareResolvedAgent(&resolved, "missing", AgentSpecOverrides{})

	require.ErrorContains(t, err, `agent "missing" not found`)
	require.Empty(t, selection.Name)
}

func testResolution(specs ...agent.Spec) agentdir.Resolution {
	return agentdir.Resolution{Bundle: resource.ContributionBundle{AgentSpecs: specs}}
}

func TestEnsureFallbackAgentAddsFallbackWhenNoAgents(t *testing.T) {
	resolved := agentdir.Resolution{}

	changed := EnsureFallbackAgent(&resolved, "", FallbackAgent{
		Enabled: true,
		Spec:    agent.Spec{Name: "default", System: "fallback"},
	})

	require.True(t, changed)
	require.Equal(t, "default", resolved.DefaultAgent)
	require.Equal(t, []agent.Spec{{Name: "default", System: "fallback"}}, resolved.Bundle.AgentSpecs)
}

func TestEnsureFallbackAgentSkipsWhenExplicitAgentRequested(t *testing.T) {
	resolved := agentdir.Resolution{}

	changed := EnsureFallbackAgent(&resolved, "coder", FallbackAgent{
		Enabled: true,
		Spec:    agent.Spec{Name: "default"},
	})

	require.False(t, changed)
	require.Empty(t, resolved.DefaultAgent)
	require.Empty(t, resolved.Bundle.AgentSpecs)
}

func TestEnsureFallbackAgentSkipsWhenAgentsExist(t *testing.T) {
	resolved := testResolution(agent.Spec{Name: "coder"})

	changed := EnsureFallbackAgent(&resolved, "", FallbackAgent{
		Enabled: true,
		Spec:    agent.Spec{Name: "default"},
	})

	require.False(t, changed)
	require.Empty(t, resolved.DefaultAgent)
	require.Equal(t, []agent.Spec{{Name: "coder"}}, resolved.Bundle.AgentSpecs)
}

func TestEnsureFallbackAgentSkipsWhenDisabled(t *testing.T) {
	resolved := agentdir.Resolution{}

	changed := EnsureFallbackAgent(&resolved, "", FallbackAgent{
		Enabled: false,
		Spec:    agent.Spec{Name: "default"},
	})

	require.False(t, changed)
	require.Empty(t, resolved.Bundle.AgentSpecs)
}

func TestResolvePluginsOrdersAndDedupesRefs(t *testing.T) {
	factory := &recordingPluginFactory{}
	plugins, err := ResolvePlugins(t.Context(), PluginLoadConfig{
		Factory: factory,
		Defaults: []agentdir.PluginRef{
			{Name: "default", Config: map[string]any{"from": "default"}},
			{Name: "  "},
		},
		Manifest: []agentdir.PluginRef{
			{Name: "manifest"},
			{Name: "default", Config: map[string]any{"from": "manifest"}},
		},
		Explicit: []agentdir.PluginRef{
			{Name: "explicit"},
			{Name: "manifest"},
		},
	})

	require.NoError(t, err)
	require.Len(t, plugins, 3)
	require.Equal(t, []string{"default", "manifest", "explicit"}, pluginNames(plugins))
	require.Equal(t, []string{"default", "manifest", "explicit"}, factory.names)
	require.Equal(t, "default", factory.configs[0]["from"])
}

func TestResolvePluginsSkipsNilFactoryWhenNoRefs(t *testing.T) {
	plugins, err := ResolvePlugins(t.Context(), PluginLoadConfig{})

	require.NoError(t, err)
	require.Empty(t, plugins)
}

func TestResolvePluginsRequiresFactoryForRefs(t *testing.T) {
	plugins, err := ResolvePlugins(t.Context(), PluginLoadConfig{
		Explicit: []agentdir.PluginRef{{Name: "plugin"}},
	})

	require.Nil(t, plugins)
	require.ErrorContains(t, err, "plugin factory is required")
}

func pluginNames(plugins []app.Plugin) []string {
	names := make([]string, 0, len(plugins))
	for _, plugin := range plugins {
		names = append(names, plugin.Name())
	}
	return names
}

type recordingPluginFactory struct {
	names   []string
	configs []map[string]any
}

func (f *recordingPluginFactory) PluginForName(_ context.Context, name string, config map[string]any) (app.Plugin, error) {
	f.names = append(f.names, name)
	f.configs = append(f.configs, config)
	return namedPlugin(name), nil
}

type namedPlugin string

func (p namedPlugin) Name() string { return string(p) }

func TestLoadSessionAppliesResumeSession(t *testing.T) {
	dir := t.TempDir()
	first, err := LoadSession(SessionLoadConfig{
		App:     AppLoadConfig{DefaultAgent: "test"},
		Session: SessionOpenConfig{StoreDir: dir},
		AppOptions: []app.Option{
			app.WithAgentSpec(agent.Spec{Name: "test", System: "system"}),
		},
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})
	require.NoError(t, err)
	require.NotEmpty(t, first.Agent.SessionStorePath())

	resumed, err := LoadSession(SessionLoadConfig{
		App: AppLoadConfig{DefaultAgent: "test"},
		Session: SessionOpenConfig{
			StoreDir: dir,
			Resume:   first.Agent.SessionStorePath(),
		},
		AppOptions: []app.Option{
			app.WithAgentSpec(agent.Spec{Name: "test", System: "system"}),
		},
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})

	require.NoError(t, err)
	require.Equal(t, first.Agent.SessionID(), resumed.Agent.SessionID())
	require.Equal(t, first.Agent.SessionStorePath(), resumed.Agent.SessionStorePath())
}

func TestLoadSessionAppliesModelPolicy(t *testing.T) {
	loaded, err := LoadSession(SessionLoadConfig{
		App: AppLoadConfig{DefaultAgent: "test"},
		Agent: AgentLoadConfig{
			ModelPolicy: agent.ModelPolicy{
				UseCase:      agent.ModelUseCaseAgenticCoding,
				ApprovedOnly: true,
			},
			ApplyModelPolicy: true,
		},
		AppOptions: []app.Option{
			app.WithAgentSpec(agent.Spec{Name: "test", System: "system"}),
		},
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})

	require.Nil(t, loaded)
	require.ErrorContains(t, err, "approved-only model policy requires auto mux routing")
}

func TestResolveAgentLoadConfigUsesResolvedModelPolicy(t *testing.T) {
	resolved := agentdir.Resolution{
		HasModelPolicy: true,
		ModelPolicy: agent.ModelPolicy{
			UseCase:      agent.ModelUseCaseAgenticCoding,
			ApprovedOnly: true,
		},
	}

	cfg := ResolveAgentLoadConfig(resolved, AgentLoadOverrides{})

	require.True(t, cfg.ApplyModelPolicy)
	require.Equal(t, agent.ModelUseCaseAgenticCoding, cfg.ModelPolicy.UseCase)
	require.True(t, cfg.ModelPolicy.ApprovedOnly)
}

func TestResolveAgentLoadConfigOverlaysCLIModelPolicy(t *testing.T) {
	resolved := agentdir.Resolution{
		HasModelPolicy: true,
		ModelPolicy: agent.ModelPolicy{
			UseCase:      agent.ModelUseCaseAgenticCoding,
			ApprovedOnly: true,
		},
	}

	cfg := ResolveAgentLoadConfig(resolved, AgentLoadOverrides{
		ModelPolicy: agent.ModelPolicy{
			AllowUntested: true,
			EvidencePath:  "evidence.json",
		},
		ApplyModelPolicy: true,
		SourceAPI:        adapt.ApiOpenAIChatCompletions,
		ApplySourceAPI:   true,
	})

	require.True(t, cfg.ApplyModelPolicy)
	require.Equal(t, agent.ModelUseCaseAgenticCoding, cfg.ModelPolicy.UseCase)
	require.True(t, cfg.ModelPolicy.ApprovedOnly)
	require.True(t, cfg.ModelPolicy.AllowUntested)
	require.Equal(t, "evidence.json", cfg.ModelPolicy.EvidencePath)
	require.Equal(t, adapt.ApiOpenAIChatCompletions, cfg.ModelPolicy.SourceAPI)
	require.Equal(t, adapt.ApiOpenAIChatCompletions, cfg.SourceAPI)
	require.True(t, cfg.ApplySourceAPI)
}

func TestLoadSessionAppliesSourceAPI(t *testing.T) {
	var got adapt.ApiKind
	loaded, err := LoadSession(SessionLoadConfig{
		App: AppLoadConfig{DefaultAgent: "test"},
		Agent: AgentLoadConfig{
			SourceAPI:      adapt.ApiOpenAIChatCompletions,
			ApplySourceAPI: true,
		},
		AppOptions: []app.Option{
			app.WithAgentSpec(agent.Spec{Name: "test", System: "system"}),
		},
		AgentOptions: []agent.Option{agent.WithAutoMux(func(opts adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
			got = opts.SourceAPI
			return adapterconfig.AutoResult{Client: runnertest.NewClient()}, nil
		})},
	})

	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, adapt.ApiOpenAIChatCompletions, got)
}

type agentSpecsPlugin struct {
	specs []agent.Spec
}

func (p agentSpecsPlugin) Name() string { return "agent-specs" }

func (p agentSpecsPlugin) AgentSpecs() []agent.Spec { return p.specs }
