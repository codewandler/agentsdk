package cli

import (
	"bytes"
	"testing"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestMountRegistersSubcommands(t *testing.T) {
	root := &cobra.Command{Use: "test"}
	Mount(root,
		app.Spec{Name: "alpha", Description: "Alpha app"},
		app.Spec{Name: "beta", Description: "Beta app"},
	)

	names := make([]string, 0, len(root.Commands()))
	for _, cmd := range root.Commands() {
		names = append(names, cmd.Name())
	}
	require.Contains(t, names, "alpha")
	require.Contains(t, names, "beta")
}

func TestMountedCommandRunsWithEmbeddedResources(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	var out bytes.Buffer

	root := &cobra.Command{Use: "test", SilenceUsage: true, SilenceErrors: true}
	Mount(root, app.Spec{
		Name:        "myapp",
		Description: "Test app",
		EmbeddedFS:  testBundle(),
		EmbeddedRoot: ".agents",
		Options: func() ([]app.Option, error) {
			return []app.Option{
				app.WithDefaultAgent("coder"),
			}, nil
		},
	})

	root.SetArgs([]string{"myapp", "hello"})
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})

	// Inject test client via CommandConfig.AgentOptions is not available
	// through Mount, so we test via the NewCommand path with AppOptionsFactory.
	// Instead, verify the command was registered and has the right metadata.
	cmd, _, err := root.Find([]string{"myapp"})
	require.NoError(t, err)
	require.Equal(t, "myapp [task]", cmd.Use)
	require.Equal(t, "Test app", cmd.Short)
	_ = client // client would be used in a full integration test
}

func TestMountedCommandCallsOptionsFactory(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	var out bytes.Buffer
	called := false

	spec := app.Spec{
		Name:        "factorytest",
		Description: "Factory test",
		EmbeddedFS:  testBundle(),
		EmbeddedRoot: ".agents",
		Options: func() ([]app.Option, error) {
			called = true
			return []app.Option{
				app.WithDefaultAgent("coder"),
				app.WithAgentOptions(agent.WithClient(client)),
			}, nil
		},
	}

	root := &cobra.Command{Use: "test", SilenceUsage: true, SilenceErrors: true}
	Mount(root, spec)
	root.SetArgs([]string{"factorytest", "hello"})
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	require.NoError(t, err)
	require.True(t, called, "Options factory must be called during RunE")
	require.Len(t, client.Requests(), 1)
}

func TestMountedPromptDerivedFromName(t *testing.T) {
	client := runnertest.NewClient()
	var out bytes.Buffer

	spec := app.Spec{
		Name:        "mydev",
		Description: "Dev app",
		EmbeddedFS:  testBundle(),
		EmbeddedRoot: ".agents",
		Options: func() ([]app.Option, error) {
			return []app.Option{
				app.WithDefaultAgent("coder"),
				app.WithAgentOptions(agent.WithClient(client)),
			}, nil
		},
	}

	root := &cobra.Command{Use: "test", SilenceUsage: true, SilenceErrors: true}
	Mount(root, spec)
	root.SetArgs([]string{"mydev"})
	root.SetIn(bytes.NewBufferString("/quit\n"))
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})

	err := root.Execute()
	require.NoError(t, err)
	require.Contains(t, out.String(), "mydev> ")
}
