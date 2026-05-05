package appconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseMinimalConfig(t *testing.T) {
	data := []byte(`
name: my-app
default_agent: main
agents:
  main:
    description: Main agent
    tools: [bash, grep]
    system: You are helpful.
`)
	cfg, err := Parse(data, "test.yaml")
	require.NoError(t, err)
	require.Equal(t, "my-app", cfg.Name)
	require.Equal(t, "main", cfg.DefaultAgent)
	require.Len(t, cfg.Agents, 1)
	require.Equal(t, "Main agent", cfg.Agents["main"].Description)
	require.Equal(t, []string{"bash", "grep"}, cfg.Agents["main"].Tools)
	require.Equal(t, "You are helpful.", cfg.Agents["main"].System)
}

func TestParseFullConfig(t *testing.T) {
	data := []byte(`
name: my-app
default_agent: main
sources:
  - .agents
  - ~/.agents
agents:
  main:
    description: Main agent
    model: gpt-5
    tools: [bash]
    system: Be helpful.
commands:
  deploy:
    description: Deploy the app
    target:
      workflow: deploy_flow
workflows:
  deploy_flow:
    description: Deployment workflow
    steps:
      - id: build
        action: shell.exec
actions:
  notify:
    description: Send notification
    kind: builtin
datasources:
  docs:
    description: Documentation
    kind: corpus
triggers:
  hourly:
    description: Hourly check
    source:
      interval: 1h
    target:
      workflow: deploy_flow
resolution:
  precedence: [local, embedded]
  aliases:
    deploy: local:deploy
plugins:
  - name: local_cli
`)
	cfg, err := Parse(data, "test.yaml")
	require.NoError(t, err)
	require.Equal(t, "my-app", cfg.Name)
	require.Len(t, cfg.Sources, 2)
	require.Len(t, cfg.Agents, 1)
	require.Len(t, cfg.Commands, 1)
	require.Equal(t, "deploy_flow", cfg.Commands["deploy"].Target.Workflow)
	require.Len(t, cfg.Workflows, 1)
	require.Len(t, cfg.Actions, 1)
	require.Len(t, cfg.Datasources, 1)
	require.Len(t, cfg.Triggers, 1)
	require.NotNil(t, cfg.Resolution)
	require.Equal(t, []string{"local", "embedded"}, cfg.Resolution.Precedence)
	require.Equal(t, "local:deploy", cfg.Resolution.Aliases["deploy"])
	require.Len(t, cfg.Plugins, 1)
}

func TestToContributionBundle(t *testing.T) {
	cfg := Config{
		Name: "test-app",
		Workflows: map[string]WorkflowConfig{
			"deploy": {Description: "Deploy"},
		},
		Commands: map[string]CommandConfig{
			"run-deploy": {
				Description: "Run deploy",
				Target:      &CommandTarget{Workflow: "deploy"},
			},
		},
		Actions: map[string]ActionConfig{
			"notify": {Description: "Notify", Kind: "builtin"},
		},
		Datasources: map[string]DatasourceConfig{
			"docs": {Description: "Docs", Kind: "corpus"},
		},
		Triggers: map[string]TriggerConfig{
			"hourly": {
				Description: "Hourly",
				Source:      map[string]any{"interval": "1h"},
				Target:      map[string]any{"workflow": "deploy"},
			},
		},
	}

	bundle := cfg.ToContributionBundle()
	require.Len(t, bundle.Workflows, 1)
	require.Equal(t, "deploy", bundle.Workflows[0].Name)
	require.Equal(t, "config:test-app:deploy", bundle.Workflows[0].RID.Address())

	require.Len(t, bundle.CommandResources, 1)
	require.Equal(t, "run-deploy", bundle.CommandResources[0].Name)
	require.Equal(t, "config:test-app:run-deploy", bundle.CommandResources[0].RID.Address())

	require.Len(t, bundle.Actions, 1)
	require.Equal(t, "notify", bundle.Actions[0].Name)

	require.Len(t, bundle.DataSources, 1)
	require.Equal(t, "docs", bundle.DataSources[0].Name)

	require.Len(t, bundle.Triggers, 1)
	require.Equal(t, "hourly", bundle.Triggers[0].Name)
}

func TestLoadFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.yaml")
	require.NoError(t, os.WriteFile(path, []byte("name: file-test\ndefault_agent: main\n"), 0o644))

	cfg, err := LoadFile(path)
	require.NoError(t, err)
	require.Equal(t, "file-test", cfg.Name)
}

func TestLoadDir(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "01-base.yaml"), []byte("name: dir-test\ndefault_agent: main\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "02-workflows.yaml"), []byte("workflows:\n  deploy:\n    description: Deploy\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("not yaml"), 0o644))

	cfg, err := LoadDir(dir)
	require.NoError(t, err)
	require.Equal(t, "dir-test", cfg.Name)
	require.Len(t, cfg.Workflows, 1)
}

func TestMergeConfigOverlay(t *testing.T) {
	base := Config{Name: "base", DefaultAgent: "a"}
	overlay := Config{DefaultAgent: "b", Agents: map[string]AgentConfig{"x": {Description: "X"}}}

	merged := mergeConfig(base, overlay)
	require.Equal(t, "base", merged.Name) // overlay Name is empty, base preserved
	require.Equal(t, "b", merged.DefaultAgent)
	require.Len(t, merged.Agents, 1)
}
