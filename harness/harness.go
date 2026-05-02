// Package harness provides the first named host/session seam over the current
// app and agent runtime stack. It intentionally delegates to app.App for now;
// later slices can move lifecycle-heavy responsibilities behind this boundary.
package harness

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/agentsdk/workflow"
)

type Service struct {
	App *app.App
}

type Session struct {
	App   *app.App
	Agent *agent.Instance
}

func NewService(app *app.App) *Service {
	return &Service{App: app}
}

func (s *Service) DefaultSession() (*Session, error) {
	if s == nil || s.App == nil {
		return nil, fmt.Errorf("harness: app is required")
	}
	inst, ok := s.App.DefaultAgent()
	if !ok || inst == nil {
		return nil, fmt.Errorf("harness: no default agent configured")
	}
	return &Session{App: s.App, Agent: inst}, nil
}

func (s *Session) Send(ctx context.Context, input string) (command.Result, error) {
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	if isWorkflowCommand(input) {
		return s.handleWorkflowCommand(ctx, input)
	}
	return s.App.Send(ctx, input)
}

func (s *Session) ExecuteWorkflow(ctx context.Context, workflowName string, input any, opts ...app.WorkflowExecutionOption) action.Result {
	if s == nil || s.App == nil {
		return action.Result{Error: fmt.Errorf("harness: app is required")}
	}
	return s.App.ExecuteWorkflow(ctx, workflowName, input, opts...)
}

func (s *Session) WorkflowRunStore() (*workflow.ThreadRunStore, bool) {
	if s == nil || s.Agent == nil {
		return nil, false
	}
	live := s.Agent.LiveThread()
	if live == nil {
		return nil, false
	}
	path := s.Agent.SessionStorePath()
	if strings.TrimSpace(path) == "" {
		return nil, false
	}
	store := threadjsonlstore.Open(filepath.Dir(path))
	return &workflow.ThreadRunStore{Store: store, Live: live, ThreadID: live.ID(), BranchID: live.BranchID()}, true
}

func (s *Session) WorkflowRunState(ctx context.Context, runID workflow.RunID) (workflow.RunState, bool, error) {
	store, ok := s.WorkflowRunStore()
	if !ok {
		return workflow.RunState{}, false, nil
	}
	return store.State(ctx, runID)
}

func (s *Session) WorkflowRuns(ctx context.Context) ([]workflow.RunSummary, bool, error) {
	store, ok := s.WorkflowRunStore()
	if !ok {
		return nil, false, nil
	}
	summaries, err := store.Runs(ctx)
	if err != nil {
		return nil, true, err
	}
	return summaries, true, nil
}

func isWorkflowCommand(input string) bool {
	input = strings.TrimSpace(input)
	return input == "/workflow" || strings.HasPrefix(input, "/workflow ")
}

func (s *Session) handleWorkflowCommand(ctx context.Context, input string) (command.Result, error) {
	_, params, err := command.Parse(input)
	if err != nil {
		return command.Result{}, err
	}
	if len(params.Args) == 0 {
		return command.Text(workflowCommandUsage()), nil
	}
	switch params.Args[0] {
	case "list":
		if len(params.Args) != 1 {
			return command.Text("usage: /workflow list"), nil
		}
		return s.workflowList(), nil
	case "show":
		if len(params.Args) != 2 {
			return command.Text("usage: /workflow show <name>"), nil
		}
		return s.workflowShow(params.Args[1]), nil
	case "start":
		if len(params.Args) < 2 {
			return command.Text("usage: /workflow start <name> [input]"), nil
		}
		return s.workflowStart(ctx, params.Args[1], strings.Join(params.Args[2:], " "))
	case "runs":
		if len(params.Args) != 1 {
			return command.Text("usage: /workflow runs"), nil
		}
		return s.workflowRuns(ctx)
	case "run":
		if len(params.Args) != 2 {
			return command.Text("usage: /workflow run <run-id>"), nil
		}
		return s.workflowRun(ctx, workflow.RunID(params.Args[1]))
	default:
		return command.Text(workflowCommandUsage()), nil
	}
}

func workflowCommandUsage() string {
	return "usage: /workflow <list|show|start|runs|run>\n  /workflow list\n  /workflow show <name>\n  /workflow start <name> [input]\n  /workflow runs\n  /workflow run <run-id>"
}

func (s *Session) workflowList() command.Result {
	if s == nil || s.App == nil {
		return command.Display(WorkflowListPayload{})
	}
	return command.Display(WorkflowListPayload{Definitions: s.App.Workflows()})
}

func (s *Session) workflowShow(name string) command.Result {
	if s == nil || s.App == nil {
		return command.Text(fmt.Sprintf("workflow %q not found", name))
	}
	def, ok := s.App.Workflow(name)
	if !ok {
		return command.Text(fmt.Sprintf("workflow %q not found", name))
	}
	return command.Display(WorkflowDefinitionPayload{Definition: def})
}

func (s *Session) workflowStart(ctx context.Context, workflowName string, input string) (command.Result, error) {
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

func (s *Session) workflowRun(ctx context.Context, runID workflow.RunID) (command.Result, error) {
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

func (s *Session) workflowRuns(ctx context.Context) (command.Result, error) {
	summaries, ok, err := s.WorkflowRuns(ctx)
	if err != nil {
		return command.Result{}, err
	}
	if !ok {
		return command.Text("workflow runs require a thread-backed session"), nil
	}
	return command.Display(WorkflowRunsPayload{Summaries: summaries}), nil
}

func (s *Session) ParamsSummary() string {
	if s == nil || s.App == nil {
		return ""
	}
	return s.App.ParamsSummary()
}

func (s *Session) SessionID() string {
	if s == nil || s.App == nil {
		return ""
	}
	return s.App.SessionID()
}

func (s *Session) Tracker() *usage.Tracker {
	if s == nil || s.App == nil {
		return nil
	}
	return s.App.Tracker()
}

func (s *Session) Out() io.Writer {
	if s == nil || s.App == nil {
		return io.Discard
	}
	return s.App.Out()
}
