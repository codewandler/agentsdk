package plannerplugin

import (
	"testing"

	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/stretchr/testify/require"
)

func TestPluginContributesPlannerFactory(t *testing.T) {
	plugin := New()
	require.Equal(t, "planner", plugin.Name())

	factories := plugin.CapabilityFactories()
	require.Len(t, factories, 1)
	require.Equal(t, planner.CapabilityName, factories[0].Name())
}
