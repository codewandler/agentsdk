package engineerapp

import (
	"testing"

	"github.com/codewandler/agentsdk/agentdir"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedResourcesResolveEngineerApp(t *testing.T) {
	resolved, err := agentdir.ResolveFS(Resources(), ResourcesRoot)
	require.NoError(t, err)
	require.NotEmpty(t, resolved.Bundle.AgentSpecs)
	require.Equal(t, "main", resolved.Bundle.AgentSpecs[0].Name)

	spec := resolved.Bundle.AgentSpecs[0]
	require.Contains(t, spec.Tools, "bash")
	require.Contains(t, spec.Tools, "file_read")
	require.Contains(t, spec.Tools, "file_edit")
	require.Contains(t, spec.Tools, "skill")

	require.NotEmpty(t, resolved.Bundle.Commands)
	require.NotEmpty(t, resolved.Bundle.Skills)
}
