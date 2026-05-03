package harness

import (
	"context"
	"fmt"

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
	Async bool   `command:"flag=async"`
}

type workflowRunCommandInput struct {
	RunID workflow.RunID `command:"arg=run-id"`
}

type workflowRunsCommandInput struct {
	Workflow string             `command:"flag=workflow"`
	Status   workflow.RunStatus `command:"flag=status"`
	Limit    int                `command:"flag=limit"`
	Offset   int                `command:"flag=offset"`
}

type workflowRerunCommandInput struct {
	RunID workflow.RunID `command:"arg=run-id"`
	Async bool           `command:"flag=async"`
}

type workflowEventsCommandInput struct {
	RunID workflow.RunID `command:"arg=run-id"`
}

type workflowCancelCommandInput struct {
	RunID  workflow.RunID `command:"arg=run-id"`
	Reason string         `command:"arg=reason"`
}

func newWorkflowCommand(session *Session) (*command.Tree, error) {
	h := WorkflowCommandHandler{Session: session}
	return command.NewTree("workflow", command.Description("Inspect and run workflows")).
		Sub("list", command.Typed(h.workflowListCommand),
			command.Description("List workflows"),
			command.WithPolicy(command.Policy{UserCallable: true, AgentCallable: true}),
			command.Output(outputDescriptor("harness.workflow.list", "Registered workflow definitions")),
		).
		Sub("show", command.Typed(h.workflowShowCommand),
			command.Description("Show workflow"),
			command.WithPolicy(command.Policy{UserCallable: true, AgentCallable: true}),
			command.TypedInput[workflowShowCommandInput](),
			command.Arg("name").Required(),
			command.Output(outputDescriptor("harness.workflow.definition", "Workflow definition detail")),
		).
		Sub("start", command.Typed(h.workflowStartCommand),
			command.Description("Start workflow"),
			command.TypedInput[workflowStartCommandInput](),
			command.Arg("name").Required(),
			command.Arg("input").Variadic(),
			command.Flag("async").Describe("Start asynchronously when true"),
			command.Output(outputDescriptor("harness.workflow.start", "Started workflow run result")),
		).
		Sub("runs", command.Typed(h.workflowRunsCommand),
			command.Description("List workflow runs"),
			command.TypedInput[workflowRunsCommandInput](),
			command.Flag("workflow"),
			command.Flag("status").Enum(string(workflow.RunQueued), string(workflow.RunRunning), string(workflow.RunSucceeded), string(workflow.RunFailed), string(workflow.RunCanceled)),
			command.Flag("limit").Describe("Maximum number of runs to return"),
			command.Flag("offset").Describe("Number of runs to skip"),
			command.Output(outputDescriptor("harness.workflow.runs", "Workflow run summaries")),
		).
		Sub("run", command.Typed(h.workflowRunCommand),
			command.Description("Show workflow run"),
			command.TypedInput[workflowRunCommandInput](),
			command.Arg("run-id").Required(),
			command.Output(outputDescriptor("harness.workflow.run", "Workflow run detail")),
		).
		Sub("rerun", command.Typed(h.workflowRerunCommand),
			command.Description("Rerun workflow from a previous run input"),
			command.TypedInput[workflowRerunCommandInput](),
			command.Arg("run-id").Required(),
			command.Flag("async").Describe("Start asynchronously when true"),
			command.Output(outputDescriptor("harness.workflow.start", "Rerun workflow result")),
		).
		Sub("events", command.Typed(h.workflowEventsCommand),
			command.Description("Show workflow run events"),
			command.TypedInput[workflowEventsCommandInput](),
			command.Arg("run-id").Required(),
			command.Output(outputDescriptor("harness.workflow.events", "Workflow run events")),
		).
		Sub("cancel", command.Typed(h.workflowCancelCommand),
			command.Description("Cancel workflow run"),
			command.TypedInput[workflowCancelCommandInput](),
			command.Arg("run-id").Required(),
			command.Arg("reason").Variadic(),
			command.Output(outputDescriptor("harness.workflow.cancel", "Workflow cancellation result")),
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
	return h.workflowStart(ctx, input.Name, input.Input, input.Async)
}

func (h WorkflowCommandHandler) workflowRunCommand(ctx context.Context, input workflowRunCommandInput) (command.Result, error) {
	return h.workflowRun(ctx, input.RunID)
}

func (h WorkflowCommandHandler) workflowRunsCommand(ctx context.Context, input workflowRunsCommandInput) (command.Result, error) {
	filters := WorkflowRunFilters{
		WorkflowName: input.Workflow,
		Status:       input.Status,
		Limit:        input.Limit,
		Offset:       input.Offset,
	}
	return h.workflowRuns(ctx, filters)
}

func (h WorkflowCommandHandler) workflowRerunCommand(ctx context.Context, input workflowRerunCommandInput) (command.Result, error) {
	return h.workflowRerun(ctx, input.RunID, input.Async)
}

func (h WorkflowCommandHandler) workflowEventsCommand(ctx context.Context, input workflowEventsCommandInput) (command.Result, error) {
	return h.workflowEvents(ctx, input.RunID)
}

func (h WorkflowCommandHandler) workflowCancelCommand(ctx context.Context, input workflowCancelCommandInput) (command.Result, error) {
	return h.workflowCancel(ctx, input.RunID, input.Reason)
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

func (h WorkflowCommandHandler) workflowStart(ctx context.Context, workflowName string, input string, async bool) (command.Result, error) {
	s := h.Session
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	if _, ok := s.App.Workflow(workflowName); !ok {
		return command.NotFound("workflow", workflowName), nil
	}
	runID := workflow.NewRunID()
	if async {
		s.StartWorkflowWithRunID(ctx, runID, workflowName, input)
		return command.Display(WorkflowStartPayload{WorkflowName: workflowName, RunID: runID, Status: workflow.RunQueued}), nil
	}
	result := s.ExecuteWorkflow(ctx, workflowName, input, workflow.WithRunID(runID), workflow.WithRunMetadata(s.WorkflowRunMetadata("command", []string{"workflow", "start"})))
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

func (h WorkflowCommandHandler) workflowRerun(ctx context.Context, runID workflow.RunID, async bool) (command.Result, error) {
	s := h.Session
	if s == nil {
		return command.Unavailable("workflow runs require a thread-backed session"), nil
	}
	state, ok, err := s.WorkflowRunState(ctx, runID)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		return command.NotFound("workflow run", string(runID)), nil
	}
	input := state.Input.Inline
	return h.workflowStart(ctx, state.WorkflowName, fmt.Sprint(input), async)
}

func (h WorkflowCommandHandler) workflowEvents(ctx context.Context, runID workflow.RunID) (command.Result, error) {
	s := h.Session
	if s == nil {
		return command.Unavailable("workflow runs require a thread-backed session"), nil
	}
	events, ok, err := s.WorkflowRunEvents(ctx, runID)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		return command.NotFound("workflow run", string(runID)), nil
	}
	return command.Display(WorkflowEventsPayload{RunID: runID, Events: events}), nil
}

func (h WorkflowCommandHandler) workflowCancel(ctx context.Context, runID workflow.RunID, reason string) (command.Result, error) {
	s := h.Session
	if s == nil {
		return command.Unavailable("workflow runs require a thread-backed session"), nil
	}
	if err := s.CancelWorkflow(ctx, runID, reason); err != nil {
		return command.Display(WorkflowCancelPayload{RunID: runID, Status: workflow.RunFailed, Error: err.Error()}), nil
	}
	state, ok, err := s.WorkflowRunState(ctx, runID)
	if err != nil {
		return command.Result{}, err
	}
	status := workflow.RunCanceled
	if ok {
		status = state.Status
	}
	return command.Display(WorkflowCancelPayload{RunID: runID, Status: status}), nil
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
	if filters.Offset > 0 {
		if filters.Offset >= len(out) {
			return nil
		}
		out = out[filters.Offset:]
	}
	if filters.Limit > 0 && filters.Limit < len(out) {
		out = out[:filters.Limit]
	}
	return out
}
