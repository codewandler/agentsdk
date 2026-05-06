package appconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseConfigKind(t *testing.T) {
	data := []byte(`
name: my-app
default_agent: main
sources:
  - .agents/**/*.yaml
`)
	docs, err := parseDocuments(data, "test.yaml")
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, KindConfig, docs[0].Kind)
}

func TestParseMultiDocYAML(t *testing.T) {
	data := []byte(`
kind: config
name: my-app
default_agent: main
---
kind: agent
name: main
description: Main agent
tools: [bash, grep]
system: You are helpful.
---
kind: workflow
name: deploy
description: Deploy workflow
`)
	docs, err := parseDocuments(data, "test.yaml")
	require.NoError(t, err)
	require.Len(t, docs, 3)
	require.Equal(t, KindConfig, docs[0].Kind)
	require.Equal(t, KindAgent, docs[1].Kind)
	require.Equal(t, KindWorkflow, docs[2].Kind)
}

func TestParseJSON(t *testing.T) {
	data := []byte(`{"name": "json-app", "default_agent": "main"}`)
	docs, err := parseDocuments(data, "test.json")
	require.NoError(t, err)
	require.Len(t, docs, 1)
	require.Equal(t, KindConfig, docs[0].Kind)
}

func TestLoadFileMultiDoc(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentsdk.app.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`
kind: config
name: multi-test
default_agent: main
---
kind: agent
name: main
description: Main
system: Be helpful.
---
kind: command
name: deploy
description: Deploy
target:
  workflow: deploy_flow
---
kind: workflow
name: deploy_flow
description: Deploy workflow
`), 0o644))

	result, err := LoadFile(path)
	require.NoError(t, err)
	require.Equal(t, "multi-test", result.Config.Name)
	require.Equal(t, "main", result.Config.DefaultAgent)
	require.Len(t, result.Agents, 1)
	require.Equal(t, "main", result.Agents[0].Name)
	require.Len(t, result.Commands, 1)
	require.Equal(t, "deploy", result.Commands[0].Name)
	require.Len(t, result.Workflows, 1)
	require.Equal(t, "deploy_flow", result.Workflows[0].Name)
}

func TestLoadFileJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentsdk.app.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"name": "json-test", "default_agent": "main"}`), 0o644))

	result, err := LoadFile(path)
	require.NoError(t, err)
	require.Equal(t, "json-test", result.Config.Name)
}

func TestLoadFindsJSONEntryFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.json"), []byte(`{"sources":[".agents"],"plugins":[{"name":"local_cli","config":{"mode":"safe"}}]}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".agents", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".agents", "agents", "main.md"), []byte("---\nname: main\n---\nsystem"), 0o644))

	result, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "local_cli", result.Config.Plugins[0].Name)
	require.Len(t, result.Bundles, 1)
	require.Equal(t, "main", result.Bundles[0].AgentSpecs[0].Name)
}

func TestLoadJSONSourcesField(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.json"), []byte(`{"sources":["resources"]}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "resources", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resources", "agents", "main.md"), []byte("---\nname: main\n---\nsystem"), 0o644))

	result, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, result.Bundles, 1)
	require.Equal(t, "main", result.Bundles[0].AgentSpecs[0].Name)
}

func TestLoadRejectsIncludeField(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.json"), []byte(`{"include":["resources"]}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "resources", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resources", "agents", "main.md"), []byte("---\nname: main\n---\nsystem"), 0o644))

	_, err := Load(dir)
	require.ErrorContains(t, err, `unsupported field "include"`)
}

func TestLoadYAMLSourcesField(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.yaml"), []byte("sources:\n  - resources\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "resources", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resources", "agents", "main.md"), []byte("---\nname: main\n---\nsystem"), 0o644))

	result, err := Load(dir)
	require.NoError(t, err)
	require.Len(t, result.Bundles, 1)
	require.Equal(t, "main", result.Bundles[0].AgentSpecs[0].Name)
}

