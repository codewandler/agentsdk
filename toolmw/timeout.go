package toolmw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/codewandler/agentsdk/tool"
	"github.com/invopop/jsonschema"
)

// TimeoutMiddleware adds a per-call timeout to any tool.
// It extends the tool's schema with an optional "timeout" string property
// so the LLM can request a custom duration (e.g. "30s", "2m", "5min").
//
// The timeout is parsed from the LLM's input, clamped to Max, stripped
// from the input before forwarding to the inner tool, and applied as a
// context deadline. If the tool times out, the result is annotated with
// the duration.
type TimeoutMiddleware struct {
	tool.HooksBase

	// Default is the timeout used when the LLM doesn't specify one.
	Default time.Duration

	// Max is the hard cap. Any requested timeout above this is clamped.
	// Zero means no cap.
	Max time.Duration
}

// NewTimeoutMiddleware creates a Middleware from a TimeoutMiddleware.
func NewTimeoutMiddleware(defaultTimeout, maxTimeout time.Duration) tool.Middleware {
	return tool.HooksMiddleware(&TimeoutMiddleware{
		Default: defaultTimeout,
		Max:     maxTimeout,
	})
}

func (m *TimeoutMiddleware) OnSchema(inner tool.Tool) *jsonschema.Schema {
	base := inner.Schema()
	extended := cloneSchema(base)
	if extended.Properties == nil {
		extended.Properties = jsonschema.NewProperties()
	}
	extended.Properties.Set("timeout", &jsonschema.Schema{
		Type:        "string",
		Description: "Per-call timeout duration (e.g. '30s', '2m', '5m'). Optional.",
		Examples:    []any{"30s", "2m", "5m"},
	})
	return extended
}

func (m *TimeoutMiddleware) OnGuidance(inner tool.Tool) string {
	base := inner.Guidance()
	extra := fmt.Sprintf(
		"Accepts an optional `timeout` parameter for long-running operations (default %s, max %s).",
		formatDuration(m.Default), formatDuration(m.Max),
	)
	if base != "" {
		return base + "\n" + extra
	}
	return extra
}

func (m *TimeoutMiddleware) OnInput(_ tool.Ctx, _ tool.Tool, input json.RawMessage, state tool.CallState) (json.RawMessage, tool.Result, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(input, &raw); err != nil {
		// Not a JSON object — pass through with default timeout.
		state["timeout"] = m.Default
		return input, nil, nil
	}

	dur := m.Default
	if timeoutRaw, has := raw["timeout"]; has {
		// Strip timeout from input before forwarding to inner tool.
		delete(raw, "timeout")
		// Marshal cannot fail here: raw was just unmarshaled from valid JSON
		// and we only deleted a key.
		stripped, _ := json.Marshal(raw)
		input = stripped

		var s string
		if json.Unmarshal(timeoutRaw, &s) == nil && s != "" {
			if parsed, err := parseDuration(s); err == nil {
				dur = parsed
			}
		}
	}

	if m.Max > 0 && dur > m.Max {
		dur = m.Max
	}
	state["timeout"] = dur
	return input, nil, nil
}

func (m *TimeoutMiddleware) OnContext(ctx tool.Ctx, state tool.CallState) (tool.Ctx, func()) {
	dur, _ := state["timeout"].(time.Duration)
	if dur > 0 {
		// ctx satisfies context.Context (tool.Ctx embeds it), so
		// WithTimeout inherits the parent's deadline chain. WrapCtx
		// then preserves the tool metadata (WorkDir, etc.) from ctx
		// while using newCtx for deadline/cancellation/values.
		newCtx, cancel := context.WithTimeout(ctx, dur)
		return tool.WrapCtx(ctx, newCtx), cancel
	}
	return ctx, nil
}

func (m *TimeoutMiddleware) OnResult(_ tool.Ctx, _ tool.Tool, _ json.RawMessage, res tool.Result, err error, state tool.CallState) (tool.Result, error) {
	if !errors.Is(err, context.DeadlineExceeded) {
		return res, err
	}

	dur, _ := state["timeout"].(time.Duration)
	label := fmt.Sprintf("[Timed out after %s]", formatDuration(dur))
	if res != nil {
		partial := res.String()
		if partial != "" {
			return tool.Error(partial + "\n\n" + label), nil
		}
	}
	return tool.Error(label), nil
}

// cloneSchema performs a JSON round-trip clone of a schema.
// This is safe and schemas are small.
func cloneSchema(s *jsonschema.Schema) *jsonschema.Schema {
	if s == nil {
		return &jsonschema.Schema{Type: "object"}
	}
	data, err := json.Marshal(s)
	if err != nil {
		// Shouldn't happen with a valid schema; return a minimal fallback.
		return &jsonschema.Schema{Type: "object"}
	}
	var cloned jsonschema.Schema
	if err := json.Unmarshal(data, &cloned); err != nil {
		return &jsonschema.Schema{Type: "object"}
	}
	return &cloned
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
