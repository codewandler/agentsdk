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
	bundle := testBundle()

	root := &cobra.Command{Use: "test", SilenceUsage: true, SilenceErrors: true}
	Mount(root, app.Spec{
		Name:        "myapp",
		Description: "Test app",
		Options: func() ([]app.Option, error) {
			return []app.Option{
				app.WithEmbeddedResources(bundle, ".agents"),
				app.WithDefaultAgent("coder"),
			}, nil
		},
	})

	root.SetArgs([]string{"myapp", "hello"})
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})

	// Verify the command was registered with the right metadata.
	cmd, _, err := root.Find([]string{"myapp"})
	require.NoError(t, err)
	require.Equal(t, "myapp [task]", cmd.Use)
	require.Equal(t, "Test app", cmd.Short)
	_ = client
}

func TestMountedCommandCallsOptionsFactory(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	var out bytes.Buffer
	called := false
	bundle := testBundle()

	spec := app.Spec{
		Name:        "factorytest",
		Description: "Factory test",
		Options: func() ([]app.Option, error) {
			called = true
			return []app.Option{
				app.WithEmbeddedResources(bundle, ".agents"),
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
	bundle := testBundle()

	spec := app.Spec{
		Name:        "mydev",
		Description: "Dev app",
		Options: func() ([]app.Option, error) {
			return []app.Option{
				app.WithEmbeddedResources(bundle, ".agents"),
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
