package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/unified"
)

func PrintStepHeader(w io.Writer, step, maxSteps int) {
	fmt.Fprintf(w, "\n%s-- %sStep %d/%d%s %s--------------------------------%s\n",
		Dim, Bold+BrightCyan, step, maxSteps, Reset, Dim, Reset)
}

func PrintResolvedModel(w io.Writer, model string) {
	if model != "" {
		fmt.Fprintf(w, "%s   model: %s%s\n", Dim, model, Reset)
	}
}

// PrintToolOutputDelta renders a single streaming output chunk from a tool.
func PrintToolOutputDelta(w io.Writer, chunk string) {
	if chunk == "" {
		return
	}
	fmt.Fprintf(w, "%s  %s%s\n", Dim, chunk, Reset)
}

// PrintToolStatus renders a status/progress message from a tool.
func PrintToolStatus(w io.Writer, message string) {
	if message == "" {
		return
	}
	fmt.Fprintf(w, "%s  ⟳ %s%s\n", Dim+Italic, message, Reset)
}

// PrintThreadEvent renders a persisted thread event. Currently handles
// planner capability state events; other event kinds are silently ignored.
func PrintThreadEvent(w io.Writer, event thread.Event) {
	if event.Kind != capability.EventStateEventDispatched {
		return
	}
	var dispatched capability.StateEventDispatched
	if err := json.Unmarshal(event.Payload, &dispatched); err != nil {
		return
	}
	if dispatched.CapabilityName != "planner" {
		return
	}
	printPlannerEvent(w, dispatched)
}

func printPlannerEvent(w io.Writer, dispatched capability.StateEventDispatched) {
	switch dispatched.EventName {
	case "plan_created":
		var p struct {
			Title string `json:"title"`
		}
		_ = json.Unmarshal(dispatched.Body, &p)
		if p.Title != "" {
			fmt.Fprintf(w, "%s  \U0001F4CB Plan: %s%s\n", Dim, p.Title, Reset)
		}
	case "step_added":
		var s struct {
			Step struct {
				Title string `json:"title"`
			} `json:"step"`
		}
		_ = json.Unmarshal(dispatched.Body, &s)
		if s.Step.Title != "" {
			fmt.Fprintf(w, "%s  + %s%s\n", Dim, s.Step.Title, Reset)
		}
	case "step_status_changed":
		var s struct {
			StepID string `json:"step_id"`
			Status string `json:"status"`
		}
		_ = json.Unmarshal(dispatched.Body, &s)
		icon := "o"
		switch s.Status {
		case "in_progress":
			icon = ">"
		case "completed":
			icon = "v"
		}
		fmt.Fprintf(w, "%s  %s %s -> %s%s\n", Dim, icon, s.StepID, s.Status, Reset)
	case "step_removed":
		var s struct {
			StepID string `json:"step_id"`
		}
		_ = json.Unmarshal(dispatched.Body, &s)
		fmt.Fprintf(w, "%s  - %s%s\n", Dim, s.StepID, Reset)
	case "current_step_changed":
		var s struct {
			StepID string `json:"step_id"`
		}
		_ = json.Unmarshal(dispatched.Body, &s)
		if s.StepID != "" {
			fmt.Fprintf(w, "%s  >> current: %s%s\n", Dim, s.StepID, Reset)
		}
	}
}

func PrintToolResult(w io.Writer, output string, isError bool) {
	prefix := BrightGreen + "ok" + Reset
	if isError {
		prefix = BrightRed + "err" + Reset
	}
	display := Truncate(strings.TrimSpace(output), 300)
	if display == "" {
		display = "(no output)"
	}
	fmt.Fprintf(w, "%s %s\n", prefix, display)
}

