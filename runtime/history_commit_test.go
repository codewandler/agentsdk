package runtime

import (
	"context"
	"errors"
	"testing"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestHistoryCommitDoesNotMutateTreeWhenThreadAppendFails(t *testing.T) {
	appendErr := errors.New("append failed")
	history := NewHistory(WithHistoryLiveThread(failingLive{err: appendErr}))
	fragment := conversation.NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{
		Role:    unified.RoleUser,
		Content: []unified.ContentPart{unified.TextPart{Text: "hello"}},
	})
	fragment.SetAssistantMessage(unified.Message{
		Role:    unified.RoleAssistant,
		Content: []unified.ContentPart{unified.TextPart{Text: "hi"}},
	})
	fragment.Complete(unified.FinishReasonStop)

	_, err := history.CommitFragment(fragment)
	require.ErrorIs(t, err, appendErr)
	messages, err := history.Messages()
	require.NoError(t, err)
	require.Empty(t, messages)
}

func TestCodexHintsUseDurableThreadIDForThreadBackedHistory(t *testing.T) {
	ctx := context.Background()
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{
		ID:     "thread_codex_hints",
		Source: thread.EventSource{Type: "session", SessionID: "session_live"},
	})
	require.NoError(t, err)
	history := NewHistory(WithHistorySessionID("session_live"), WithHistoryLiveThread(live))

	req, err := history.BuildRequestForProvider(
		conversation.NewRequest().User("hi").Build(),
		conversation.ProviderIdentity{ProviderName: "codex", APIKind: "codex.responses", NativeModel: "gpt-test"},
	)
	require.NoError(t, err)
	codex, warnings := unified.CodexExtensionsFrom(req.Extensions)
	require.Empty(t, warnings)
	require.Equal(t, "thread_codex_hints", codex.SessionID)
	require.Equal(t, "main", codex.BranchID)
	require.NotEmpty(t, codex.InputBaseHash)
}

type failingLive struct {
	err error
}

func (l failingLive) ID() thread.ID { return "thread_fail" }

func (l failingLive) BranchID() thread.BranchID { return thread.MainBranch }

func (l failingLive) Append(context.Context, ...thread.Event) error { return l.err }

func (l failingLive) Flush(context.Context) error { return nil }

func (l failingLive) Shutdown(context.Context) error { return nil }

func (l failingLive) Discard(context.Context) error { return nil }
