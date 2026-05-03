package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/stretchr/testify/require"
)

func TestRunExecutesOneShotTask(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	var out bytes.Buffer

	err := Run(t.Context(), Config{
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		Task:         "hello",
		Workspace:    t.TempDir(),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &out,
		Err:          &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
	require.Contains(t, out.String(), "session")
}

func TestRunRendersOneShotSessionCommandResult(t *testing.T) {
	client := runnertest.NewClient()
	var out bytes.Buffer

	err := Run(t.Context(), Config{
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		Task:         "/session info",
		Workspace:    t.TempDir(),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &out,
		Err:          &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.Empty(t, client.Requests())
	require.Contains(t, out.String(), "session:")
	require.Contains(t, out.String(), "agent: coder")
}

func TestRunRendersOneShotWorkflowCommandResult(t *testing.T) {
	client := runnertest.NewClient()
	var out bytes.Buffer

	err := Run(t.Context(), Config{
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		Task:         "/workflow list",
		Workspace:    t.TempDir(),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &out,
		Err:          &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.Empty(t, client.Requests())
	require.Contains(t, out.String(), "No workflows registered.")
}

func TestRunStartsResourceBackedWorkflowWithSessionTurnAction(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("workflow answer"))
	var out bytes.Buffer

	err := Run(t.Context(), Config{
		Resources:    EmbeddedResources(workflowBundle(), ".agents"),
		Task:         "/workflow start ask_agent_flow hello from workflow",
		Workspace:    t.TempDir(),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &out,
		Err:          &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
	require.Contains(t, out.String(), "workflow completed: ask_agent_flow")
	require.Contains(t, out.String(), "output: workflow answer")
}

func TestRunStartsREPLWithoutTask(t *testing.T) {
	var out bytes.Buffer

	err := Run(t.Context(), Config{
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		Workspace:    t.TempDir(),
		In:           bytes.NewBufferString("/quit\n"),
		Out:          &out,
		Err:          &bytes.Buffer{},
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})

	require.NoError(t, err)
	require.Contains(t, out.String(), "agent(coder)> ")
	require.Contains(t, out.String(), "session")
}

func TestRunUsesLocalCLIPluginAgentWhenNoSpecsFound(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	var out bytes.Buffer

	err := Run(t.Context(), Config{
		Resources:    ResolvedResources(agentdir.Resolution{}),
		Task:         "hello",
		Workspace:    t.TempDir(),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &out,
		Err:          &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
}

func TestRunUsesLocalCLIPluginAgentEvenWhenEmptyManifestNamesDefault(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))

	err := Run(t.Context(), Config{
		Resources: ResolvedResources(agentdir.Resolution{
			DefaultAgent: "missing",
		}),
		Task:         "hello",
		Workspace:    t.TempDir(),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &bytes.Buffer{},
		Err:          &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
}

func TestLoadUsesDefaultLocalCLIPlugin(t *testing.T) {
	factory := &recordingCLIPluginFactory{}

	loaded, err := Load(t.Context(), Config{
		Resources:     EmbeddedResources(testBundle(), ".agents"),
		Workspace:     t.TempDir(),
		PluginFactory: factory,
		AgentOptions:  []agent.Option{agent.WithClient(runnertest.NewClient())},
		Out:           &bytes.Buffer{},
		Err:           &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, []string{"local_cli"}, factory.names)
}

func TestLoadDisablesDefaultLocalCLIPlugin(t *testing.T) {
	factory := &recordingCLIPluginFactory{}

	loaded, err := Load(t.Context(), Config{
		Resources:        EmbeddedResources(testBundle(), ".agents"),
		Workspace:        t.TempDir(),
		NoDefaultPlugins: true,
		PluginFactory:    factory,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		Out:              &bytes.Buffer{},
		Err:              &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Empty(t, factory.names)
}

func TestLoadAppliesManifestAndExplicitPluginRefs(t *testing.T) {
	factory := &recordingCLIPluginFactory{}
	resolution := agentdir.Resolution{
		Bundle:   resource.ContributionBundle{AgentSpecs: []agent.Spec{{Name: "coder", System: "system"}}},
		Manifest: &agentdir.AppManifest{Plugins: []agentdir.PluginRef{{Name: "manifest"}}},
	}

	loaded, err := Load(t.Context(), Config{
		Resources:        ResolvedResources(resolution),
		Workspace:        t.TempDir(),
		NoDefaultPlugins: true,
		PluginNames:      []string{"explicit"},
		PluginFactory:    factory,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		Out:              &bytes.Buffer{},
		Err:              &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.NotNil(t, loaded)
	require.Equal(t, []string{"manifest", "explicit"}, factory.names)
}

func TestRunCanDisableDefaultPlugins(t *testing.T) {
	err := Run(t.Context(), Config{
		Resources:        ResolvedResources(agentdir.Resolution{}),
		Task:             "hello",
		Workspace:        t.TempDir(),
		NoDefaultPlugins: true,
		AgentOptions:     []agent.Option{agent.WithClient(runnertest.NewClient())},
		Out:              &bytes.Buffer{},
		Err:              &bytes.Buffer{},
	})

	require.ErrorContains(t, err, "no agents found")
}

func TestRunAppliesSelectedAgentSpecOverride(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	inference := agent.DefaultInferenceOptions()
	inference.Model = "override/model"
	inference.MaxTokens = 123

	err := Run(t.Context(), Config{
		Resources:      EmbeddedResources(testBundle(), ".agents"),
		Task:           "hello",
		Workspace:      t.TempDir(),
		Inference:      inference,
		ApplyInference: true,
		MaxSteps:       7,
		ApplyMaxSteps:  true,
		AgentOptions:   []agent.Option{agent.WithClient(client)},
		Out:            &bytes.Buffer{},
		Err:            &bytes.Buffer{},
	})

	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
	require.Equal(t, "override/model", client.RequestAt(0).Model)
	require.NotNil(t, client.RequestAt(0).MaxOutputTokens)
	require.Equal(t, 123, *client.RequestAt(0).MaxOutputTokens)
}

func TestResolveSessionPathFindsNewestAndIDs(t *testing.T) {
	dir := t.TempDir()
	oldPath := filepath.Join(dir, "20260425T100000Z-old.jsonl")
	newPath := filepath.Join(dir, "20260425T110000Z-new.jsonl")
	require.NoError(t, os.WriteFile(oldPath, []byte("{}\n"), 0o600))
	require.NoError(t, os.WriteFile(newPath, []byte("{}\n"), 0o600))
	oldTime := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)
	require.NoError(t, os.Chtimes(oldPath, oldTime, oldTime))
	require.NoError(t, os.Chtimes(newPath, newTime, newTime))

	got, err := ResolveSessionPath(dir, "", true)
	require.NoError(t, err)
	require.Equal(t, newPath, got)

	got, err = ResolveSessionPath(dir, "old", false)
	require.NoError(t, err)
	require.Equal(t, oldPath, got)
}

func TestCommandRunsWithEmbeddedResources(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	var out bytes.Buffer
	cmd := NewCommand(CommandConfig{
		Name:         "testagent",
		Use:          "testagent [task]",
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &out,
		Err:          &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"hello"})

	err := cmd.Execute()

	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
}

func TestCommandParsesModelPolicyFlags(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	var out bytes.Buffer
	cmd := NewCommand(CommandConfig{
		Name:         "testagent",
		Use:          "testagent [task]",
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &out,
		Err:          &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"--source-api", "auto", "--model-use-case", "agentic_coding", "hello"})

	err := cmd.Execute()

	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)
}

func TestCommandRejectsOldModelPolicyFlagNames(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	cmd := NewCommand(CommandConfig{
		Name:         "testagent",
		Use:          "testagent [task]",
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &bytes.Buffer{},
		Err:          &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"--use-case", "agentic_coding", "hello"})

	err := cmd.Execute()

	require.Error(t, err)
}

func TestCommandRejectsUnknownSourceAPI(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	cmd := NewCommand(CommandConfig{
		Name:         "testagent",
		Use:          "testagent [task]",
		Resources:    EmbeddedResources(testBundle(), ".agents"),
		AgentOptions: []agent.Option{agent.WithClient(client)},
		Out:          &bytes.Buffer{},
		Err:          &bytes.Buffer{},
	})
	cmd.SetArgs([]string{"--source-api", "bad", "hello"})

	err := cmd.Execute()

	require.Error(t, err)
}

func TestCommandHelpGroupsFlags(t *testing.T) {
	cmd := NewCommand(CommandConfig{
		Name:      "testagent",
		Use:       "testagent [task]",
		Resources: EmbeddedResources(testBundle(), ".agents"),
		Out:       &bytes.Buffer{},
		Err:       &bytes.Buffer{},
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()

	require.NoError(t, err)
	text := out.String()
	require.Contains(t, text, "Inference:")
	require.Contains(t, text, "--model")
	require.Contains(t, text, "Model Compatibility:")
	require.Contains(t, text, "--model-use-case")
	require.NotContains(t, text, "--use-case")
}

func TestCommandProfileCanDisableGroups(t *testing.T) {
	cmd := NewCommand(CommandConfig{
		Name:      "testagent",
		Use:       "testagent [task]",
		Resources: EmbeddedResources(testBundle(), ".agents"),
		Profile:   Profile{Groups: Groups(GroupCore, GroupRuntime)},
		Out:       &bytes.Buffer{},
		Err:       &bytes.Buffer{},
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()

	require.NoError(t, err)
	text := out.String()
	require.Contains(t, text, "Core:")
	require.Contains(t, text, "Runtime:")
	require.NotContains(t, text, "Inference:")
	require.NotContains(t, text, "--model")
	require.NotContains(t, text, "Model Compatibility:")
}

func TestCommandProfileDefaultsSetFlagDefaults(t *testing.T) {
	cmd := NewCommand(CommandConfig{
		Name:      "testagent",
		Use:       "testagent [task]",
		Resources: EmbeddedResources(testBundle(), ".agents"),
		Profile: Profile{Defaults: Defaults{
			Model:       "custom/model",
			MaxSteps:    42,
			Prompt:      "custom> ",
			ModelPolicy: agent.ModelPolicy{UseCase: agent.ModelUseCaseAgenticCoding},
		}},
		Out: &bytes.Buffer{},
		Err: &bytes.Buffer{},
	})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()

	require.NoError(t, err)
	text := out.String()
	require.Contains(t, text, `--model string`)
	require.Contains(t, text, `default "custom/model"`)
	require.Contains(t, text, `--max-steps int`)
	require.Contains(t, text, `default "42"`)
	require.Contains(t, text, `--model-use-case string`)
	require.Contains(t, text, `default "agentic_coding"`)
}

func TestResourceArgCommandDefaultsToCurrentDirectory(t *testing.T) {
	client := runnertest.NewClient()
	var out bytes.Buffer
	cmd := NewCommand(CommandConfig{
		Name:         "testagent",
		Use:          "testagent [path] [task]",
		ResourceArg:  true,
		AgentOptions: []agent.Option{agent.WithClient(client)},
		In:           bytes.NewBufferString("/quit\n"),
		Out:          &out,
		Err:          &bytes.Buffer{},
	})
	cmd.SetArgs(nil)

	err := cmd.Execute()

	require.NoError(t, err)
	require.Contains(t, out.String(), "agent(default)> ")
}

func testBundle() fstest.MapFS {
	return fstest.MapFS{
		".agents/agents/coder.md": {Data: []byte(`---
name: coder
model: test/model
max-tokens: 1000
---
You are a test agent.`)},
	}
}

func workflowBundle() fstest.MapFS {
	bundle := testBundle()
	bundle[".agents/workflows/ask-agent.yaml"] = &fstest.MapFile{Data: []byte(`name: ask_agent_flow
description: Ask the default agent through a resource workflow
steps:
  - id: ask
    action: agent.turn
`)}
	return bundle
}

type recordingCLIPluginFactory struct {
	names []string
}

func (f *recordingCLIPluginFactory) PluginForName(_ context.Context, name string, _ map[string]any) (app.Plugin, error) {
	f.names = append(f.names, name)
	return namedCLIPlugin(name), nil
}

type namedCLIPlugin string

func (p namedCLIPlugin) Name() string { return string(p) }
