package harness

import (
	"context"
	"fmt"

	"github.com/codewandler/agentsdk/trigger"
	"github.com/codewandler/agentsdk/workflow"
)

// TriggerExecutor maps normalized trigger executions to harness sessions and
// workflows/agent turns. It keeps background execution on the harness runtime
// instead of introducing a second daemon-specific runtime.
type TriggerExecutor struct {
	Service  *Service
	StoreDir string
}

func (e TriggerExecutor) ExecuteTrigger(ctx context.Context, execution trigger.Execution) (trigger.ExecutionResult, error) {
	rule := execution.Rule.Normalized()
	session, err := e.sessionFor(ctx, rule)
	if err != nil {
		return trigger.ExecutionResult{}, err
	}
	result := trigger.ExecutionResult{TargetKind: rule.Target.Kind, TargetName: rule.Target.Name(), SessionName: session.Name, SessionID: session.SessionID()}
	input := triggerInput(rule.Target, execution.Event)
	switch rule.Target.Kind {
	case trigger.TargetWorkflow:
		if rule.Target.WorkflowName == "" {
			return result, fmt.Errorf("trigger: workflow target requires workflow name")
		}
		runID := workflow.NewRunID()
		wfResult := session.ExecuteWorkflow(ctx, rule.Target.WorkflowName, input,
			workflow.WithRunID(runID),
			workflow.WithRunMetadata(session.WorkflowRunMetadata("trigger", []string{"trigger", string(rule.ID)})),
		)
		result.WorkflowRunID = string(runID)
		if wfResult.Error != nil {
			return result, wfResult.Error
		}
		if data, ok := wfResult.Data.(workflow.Result); ok {
			result.Output = data.Data
		} else {
			result.Output = wfResult.Data
		}
		return result, nil
	case trigger.TargetAgentPrompt:
		prompt := rule.Target.Prompt
		if prompt == "" {
			prompt = fmt.Sprint(input)
		}
		_, err := session.runAgentTurn(ctx, prompt, 0)
		return result, err
	case trigger.TargetAction:
		return result, fmt.Errorf("trigger: direct action targets require explicit policy and are not enabled yet")
	default:
		return result, fmt.Errorf("trigger: unsupported target kind %q", rule.Target.Kind)
	}
}

func (e TriggerExecutor) sessionFor(ctx context.Context, rule trigger.Rule) (*Session, error) {
	if e.Service == nil {
		return nil, fmt.Errorf("trigger: harness service is required")
	}
	name := rule.Session.Name
	if name == "" && rule.Session.Mode == trigger.SessionTriggerOwned {
		name = "trigger-" + string(rule.ID)
	}
	agentName := rule.Session.AgentName
	if agentName == "" {
		agentName = rule.Target.AgentName
	}
	switch rule.Session.Mode {
	case trigger.SessionShared, trigger.SessionTriggerOwned:
		if name != "" {
			if session, ok := e.Service.Session(name); ok {
				return session, nil
			}
		}
		return e.Service.OpenSession(ctx, SessionOpenRequest{Name: name, AgentName: agentName, StoreDir: e.StoreDir})
	case trigger.SessionResumeOrCreate:
		if name != "" {
			if session, ok := e.Service.Session(name); ok {
				return session, nil
			}
		}
		return e.Service.OpenSession(ctx, SessionOpenRequest{Name: name, AgentName: agentName, StoreDir: e.StoreDir, Resume: name})
	case trigger.SessionEphemeral:
		return e.Service.OpenSession(ctx, SessionOpenRequest{Name: name, AgentName: agentName})
	default:
		return e.Service.OpenSession(ctx, SessionOpenRequest{Name: name, AgentName: agentName, StoreDir: e.StoreDir})
	}
}

func triggerInput(target trigger.Target, event trigger.Event) any {
	if target.IncludeEvent {
		return map[string]any{"input": target.Input, "event": event}
	}
	if target.Input != nil {
		return target.Input
	}
	return event
}