func TestLoadModelPolicy(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.yaml"), []byte(`model_policy:
  use_case: agentic_coding
  source_api: anthropic.messages
  approved_only: true
  allow_degraded: true
  allow_untested: false
  evidence_path: .agentsdk/compatibility/agentic_coding.json
`), 0o644))

	result, err := Load(dir)
	require.NoError(t, err)
	require.NotNil(t, result.Config.ModelPolicy)
	policy, ok, err := result.Config.ModelPolicy.AgentPolicy(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "agentic_coding", string(policy.UseCase))
	require.Equal(t, "anthropic.messages", string(policy.SourceAPI))
	require.True(t, policy.ApprovedOnly)
	require.True(t, policy.AllowDegraded)
	require.False(t, policy.AllowUntested)
	require.Equal(t, filepath.Join(dir, ".agentsdk", "compatibility", "agentic_coding.json"), policy.EvidencePath)
}

func TestMaterializedConfigSourcesMatchLoadedSources(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.yaml"), []byte("sources:\n  - resources\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "resources", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resources", "agents", "main.md"), []byte("---\nname: main\n---\nsystem"), 0o644))

	result, err := Load(dir)
	require.NoError(t, err)
	cfg := result.MaterializedConfig()
	require.Contains(t, cfg.Sources, filepath.Join(dir, "agentsdk.app.yaml"))
	require.Contains(t, cfg.Sources, filepath.Join(dir, "resources"))
}

func TestLoadWithSources(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "resources"), 0o755))

	// Entry file with source glob.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.yaml"), []byte(`
kind: config
name: source-test
default_agent: main
sources:
  - resources/*.yaml
`), 0o644))

	// Sourced agent file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resources", "agent.yaml"), []byte(`
kind: agent
name: main
description: Main agent
system: Hello.
`), 0o644))

	// Sourced workflow file.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resources", "deploy.yaml"), []byte(`
kind: workflow
name: deploy
description: Deploy
`), 0o644))

	// Non-YAML file should be ignored.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "resources", "README.md"), []byte("not yaml"), 0o644))

	result, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "source-test", result.Config.Name)
	require.Len(t, result.Agents, 1)
	require.Equal(t, "main", result.Agents[0].Name)
	require.Len(t, result.Workflows, 1)
	require.Equal(t, "deploy", result.Workflows[0].Name)
}

func TestFindEntryFile(t *testing.T) {
	dir := t.TempDir()

	// No entry file.
	_, ok := FindEntryFile(dir)
	require.False(t, ok)

	// YAML entry.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "agentsdk.app.yaml"), []byte("name: test"), 0o644))
	path, ok := FindEntryFile(dir)
	require.True(t, ok)
	require.Contains(t, path, "agentsdk.app.yaml")
}

func TestToAppOptions(t *testing.T) {
	result := LoadResult{
		Config:    Config{Name: "test", DefaultAgent: "main"},
		Agents:    []AgentDoc{{Name: "main", Description: "Main", System: "Hi"}},
		Workflows: []WorkflowDoc{{Name: "deploy", Description: "Deploy"}},
	}

	opts := result.ToAppOptions()
	require.NotEmpty(t, opts)
}

func TestToContributionBundle(t *testing.T) {
	result := LoadResult{
		Config:    Config{Name: "test-app"},
		Workflows: []WorkflowDoc{{Name: "deploy", Description: "Deploy"}},
		Commands:  []CommandDoc{{Name: "run", Description: "Run", Target: &CommandTarget{Workflow: "deploy"}}},
		Actions:   []ActionDoc{{Name: "notify", Description: "Notify"}},
	}

	bundle := result.ToContributionBundle()
	require.Len(t, bundle.Workflows, 1)
	require.Equal(t, "config:test-app:deploy", bundle.Workflows[0].RID.Address())
	require.Len(t, bundle.CommandResources, 1)
	require.Len(t, bundle.Actions, 1)
}

func TestToAgentSpecs(t *testing.T) {
	result := LoadResult{
		Agents: []AgentDoc{
			{Name: "main", Description: "Main", Model: "gpt-5", MaxSteps: 50, System: "Be helpful."},
		},
	}

	specs := result.ToAgentSpecs()
	require.Len(t, specs, 1)
	require.Equal(t, "main", specs[0].Name)
	require.Equal(t, "gpt-5", specs[0].Inference.Model)
	require.Equal(t, 50, specs[0].MaxSteps)
	require.Equal(t, "Be helpful.", specs[0].System)
}

func TestExpandVars(t *testing.T) {
	base := "/home/user/project"

	// Relative path.
	require.Equal(t, "/home/user/project/foo/*.yaml", expandVars("foo/*.yaml", base))

	// Absolute path stays absolute.
	require.Equal(t, "/etc/agents/*.yaml", expandVars("/etc/agents/*.yaml", base))

	// ~ expansion.
	result := expandVars("~/.agents/*.yaml", base)
	require.NotContains(t, result, "~")
	require.Contains(t, result, ".agents/*.yaml")
}
