package actionmw

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/stretchr/testify/require"
)

func TestTimeoutMiddlewareStripsJSONTimeoutAndSetsDeadline(t *testing.T) {
	var received json.RawMessage
	a := action.New(action.Spec{Name: "json"}, func(ctx action.Ctx, input any) action.Result {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Greater(t, time.Until(deadline), time.Minute)
		received = input.(json.RawMessage)
		return action.Result{Data: "ok"}
	})

	wrapped := action.Apply(a, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))
	result := wrapped.Execute(action.NewCtx(context.Background()), json.RawMessage(`{"query":"x","timeout":"2m"}`))
	require.NoError(t, result.Error)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(received, &parsed))
	require.Equal(t, "x", parsed["query"])
	require.NotContains(t, parsed, "timeout")
}

func TestTimeoutMiddlewareClampsToMax(t *testing.T) {
	a := action.New(action.Spec{Name: "clamp"}, func(ctx action.Ctx, input any) action.Result {
		deadline, ok := ctx.Deadline()
		require.True(t, ok)
		require.Less(t, time.Until(deadline), 2*time.Second)
		return action.Result{Data: input}
	})

	wrapped := action.Apply(a, NewTimeoutMiddleware(30*time.Second, time.Second))
	result := wrapped.Execute(action.NewCtx(context.Background()), json.RawMessage(`{"timeout":"10m"}`))
	require.NoError(t, result.Error)
}

func TestTimeoutMiddlewareLeavesGoNativeInputUntouched(t *testing.T) {
	type input struct{ Name string }
	a := action.New(action.Spec{Name: "native"}, func(ctx action.Ctx, v any) action.Result {
		_, ok := ctx.Deadline()
		require.True(t, ok)
		require.Equal(t, input{Name: "Ada"}, v)
		return action.Result{Data: "ok"}
	})

	wrapped := action.Apply(a, NewTimeoutMiddleware(30*time.Second, 0))
	result := wrapped.Execute(action.NewCtx(context.Background()), input{Name: "Ada"})
	require.NoError(t, result.Error)
}

func TestTimeoutMiddlewareAnnotatesDeadlineError(t *testing.T) {
	a := action.New(action.Spec{Name: "slow"}, func(ctx action.Ctx, _ any) action.Result {
		<-ctx.Done()
		return action.Result{Error: ctx.Err()}
	})

	wrapped := action.Apply(a, NewTimeoutMiddleware(time.Millisecond, 0))
	result := wrapped.Execute(action.NewCtx(context.Background()), nil)
	require.Error(t, result.Error)
	require.True(t, errors.Is(result.Error, context.DeadlineExceeded))
	require.Contains(t, result.Error.Error(), "timed out after")
}

func TestParseDuration(t *testing.T) {
	dur, err := ParseDuration("2min")
	require.NoError(t, err)
	require.Equal(t, 2*time.Minute, dur)
}
