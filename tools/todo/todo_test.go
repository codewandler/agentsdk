package todo

import (
	"context"

	"github.com/codewandler/agentsdk/action"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

type testCtx struct {
	action.BaseCtx
	sessionID string
}

func (c testCtx) WorkDir() string       { return "/tmp" }
func (c testCtx) AgentID() string       { return "test-agent" }
func (c testCtx) SessionID() string     { return c.sessionID }
func (c testCtx) Extra() map[string]any { return nil }

func execTodo(t *testing.T, sessionID, raw string) (tool.Result, error) {
	t.Helper()
	return Tools()[0].Execute(testCtx{BaseCtx: action.BaseCtx{Context: context.Background()}, sessionID: sessionID}, json.RawMessage(raw))
}

func boolPtr(v bool) *bool { return &v }

func TestTodoTool_CreateListGetUpdateDelete(t *testing.T) {
	resetTodosForTest()

	res, err := execTodo(t, "s1", `{"action":"create","title":"  Buy milk  "}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "Created todo #1")

	res, err = execTodo(t, "s1", `{"action":"create","title":"Write report"}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "Created todo #2")

	res, err = execTodo(t, "s1", `{"action":"list"}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, "1. [ ] Buy milk\n2. [ ] Write report", res.String())

	res, err = execTodo(t, "s1", `{"action":"get","id":1}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, "Todo #1 [open]: Buy milk", res.String())

	res, err = execTodo(t, "s1", `{"action":"update","id":1,"title":"Buy oat milk","done":true}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, "Updated todo #1 [done]: Buy oat milk", res.String())

	res, err = execTodo(t, "s1", `{"action":"delete","id":2}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, "Deleted todo #2: Write report", res.String())

	res, err = execTodo(t, "s1", `{"action":"list"}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, "1. [x] Buy oat milk", res.String())
}

func TestTodoTool_IsSessionScoped(t *testing.T) {
	resetTodosForTest()

	_, err := execTodo(t, "session-a", `{"action":"create","title":"A1"}`)
	require.NoError(t, err)
	_, err = execTodo(t, "session-a", `{"action":"create","title":"A2"}`)
	require.NoError(t, err)
	_, err = execTodo(t, "session-b", `{"action":"create","title":"B1"}`)
	require.NoError(t, err)

	res, err := execTodo(t, "session-a", `{"action":"list"}`)
	require.NoError(t, err)
	require.Equal(t, "1. [ ] A1\n2. [ ] A2", res.String())

	res, err = execTodo(t, "session-b", `{"action":"list"}`)
	require.NoError(t, err)
	require.Equal(t, "1. [ ] B1", res.String())

	res, err = execTodo(t, "session-b", `{"action":"get","id":2}`)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "todo with id 2 not found")
}

func TestTodoTool_ListEmpty(t *testing.T) {
	resetTodosForTest()

	res, err := execTodo(t, "s1", `{"action":"list"}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, "No todos.", res.String())
}

func TestTodoTool_Errors(t *testing.T) {
	resetTodosForTest()

	cases := []struct {
		name string
		raw  string
		msg  string
	}{
		{"unknown action", `{"action":"archive"}`, "unknown action: archive"},
		{"empty title create", `{"action":"create","title":"   "}`, "title must be a non-empty string"},
		{"invalid id get", `{"action":"get","id":0}`, "id must be a positive integer"},
		{"not found get", `{"action":"get","id":99}`, "todo with id 99 not found"},
		{"update no fields", `{"action":"update","id":1}`, "update requires at least one of title or done"},
		{"delete not found", `{"action":"delete","id":42}`, "todo with id 42 not found"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := execTodo(t, "s1", tc.raw)
			require.NoError(t, err)
			require.True(t, res.IsError())
			require.Contains(t, res.String(), tc.msg)
		})
	}
}

func TestTodoTool_UpdateDoneFalseAndJSON(t *testing.T) {
	resetTodosForTest()

	_, err := execTodo(t, "s1", `{"action":"create","title":"Task"}`)
	require.NoError(t, err)
	_, err = execTodo(t, "s1", `{"action":"update","id":1,"done":true}`)
	require.NoError(t, err)
	res, err := execTodo(t, "s1", `{"action":"update","id":1,"done":false}`)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, "Updated todo #1 [open]: Task", res.String())

	b, err := res.MarshalJSON()
	require.NoError(t, err)
	require.Contains(t, string(b), `"type":"todo_item"`)
	require.Contains(t, string(b), `"done":false`)
}

func TestTodoTool_ParamsSupportDonePointer(t *testing.T) {
	p := Params{Action: "update", ID: 1, Done: boolPtr(false)}
	require.NotNil(t, p.Done)
	require.False(t, *p.Done)
}

func TestTodoTool_MissingActionValidationError(t *testing.T) {
	resetTodosForTest()

	_, err := execTodo(t, "s1", `{}`)
	require.Error(t, err)
	require.Contains(t, err.Error(), "validate todo input")
	require.Contains(t, err.Error(), "action")
}
