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
	threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/agentsdk/workflow"
)

type Service struct {
	App *app.App
}

type Session struct {
	App    *app.App
	Agent  *agent.Instance
	turnID int
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
	if trimmed == "" {
		return command.Handled(), nil
	}
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
			result, err := cmd.Execute(ctx, params)
			if err != nil {
				return command.Result{}, err
			}
			return s.applyResult(ctx, result, 0)
		}
		if s.App.Commands() != nil {
			result, err := s.App.Commands().ExecuteUser(ctx, trimmed)
			if err != nil {
				return command.Result{}, err
			}
			return s.applyResult(ctx, result, 0)
		}
	}
	return s.runAgentTurn(ctx, trimmed, 0)
}

func (s *Session) applyResult(ctx context.Context, result command.Result, turnID int) (command.Result, error) {
	switch result.Kind {
	case command.ResultAgentTurn:
		input, ok := command.AgentTurnInput(result)
		if !ok || strings.TrimSpace(input) == "" {
			return command.Handled(), nil
		}
		return s.runAgentTurn(ctx, input, turnID)
	case command.ResultReset:
		if s != nil && s.Agent != nil {
			s.Agent.Reset()
		}
		if s != nil {
			s.turnID = 0
		}
		return command.Handled(), nil
	default:
		return result, nil
	}
}

func (s *Session) runAgentTurn(ctx context.Context, input string, turnID int) (command.Result, error) {
	if s == nil || s.Agent == nil {
		return command.Result{}, fmt.Errorf("harness: no agent configured")
	}
	if turnID <= 0 {
		s.turnID++
		turnID = s.turnID
	}
	return command.Handled(), s.Agent.RunTurn(ctx, turnID, input)
}

func (s *Session) Commands() (*command.Registry, error) {
	builders := []func(*Session) (*command.Tree, error){
		newHelpCommand,
		newAgentsCommand,
		newNewCommand,
		newQuitCommand,
		newTurnCommand,
		newContextCommand,
		newSkillsCommand,
		newSkillCommand,
		newCompactCommand,
		newWorkflowCommand,
		newSessionCommand,
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
	if s.Agent != nil {
		spec := s.Agent.Spec()
		info.AgentName = spec.Name
		info.SessionID = s.Agent.SessionID()
		info.ParamsSummary = s.Agent.ParamsSummary()
		if live := s.Agent.LiveThread(); live != nil {
			info.ThreadID = live.ID()
			info.BranchID = live.BranchID()
			info.ThreadBacked = true
		}
	}
	return info
}

func (s *Session) ParamsSummary() string {
	if s == nil || s.Agent == nil {
		return ""
	}
	return s.Agent.ParamsSummary()
}

func (s *Session) SessionID() string {
	return s.Info().SessionID
}

func (s *Session) Tracker() *usage.Tracker {
	if s == nil || s.Agent == nil {
		return nil
	}
	return s.Agent.Tracker()
}

func (s *Session) Out() io.Writer {
	if s == nil || s.Agent == nil {
		return io.Discard
	}
	return s.Agent.Out()
}
