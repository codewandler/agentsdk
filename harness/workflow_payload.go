package harness

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/workflow"
)

type WorkflowListPayload struct {
	Definitions []workflow.Definition
}

func (p WorkflowListPayload) Display(command.DisplayMode) (string, error) {
	if len(p.Definitions) == 0 {
		return "No workflows registered.", nil
	}
	var b strings.Builder
	b.WriteString("Workflows:")
	for _, def := range p.Definitions {
		fmt.Fprintf(&b, "\n- %s", def.Name)
		if def.Description != "" {
			fmt.Fprintf(&b, ": %s", def.Description)
		}
	}
	return b.String(), nil
}

type WorkflowDefinitionPayload struct {
	Definition workflow.Definition
}

func (p WorkflowDefinitionPayload) Display(command.DisplayMode) (string, error) {
	def := p.Definition
	var b strings.Builder
	fmt.Fprintf(&b, "workflow: %s", def.Name)
	if def.Description != "" {
		fmt.Fprintf(&b, "\ndescription: %s", def.Description)
	}
	if def.Version != "" {
		fmt.Fprintf(&b, "\nversion: %s", def.Version)
	}
	if hash := workflow.DefinitionHash(def); hash != "" {
		fmt.Fprintf(&b, "\nhash: %s", hash)
	}
	if len(def.Steps) == 0 {
		return b.String(), nil
	}
	b.WriteString("\n\nsteps:")
	for _, step := range def.Steps {
		fmt.Fprintf(&b, "\n- %s: %s", step.ID, step.Action.Name)
		if step.Action.Version != "" {
			fmt.Fprintf(&b, "@%s", step.Action.Version)
		}
		if len(step.DependsOn) > 0 {
			fmt.Fprintf(&b, " depends_on=%s", strings.Join(step.DependsOn, ","))
		}
	}
	return b.String(), nil
}

type WorkflowStartPayload struct {
	WorkflowName string
	RunID        workflow.RunID
	Status       workflow.RunStatus
	Output       any
	Error        string
}

func (p WorkflowStartPayload) Display(command.DisplayMode) (string, error) {
	var b strings.Builder
	switch p.Status {
	case workflow.RunFailed:
		fmt.Fprintf(&b, "workflow failed: %s\n", p.WorkflowName)
	case workflow.RunQueued, workflow.RunRunning:
		fmt.Fprintf(&b, "workflow started: %s\n", p.WorkflowName)
	default:
		fmt.Fprintf(&b, "workflow completed: %s\n", p.WorkflowName)
	}
	fmt.Fprintf(&b, "run: %s\n", p.RunID)
	fmt.Fprintf(&b, "status: %s", p.Status)
	if p.Error != "" {
		fmt.Fprintf(&b, "\nerror: %s", p.Error)
	}
	if p.Output != nil {
		fmt.Fprintf(&b, "\noutput: %v", p.Output)
	}
	return b.String(), nil
}

type WorkflowRunFilters struct {
	WorkflowName string
	Status       workflow.RunStatus
	Limit        int
	Offset       int
}

func (f WorkflowRunFilters) IsZero() bool {
	return f.WorkflowName == "" && f.Status == "" && f.Limit <= 0 && f.Offset <= 0
}

type WorkflowRunsPayload struct {
	Summaries []workflow.RunSummary
	Filters   WorkflowRunFilters
}

func (p WorkflowRunsPayload) Display(command.DisplayMode) (string, error) {
	if len(p.Summaries) == 0 {
		if !p.Filters.IsZero() {
			return "No workflow runs matched filters.", nil
		}
		return "No workflow runs recorded.", nil
	}
	var b strings.Builder
	b.WriteString("Workflow runs:")
	if !p.Filters.IsZero() {
		b.WriteString("\nfilters:")
		if p.Filters.WorkflowName != "" {
			fmt.Fprintf(&b, " workflow=%s", p.Filters.WorkflowName)
		}
		if p.Filters.Status != "" {
			fmt.Fprintf(&b, " status=%s", p.Filters.Status)
		}
		if p.Filters.Limit > 0 {
			fmt.Fprintf(&b, " limit=%d", p.Filters.Limit)
		}
		if p.Filters.Offset > 0 {
			fmt.Fprintf(&b, " offset=%d", p.Filters.Offset)
		}
	}
	b.WriteByte('\n')
	fmt.Fprintf(&b, "%-18s  %-20s  %-10s  %-20s  %s", "RUN ID", "WORKFLOW", "STATUS", "STARTED", "DURATION")
	for _, summary := range p.Summaries {
		fmt.Fprintf(&b, "\n%-18s  %-20s  %-10s  %-20s  %s", summary.ID, summary.WorkflowName, summary.Status, formatWorkflowTime(summary.StartedAt), formatWorkflowDuration(summary.Duration))
		if summary.Error != "" {
			fmt.Fprintf(&b, "  error=%s", summary.Error)
		}
	}
	return b.String(), nil
}

type WorkflowRunPayload struct {
	State workflow.RunState
}

