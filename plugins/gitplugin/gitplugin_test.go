package gitplugin

import (
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/app"
	"github.com/stretchr/testify/require"
)

// Compile-time interface assertions.
var (
	_ app.Plugin                 = (*Plugin)(nil)
	_ app.ToolsPlugin            = (*Plugin)(nil)
	_ app.ContextProvidersPlugin = (*Plugin)(nil)
)

func TestPluginName(t *testing.T) {
	p := New()
	require.Equal(t, "git", p.Name())
}

func TestPluginToolsReturnsGitTools(t *testing.T) {
	p := New()
	tools := p.Tools()
	require.Len(t, tools, 2)
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name()
	}
	require.Contains(t, names, "git_status")
	require.Contains(t, names, "git_diff")
}

func TestPluginContextProvidersReturnsGitProvider(t *testing.T) {
	p := New()
	providers := p.ContextProviders()
	require.Len(t, providers, 1)
	require.Equal(t, agentcontext.ProviderKey("git"), providers[0].Key())
}

func TestPluginDefaultMode(t *testing.T) {
	p := New()
	require.Equal(t, contextproviders.GitMinimal, p.gitMode)
}

func TestPluginWithMode(t *testing.T) {
	p := New(WithMode(contextproviders.GitChangedFiles))
	require.Equal(t, contextproviders.GitChangedFiles, p.gitMode)
}

func TestPluginWithWorkDir(t *testing.T) {
	p := New(WithWorkDir("/tmp/repo"))
	require.Equal(t, "/tmp/repo", p.workDir)
}

func TestPluginWithGitOption(t *testing.T) {
	p := New(WithGitOption(contextproviders.WithGitMaxFiles(10)))
	require.Len(t, p.gitOpts, 1)
}

func TestPluginContextProviderKeyIsGit(t *testing.T) {
	// Verify the provider key doesn't collide with any built-in agent
	// provider keys (environment, time, model, tools, skills, agents_markdown).
	p := New()
	providers := p.ContextProviders()
	require.Len(t, providers, 1)
	key := providers[0].Key()
	builtinKeys := []agentcontext.ProviderKey{
		"environment", "time", "model", "tools", "skills", "agents_markdown",
	}
	for _, bk := range builtinKeys {
		require.NotEqual(t, bk, key, "git provider key %q collides with built-in %q", key, bk)
	}
}
