package standard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultPluginsIncludesSkillToolMgmtAndProjectInventory(t *testing.T) {
	plugins := DefaultPlugins()
	require.Len(t, plugins, 3)
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name()
	}
	require.Contains(t, names, "skills")
	require.Contains(t, names, "toolmgmt")
	require.Contains(t, names, "project_inventory")
}

func TestDefaultPluginsExcludesGit(t *testing.T) {
	plugins := DefaultPlugins()
	for _, p := range plugins {
		require.NotEqual(t, "git", p.Name())
	}
}

func TestPluginsWithGitIncludesGit(t *testing.T) {
	plugins := Plugins(Options{IncludeGit: true})
	require.Len(t, plugins, 3)
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name()
	}
	require.Contains(t, names, "skills")
	require.Contains(t, names, "toolmgmt")
	require.Contains(t, names, "git")
}

func TestPluginsCanIncludeGitAndProjectInventory(t *testing.T) {
	plugins := Plugins(Options{IncludeGit: true, IncludeProjectInventory: true})
	require.Len(t, plugins, 4)
	names := make([]string, len(plugins))
	for i, p := range plugins {
		names[i] = p.Name()
	}
	require.Contains(t, names, "skills")
	require.Contains(t, names, "toolmgmt")
	require.Contains(t, names, "project_inventory")
	require.Contains(t, names, "git")
}
