package conversation

import (
	"errors"
	"testing"

	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestCommitFragmentAppendsAtomicallyAfterCompletion(t *testing.T) {
	sess := New()
	fragment := NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{
		Role:    unified.RoleUser,
		Content: []unified.ContentPart{unified.TextPart{Text: "hello"}},
	})
	fragment.SetAssistantMessage(unified.Message{
		Role:    unified.RoleAssistant,
		Content: []unified.ContentPart{unified.TextPart{Text: "hi"}},
	})
	fragment.SetUsage(unified.Usage{
		Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 1}},
	})
	fragment.AddContinuation(NewProviderContinuation(
		ProviderIdentity{ProviderName: "openrouter", APIKind: "responses", NativeModel: "model"},
		"resp_123",
		unified.Extensions{},
	))
	fragment.Complete(unified.FinishReasonStop)

	ids, err := sess.CommitFragment(fragment)
	require.NoError(t, err)
	require.Len(t, ids, 2)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, unified.RoleUser, messages[0].Role)
	require.Equal(t, unified.RoleAssistant, messages[1].Role)

	continuation, ok, err := ContinuationAtHead(sess.Tree(), sess.Branch(), ProviderIdentity{ProviderName: "openrouter"})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_123", continuation.ResponseID)
}

func TestCommitFragmentRejectsIncompleteOrFailedFragments(t *testing.T) {
	sess := New()
	fragment := NewTurnFragment()
	fragment.AddRequestMessages(unified.Message{Role: unified.RoleUser})

	_, err := sess.CommitFragment(fragment)
	require.Error(t, err)

	fragment.Fail(errors.New("stream failed"))
	_, err = sess.CommitFragment(fragment)
	require.Error(t, err)

	messages, err := sess.Messages()
	require.NoError(t, err)
	require.Empty(t, messages)
}
