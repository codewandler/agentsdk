package command

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/runtime"
	"github.com/stretchr/testify/require"
)

func TestParseFlagsArgsAndQuotes(t *testing.T) {
	name, params, err := Parse(`/review "auth flow" --query=security --dry-run --path src/app`)
	require.NoError(t, err)
	require.Equal(t, "review", name)
	require.Equal(t, []string{"auth flow"}, params.Args)
	require.Equal(t, "security", params.Flags["query"])
	require.Equal(t, "true", params.Flags["dry-run"])
	require.Equal(t, "src/app", params.Flags["path"])
}

func TestRegistryExecutesAliases(t *testing.T) {
	reg := NewRegistry()
	err := reg.Register(New(Spec{Name: "quit", Aliases: []string{"q"}}, func(context.Context, Params) (Result, error) {
		return Exit(), nil
	}))
	require.NoError(t, err)

	result, err := reg.Execute(context.Background(), "/q")
	require.NoError(t, err)
	require.Equal(t, ResultExit, result.Kind)
}

func TestRegistryRejectsDuplicateAlias(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(New(Spec{Name: "one", Aliases: []string{"x"}}, nil)))
	err := reg.Register(New(Spec{Name: "two", Aliases: []string{"x"}}, nil))
	var dup ErrDuplicate
	require.ErrorAs(t, err, &dup)
}

func TestHelpTextOnlyShowsUserCallableCommands(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(
		New(Spec{Name: "visible"}, nil),
		New(Spec{Name: "agent_only", Policy: Policy{AgentCallable: true}}, nil),
		New(Spec{Name: "both", Policy: Policy{UserCallable: true, AgentCallable: true}}, nil),
	))

	help := reg.HelpText()
	require.Contains(t, help, "/visible")
	require.Contains(t, help, "/both")
	require.NotContains(t, help, "/agent_only")
}

func TestRegistryExecuteUserOnlyRunsUserCallableCommands(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(
		New(Spec{Name: "visible"}, func(context.Context, Params) (Result, error) {
			return Text("ok"), nil
		}),
		New(Spec{Name: "agent_only", Policy: Policy{AgentCallable: true}}, func(context.Context, Params) (Result, error) {
			return Text("no"), nil
		}),
	))

	result, err := reg.ExecuteUser(context.Background(), "/visible")
	require.NoError(t, err)
	require.Equal(t, "ok", renderCommandResult(t, result))

	_, err = reg.ExecuteUser(context.Background(), "/agent_only")
	var notCallable ErrNotCallable
	require.ErrorAs(t, err, &notCallable)
	require.Equal(t, "user", notCallable.Caller)
}

func TestCommandRunToolOnlyRunsAgentCallableCommands(t *testing.T) {
	reg := NewRegistry()
	require.NoError(t, reg.Register(
		New(Spec{Name: "visible"}, func(context.Context, Params) (Result, error) {
			return Text("no"), nil
		}),
		New(Spec{Name: "agent_only", Policy: Policy{AgentCallable: true}}, func(context.Context, Params) (Result, error) {
			return Text("ok"), nil
		}),
	))
	tl := Tool(reg)
	ctx := runtime.NewToolContext(context.Background())

	okResult, err := tl.Execute(ctx, json.RawMessage(`{"command":"/agent_only"}`))
	require.NoError(t, err)
	require.Equal(t, "ok", okResult.String())

	blocked, err := tl.Execute(ctx, json.RawMessage(`{"command":"/visible"}`))
	require.NoError(t, err)
	require.True(t, blocked.IsError())
	require.Contains(t, blocked.String(), "not callable")
}

func renderCommandResult(t *testing.T, result Result) string {
	t.Helper()
	text, err := Render(result, DisplayTerminal)
	require.NoError(t, err)
	return text
}
