package harness

import (
	"testing"

	"github.com/codewandler/agentsdk/command"
	"github.com/stretchr/testify/require"
)

func TestCommandCatalogFiltersByPolicy(t *testing.T) {
	descriptors := []command.Descriptor{
		{
			Name: "root",
			Path: []string{"root"},
			Subcommands: []command.Descriptor{
				{Name: "visible", Path: []string{"root", "visible"}, Executable: true},
				{Name: "agent", Path: []string{"root", "agent"}, Executable: true, Policy: command.Policy{AgentCallable: true}},
				{Name: "both", Path: []string{"root", "both"}, Executable: true, Policy: command.Policy{UserCallable: true, AgentCallable: true}},
				{Name: "internal", Path: []string{"root", "internal"}, Executable: true, Policy: command.Policy{Internal: true}},
			},
		},
	}

	all := commandCatalogFromDescriptors(descriptors)
	require.Equal(t, []string{"visible", "agent", "both"}, catalogNames(all))

	agent := commandCatalogFromDescriptors(descriptors, CommandCatalogAgentCallable())
	require.Equal(t, []string{"agent", "both"}, catalogNames(agent))

	user := commandCatalogFromDescriptors(descriptors, CommandCatalogUserCallable())
	require.Equal(t, []string{"visible", "both"}, catalogNames(user))

	both := commandCatalogFromDescriptors(descriptors, CommandCatalogAgentCallable(), CommandCatalogUserCallable())
	require.Equal(t, []string{"both"}, catalogNames(both))
}

func catalogNames(entries []CommandCatalogEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Descriptor.Name)
	}
	return names
}
