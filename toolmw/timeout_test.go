package toolmw

import (
	"context"

	"github.com/codewandler/agentsdk/action"
	"encoding/json"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/tool"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/require"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

type stubTool struct {
	name    string
	schema  *jsonschema.Schema
	guid    string
	handler func(ctx tool.Ctx, input json.RawMessage) (tool.Result, error)
}

func (t *stubTool) Name() string               { return t.name }
func (t *stubTool) Description() string        { return "stub" }
func (t *stubTool) Guidance() string           { return t.guid }
func (t *stubTool) Schema() *jsonschema.Schema { return t.schema }
func (t *stubTool) Execute(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
	if t.handler != nil {
		return t.handler(ctx, input)
	}
	return tool.Text("ok"), nil
}

type stubCtx struct {
	action.BaseCtx
}

func (c stubCtx) WorkDir() string       { return "/tmp" }
func (c stubCtx) AgentID() string       { return "test" }
func (c stubCtx) SessionID() string     { return "sess" }
func (c stubCtx) Extra() map[string]any { return nil }

func testCtx() tool.Ctx {
	return stubCtx{BaseCtx: action.BaseCtx{Context: context.Background()}}
}

// ── Schema extension ──────────────────────────────────────────────────────────

func TestTimeoutMiddleware_ExtendsSchema(t *testing.T) {
	base := &stubTool{
		name: "test",
		schema: func() *jsonschema.Schema {
			s := &jsonschema.Schema{Type: "object"}
			s.Properties = jsonschema.NewProperties()
			s.Properties.Set("cmd", &jsonschema.Schema{Type: "string"})
			return s
		}(),
	}

	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))
	schema := wrapped.Schema()

	// Original property preserved.
	cmd, ok := schema.Properties.Get("cmd")
	require.True(t, ok)
	require.Equal(t, "string", cmd.Type)

	// Timeout property added.
	timeout, ok := schema.Properties.Get("timeout")
	require.True(t, ok)
	require.Equal(t, "string", timeout.Type)
}

func TestTimeoutMiddleware_ExtendsSchema_NilBase(t *testing.T) {
	base := &stubTool{name: "test", schema: nil}
	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 0))
	schema := wrapped.Schema()

	require.NotNil(t, schema)
	timeout, ok := schema.Properties.Get("timeout")
	require.True(t, ok)
	require.Equal(t, "string", timeout.Type)
}

func TestTimeoutMiddleware_DoesNotMutateOriginalSchema(t *testing.T) {
	original := &jsonschema.Schema{
		Type:       "object",
		Properties: jsonschema.NewProperties(),
	}
	original.Properties.Set("cmd", &jsonschema.Schema{Type: "string"})

	base := &stubTool{name: "test", schema: original}
	_ = tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))

	// Original should NOT have timeout property.
	_, ok := original.Properties.Get("timeout")
	require.False(t, ok, "original schema should not be mutated")
}

// ── Guidance ──────────────────────────────────────────────────────────────────

func TestTimeoutMiddleware_AppendsGuidance(t *testing.T) {
	base := &stubTool{name: "test", guid: "Run commands."}
	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))

	guid := wrapped.Guidance()
	require.Contains(t, guid, "Run commands.")
	require.Contains(t, guid, "timeout")
	require.Contains(t, guid, "30s")
	require.Contains(t, guid, "5m")
}

func TestTimeoutMiddleware_GuidanceWhenEmpty(t *testing.T) {
	base := &stubTool{name: "test", guid: ""}
	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))

	guid := wrapped.Guidance()
	require.Contains(t, guid, "timeout")
	require.NotContains(t, guid, "\n") // no leading newline
}

// ── Input stripping ──────────────────────────────────────────────────────────

