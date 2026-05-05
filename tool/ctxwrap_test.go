package tool

import (
	"context"

	"github.com/codewandler/agentsdk/action"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWrapCtx_PreservesMetadata(t *testing.T) {
	base := fakeCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}
	newCtx := context.WithValue(context.Background(), testKey{}, "injected")

	wrapped := WrapCtx(base, newCtx)

	require.Equal(t, "/tmp", wrapped.WorkDir())
	require.Equal(t, "test-agent", wrapped.AgentID())
	require.Equal(t, "test-session", wrapped.SessionID())
}

func TestWrapCtx_UsesNewContextDeadline(t *testing.T) {
	base := fakeCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}
	deadline := time.Now().Add(5 * time.Second)
	newCtx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	wrapped := WrapCtx(base, newCtx)

	got, ok := wrapped.Deadline()
	require.True(t, ok)
	require.Equal(t, deadline, got)

	// Base has no deadline.
	_, ok = base.Deadline()
	require.False(t, ok)
}

func TestWrapCtx_UsesNewContextDone(t *testing.T) {
	base := fakeCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}
	newCtx, cancel := context.WithCancel(context.Background())

	wrapped := WrapCtx(base, newCtx)

	select {
	case <-wrapped.Done():
		t.Fatal("should not be done yet")
	default:
	}

	cancel()

	select {
	case <-wrapped.Done():
		// expected
	case <-time.After(time.Second):
		t.Fatal("should be done after cancel")
	}

	require.Error(t, wrapped.Err())
}

func TestWrapCtx_UsesNewContextValues(t *testing.T) {
	base := fakeCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}
	newCtx := context.WithValue(context.Background(), testKey{}, "from-new-ctx")

	wrapped := WrapCtx(base, newCtx)

	val := wrapped.Value(testKey{})
	require.Equal(t, "from-new-ctx", val)

	// Base context doesn't have this value.
	require.Nil(t, base.Value(testKey{}))
}

func TestWrapCtx_ApproverViaContext(t *testing.T) {
	base := fakeCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}
	called := false
	approver := Approver(func(_ Ctx, _ Intent, _ any) (bool, error) {
		called = true
		return true, nil
	})

	newCtx := CtxWithApprover(context.Background(), approver)
	wrapped := WrapCtx(base, newCtx)

	got := ApproverFrom(wrapped)
	require.NotNil(t, got)

	approved, err := got(wrapped, Intent{}, nil)
	require.NoError(t, err)
	require.True(t, approved)
	require.True(t, called)
}

type testKey struct{}
