package repl

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/usage"
	"github.com/stretchr/testify/require"
)

type fakeAgent struct {
	out     io.Writer
	reg     *command.Registry
	tracker *usage.Tracker
	session string
	turns   []string
	resets  int
}

func newFakeAgent(out io.Writer) *fakeAgent {
	reg := command.NewRegistry()
	_ = reg.Register(
		command.New(command.Spec{Name: "quit"}, func(context.Context, command.Params) (command.Result, error) { return command.Exit(), nil }),
		command.New(command.Spec{Name: "new"}, func(context.Context, command.Params) (command.Result, error) { return command.Reset(), nil }),
	)
	return &fakeAgent{out: out, reg: reg, tracker: usage.NewTracker(), session: "sess1"}
}

func (a *fakeAgent) RunTurn(_ context.Context, _ int, task string) error {
	a.turns = append(a.turns, task)
	return nil
}

func (a *fakeAgent) Send(ctx context.Context, input string) (command.Result, error) {
	if strings.HasPrefix(input, "/") {
		result, err := a.reg.Execute(ctx, input)
		if err != nil {
			return command.Result{}, err
		}
		switch result.Kind {
		case command.ResultAgentTurn:
			input, ok := command.AgentTurnInput(result)
			if ok {
				a.turns = append(a.turns, input)
			}
			return command.Handled(), nil
		case command.ResultReset:
			a.Reset()
			return command.Handled(), nil
		default:
			return result, nil
		}
	}
	a.turns = append(a.turns, input)
	return command.Handled(), nil
}

func (a *fakeAgent) Reset() {
	a.resets++
	a.turns = nil
}

func (a *fakeAgent) ParamsSummary() string   { return "model: test" }
func (a *fakeAgent) SessionID() string       { return a.session }
func (a *fakeAgent) Tracker() *usage.Tracker { return a.tracker }
func (a *fakeAgent) Out() io.Writer          { return a.out }

func TestRunDispatchesMarkdownStyleCommandResult(t *testing.T) {
	var out bytes.Buffer
	a := newFakeAgent(&out)
	require.NoError(t, a.reg.Register(command.New(command.Spec{Name: "ask"}, func(context.Context, command.Params) (command.Result, error) {
		return command.AgentTurn("expanded prompt"), nil
	})))

	err := Run(context.Background(), a, strings.NewReader("/ask topic\n/quit\n"), WithPrompt("agent> "))
	require.NoError(t, err)
	require.Equal(t, []string{"expanded prompt"}, a.turns)
	require.Contains(t, out.String(), "model: test")
	require.Contains(t, out.String(), "-- session sess1 --")
}

func TestRunBuiltInReset(t *testing.T) {
	var out bytes.Buffer
	a := newFakeAgent(&out)

	err := Run(context.Background(), a, strings.NewReader("hello\n/new\n/quit\n"), WithPrompt("> "))
	require.NoError(t, err)
	require.Equal(t, 1, a.resets)
	require.Empty(t, a.turns)
}
