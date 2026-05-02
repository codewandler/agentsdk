package harness

import (
	"fmt"
	"sort"
	"strings"

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
	if len(def.Steps) == 0 {
		return b.String(), nil
	}
	b.WriteString("\n\nsteps:")
	for _, step := range def.Steps {
		fmt.Fprintf(&b, "\n- %s: %s", step.ID, step.Action.Name)
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
	if p.Status == workflow.RunFailed {
		fmt.Fprintf(&b, "workflow failed: %s\n", p.WorkflowName)
	} else {
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

type WorkflowRunsPayload struct {
	Summaries []workflow.RunSummary
}

func (p WorkflowRunsPayload) Display(command.DisplayMode) (string, error) {
	if len(p.Summaries) == 0 {
		return "No workflow runs recorded.", nil
	}
	var b strings.Builder
	b.WriteString("Workflow runs:\n")
	fmt.Fprintf(&b, "%-18s  %-20s  %s", "RUN ID", "WORKFLOW", "STATUS")
	for _, summary := range p.Summaries {
		fmt.Fprintf(&b, "\n%-18s  %-20s  %s", summary.ID, summary.WorkflowName, summary.Status)
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
	if state.Error != "" {
		fmt.Fprintf(&b, "\nerror: %s", state.Error)
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
