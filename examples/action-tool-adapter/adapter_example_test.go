package actiontooladapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

type testToolCtx struct{ action.BaseCtx }

func (c testToolCtx) WorkDir() string       { return "." }
func (c testToolCtx) AgentID() string       { return "example" }
func (c testToolCtx) SessionID() string     { return "session-example" }
func (c testToolCtx) Extra() map[string]any { return nil }

func TestActionBackedTool(t *testing.T) {
	tl := NormalizeTool()
	require.Equal(t, "normalize_text", tl.Name())
	require.NotNil(t, tl.Schema())

	out, err := ExecuteNormalize(tl, testToolCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, "  Hello SDK  ")
	require.NoError(t, err)
	require.Equal(t, "hello sdk", out.Normalized)
}

func TestToolToActionAdapter(t *testing.T) {
	tl := tool.New("echo_text", "echo text", func(_ tool.Ctx, input NormalizeInput) (tool.Result, error) {
		return tool.Text(input.Text), nil
	})

	res := tool.ToAction(tl).Execute(testToolCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}, json.RawMessage(`{"text":"ok"}`))
	require.NoError(t, res.Err())
	require.False(t, res.IsError())

	out, ok := res.Data.(tool.Result)
	require.True(t, ok)
	require.Equal(t, "ok", out.String())
}

var _ action.Action = NormalizeAction()
