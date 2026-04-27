package skillplugin

import (
	"testing"
	"testing/fstest"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/skill"
	"github.com/stretchr/testify/require"
)

// Compile-time interface assertions.
var (
	_ app.Plugin             = (*Plugin)(nil)
	_ app.ToolsPlugin        = (*Plugin)(nil)
	_ app.SkillsPlugin       = (*Plugin)(nil)
	_ app.AgentContextPlugin = (*Plugin)(nil)
)

func TestPluginName(t *testing.T) {
	p := New()
	require.Equal(t, "skills", p.Name())
}

func TestPluginToolsReturnsSkillTool(t *testing.T) {
	p := New()
	tools := p.Tools()
	require.Len(t, tools, 1)
	require.Equal(t, "skill", tools[0].Name())
}

func TestPluginSkillSourcesWithExplicitSources(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/test/SKILL.md": {Data: []byte("---\nname: test\ndescription: Test\n---\n# Test")},
	}
	src := skill.FSSource("test", "test", fsys, "skills", skill.SourceEmbedded, 0)
	p := New(WithSources(src))
	sources := p.SkillSources()
	require.Len(t, sources, 1)
	require.Equal(t, "test", sources[0].ID)
}

func TestPluginAgentContextProvidersWithSkillState(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/arch/SKILL.md": {Data: []byte("---\nname: arch\ndescription: Architecture\n---\n# Arch")},
	}
	src := skill.FSSource("test", "test", fsys, "skills", skill.SourceEmbedded, 0)
	repo, err := skill.NewRepository([]skill.Source{src}, []string{"arch"})
	require.NoError(t, err)
	state, err := skill.NewActivationState(repo, repo.LoadedNames())
	require.NoError(t, err)

	p := New()
	providers := p.AgentContextProviders(app.AgentContextInfo{
		SkillRepository: repo,
		SkillState:      state,
	})
	require.Len(t, providers, 1)
	require.Equal(t, agentcontext.ProviderKey("skills"), providers[0].Key())
}

func TestPluginAgentContextProvidersNilState(t *testing.T) {
	p := New()
	providers := p.AgentContextProviders(app.AgentContextInfo{})
	require.Empty(t, providers)
}

func TestPluginContextProviderKeyIsSkills(t *testing.T) {
	// The skill inventory provider key must match the agent's built-in key
	// so the key-set dedup in agent.contextProviders() skips the built-in.
	fsys := fstest.MapFS{
		"skills/test/SKILL.md": {Data: []byte("---\nname: test\ndescription: Test\n---\n# Test")},
	}
	src := skill.FSSource("test", "test", fsys, "skills", skill.SourceEmbedded, 0)
	repo, err := skill.NewRepository([]skill.Source{src}, nil)
	require.NoError(t, err)
	state, err := skill.NewActivationState(repo, nil)
	require.NoError(t, err)

	p := New()
	providers := p.AgentContextProviders(app.AgentContextInfo{
		SkillRepository: repo,
		SkillState:      state,
	})
	require.Len(t, providers, 1)
	require.Equal(t, agentcontext.ProviderKey("skills"), providers[0].Key())
}