func (p WorkflowRunPayload) Display(command.DisplayMode) (string, error) {
	state := p.State
	var b strings.Builder
	fmt.Fprintf(&b, "workflow run: %s\n", state.ID)
	fmt.Fprintf(&b, "workflow: %s\n", state.WorkflowName)
	fmt.Fprintf(&b, "status: %s", state.Status)
	if !state.StartedAt.IsZero() {
		fmt.Fprintf(&b, "\nstarted: %s", formatWorkflowTime(state.StartedAt))
	}
	if !state.CompletedAt.IsZero() {
		fmt.Fprintf(&b, "\ncompleted: %s", formatWorkflowTime(state.CompletedAt))
	}
	if state.Duration > 0 {
		fmt.Fprintf(&b, "\nduration: %s", formatWorkflowDuration(state.Duration))
	}
	if state.Error != "" {
		fmt.Fprintf(&b, "\nerror: %s", state.Error)
	}
	writeWorkflowMetadata(&b, state.Metadata)
	if !emptyWorkflowValue(state.Input) {
		fmt.Fprintf(&b, "\ninput: %s", renderWorkflowValue(state.Input))
	}
	if state.DefinitionVersion != "" {
		fmt.Fprintf(&b, "\ndefinition_version: %s", state.DefinitionVersion)
	}
	if state.DefinitionHash != "" {
		fmt.Fprintf(&b, "\ndefinition_hash: %s", state.DefinitionHash)
	}
	if !emptyWorkflowValue(state.Output) {
		fmt.Fprintf(&b, "\noutput: %s", renderWorkflowValue(state.Output))
	}
	if len(state.Steps) == 0 {
		return b.String(), nil
	}
	b.WriteString("\n\nsteps:")
	ids := make([]string, 0, len(state.Steps))
	for id := range state.Steps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		step := state.Steps[id]
		fmt.Fprintf(&b, "\n- %s", step.ID)
		if step.ActionName != "" && step.ActionName != step.ID {
			fmt.Fprintf(&b, "\n  action: %s", step.ActionName)
		}
		fmt.Fprintf(&b, "\n  status: %s", step.Status)
		if step.Attempt > 0 {
			fmt.Fprintf(&b, "\n  attempt: %d", step.Attempt)
		}
		if !emptyWorkflowValue(step.Output) {
			fmt.Fprintf(&b, "\n  output: %s", renderWorkflowValue(step.Output))
		}
		if step.Error != "" {
			fmt.Fprintf(&b, "\n  error: %s", step.Error)
		}
	}
	return b.String(), nil
}

type WorkflowEventsPayload struct {
	RunID  workflow.RunID
	Events []any
}

func (p WorkflowEventsPayload) Display(command.DisplayMode) (string, error) {
	if len(p.Events) == 0 {
		return fmt.Sprintf("No workflow events recorded for %s.", p.RunID), nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "workflow events: %s", p.RunID)
	for _, event := range p.Events {
		fmt.Fprintf(&b, "\n- %s", workflowEventName(event))
	}
	return b.String(), nil
}

type WorkflowCancelPayload struct {
	RunID  workflow.RunID
	Status workflow.RunStatus
	Error  string
}

func (p WorkflowCancelPayload) Display(command.DisplayMode) (string, error) {
	if p.Error != "" {
		return fmt.Sprintf("workflow cancel failed: %s\nrun: %s", p.Error, p.RunID), nil
	}
	return fmt.Sprintf("workflow canceled: %s\nstatus: %s", p.RunID, p.Status), nil
}

func writeWorkflowMetadata(b *strings.Builder, metadata workflow.RunMetadata) {
	if metadata.IsZero() {
		return
	}
	b.WriteString("\nmetadata:")
	if metadata.SessionID != "" {
		fmt.Fprintf(b, "\n  session: %s", metadata.SessionID)
	}
	if metadata.AgentName != "" {
		fmt.Fprintf(b, "\n  agent: %s", metadata.AgentName)
	}
	if metadata.ThreadID != "" {
		fmt.Fprintf(b, "\n  thread: %s", metadata.ThreadID)
	}
	if metadata.BranchID != "" {
		fmt.Fprintf(b, "\n  branch: %s", metadata.BranchID)
	}
	if metadata.Trigger != "" {
		fmt.Fprintf(b, "\n  trigger: %s", metadata.Trigger)
	}
	if len(metadata.CommandPath) > 0 {
		fmt.Fprintf(b, "\n  command: /%s", strings.Join(metadata.CommandPath, " "))
	}
}

func workflowEventName(event any) string {
	switch event.(type) {
	case workflow.Queued:
		return string(workflow.EventQueued)
	case workflow.Started:
		return string(workflow.EventStarted)
	case workflow.StepStarted:
		return string(workflow.EventStepStarted)
	case workflow.StepCompleted:
		return string(workflow.EventStepCompleted)
	case workflow.StepFailed:
		return string(workflow.EventStepFailed)
	case workflow.Completed:
		return string(workflow.EventCompleted)
	case workflow.Failed:
		return string(workflow.EventFailed)
	case workflow.Canceled:
		return string(workflow.EventCanceled)
	default:
		return fmt.Sprintf("%T", event)
	}
}

func formatWorkflowTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format(time.RFC3339)
}

func formatWorkflowDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	return d.Round(time.Millisecond).String()
}

func emptyWorkflowValue(value workflow.ValueRef) bool {
	return value.ID == "" && value.MediaType == "" && value.ExternalURI == "" && value.Inline == nil && !value.Redacted
}

func renderWorkflowValue(value workflow.ValueRef) string {
	switch {
	case value.Redacted:
		if value.ID != "" {
			return fmt.Sprintf("redacted:%s", value.ID)
		}
		return "redacted"
	case value.ExternalURI != "":
		if value.MediaType != "" {
			return fmt.Sprintf("%s (%s)", value.ExternalURI, value.MediaType)
		}
		return value.ExternalURI
	case value.ID != "":
		return value.ID
	default:
		return fmt.Sprint(value.Inline)
	}
}
