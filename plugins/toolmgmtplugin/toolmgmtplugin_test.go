package toolmgmtplugin

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

// Compile-time interface assertions.
var (
	_ app.Plugin             = (*Plugin)(nil)
	_ app.ToolsPlugin        = (*Plugin)(nil)
	_ app.AgentContextPlugin = (*Plugin)(nil)
)

func TestPluginName(t *testing.T) {
	p := New()
	require.Equal(t, "toolmgmt", p.Name())
}

func TestPluginToolsReturnsToolMgmtTools(t *testing.T) {
	p := New()
	tools := p.Tools()
	require.Len(t, tools, 3)
	names := make([]string, len(tools))
	for i, tl := range tools {
		names[i] = tl.Name()
	}
	require.Contains(t, names, "tools_list")
	require.Contains(t, names, "tools_activate")
	require.Contains(t, names, "tools_deactivate")
}

func TestPluginAgentContextProvidersWithActiveTools(t *testing.T) {
	p := New()
	dummyTool := tool.New("dummy", "A dummy tool", func(tool.Ctx, struct{}) (tool.Result, error) {
		return tool.NewResult().Text("ok").Build(), nil
	})
	providers := p.AgentContextProviders(app.AgentContextInfo{
		ActiveTools: func() []tool.Tool { return []tool.Tool{dummyTool} },
	})
	require.Len(t, providers, 1)
	require.Equal(t, agentcontext.ProviderKey("tools"), providers[0].Key())
}

func TestPluginAgentContextProvidersNilActiveTools(t *testing.T) {
	p := New()
	providers := p.AgentContextProviders(app.AgentContextInfo{})
	require.Empty(t, providers)
}

func TestPluginContextProviderKeyIsTools(t *testing.T) {
	// The tools provider key must match the agent's built-in key so the
	// key-set dedup in agent.contextProviders() skips the built-in.
	p := New()
	providers := p.AgentContextProviders(app.AgentContextInfo{
		ActiveTools: func() []tool.Tool { return nil },
	})
	require.Len(t, providers, 1)
	require.Equal(t, agentcontext.ProviderKey("tools"), providers[0].Key())
}

func TestLazyToolsProviderReflectsChanges(t *testing.T) {
	// Verify the provider calls ActiveTools on each GetContext, not just once.
	toolA := tool.New("alpha", "Tool A", func(tool.Ctx, struct{}) (tool.Result, error) {
		return tool.NewResult().Text("a").Build(), nil
	})
	toolB := tool.New("beta", "Tool B", func(tool.Ctx, struct{}) (tool.Result, error) {
		return tool.NewResult().Text("b").Build(), nil
	})

	current := []tool.Tool{toolA}
	p := New()
	providers := p.AgentContextProviders(app.AgentContextInfo{
		ActiveTools: func() []tool.Tool { return current },
	})
	require.Len(t, providers, 1)
	prov := providers[0]

	// First render: only toolA.
	ctx1, err := prov.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Len(t, ctx1.Fragments, 1)
	require.Contains(t, ctx1.Fragments[0].Content, "alpha")
	require.NotContains(t, ctx1.Fragments[0].Content, "beta")

	// Mutate the active tools.
	current = []tool.Tool{toolA, toolB}

	// Second render: both tools visible.
	ctx2, err := prov.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Len(t, ctx2.Fragments, 1)
	require.Contains(t, ctx2.Fragments[0].Content, "alpha")
	require.Contains(t, ctx2.Fragments[0].Content, "beta")

	// Fingerprints should differ.
	require.NotEqual(t, ctx1.Fingerprint, ctx2.Fingerprint)
}
