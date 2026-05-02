package agentdir

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/resource"
	"github.com/stretchr/testify/require"
)

func TestLoadFSLoadsAgentsCommandsAndSkills(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/agents/coder.md": {
			Data: []byte(`---
description: Coder agent
model: test/model
max-steps: 12
tools: [bash, file_read]
skills: [coder]
commands: [review]
---
You are a coder.`),
		},
		".agents/commands/review.md": {
			Data: []byte("---\ndescription: Review\n---\nReview {{.Query}}"),
		},
		".agents/skills/coder/SKILL.md": {
			Data: []byte("---\nname: coder\ndescription: Coder skill\n---\n# Coder"),
		},
		".agents/datasources/docs.yaml": {
			Data: []byte("name: docs\ndescription: Documentation corpus\nkind: corpus\nconfig:\n  path: docs\n"),
		},
		".agents/workflows/sync-docs.yaml": {
			Data: []byte("name: sync_docs\ndescription: Sync documentation\nsteps:\n  - id: fetch\n    action: docs.fetch\n"),
		},
	}

	bundle, err := LoadFS(fsys, ".")
	require.NoError(t, err)
	require.Len(t, bundle.AgentSpecs, 1)
	require.Equal(t, "coder", bundle.AgentSpecs[0].Name)
	require.Equal(t, "Coder agent", bundle.AgentSpecs[0].Description)
	require.Equal(t, "test/model", bundle.AgentSpecs[0].Inference.Model)
	require.Equal(t, 12, bundle.AgentSpecs[0].MaxSteps)
	require.Equal(t, []string{"bash", "file_read"}, bundle.AgentSpecs[0].Tools)
	require.Equal(t, []string{"coder"}, bundle.AgentSpecs[0].Skills)
	require.Equal(t, []string{"review"}, bundle.AgentSpecs[0].Commands)
	require.Contains(t, bundle.AgentSpecs[0].System, "You are a coder.")
	require.Len(t, bundle.Commands, 1)
	require.Equal(t, "review", bundle.Commands[0].Spec().Name)
	require.Len(t, bundle.SkillSources, 1)
	require.Equal(t, ".agents/skills", bundle.SkillSources[0].Root)
	require.Len(t, bundle.Skills, 1)
	require.Equal(t, []string{".agents/agents/AGENTS.md", ".agents/AGENTS.md", "AGENTS.md"}, bundle.AgentSpecs[0].InstructionPaths)
	require.Equal(t, "coder", bundle.Skills[0].Name)
	require.Equal(t, "Coder skill", bundle.Skills[0].Description)
	require.Len(t, bundle.DataSources, 1)
	require.Equal(t, "docs", bundle.DataSources[0].Name)
	require.Equal(t, "Documentation corpus", bundle.DataSources[0].Description)
	require.Equal(t, "corpus", bundle.DataSources[0].Kind)
	require.Equal(t, "agents:embedded:docs#.agents/datasources/docs.yaml", bundle.DataSources[0].ID)
	require.Len(t, bundle.Workflows, 1)
	require.Equal(t, "sync_docs", bundle.Workflows[0].Name)
	require.Equal(t, "Sync documentation", bundle.Workflows[0].Description)
	require.Equal(t, "agents:embedded:sync_docs#.agents/workflows/sync-docs.yaml", bundle.Workflows[0].ID)
}

func TestLoadFSAcceptsClaudeStringToolList(t *testing.T) {
	fsys := fstest.MapFS{
		".claude/agents/helper.md": {
			Data: []byte("---\nname: helper\ntools: Bash, Grep, Read\n---\nhelper"),
		},
	}
	bundle, err := LoadFS(fsys, ".")
	require.NoError(t, err)
	require.Len(t, bundle.AgentSpecs, 1)
	require.Equal(t, []string{"Bash", "Grep", "Read"}, bundle.AgentSpecs[0].Tools)
}