func PrintStepUsage(w io.Writer, step int, rec usage.Record, model string) {
	parts := FormatUsageParts(rec)
	modelPart := ""
	if model != "" {
		modelPart = fmt.Sprintf("  model: %s", model)
	}
	if parts == "" && modelPart == "" {
		return
	}
	if parts == "" {
		fmt.Fprintf(w, "%s   -- step %d --%s%s\n", Dim, step, modelPart, Reset)
	} else {
		fmt.Fprintf(w, "%s   -- step %d -- %s%s%s\n", Dim, step, parts, modelPart, Reset)
	}
	printStepUsageDetails(w, rec)
}

func PrintTurnUsage(w io.Writer, turnID int, rec usage.Record) {
	parts := FormatUsageParts(rec)
	if parts != "" {
		fmt.Fprintf(w, "%s   -- turn %d -- %s%s\n", Dim, turnID, parts, Reset)
	}
}

func PrintSessionUsage(w io.Writer, sessionID string, rec usage.Record) {
	parts := FormatUsageParts(rec)
	if parts == "" {
		fmt.Fprintf(w, "-- session %s --\n", sessionID)
		return
	}
	fmt.Fprintf(w, "-- session %s -- %s\n", sessionID, parts)
}

func PrintError(w io.Writer, err error) {
	fmt.Fprintf(w, "\n%sError: %s%s\n", BrightRed, err, Reset)
}

func printStepUsageDetails(w io.Writer, rec usage.Record) {
	if parts := stepUsageDimsParts(rec); len(parts) > 0 {
		fmt.Fprintf(w, "%s   dims: %s%s\n", Dim, strings.Join(parts, " "), Reset)
	}
	if parts := stepUsageUsageParts(rec); len(parts) > 0 {
		fmt.Fprintf(w, "%s   usage: %s%s\n", Dim, strings.Join(parts, " "), Reset)
	}
	if parts := stepUsageCostParts(rec); len(parts) > 0 {
		fmt.Fprintf(w, "%s   costs: %s%s\n", Dim, strings.Join(parts, " "), Reset)
	}
}

func stepUsageDimsParts(rec usage.Record) []string {
	var parts []string
	if rec.Dims.Provider != "" {
		parts = append(parts, fmt.Sprintf("provider=%s", rec.Dims.Provider))
	}
	if rec.Dims.Model != "" {
		parts = append(parts, fmt.Sprintf("model=%s", rec.Dims.Model))
	}
	if rec.Dims.RequestID != "" {
		parts = append(parts, fmt.Sprintf("request_id=%s", rec.Dims.RequestID))
	}
	if rec.Dims.TurnID != "" {
		parts = append(parts, fmt.Sprintf("turn_id=%s", rec.Dims.TurnID))
	}
	if rec.Dims.SessionID != "" {
		parts = append(parts, fmt.Sprintf("session_id=%s", rec.Dims.SessionID))
	}
	return parts
}

func stepUsageUsageParts(rec usage.Record) []string {
	var parts []string
	if v := rec.Usage.Tokens.InputTotal(); v != 0 {
		parts = append(parts, fmt.Sprintf("total_input=%d", v))
	}
	if v := rec.Usage.Tokens.Count(unified.TokenKindInputNew); v != 0 {
		parts = append(parts, fmt.Sprintf("input=%d", v))
	}
	if v := rec.Usage.Tokens.Count(unified.TokenKindInputCacheRead); v != 0 {
		parts = append(parts, fmt.Sprintf("cache_read=%d", v))
	}
	if v := rec.Usage.Tokens.Count(unified.TokenKindInputCacheWrite); v != 0 {
		parts = append(parts, fmt.Sprintf("cache_write=%d", v))
	}
	if v := rec.Usage.Tokens.OutputTotal(); v != 0 {
		parts = append(parts, fmt.Sprintf("total_output=%d", v))
	}
	return parts
}

func stepUsageCostParts(rec usage.Record) []string {
	var parts []string
	if v := rec.Usage.Costs.Total(); v != 0 {
		parts = append(parts, fmt.Sprintf("total=%.6f", v))
	}
	return parts
}
