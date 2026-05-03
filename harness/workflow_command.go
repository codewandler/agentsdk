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

type workflowListCommandInput struct{}

type workflowShowCommandInput struct {
	Name string `command:"arg=name"`
}

type workflowStartCommandInput struct {
	Name  string `command:"arg=name"`
	Input string `command:"arg=input"`
}

type workflowRunCommandInput struct {
	RunID workflow.RunID `command:"arg=run-id"`
}

type workflowRunsCommandInput struct {
	Workflow string             `command:"flag=workflow"`
	Status   workflow.RunStatus `command:"flag=status"`
}

func newWorkflowCommand(session *Session) (*command.Tree, error) {
	h := WorkflowCommandHandler{Session: session}
	return command.NewTree("workflow", command.Description("Inspect and run workflows")).
		Sub("list", command.Typed(h.workflowListCommand),
			command.Description("List workflows"),
			command.WithPolicy(command.Policy{UserCallable: true, AgentCallable: true}),
		).
		Sub("show", command.Typed(h.workflowShowCommand),
			command.Description("Show workflow"),
			command.WithPolicy(command.Policy{UserCallable: true, AgentCallable: true}),
			command.TypedInput[workflowShowCommandInput](),
			command.Arg("name").Required(),
		).
		Sub("start", command.Typed(h.workflowStartCommand),
			command.Description("Start workflow"),
			command.TypedInput[workflowStartCommandInput](),
			command.Arg("name").Required(),
			command.Arg("input").Variadic(),
		).
		Sub("runs", command.Typed(h.workflowRunsCommand),
			command.Description("List workflow runs"),
			command.TypedInput[workflowRunsCommandInput](),
			command.Flag("workflow"),
			command.Flag("status").Enum(string(workflow.RunRunning), string(workflow.RunSucceeded), string(workflow.RunFailed)),
		).
		Sub("run", command.Typed(h.workflowRunCommand),
			command.Description("Show workflow run"),
			command.TypedInput[workflowRunCommandInput](),
			command.Arg("run-id").Required(),
		).
		Build()
}

func (h WorkflowCommandHandler) workflowListCommand(context.Context, workflowListCommandInput) (command.Result, error) {
	return h.workflowList(), nil
}

func (h WorkflowCommandHandler) workflowShowCommand(_ context.Context, input workflowShowCommandInput) (command.Result, error) {
	return h.workflowShow(input.Name), nil
}

func (h WorkflowCommandHandler) workflowStartCommand(ctx context.Context, input workflowStartCommandInput) (command.Result, error) {
	return h.workflowStart(ctx, input.Name, input.Input)
}

func (h WorkflowCommandHandler) workflowRunCommand(ctx context.Context, input workflowRunCommandInput) (command.Result, error) {
	return h.workflowRun(ctx, input.RunID)
}

func (h WorkflowCommandHandler) workflowRunsCommand(ctx context.Context, input workflowRunsCommandInput) (command.Result, error) {
	filters := WorkflowRunFilters{
		WorkflowName: input.Workflow,
		Status:       input.Status,
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
		return command.NotFound("workflow", name)
	}
	def, ok := s.App.Workflow(name)
	if !ok {
		return command.NotFound("workflow", name)
	}
	return command.Display(WorkflowDefinitionPayload{Definition: def})
}

func (h WorkflowCommandHandler) workflowStart(ctx context.Context, workflowName string, input string) (command.Result, error) {
	s := h.Session
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	if _, ok := s.App.Workflow(workflowName); !ok {
		return command.NotFound("workflow", workflowName), nil
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
		return command.Unavailable("workflow runs require a thread-backed session"), nil
	}
	state, ok, err := s.WorkflowRunState(ctx, runID)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		if _, hasStore := s.WorkflowRunStore(); !hasStore {
			return command.Unavailable("workflow runs require a thread-backed session"), nil
		}
		return command.NotFound("workflow run", string(runID)), nil
	}
	return command.Display(WorkflowRunPayload{State: state}), nil
}

func (h WorkflowCommandHandler) workflowRuns(ctx context.Context, filters WorkflowRunFilters) (command.Result, error) {
	s := h.Session
	if s == nil {
		return command.Unavailable("workflow runs require a thread-backed session"), nil
	}
	summaries, ok, err := s.WorkflowRuns(ctx)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		return command.Unavailable("workflow runs require a thread-backed session"), nil
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
