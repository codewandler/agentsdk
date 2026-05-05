package actionmw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/codewandler/agentsdk/action"
)

// TimeoutMiddleware adds a per-call timeout to an action.
//
// For JSON-shaped inputs, an optional top-level "timeout" string field is
// parsed, clamped, stripped, and the remaining input is forwarded. For other
// Go-native inputs, Default is applied without input mutation.
type TimeoutMiddleware struct {
	action.HooksBase

	Default time.Duration
	Max     time.Duration
}

// NewTimeoutMiddleware creates action middleware for per-call deadlines.
func NewTimeoutMiddleware(defaultTimeout, maxTimeout time.Duration) action.Middleware {
	return action.HooksMiddleware(&TimeoutMiddleware{Default: defaultTimeout, Max: maxTimeout})
}

func (m *TimeoutMiddleware) OnInput(_ action.Ctx, _ action.Action, input any, state action.CallState) (any, action.Result, bool) {
	next, dur := extractTimeout(input, m.Default)
	if m.Max > 0 && dur > m.Max {
		dur = m.Max
	}
	state["timeout"] = dur
	return next, action.Result{}, false
}

func (m *TimeoutMiddleware) OnContext(ctx action.Ctx, state action.CallState) (action.Ctx, func()) {
	dur, _ := state["timeout"].(time.Duration)
	if dur <= 0 {
		return ctx, nil
	}
	deadlineCtx, cancel := context.WithTimeout(ctx, dur)
	wrapped := action.NewCtx(deadlineCtx,
		action.WithOutput(ctx.Output()),
		action.WithEmit(ctx.Emit),
	)
	return wrapped, cancel
}

func (m *TimeoutMiddleware) OnResult(_ action.Ctx, _ action.Action, _ any, result action.Result, state action.CallState) action.Result {
	if !errors.Is(result.Error, context.DeadlineExceeded) {
		return result
	}
	dur, _ := state["timeout"].(time.Duration)
	label := fmt.Sprintf("timed out after %s", formatDuration(dur))
	result.Error = fmt.Errorf("%s: %w", label, result.Error)
	return result
}

func extractTimeout(input any, fallback time.Duration) (any, time.Duration) {
	switch v := input.(type) {
	case json.RawMessage:
		return stripTimeoutFromJSON(v, fallback)
	case []byte:
		next, dur := stripTimeoutFromJSON(json.RawMessage(v), fallback)
		if raw, ok := next.(json.RawMessage); ok {
			return []byte(raw), dur
		}
		return next, dur
	case string:
		next, dur := stripTimeoutFromJSON(json.RawMessage(v), fallback)
		if raw, ok := next.(json.RawMessage); ok {
			return string(raw), dur
		}
		return next, dur
	default:
		return input, fallback
	}
}

func stripTimeoutFromJSON(input json.RawMessage, fallback time.Duration) (any, time.Duration) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input, &raw); err != nil {
		return input, fallback
	}
	dur := fallback
	if timeoutRaw, has := raw["timeout"]; has {
		delete(raw, "timeout")
		stripped, _ := json.Marshal(raw)
		input = stripped
		var s string
		if json.Unmarshal(timeoutRaw, &s) == nil && s != "" {
			if parsed, err := ParseDuration(s); err == nil {
				dur = parsed
			}
		}
	}
	return input, dur
}

// formatDuration returns a human-friendly duration string.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "none"
	}
	if d >= time.Hour {
		if d%time.Hour == 0 {
			return fmt.Sprintf("%dh", int(d.Hours()))
		}
		return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
	}
	if d >= time.Minute {
		if d%time.Minute == 0 {
			return fmt.Sprintf("%dm", int(d.Minutes()))
		}
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