func TestTimeoutMiddleware_StripsTimeoutFromInput(t *testing.T) {
	var receivedInput json.RawMessage
	base := &stubTool{
		name: "test",
		handler: func(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
			receivedInput = input
			return tool.Text("ok"), nil
		},
	}

	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))
	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{"cmd":"ls","timeout":"2m"}`))
	require.NoError(t, err)

	// Inner tool should NOT see the timeout field.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(receivedInput, &parsed))
	require.Equal(t, "ls", parsed["cmd"])
	_, hasTimeout := parsed["timeout"]
	require.False(t, hasTimeout, "timeout should be stripped from input")
}

func TestTimeoutMiddleware_PassthroughWhenNoTimeout(t *testing.T) {
	var receivedInput json.RawMessage
	base := &stubTool{
		name: "test",
		handler: func(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
			receivedInput = input
			return tool.Text("ok"), nil
		},
	}

	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))
	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{"cmd":"ls"}`))
	require.NoError(t, err)

	var parsed map[string]any
	require.NoError(t, json.Unmarshal(receivedInput, &parsed))
	require.Equal(t, "ls", parsed["cmd"])
}

// ── Timeout clamping ─────────────────────────────────────────────────────────

func TestTimeoutMiddleware_ClampsToMax(t *testing.T) {
	base := &stubTool{
		name: "test",
		handler: func(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			remaining := time.Until(deadline)
			// Should be clamped to ~1s (max), not 10m.
			require.Less(t, remaining, 2*time.Second)
			return tool.Text("ok"), nil
		},
	}

	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 1*time.Second))
	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{"timeout":"10m"}`))
	require.NoError(t, err)
}

// ── Context deadline ─────────────────────────────────────────────────────────

func TestTimeoutMiddleware_SetsDeadline(t *testing.T) {
	base := &stubTool{
		name: "test",
		handler: func(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			remaining := time.Until(deadline)
			require.Greater(t, remaining, time.Duration(0))
			require.Less(t, remaining, 35*time.Second)
			return tool.Text("ok"), nil
		},
	}

	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))
	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
}

func TestTimeoutMiddleware_SetsDeadlineFromLLMInput(t *testing.T) {
	base := &stubTool{
		name: "test",
		handler: func(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			remaining := time.Until(deadline)
			// Should be ~2m, not 30s default.
			require.Greater(t, remaining, time.Minute)
			return tool.Text("ok"), nil
		},
	}

	wrapped := tool.Apply(base, NewTimeoutMiddleware(30*time.Second, 5*time.Minute))
	_, err := wrapped.Execute(testCtx(), json.RawMessage(`{"timeout":"2m"}`))
	require.NoError(t, err)
}

// ── Timeout result annotation ────────────────────────────────────────────────

func TestTimeoutMiddleware_AnnotatesTimeoutResult(t *testing.T) {
	base := &stubTool{
		name: "test",
		handler: func(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
			// Simulate a tool that blocks until context expires.
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	// Use a very short timeout so the test is fast.
	wrapped := tool.Apply(base, NewTimeoutMiddleware(10*time.Millisecond, 0))
	res, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))

	// Timeout is converted from error to result (policy decision, not infra failure).
	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "Timed out")
}

func TestTimeoutMiddleware_AnnotatesTimeoutWithPartialResult(t *testing.T) {
	base := &stubTool{
		name: "test",
		handler: func(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
			<-ctx.Done()
			return tool.Text("partial output"), ctx.Err()
		},
	}

	wrapped := tool.Apply(base, NewTimeoutMiddleware(10*time.Millisecond, 0))
	res, err := wrapped.Execute(testCtx(), json.RawMessage(`{}`))

	require.NoError(t, err)
	require.NotNil(t, res)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "partial output")
	require.Contains(t, res.String(), "Timed out")
}

// ── formatDuration ───────────────────────────────────────────────────────────

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "none"},
		{30 * time.Second, "30s"},
		{2 * time.Minute, "2m"},
		{90 * time.Second, "1m30s"},
		{time.Hour, "1h"},
		{90 * time.Minute, "1h30m"},
		{5 * time.Minute, "5m"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			require.Equal(t, tt.want, formatDuration(tt.d))
		})
	}
}
