package jsonlstore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/conversation/jsonlstore"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestStorePersistsAndResumesSession(t *testing.T) {
	ctx := context.Background()
	store := jsonlstore.Open(filepath.Join(t.TempDir(), "session.jsonl"))
	sess := conversation.New(
		conversation.WithStore(store),
		conversation.WithConversationID("conv_test"),
		conversation.WithSessionID("sess_test"),
		conversation.WithModel("model-a"),
	)

	_, err := sess.AddUser("hello")
	require.NoError(t, err)
	_, err = sess.AppendMessage(unified.Message{
		Role:    unified.RoleAssistant,
		Content: []unified.ContentPart{unified.TextPart{Text: "hi"}},
	})
	require.NoError(t, err)

	resumed, err := conversation.Resume(ctx, store, "")
	require.NoError(t, err)
	require.Equal(t, conversation.ConversationID("conv_test"), resumed.ConversationID())
	require.Equal(t, conversation.SessionID("sess_test"), resumed.SessionID())

	messages, err := resumed.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, unified.RoleUser, messages[0].Role)
	requireText(t, messages[0], "hello")
	require.Equal(t, unified.RoleAssistant, messages[1].Role)
	requireText(t, messages[1], "hi")
}

func requireText(t *testing.T, msg unified.Message, want string) {
	t.Helper()
	require.Len(t, msg.Content, 1)
	text, ok := msg.Content[0].(unified.TextPart)
	require.True(t, ok)
	require.Equal(t, want, text.Text)
}
