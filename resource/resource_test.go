package resource

import (
	"testing"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/stretchr/testify/require"
)

func TestQualifiedIDAlwaysIncludesStablePath(t *testing.T) {
	id := QualifiedID(SourceRef{Ecosystem: "agents", Scope: ScopeProject}, "agent", "reviewer", "agents/reviewer.md")
	require.Equal(t, "agents:project:reviewer#agents/reviewer.md", id)
}

func TestQualifiedIDPreservesURLSourceRefs(t *testing.T) {
	id := QualifiedID(SourceRef{Ecosystem: "agents", Scope: ScopeGit}, "source", "", "git+https://github.com/codewandler/agentplugins.git#main")
	require.Equal(t, "agents:git:source#git+https://github.com/codewandler/agentplugins.git%23main", id)
}

func TestRegistryFirstShortNameWins(t *testing.T) {
	reg := NewRegistry()
	reg.RegisterAgent(SourceRef{Ecosystem: "agents", Scope: ScopeProject}, "agents:project:reviewer#agents/reviewer.md", agentconfig.Spec{Name: "reviewer", System: "one"})
	reg.RegisterAgent(SourceRef{Ecosystem: "claude", Scope: ScopeProject}, "claude:project:reviewer#agents/reviewer.md", agentconfig.Spec{Name: "reviewer", System: "two"})

	require.Equal(t, []string{"reviewer"}, reg.AgentNames())
	require.Len(t, reg.Diagnostics(), 1)
	require.Equal(t, SeverityWarning, reg.Diagnostics()[0].Severity)
	spec, ok := reg.Agent("reviewer")
	require.True(t, ok)
	require.Equal(t, "one", spec.System)
	spec, ok = reg.AgentByID("claude:project:reviewer#agents/reviewer.md")
	require.True(t, ok)
	require.Equal(t, "two", spec.System)
}
