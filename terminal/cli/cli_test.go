package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
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

func TestRunUsesBuiltInDefaultAgentWhenNoSpecsFound(t *testing.T) {
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

func TestRunUsesBuiltInDefaultAgentEvenWhenEmptyManifestNamesDefault(t *testing.T) {
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
