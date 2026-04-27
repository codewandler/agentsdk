package contextproviders

import (
	"context"
	"fmt"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/stretchr/testify/require"
)

func TestCmdProviderRendersKeyValueLines(t *testing.T) {
	runner := fakeRunner(map[string]string{
		"git rev-parse --abbrev-ref HEAD": "main",
		"git rev-parse --short HEAD":      "abc1234",
	})
	p := CmdContext("test", "test/state", []Cmd{
		{Key: "branch", Command: "git", Args: []string{"rev-parse", "--abbrev-ref", "HEAD"}},
		{Key: "head", Command: "git", Args: []string{"rev-parse", "--short", "HEAD"}},
	}, WithCmdRunner(runner))

	result, err := p.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Len(t, result.Fragments, 1)
	require.Contains(t, result.Fragments[0].Content, "branch: main")
	require.Contains(t, result.Fragments[0].Content, "head: abc1234")
}

func TestCmdProviderOmitsEmptyOutput(t *testing.T) {
	runner := fakeRunner(map[string]string{
		"git rev-parse --abbrev-ref HEAD": "main",
		"git rev-parse --short HEAD":      "",
	})
	p := CmdContext("test", "test/state", []Cmd{
		{Key: "branch", Command: "git", Args: []string{"rev-parse", "--abbrev-ref", "HEAD"}},
		{Key: "head", Command: "git", Args: []string{"rev-parse", "--short", "HEAD"}},
	}, WithCmdRunner(runner))

	result, err := p.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Len(t, result.Fragments, 1)
	require.Contains(t, result.Fragments[0].Content, "branch: main")
	require.NotContains(t, result.Fragments[0].Content, "head:")
}

func TestCmdProviderOptionalCommandSkipsOnError(t *testing.T) {
	runner := func(_ context.Context, _ string, name string, args ...string) (string, error) {
		key := name + " " + joinArgs(args)
		if key == "git rev-parse --short HEAD" {
			return "", fmt.Errorf("not a git repo")
		}
		return "main", nil
	}
	p := CmdContext("test", "test/state", []Cmd{
		{Key: "branch", Command: "git", Args: []string{"rev-parse", "--abbrev-ref", "HEAD"}},
		{Key: "head", Command: "git", Args: []string{"rev-parse", "--short", "HEAD"}, Optional: true},
	}, WithCmdRunner(runner))

	result, err := p.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Len(t, result.Fragments, 1)
	require.Contains(t, result.Fragments[0].Content, "branch: main")
	require.NotContains(t, result.Fragments[0].Content, "head:")
}

func TestCmdProviderRequiredCommandFails(t *testing.T) {
	runner := func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
		return "", fmt.Errorf("boom")
	}
	p := CmdContext("test", "test/state", []Cmd{
		{Key: "branch", Command: "git", Args: []string{"rev-parse", "--abbrev-ref", "HEAD"}},
	}, WithCmdRunner(runner))

	_, err := p.GetContext(context.Background(), agentcontext.Request{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "branch")
}

func TestCmdProviderEmptyOutputReturnsNoFragments(t *testing.T) {
	runner := fakeRunner(map[string]string{})
	p := CmdContext("test", "test/state", []Cmd{
		{Key: "branch", Command: "git", Args: []string{"rev-parse", "--abbrev-ref", "HEAD"}, Optional: true},
	}, WithCmdRunner(runner))

	result, err := p.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Empty(t, result.Fragments)
	require.NotEmpty(t, result.Fingerprint)
}

func TestCmdProviderFingerprint(t *testing.T) {
	runner := fakeRunner(map[string]string{
		"git rev-parse --abbrev-ref HEAD": "main",
	})
	p := CmdContext("test", "test/state", []Cmd{
		{Key: "branch", Command: "git", Args: []string{"rev-parse", "--abbrev-ref", "HEAD"}},
	}, WithCmdRunner(runner))

	fp, ok, err := p.StateFingerprint(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.True(t, ok)
	require.NotEmpty(t, fp)

	// Same input → same fingerprint.
	fp2, _, _ := p.StateFingerprint(context.Background(), agentcontext.Request{})
	require.Equal(t, fp, fp2)
}

func fakeRunner(outputs map[string]string) func(context.Context, string, string, ...string) (string, error) {
	return func(_ context.Context, _ string, name string, args ...string) (string, error) {
		key := name + " " + joinArgs(args)
		out, ok := outputs[key]
		if !ok {
			return "", fmt.Errorf("command not found: %s", key)
		}
		return out, nil
	}
}

func joinArgs(args []string) string {
	result := ""
	for i, arg := range args {
		if i > 0 {
			result += " "
		}
		result += arg
	}
	return result
}
