package conversation

import (
	"testing"

	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestSessionReplayProjection(t *testing.T) {
	s := New(WithModel("model-a"), WithSystem("system"))
	_, err := s.AddUser("hello")
	require.NoError(t, err)
	_, err = s.AppendMessage(unified.Message{Role: unified.RoleAssistant, ID: "resp_1", Content: []unified.ContentPart{unified.TextPart{Text: "hi"}}})
	require.NoError(t, err)

	req, err := s.BuildRequest(NewRequest().User("next").Stream(true).Build())
	require.NoError(t, err)

	require.Equal(t, "model-a", req.Model)
	require.True(t, req.Stream)
	require.Len(t, req.Instructions, 1)
	require.Len(t, req.Messages, 3)
	require.Equal(t, unified.RoleUser, req.Messages[0].Role)
	require.Equal(t, unified.RoleAssistant, req.Messages[1].Role)
	require.Empty(t, req.Messages[1].ID)
	require.Equal(t, unified.RoleUser, req.Messages[2].Role)
}

func TestSessionForkUsesSelectedBranchPath(t *testing.T) {
	s := New()
	_, err := s.AddUser("root")
	require.NoError(t, err)

	require.NoError(t, s.Fork("alt"))
	_, err = s.AddUser("alt only")
	require.NoError(t, err)

	require.NoError(t, s.Checkout(MainBranch))
	_, err = s.AddUser("main only")
	require.NoError(t, err)

	mainMsgs, err := s.Messages()
	require.NoError(t, err)
	require.Len(t, mainMsgs, 2)
	requireText(t, mainMsgs[1], "main only")

	require.NoError(t, s.Checkout("alt"))
	altMsgs, err := s.Messages()
	require.NoError(t, err)
	require.Len(t, altMsgs, 2)
	requireText(t, altMsgs[1], "alt only")
}

func requireText(t *testing.T, msg unified.Message, want string) {
	t.Helper()
	require.Len(t, msg.Content, 1)
	text, ok := msg.Content[0].(unified.TextPart)
	require.True(t, ok)
	require.Equal(t, want, text.Text)
}
