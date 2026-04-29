package standard

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
	"github.com/stretchr/testify/require"
)

func TestToolsIncludesBaseAndOptionals(t *testing.T) {
	tools := Tools(Options{
		IncludeGit:            true,
		IncludeTodo:           true,
		IncludeToolManagement: true,
		IncludeTurnDone:       true,
	})

	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name()] = true
	}

	for _, name := range []string{
		"bash",
		"file_read",
		"web_fetch",
		"git_status",
		"todo",
		"tools_list",
		"turn_done",
	} {
		require.True(t, names[name], "missing %s", name)
	}
}

func TestDefaultToolsIncludesToolManagement(t *testing.T) {
	tools := DefaultTools()

	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name()] = true
	}

	require.True(t, names["bash"])
	require.True(t, names["file_read"])
	require.True(t, names["web_fetch"])
	require.True(t, names["tools_list"])
}

func TestCatalogToolsIncludesOptionalStandardTools(t *testing.T) {
	tools := CatalogTools()

	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name()] = true
	}

	for _, name := range []string{
		"git_status",
		"git_diff",
		"notify_send",
		"todo",
		"turn_done",
		"web_search",
	} {
		require.True(t, names[name], "missing %s", name)
	}
}

func TestDefaultToolsetOwnsActivationState(t *testing.T) {
	toolset := DefaultToolset()

	require.NotNil(t, toolset.Activation())
	require.NotEmpty(t, toolset.Tools())
	require.Equal(t, len(toolset.Tools()), len(toolset.ActiveTools()))

	deactivated := toolset.Activation().Deactivate("file_*")
	require.NotEmpty(t, deactivated)

	activeNames := map[string]bool{}
	for _, t := range toolset.ActiveTools() {
		activeNames[t.Name()] = true
	}
	require.False(t, activeNames["file_read"])
	require.True(t, activeNames["bash"])
}

func TestNewToolsetFromToolsUsesExplicitTools(t *testing.T) {
	custom := tool.New("custom", "test", func(ctx tool.Ctx, p struct{}) (tool.Result, error) {
		return tool.Text("ok"), nil
	})

	toolset := NewToolsetFromTools(custom)

	require.Len(t, toolset.Tools(), 1)
	require.Equal(t, "custom", toolset.Tools()[0].Name())
	require.Equal(t, []string{"custom"}, toolNames(toolset.ActiveTools()))
}

func toolNames(tools []tool.Tool) []string {
	out := make([]string, len(tools))
	for i, t := range tools {
		out[i] = t.Name()
	}
	return out
}

func TestDefaultTools_BashHasRiskAnalyzer(t *testing.T) {
	tools := DefaultTools()

	var bash tool.Tool
	for _, tt := range tools {
		if tt.Name() == "bash" {
			bash = tt
			break
		}
	}
	require.NotNil(t, bash, "bash tool should be in default tools")

	// The innermost tool should implement IntentProvider.
	inner := tool.Innermost(bash)
	provider, ok := inner.(tool.IntentProvider)
	require.True(t, ok, "bash tool should implement IntentProvider")

	// Declare intent for a simple command — should NOT be opaque
	// because the default analyzer is wired.
	ctx := testToolCtx{Context: context.Background(), workDir: "/tmp/project"}
	intent, err := provider.DeclareIntent(ctx, json.RawMessage(`{"cmd": "ls -la"}`))
	require.NoError(t, err)
	require.False(t, intent.Opaque, "bash intent should not be opaque when default analyzer is wired")
	require.NotEmpty(t, intent.Confidence)
	require.Equal(t, "ls -la", intent.Summary)
}

func TestTools_NoDefaultRiskAnalyzer(t *testing.T) {
	tools := Tools(Options{NoDefaultRiskAnalyzer: true})

	var bash tool.Tool
	for _, tt := range tools {
		if tt.Name() == "bash" {
			bash = tt
			break
		}
	}
	require.NotNil(t, bash)

	inner := tool.Innermost(bash)
	provider, ok := inner.(tool.IntentProvider)
	require.True(t, ok)

	ctx := testToolCtx{Context: context.Background(), workDir: "/tmp/project"}
	intent, err := provider.DeclareIntent(ctx, json.RawMessage(`{"cmd": "ls -la"}`))
	require.NoError(t, err)
	require.True(t, intent.Opaque, "bash intent should be opaque when analyzer is disabled")
}

func TestTools_ExplicitRiskAnalyzer(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	tools := Tools(Options{RiskAnalyzer: analyzer})

	var bash tool.Tool
	for _, tt := range tools {
		if tt.Name() == "bash" {
			bash = tt
			break
		}
	}
	require.NotNil(t, bash)

	inner := tool.Innermost(bash)
	provider, ok := inner.(tool.IntentProvider)
	require.True(t, ok)

	ctx := testToolCtx{Context: context.Background(), workDir: "/tmp/project"}
	intent, err := provider.DeclareIntent(ctx, json.RawMessage(`{"cmd": "echo hello"}`))
	require.NoError(t, err)
	require.False(t, intent.Opaque)
}

type testToolCtx struct {
	context.Context
	workDir string
}

func (c testToolCtx) WorkDir() string       { return c.workDir }
func (c testToolCtx) AgentID() string       { return "test" }
func (c testToolCtx) SessionID() string     { return "sess" }
func (c testToolCtx) Extra() map[string]any { return map[string]any{} }