func TestResolveDirPrefersManifest(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"default_agent":"main","sources":["plugin"]}`)
	writeFile(t, filepath.Join(dir, ".claude", "agents", "ignored.md"), "---\nname: ignored\n---\nignored")
	writeFile(t, filepath.Join(dir, "plugin", "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, "main", resolved.DefaultAgent)
	require.Len(t, resolved.Bundle.AgentSpecs, 1)
	require.Equal(t, "main", resolved.Bundle.AgentSpecs[0].Name)
}

func TestResolveDirManifestModelPolicy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{
		"sources":["plugin"],
		"model_policy":{
			"use_case":"agentic_coding",
			"source_api":"anthropic.messages",
			"approved_only":true,
			"allow_degraded":true,
			"evidence_path":"compat/evidence.json"
		}
	}`)
	writeFile(t, filepath.Join(dir, "plugin", "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.True(t, resolved.HasModelPolicy)
	require.Equal(t, agent.ModelUseCaseAgenticCoding, resolved.ModelPolicy.UseCase)
	require.Equal(t, "anthropic.messages", string(resolved.ModelPolicy.SourceAPI))
	require.True(t, resolved.ModelPolicy.ApprovedOnly)
	require.True(t, resolved.ModelPolicy.AllowDegraded)
	require.Equal(t, filepath.Join(dir, "compat", "evidence.json"), resolved.ModelPolicy.EvidencePath)
}

func TestResolveDirProbesClaudeAndAgentsBeforeRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".claude", "agents", "main.md"), "---\nname: main\n---\nmain")
	writeFile(t, filepath.Join(dir, ".agents", "agents", "reviewer.md"), "---\nname: reviewer\n---\nreviewer")
	writeFile(t, filepath.Join(dir, "agents", "ignored.md"), "---\nname: ignored\n---\nignored")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	var names []string
	for _, spec := range resolved.Bundle.AgentSpecs {
		names = append(names, spec.Name)
	}
	require.Equal(t, []string{"reviewer", "main"}, names)
}

func TestResolveDefaultAgentUsesUniqueFirstWinNames(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".agents", "agents", "reviewer.md"), "---\nname: reviewer\n---\nagents")
	writeFile(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"), "---\nname: reviewer\n---\nclaude")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"reviewer"}, resolved.AgentNames())
	name, err := resolved.ResolveDefaultAgent("")
	require.NoError(t, err)
	require.Equal(t, "reviewer", name)
}

func TestAgentResourceIDsIncludeSourcePathWhenNeeded(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"sources":[".agents","."]}`)
	writeFile(t, filepath.Join(dir, ".agents", "agents", "reviewer.md"), "---\nname: reviewer\n---\nagents")
	writeFile(t, filepath.Join(dir, "agents", "reviewer.md"), "---\nname: reviewer\n---\nroot")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	var ids []string
	for _, spec := range resolved.Bundle.AgentSpecs {
		ids = append(ids, spec.ResourceID)
	}
	require.Contains(t, ids, "agents:project:reviewer#.agents/agents/reviewer.md")
	require.Contains(t, ids, "agents:project:reviewer#agents/reviewer.md")

}

func TestLoadFSPopulatesInstructionPathsForNestedAgent(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/agents/reviewer.md": {Data: []byte("---\nname: reviewer\n---\nReview carefully.")},
	}
	bundle, err := LoadFS(fsys, ".")
	require.NoError(t, err)
	require.Len(t, bundle.AgentSpecs, 1)
	require.Equal(t, []string{".agents/agents/AGENTS.md", ".agents/AGENTS.md", "AGENTS.md"}, bundle.AgentSpecs[0].InstructionPaths)
}

func TestResolveDefaultAgentDeduplicatesNames(t *testing.T) {
	name, err := ResolveDefaultAgent([]string{"reviewer", "reviewer"}, "", "")
	require.NoError(t, err)
	require.Equal(t, "reviewer", name)
}

func TestResolveDirFallsBackToPluginRoot(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Len(t, resolved.Bundle.AgentSpecs, 1)
	require.Equal(t, "main", resolved.Bundle.AgentSpecs[0].Name)
}

func TestResolveDirFallsBackToPluginRootWhenAgentsDirHasNoResources(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".agents", "plans", "note.md"), "not a resource")
	writeFile(t, filepath.Join(dir, "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, resolved.AgentNames())
	require.Equal(t, []string{dir}, resolved.Sources)
}

func TestResolveDirIncludesGlobalResourcesWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\n---\nmain")
	writeFile(t, filepath.Join(home, ".agents", "agents", "helper.md"), "---\nname: helper\n---\nhelper")

	resolved, err := ResolveDirWithOptions(dir, ResolveOptions{
		HomeDir: home,
		Policy:  resource.DiscoveryPolicy{IncludeGlobalUserResources: true},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"helper", "main"}, resolved.AgentNames())
}

func TestResolveDirLocalOnlyIgnoresGlobalResourcesWithoutManifest(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\n---\nmain")
	writeFile(t, filepath.Join(home, ".agents", "agents", "helper.md"), "---\nname: helper\n---\nhelper")

	resolved, err := ResolveDirWithOptions(dir, ResolveOptions{
		HomeDir:   home,
		LocalOnly: true,
		Policy:    resource.DiscoveryPolicy{IncludeGlobalUserResources: true},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, resolved.AgentNames())
}

func TestResolveDirRootFallbackRunsBeforeGlobalResources(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(dir, "agents", "main.md"), "---\nname: main\n---\nmain")
	writeFile(t, filepath.Join(home, ".agents", "agents", "helper.md"), "---\nname: helper\n---\nhelper")

	resolved, err := ResolveDirWithOptions(dir, ResolveOptions{
		HomeDir: home,
		Policy:  resource.DiscoveryPolicy{IncludeGlobalUserResources: true},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"helper", "main"}, resolved.AgentNames())
	require.Equal(t, []string{dir, filepath.Join(home, ".agents")}, resolved.Sources)
}

func TestResolveDirRejectsLegacyPluginPathRefs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"plugins":[{"path":"plugin"}]}`)

	_, err := ResolveDir(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "plugin path references")
	require.Contains(t, err.Error(), "plugin name references")
}

