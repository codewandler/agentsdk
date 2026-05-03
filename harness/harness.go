// Package harness provides the first named host/session seam over the current
// app and agent runtime stack. It intentionally delegates to app.App for now;
// later slices can move lifecycle-heavy responsibilities behind this boundary.
package harness

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/thread"
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
	session := &Session{App: s.App, Agent: inst}
	if err := session.AttachAgentProjection(session.AgentCommandProjection()); err != nil {
		return nil, err
	}
	return session, nil
}

func (s *Session) Send(ctx context.Context, input string) (command.Result, error) {
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "/") {
		name, params, err := command.Parse(trimmed)
		if err != nil {
			return command.Result{}, err
		}
		commands, err := s.Commands()
		if err != nil {
			return command.Result{}, err
		}
		if cmd, ok := commands.Get(name); ok {
			return cmd.Execute(ctx, params)
		}
	}
	return s.App.Send(ctx, input)
}

func (s *Session) Commands() (*command.Registry, error) {
	builders := []func(*Session) (*command.Tree, error){
		NewHelpCommand,
		NewAgentsCommand,
		NewNewCommand,
		NewQuitCommand,
		NewTurnCommand,
		NewContextCommand,
		NewSkillsCommand,
		NewSkillCommand,
		NewCompactCommand,
		NewWorkflowCommand,
		NewSessionCommand,
	}
	registry := command.NewRegistry()
	for _, build := range builders {
		tree, err := build(s)
		if err != nil {
			return nil, err
		}
		if err := registry.Register(tree); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func (s *Session) CommandDescriptors() []command.Descriptor {
	commands, err := s.Commands()
	if err != nil {
		return nil
	}
	return commands.Descriptors()
}

func (s *Session) ExecuteCommand(ctx context.Context, path []string, input map[string]any) (command.Result, error) {
	if s == nil || s.App == nil {
		return command.Result{}, fmt.Errorf("harness: app is required")
	}
	commands, err := s.Commands()
	if err != nil {
		return command.Result{}, err
	}
	return commands.ExecuteMap(ctx, path, input)
}

func (s *Session) ExecuteWorkflow(ctx context.Context, workflowName string, input any, opts ...app.WorkflowExecutionOption) action.Result {
	if s == nil || s.App == nil {
		return action.Result{Error: fmt.Errorf("harness: app is required")}
	}
	execOpts, recorder := s.workflowExecutionOptions(opts)
	result := s.App.ExecuteWorkflow(ctx, workflowName, input, execOpts...)
	if recorder != nil {
		result.Error = errors.Join(result.Error, recorder.Err())
	}
	return result
}

func (s *Session) workflowExecutionOptions(opts []app.WorkflowExecutionOption) ([]app.WorkflowExecutionOption, *workflow.ThreadRecorder) {
	if s == nil || s.Agent == nil || s.Agent.LiveThread() == nil {
		return opts, nil
	}
	recorder := &workflow.ThreadRecorder{Live: s.Agent.LiveThread()}
	out := append([]app.WorkflowExecutionOption(nil), opts...)
	out = append(out, app.WithWorkflowEventHandler(recorder.OnEvent))
	return out, recorder
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

func (s *Session) Info() SessionInfo {
	info := SessionInfo{}
	if s == nil {
		return info
	}
	if s.App != nil {
		info.SessionID = s.App.SessionID()
		info.ParamsSummary = s.App.ParamsSummary()
	}
	if s.Agent != nil {
		spec := s.Agent.Spec()
		info.AgentName = spec.Name
		if info.SessionID == "" {
			info.SessionID = s.Agent.SessionID()
		}
		if live := s.Agent.LiveThread(); live != nil {
			info.ThreadID = live.ID()
			info.BranchID = live.BranchID()
			info.ThreadBacked = true
		}
	}
	return info
}

func (s *Session) AgentName() string {
	return s.Info().AgentName
}

func (s *Session) ThreadID() (thread.ID, bool) {
	info := s.Info()
	if info.ThreadID == "" {
		return "", false
	}
	return info.ThreadID, true
}

func (s *Session) ParamsSummary() string {
	if s == nil || s.App == nil {
		return ""
	}
	return s.App.ParamsSummary()
}

func (s *Session) SessionID() string {
	return s.Info().SessionID
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
