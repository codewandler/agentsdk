package builderapp

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/terminal/cli"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedResourcesResolveBuilderApp(t *testing.T) {
	resolved, err := agentdir.ResolveFS(Resources(), ResourcesRoot)
	require.NoError(t, err)
	require.NotEmpty(t, resolved.Bundle.AgentSpecs)
	require.Equal(t, "builder", resolved.Bundle.AgentSpecs[0].Name)
	require.NotEmpty(t, resolved.Bundle.Workflows)
	require.NotEmpty(t, resolved.Bundle.CommandResources)
}

func TestBuilderAppOptionsExposeActionsAndProjectContext(t *testing.T) {
	dir := t.TempDir()
	opts, err := AppOptions(Config{ProjectDir: dir})
	require.NoError(t, err)
	resolved, err := agentdir.ResolveFS(Resources(), ResourcesRoot)
	require.NoError(t, err)
	loaded, err := app.New(append([]app.Option{app.WithResourceBundle(resolved.Bundle)}, opts...)...)
	require.NoError(t, err)
	actions := loaded.Actions()
	require.NotEmpty(t, actions)
	var names []string
	for _, a := range actions {
		names = append(names, a.Spec().Name)
	}
	require.Contains(t, names, "builder_inspect_project")
	catalog := loaded.ToolCatalog()
	require.Contains(t, catalog.Names(), "web_fetch")
	require.Contains(t, catalog.Names(), "web_search")
	require.Contains(t, catalog.Names(), "tools_activate")
}

func TestBuilderHelpersInspectDiscoverAndSmokeTarget(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.json"), []byte(`{"sources":[".agents"]}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".agents", "agents", "main.md"), []byte("---\nname: main\n---\nsystem"), 0o644))
	cfg := Config{ProjectDir: dir}
	inspect, err := InspectProject(context.Background(), cfg, InspectProjectInput{})
	require.NoError(t, err)
	require.True(t, inspect.HasManifest)
	require.True(t, inspect.HasAgentsDir)
	discovered, err := DiscoverTarget(context.Background(), cfg, DiscoverTargetInput{})
	require.NoError(t, err)
	require.Contains(t, discovered.Agents, "main")
	smoke, err := RunTargetSmoke(context.Background(), cfg, RunTargetSmokeInput{})
	require.NoError(t, err)
	require.NotEmpty(t, smoke.TargetSessionID)
	require.Contains(t, smoke.Checks, SmokeCheck{Name: "discover target app", Status: "passed"})
}

func TestWriteProjectFileRejectsEscapes(t *testing.T) {
	dir := t.TempDir()
	_, err := WriteProjectFile(context.Background(), Config{ProjectDir: dir}, WriteProjectFileInput{Path: "../escape.txt", Content: "bad"})
	require.ErrorContains(t, err, "must be relative")
	out, err := WriteProjectFile(context.Background(), Config{ProjectDir: dir}, WriteProjectFileInput{Path: "docs/ok.md", Content: "ok"})
	require.NoError(t, err)
	require.Equal(t, "docs/ok.md", out.Path)
	require.FileExists(t, filepath.Join(dir, "docs", "ok.md"))
}

func TestBuilderCLILoadsEmbeddedResourcesWithProjectWorkspace(t *testing.T) {
	dir := t.TempDir()
	opts, err := AppOptions(Config{ProjectDir: dir})
	require.NoError(t, err)
	loaded, err := cli.Load(context.Background(), cli.Config{
		Resources:        cli.EmbeddedResources(Resources(), ResourcesRoot),
		AgentName:        "builder",
		Workspace:        dir,
		SessionsDir:      DefaultSessionsDir(dir),
		NoDefaultPlugins: true,
		AppOptions:       opts,
	})
	require.NoError(t, err)
	require.Equal(t, "builder", loaded.AgentName)
	require.Equal(t, dir, loaded.Workspace)
}