func TestResolveDirParsesPluginRefs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"sources":[".agents"],"plugins":["local_cli",{"name":"planner","config":{"mode":"test"}}]}`)
	writeFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, []PluginRef{
		{Name: "local_cli"},
		{Name: "planner", Config: map[string]any{"mode": "test"}},
	}, resolved.ManifestPluginRefs())
}

func TestManifestDiscoveryCanDisableGlobalResources(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"discovery":{"include_global_user_resources":false},"sources":[".agents"]}`)
	writeFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\n---\nmain")
	writeFile(t, filepath.Join(home, ".agents", "agents", "helper.md"), "---\nname: helper\n---\nhelper")

	resolved, err := ResolveDirWithOptions(dir, ResolveOptions{
		HomeDir: home,
		Policy:  resource.DiscoveryPolicy{IncludeGlobalUserResources: true},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, resolved.AgentNames())
}

func TestResolveLocalOnlySkipsManifestRemotePolicy(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"discovery":{"allow_remote":true},"sources":[".agents","git+https://example.invalid/repo.git"]}`)
	writeFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDirWithOptions(dir, ResolveOptions{LocalOnly: true})
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, resolved.AgentNames())
	require.Len(t, resolved.Bundle.Diagnostics, 1)
	require.Contains(t, resolved.Bundle.Diagnostics[0].Message, "skipped")
}

func TestResolveLocalOnlyKeepsLocalExternalSuggestions(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"discovery":{"include_external_ecosystems":true},"sources":[".agents"]}`)
	writeFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\n---\nmain")
	writeFile(t, filepath.Join(dir, "Makefile"), "test:\n\tgo test ./...\n")

	resolved, err := ResolveDirWithOptions(dir, ResolveOptions{
		LocalOnly: true,
		Policy:    resource.DiscoveryPolicy{IncludeExternalEcosystems: true},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, resolved.AgentNames())
	require.Len(t, resolved.Bundle.Tools, 1)
	require.False(t, resolved.Bundle.Tools[0].Enabled)
}

func TestResolveDirManifestSourcesUseURLStrings(t *testing.T) {
	dir := t.TempDir()
	external := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"default_agent":"main","sources":[".agents","file://`+filepath.ToSlash(external)+`"]}`)
	writeFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\n---\nmain")
	writeFile(t, filepath.Join(external, "agents", "helper.md"), "---\nname: helper\n---\nhelper")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"helper", "main"}, resolved.AgentNames())
}

func TestResolveDirManifestGitSource(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".agents", "agents", "remote.md"), "---\nname: remote\n---\nremote")
	runGitTest(t, repo, "init", "-b", "main")
	runGitTest(t, repo, "config", "user.email", "test@example.com")
	runGitTest(t, repo, "config", "user.name", "Test")
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "-m", "init")
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"discovery":{"allow_remote":true,"trust_store_dir":".agentsdk"},"sources":["git+file://`+filepath.ToSlash(repo)+`#main"]}`)

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"remote"}, resolved.AgentNames())
	require.DirExists(t, filepath.Join(dir, ".agentsdk", "cache", "git"))
	matches, err := filepath.Glob(filepath.Join(dir, ".agentsdk", "cache", "git", "*", "*", "*", "refs", "main", "meta.json"))
	require.NoError(t, err)
	require.NotEmpty(t, matches)

	writeFile(t, filepath.Join(repo, ".agents", "agents", "second.md"), "---\nname: second\n---\nsecond")
	runGitTest(t, repo, "add", ".")
	runGitTest(t, repo, "commit", "-m", "second")
	resolved, err = ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"remote", "second"}, resolved.AgentNames())
}

func TestResolveFSLoadsEmbeddedPluginRoot(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/agents/coder.md": {Data: []byte("---\nname: coder\n---\ncoder")},
	}

	resolved, err := ResolveFS(fsys, ".agents")
	require.NoError(t, err)
	require.Equal(t, []string{".agents"}, resolved.Sources)
	require.Len(t, resolved.Bundle.AgentSpecs, 1)
	require.Equal(t, "coder", resolved.Bundle.AgentSpecs[0].Name)
}

