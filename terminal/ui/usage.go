package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/codewandler/agentsdk/capabilities/planner"
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

// PrintThreadEvent renders a persisted thread event that is not handled
// by a specific capability renderer. Currently a no-op for unrecognized
// event kinds.
func PrintThreadEvent(w io.Writer, event thread.Event) {
	// Non-planner thread events are silently ignored for now.
	// Planner events are handled by EventDisplay.handleThreadEvent.
}

// applyPlanEvent decodes a thread event as a planner state event and applies
// it to the plan. Returns true if the event was a planner event.
func applyPlanEvent(plan *planner.Plan, created *bool, event thread.Event) bool {
	if event.Kind != capability.EventStateEventDispatched {
		return false
	}
	var dispatched capability.StateEventDispatched
	if err := json.Unmarshal(event.Payload, &dispatched); err != nil {
		return false
	}
	if dispatched.CapabilityName != "planner" {
		return false
	}
	switch dispatched.EventName {
	case "plan_created":
		var p planner.PlanCreated
		if json.Unmarshal(dispatched.Body, &p) != nil {
			return false
		}
		*plan = planner.Plan{ID: p.PlanID, Title: p.Title}
		*created = true
	case "step_added":
		var s planner.StepAdded
		if json.Unmarshal(dispatched.Body, &s) != nil {
			return false
		}
		if s.Step.Status == "" {
			s.Step.Status = planner.StepPending
		}
		plan.Steps = append(plan.Steps, s.Step)
	case "step_removed":
		var s planner.StepRemoved
		if json.Unmarshal(dispatched.Body, &s) != nil {
			return false
		}
		for i, step := range plan.Steps {
			if step.ID == s.StepID {
				plan.Steps = append(plan.Steps[:i], plan.Steps[i+1:]...)
				break
			}
		}
	case "step_status_changed":
		var s planner.StepStatusChanged
		if json.Unmarshal(dispatched.Body, &s) != nil {
			return false
		}
		for i := range plan.Steps {
			if plan.Steps[i].ID == s.StepID {
				plan.Steps[i].Status = s.Status
				break
			}
		}
	case "step_title_changed":
		var s planner.StepTitleChanged
		if json.Unmarshal(dispatched.Body, &s) != nil {
			return false
		}
		for i := range plan.Steps {
			if plan.Steps[i].ID == s.StepID {
				plan.Steps[i].Title = s.Title
				break
			}
		}
	case "current_step_changed":
		var s planner.CurrentStepChanged
		if json.Unmarshal(dispatched.Body, &s) != nil {
			return false
		}
		plan.CurrentStepID = s.StepID
	default:
		return false
	}
	return true
}

// PrintPlan renders the full plan as a markdown checklist.
func PrintPlan(w io.Writer, plan planner.Plan) {
	var md strings.Builder
	title := plan.Title
	if title == "" {
		title = plan.ID
	}
	fmt.Fprintf(&md, "**Plan: %s**\n\n", title)
	for _, step := range plan.Steps {
		box := "[ ]"
		switch step.Status {
		case planner.StepCompleted:
			box = "[x]"
		case planner.StepInProgress:
			box = "[>]"
		}
		prefix := ""
		if step.ParentID != "" {
			prefix = "  "
		}
		cursor := ""
		if step.ID == plan.CurrentStepID {
			cursor = " <<"
		}
		fmt.Fprintf(&md, "%s- %s %s%s\n", prefix, box, step.Title, cursor)
	}

	var buf bytes.Buffer
	sr := newLiveMarkdownRenderer(&buf)
	_, _ = sr.Write([]byte(md.String()))
	_ = sr.Flush()
	rendered := TrimOuterRenderedBlankLines(buf.String())
	if rendered != "" {
		fmt.Fprintf(w, "\n%s\n\n", rendered)
	}
}

// PrintToolResultCompact prints a one-line ok/err status for a tool call.
func PrintToolResultCompact(w io.Writer, name string, isError bool) {
	if isError {
		fmt.Fprintf(w, "%serr%s\n", BrightRed, Reset)
	} else {
		fmt.Fprintf(w, "%sok%s\n", BrightGreen, Reset)
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

// PrintStepUsage renders a compact step usage summary line.
// When debug is true, additional detail lines are printed.
func PrintStepUsage(w io.Writer, step int, rec usage.Record, model string) {
	printStepUsageWithDebug(w, step, rec, model, false)
}

// PrintStepUsageDebug renders step usage with full detail lines.
func PrintStepUsageDebug(w io.Writer, step int, rec usage.Record, model string) {
	printStepUsageWithDebug(w, step, rec, model, true)
}

func printStepUsageWithDebug(w io.Writer, step int, rec usage.Record, model string, debug bool) {
	parts := FormatUsageParts(rec)
	if parts == "" {
		return
	}
	fmt.Fprintf(w, "%s   step %d \u00b7 %s%s\n", Dim, step, parts, Reset)
	if debug {
		printStepUsageDetails(w, rec)
	}
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
