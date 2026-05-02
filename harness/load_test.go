package harness

import (
	"testing"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/stretchr/testify/require"
)

func TestLoadSessionCreatesDefaultHarnessSession(t *testing.T) {
	loaded, err := LoadSession(SessionLoadConfig{
		DefaultAgent: "test",
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
