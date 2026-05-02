package harness

import (
	"context"
	"fmt"

	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/workflow"
)

type WorkflowCommandHandler struct {
	Session *Session
}

func NewWorkflowCommand(session *Session) (*command.Tree, error) {
	h := WorkflowCommandHandler{Session: session}
	return command.NewTree("workflow", command.Description("Inspect and run workflows")).
		Sub("list", h.workflowListCommand,
			command.Description("List workflows"),
		).
		Sub("show", h.workflowShowCommand,
			command.Description("Show workflow"),
			command.Arg("name").Required(),
		).
		Sub("start", h.workflowStartCommand,
			command.Description("Start workflow"),
			command.Arg("name").Required(),
			command.Arg("input").Variadic(),
		).
		Sub("runs", h.workflowRunsCommand,
			command.Description("List workflow runs"),
			command.Flag("workflow"),
			command.Flag("status").Enum(string(workflow.RunRunning), string(workflow.RunSucceeded), string(workflow.RunFailed)),
		).
		Sub("run", h.workflowRunCommand,
			command.Description("Show workflow run"),
			command.Arg("run-id").Required(),
		).
		Build()
}

func (h WorkflowCommandHandler) workflowListCommand(context.Context, command.Invocation) (command.Result, error) {
	return h.workflowList(), nil
}

func (h WorkflowCommandHandler) workflowShowCommand(_ context.Context, inv command.Invocation) (command.Result, error) {
	return h.workflowShow(inv.Arg("name")), nil
}

func (h WorkflowCommandHandler) workflowStartCommand(ctx context.Context, inv command.Invocation) (command.Result, error) {
	return h.workflowStart(ctx, inv.Arg("name"), inv.Arg("input"))
}

func (h WorkflowCommandHandler) workflowRunCommand(ctx context.Context, inv command.Invocation) (command.Result, error) {
	return h.workflowRun(ctx, workflow.RunID(inv.Arg("run-id")))
}

func (h WorkflowCommandHandler) workflowRunsCommand(ctx context.Context, inv command.Invocation) (command.Result, error) {
	filters := WorkflowRunFilters{
		WorkflowName: inv.Flag("workflow"),
		Status:       workflow.RunStatus(inv.Flag("status")),
	}
	return h.workflowRuns(ctx, filters)
}

func (h WorkflowCommandHandler) workflowList() command.Result {
	s := h.Session
	if s == nil || s.App == nil {
		return command.Display(WorkflowListPayload{})
	}
	return command.Display(WorkflowListPayload{Definitions: s.App.Workflows()})
}

func (h WorkflowCommandHandler) workflowShow(name string) command.Result {
	s := h.Session
	if s == nil || s.App == nil {
		return command.Text(fmt.Sprintf("workflow %q not found", name))
	}
	def, ok := s.App.Workflow(name)
	if !ok {
		return command.Text(fmt.Sprintf("workflow %q not found", name))
	}
	return command.Display(WorkflowDefinitionPayload{Definition: def})
}

func (h WorkflowCommandHandler) workflowStart(ctx context.Context, workflowName string, input string) (command.Result, error) {
	s := h.Session
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	if _, ok := s.App.Workflow(workflowName); !ok {
		return command.Text(fmt.Sprintf("workflow %q not found", workflowName)), nil
	}
	runID := workflow.NewRunID()
	result := s.ExecuteWorkflow(ctx, workflowName, input, app.WithWorkflowRunID(runID))
	if result.Error != nil {
		return command.Display(WorkflowStartPayload{WorkflowName: workflowName, RunID: runID, Status: workflow.RunFailed, Error: result.Error.Error()}), nil
	}
	data := result.Data
	if wfResult, ok := data.(workflow.Result); ok {
		data = wfResult.Data
	}
	return command.Display(WorkflowStartPayload{WorkflowName: workflowName, RunID: runID, Status: workflow.RunSucceeded, Output: data}), nil
}

func (h WorkflowCommandHandler) workflowRun(ctx context.Context, runID workflow.RunID) (command.Result, error) {
	s := h.Session
	if s == nil {
		return command.Text("workflow runs require a thread-backed session"), nil
	}
	state, ok, err := s.WorkflowRunState(ctx, runID)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		if _, hasStore := s.WorkflowRunStore(); !hasStore {
			return command.Text("workflow runs require a thread-backed session"), nil
		}
		return command.Text(fmt.Sprintf("workflow run %q not found", runID)), nil
	}
	return command.Display(WorkflowRunPayload{State: state}), nil
}

func (h WorkflowCommandHandler) workflowRuns(ctx context.Context, filters WorkflowRunFilters) (command.Result, error) {
	s := h.Session
	if s == nil {
		return command.Text("workflow runs require a thread-backed session"), nil
	}
	summaries, ok, err := s.WorkflowRuns(ctx)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		return command.Text("workflow runs require a thread-backed session"), nil
	}
	return command.Display(WorkflowRunsPayload{Summaries: filterWorkflowRuns(summaries, filters), Filters: filters}), nil
}

func filterWorkflowRuns(summaries []workflow.RunSummary, filters WorkflowRunFilters) []workflow.RunSummary {
	if filters.IsZero() {
		return summaries
	}
	out := make([]workflow.RunSummary, 0, len(summaries))
	for _, summary := range summaries {
		if filters.WorkflowName != "" && summary.WorkflowName != filters.WorkflowName {
			continue
		}
		if filters.Status != "" && summary.Status != filters.Status {
			continue
		}
		out = append(out, summary)
	}
	return out
}
