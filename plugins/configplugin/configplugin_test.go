package configplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/command"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestPluginCommandsIncludesConfigAndRootDiscover(t *testing.T) {
	plugin := New()
	commands := plugin.Commands()
	require.Len(t, commands, 2)

	configCommand := requireCommand(t, commands, "config")
	rootDiscover := requireCommand(t, commands, "discover")
	require.NotNil(t, rootDiscover)

	desc := configCommand.Descriptor()
	var names []string
	for _, sub := range desc.Subcommands {
		names = append(names, sub.Name)
	}
	require.Contains(t, names, "discover")
}

func TestDiscoverCommandRendersResourceTree(t *testing.T) {
	dir := t.TempDir()
	writeConfigPluginTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), "name: test-app\n---\nkind: agent\nname: coder\nsystem: test\n")

	plugin := New(WithWorkspace(dir))
	configCommand := requireCommand(t, plugin.Commands(), "config")

	result, err := configCommand.Execute(context.Background(), command.Params{Raw: "discover", Args: []string{"discover"}})
	require.NoError(t, err)

	text, err := command.Render(result, command.DisplayTerminal)
	require.NoError(t, err)
	assertDiscoverTree(t, text)
}

func TestRootDiscoverCommandRendersResourceTree(t *testing.T) {
	dir := t.TempDir()
	writeConfigPluginTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), "name: test-app\n---\nkind: agent\nname: coder\nsystem: test\n")

	plugin := New(WithWorkspace(dir))
	rootDiscover := requireCommand(t, plugin.Commands(), "discover")

	result, err := rootDiscover.Execute(context.Background(), command.Params{})
	require.NoError(t, err)

	text, err := command.Render(result, command.DisplayTerminal)
	require.NoError(t, err)
	assertDiscoverTree(t, text)
}

func TestPrintCommandRendersInlineResourceDocuments(t *testing.T) {
	dir := t.TempDir()
	writeConfigPluginTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), `kind: config
name: test-app
---
kind: command
name: review
target:
  prompt: Review this repository.
`)

	plugin := New(WithWorkspace(dir))
	configCommand := requireCommand(t, plugin.Commands(), "config")

	result, err := configCommand.Execute(context.Background(), command.Params{Raw: "print", Args: []string{"print"}})
	require.NoError(t, err)

	text, err := command.Render(result, command.DisplayTerminal)
	require.NoError(t, err)
	require.Contains(t, text, "kind: command")
	require.Contains(t, text, "name: review")
	require.Contains(t, text, "prompt: Review this repository.")
}

func TestDiscoverJSONAndYAMLRenderMachineReadablePayload(t *testing.T) {
	dir := t.TempDir()
	writeConfigPluginTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), "name: test-app\n---\nkind: agent\nname: coder\ndescription: Test coder\nsystem: test\n")

	resolved, cfg, err := DiscoverResources(dir)
	require.NoError(t, err)
	require.NotNil(t, cfg)

	var jsonOut bytes.Buffer
	require.NoError(t, PrintDiscoveryJSON(&jsonOut, resolved))
	var jsonPayload struct {
		Sources []string `json:"sources"`
		Agents  []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(jsonOut.Bytes(), &jsonPayload))
	require.NotEmpty(t, jsonPayload.Sources)
	require.Len(t, jsonPayload.Agents, 1)
	require.Equal(t, "coder", jsonPayload.Agents[0].Name)
	require.Equal(t, "Test coder", jsonPayload.Agents[0].Description)

	var yamlOut bytes.Buffer
	require.NoError(t, PrintDiscoveryYAML(&yamlOut, resolved))
	var yamlPayload struct {
		Agents []struct {
			Name string `yaml:"name"`
		} `yaml:"agents"`
	}
	require.NoError(t, yaml.Unmarshal(yamlOut.Bytes(), &yamlPayload))
	require.Len(t, yamlPayload.Agents, 1)
	require.Equal(t, "coder", yamlPayload.Agents[0].Name)
}

func requireCommand(t *testing.T, commands []command.Command, name string) command.Command {
	t.Helper()
	for _, cmd := range commands {
		if cmd.Descriptor().Name == name {
			return cmd
		}
	}
	require.Failf(t, "missing command", "command %q not found", name)
	return nil
}

func assertDiscoverTree(t *testing.T, text string) {
	t.Helper()
	require.Contains(t, text, "Sources:")
	require.Contains(t, text, "agents")
	require.Contains(t, text, "coder")
	require.Contains(t, text, "Resolution:")
}

func writeConfigPluginTestFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(strings.TrimLeft(content, "\n")), 0o644))
}
