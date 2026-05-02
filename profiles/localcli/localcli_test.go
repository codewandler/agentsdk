package localcli

import (
	"testing"

	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/stretchr/testify/require"
)

func TestDefaultAgentIsLocalCLIProfileSpec(t *testing.T) {
	spec := DefaultAgent()
	require.Equal(t, "default", spec.Name)
	require.Contains(t, spec.System, "terminal")
	require.Len(t, spec.Capabilities, 1)
	require.Equal(t, planner.CapabilityName, spec.Capabilities[0].CapabilityName)
}

func TestPluginContributesToolsAndPlannerFactory(t *testing.T) {
	plugin := New()
	require.Equal(t, PluginName, plugin.Name())
	require.NotEmpty(t, plugin.DefaultTools())
	require.NotEmpty(t, plugin.CatalogTools())
	require.Len(t, plugin.CapabilityFactories(), 1)
	require.Equal(t, planner.CapabilityName, plugin.CapabilityFactories()[0].Name())
}

func TestPluginForName(t *testing.T) {
	plugin, err := PluginForName(PluginName, nil)
	require.NoError(t, err)
	require.Equal(t, PluginName, plugin.Name())

	plugin, err = PluginForName("planner", nil)
	require.NoError(t, err)
	require.Equal(t, "planner", plugin.Name())

	_, err = PluginForName("missing", nil)
	require.ErrorContains(t, err, "not registered")
}