func TestAgentFrontmatterSkillSourcesAreLoadedOnSpec(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/agents/coder.md": {Data: []byte(`---
name: coder
skills: [coder-extra]
skill-sources: [../extra-skills]
---
coder`)},
		".agents/extra-skills/coder/SKILL.md": {Data: []byte("---\nname: coder-extra\ndescription: Extra\n---\n# Extra")},
	}

	bundle, err := LoadFS(fsys, ".")
	require.NoError(t, err)
	require.Len(t, bundle.AgentSpecs, 1)
	require.Len(t, bundle.AgentSpecs[0].SkillSources, 1)
	require.Equal(t, ".agents/extra-skills", bundle.AgentSpecs[0].SkillSources[0].Root)
	require.Len(t, bundle.Skills, 1)
	require.Equal(t, "coder-extra", bundle.Skills[0].Name)
	require.Equal(t, "Extra", bundle.Skills[0].Description)
}

func TestResolveDefaultAgent(t *testing.T) {
	name, err := ResolveDefaultAgent([]string{"reviewer", "main"}, "", "")
	require.NoError(t, err)
	require.Equal(t, "main", name)

	name, err = ResolveDefaultAgent([]string{"reviewer", "helper"}, "helper", "")
	require.NoError(t, err)
	require.Equal(t, "helper", name)

	_, err = ResolveDefaultAgent([]string{"reviewer", "helper"}, "", "")
	require.Error(t, err)
	require.Contains(t, err.Error(), "--agent")
}

func TestResolutionHelpers(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "app.manifest.json"), `{"default_agent":"main","sources":["plugin"]}`)
	writeFile(t, filepath.Join(dir, "plugin", "agents", "main.md"), "---\nname: main\n---\nmain")

	resolved, err := ResolveDir(dir)
	require.NoError(t, err)
	require.Equal(t, []string{"main"}, resolved.AgentNames())

	name, err := resolved.ResolveDefaultAgent("")
	require.NoError(t, err)
	require.Equal(t, "main", name)

	err = resolved.UpdateAgentSpec("main", func(spec *agent.Spec) {
		spec.MaxSteps = 42
	})
	require.NoError(t, err)
	require.Equal(t, 42, resolved.Bundle.AgentSpecs[0].MaxSteps)

	err = resolved.UpdateAgentSpec("missing", nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "available agents")
}

func TestParseAgentSpecCapabilitiesShortForm(t *testing.T) {
	spec, err := ParseAgentSpec("test.md", []byte(`---
name: test
capabilities: [planner]
---
Test agent.`))
	require.NoError(t, err)
	require.Len(t, spec.Capabilities, 1)
	require.Equal(t, planner.CapabilityName, spec.Capabilities[0].CapabilityName)
	require.Equal(t, "default", spec.Capabilities[0].InstanceID)
}

func TestParseAgentSpecCapabilitiesLongForm(t *testing.T) {
	spec, err := ParseAgentSpec("test.md", []byte(`---
name: test
capabilities:
  - name: planner
    instance-id: my-planner
---
Test agent.`))
	require.NoError(t, err)
	require.Len(t, spec.Capabilities, 1)
	require.Equal(t, planner.CapabilityName, spec.Capabilities[0].CapabilityName)
	require.Equal(t, "my-planner", spec.Capabilities[0].InstanceID)
}

func TestParseAgentSpecCapabilitiesMixedForm(t *testing.T) {
	spec, err := ParseAgentSpec("test.md", []byte(`---
name: test
capabilities:
  - planner
  - name: custom
    instance-id: custom-1
---
Test agent.`))
	require.NoError(t, err)
	require.Len(t, spec.Capabilities, 2)
	require.Equal(t, capability.AttachSpec{CapabilityName: "planner", InstanceID: "default"}, spec.Capabilities[0])
	require.Equal(t, capability.AttachSpec{CapabilityName: "custom", InstanceID: "custom-1"}, spec.Capabilities[1])
}

func TestParseAgentSpecCapabilitiesCommaString(t *testing.T) {
	spec, err := ParseAgentSpec("test.md", []byte("---\nname: test\ncapabilities: planner, custom\n---\nTest."))
	require.NoError(t, err)
	require.Len(t, spec.Capabilities, 2)
	require.Equal(t, "planner", spec.Capabilities[0].CapabilityName)
	require.Equal(t, "custom", spec.Capabilities[1].CapabilityName)
}

func TestParseAgentSpecNoCapabilities(t *testing.T) {
	spec, err := ParseAgentSpec("test.md", []byte("---\nname: test\n---\nTest."))
	require.NoError(t, err)
	require.Empty(t, spec.Capabilities)
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func runGitTest(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}
