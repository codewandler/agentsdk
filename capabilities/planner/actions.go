package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/thread"
)

const (
	ActionCreatePlan       = "create_plan"
	ActionAddStep          = "add_step"
	ActionRemoveStep       = "remove_step"
	ActionSetStepTitle     = "set_step_title"
	ActionSetStepStatus    = "set_step_status"
	ActionSetStepDependsOn = "set_step_depends_on"
	ActionSetStepParent    = "set_step_parent"
	ActionSetCurrentStep   = "set_current_step"
)

type PlanPatch struct {
	ID    string `json:"id,omitempty"`
	Title string `json:"title,omitempty"`
}

type StepPatch struct {
	ID        string     `json:"id,omitempty"`
	Title     string     `json:"title,omitempty"`
	Status    StepStatus `json:"status,omitempty"`
	DependsOn []string   `json:"depends_on,omitempty"`
	ParentID  string     `json:"parent_id,omitempty"`
}

type Action struct {
	Action    string     `json:"action" jsonschema:"required,enum=create_plan,enum=add_step,enum=remove_step,enum=set_step_title,enum=set_step_status,enum=set_step_depends_on,enum=set_step_parent,enum=set_current_step"`
	Plan      *PlanPatch `json:"plan,omitempty"`
	Step      *StepPatch `json:"step,omitempty"`
	StepID    string     `json:"step_id,omitempty"`
	Status    StepStatus `json:"status,omitempty" jsonschema:"enum=pending,enum=in_progress,enum=completed"`
	Title     string     `json:"title,omitempty"`
	DependsOn []string   `json:"depends_on,omitempty"`
	ParentID  string     `json:"parent_id,omitempty"`
}

type ApplyActionsResult struct {
	Message string `json:"message"`
	Plan    Plan   `json:"plan"`
}

func (p *Planner) ApplyActions(ctx context.Context, actions []Action) (ApplyActionsResult, error) {
	if len(actions) == 0 {
		return ApplyActionsResult{}, fmt.Errorf("planner: at least one action is required")
	}
	if p.runtime == nil {
		return ApplyActionsResult{}, fmt.Errorf("planner: runtime is required")
	}

	draft := clonePlan(p.plan)
	created := p.created
	events := make([]capability.StateEvent, 0, len(actions))
	for _, action := range actions {
		built, err := buildActionEvent(&draft, &created, action)
		if err != nil {
			return ApplyActionsResult{}, err
		}
		events = append(events, built...)
		for _, event := range built {
			if err := applyEventTo(&draft, &created, event); err != nil {
				return ApplyActionsResult{}, err
			}
		}
	}

	threadEvents := make([]thread.Event, 0, len(events))
	for _, event := range events {
		wrapped, err := capability.DispatchEvent(p.spec, event)
		if err != nil {
			return ApplyActionsResult{}, err
		}
		threadEvents = append(threadEvents, wrapped)
	}
	if err := p.runtime.AppendEvents(ctx, threadEvents...); err != nil {
		return ApplyActionsResult{}, err
	}

	for _, event := range events {
		if err := p.ApplyEvent(ctx, event); err != nil {
			return ApplyActionsResult{}, err
		}
	}
	return ApplyActionsResult{
		Message: summarizeActions(actions),
		Plan:    clonePlan(p.plan),
	}, nil
}

func buildActionEvent(plan *Plan, created *bool, action Action) ([]capability.StateEvent, error) {
	switch strings.TrimSpace(action.Action) {
	case ActionCreatePlan:
		if *created {
			return nil, fmt.Errorf("planner: plan already created")
		}
		if action.Plan == nil {
			return nil, fmt.Errorf("planner: create_plan requires plan")
		}
		id := action.Plan.ID
		if id == "" {
			generated, err := newPlanID()
			if err != nil {
				return nil, err
			}
			id = generated
		}
		event, err := stateEvent(EventPlanCreated, PlanCreated{PlanID: id, Title: action.Plan.Title})
		if err != nil {
			return nil, err
		}
		return []capability.StateEvent{event}, nil
	case ActionAddStep:
		if err := requireCreated(*created); err != nil {
			return nil, err
		}
		if action.Step == nil {
			return nil, fmt.Errorf("planner: add_step requires step")
		}
		step := Step{
			ID:        action.Step.ID,
			Title:     action.Step.Title,
			Status:    action.Step.Status,
			DependsOn: append([]string(nil), action.Step.DependsOn...),
			ParentID:  action.Step.ParentID,
		}
		if step.ID == "" {
			generated, err := newStepID()
			if err != nil {
				return nil, err
			}
			step.ID = generated
		}
		if step.Title == "" {
			return nil, fmt.Errorf("planner: step title is required")
		}
		if step.Status == "" {
			step.Status = StepPending
		}
		if !validStatus(step.Status) {
			return nil, fmt.Errorf("planner: invalid step status %q", step.Status)
		}
		if _, ok := findStep(*plan, step.ID); ok {
			return nil, fmt.Errorf("planner: step %q already exists", step.ID)
		}
		if step.ParentID == step.ID {
			return nil, fmt.Errorf("planner: step cannot be its own parent")
		}
		if step.ParentID != "" {
			if _, ok := findStep(*plan, step.ParentID); !ok {
				return nil, fmt.Errorf("planner: parent step %q not found", step.ParentID)
			}
		}
		for _, dep := range step.DependsOn {
			if dep == step.ID {
				return nil, fmt.Errorf("planner: step cannot depend on itself")
			}
			if _, ok := findStep(*plan, dep); !ok {
				return nil, fmt.Errorf("planner: dependency step %q not found", dep)
			}
		}
		_, err := topoSortSteps(append(cloneSteps(plan.Steps), step))
		if err != nil {
			return nil, err
		}
		event, err := stateEvent(EventStepAdded, StepAdded{Step: step})
		if err != nil {
			return nil, err
		}
		return []capability.StateEvent{event}, nil
	case ActionRemoveStep:
		if err := requireCreated(*created); err != nil {
			return nil, err
		}
		stepID := requireStepID(action)
		if _, ok := findStep(*plan, stepID); !ok {
			return nil, fmt.Errorf("planner: step %q not found", stepID)
		}
		for _, s := range plan.Steps {
			if s.ParentID == stepID {
				return nil, fmt.Errorf("planner: step %q has sub-tasks", stepID)
			}
		}
		event, err := stateEvent(EventStepRemoved, StepRemoved{StepID: stepID})
		if err != nil {
			return nil, err
		}
		return []capability.StateEvent{event}, nil
	case ActionSetStepTitle:
		if err := requireCreated(*created); err != nil {
			return nil, err
		}
		stepID := requireStepID(action)
		if _, ok := findStep(*plan, stepID); !ok {
			return nil, fmt.Errorf("planner: step %q not found", stepID)
		}
		if action.Title == "" {
			return nil, fmt.Errorf("planner: title is required")
		}
		event, err := stateEvent(EventStepTitleChanged, StepTitleChanged{StepID: stepID, Title: action.Title})
		if err != nil {
			return nil, err
		}
		return []capability.StateEvent{event}, nil
	case ActionSetStepStatus:
		if err := requireCreated(*created); err != nil {
			return nil, err
		}
		stepID := requireStepID(action)
		if _, ok := findStep(*plan, stepID); !ok {
			return nil, fmt.Errorf("planner: step %q not found", stepID)
		}
		if !validStatus(action.Status) {
			return nil, fmt.Errorf("planner: invalid step status %q", action.Status)
		}
		event, err := stateEvent(EventStepStatusChanged, StepStatusChanged{StepID: stepID, Status: action.Status})
		if err != nil {
			return nil, err
		}
		return []capability.StateEvent{event}, nil
	case ActionSetStepDependsOn:
		if err := requireCreated(*created); err != nil {
			return nil, err
		}
		stepID := requireStepID(action)
		if _, ok := findStep(*plan, stepID); !ok {
			return nil, fmt.Errorf("planner: step %q not found", stepID)
		}
		for _, dep := range action.DependsOn {
			if dep == stepID {
				return nil, fmt.Errorf("planner: step cannot depend on itself")
			}
			if _, ok := findStep(*plan, dep); !ok {
				return nil, fmt.Errorf("planner: dependency step %q not found", dep)
			}
		}
		idx, _ := findStep(*plan, stepID)
		temp := cloneSteps(plan.Steps)
		temp[idx].DependsOn = append([]string(nil), action.DependsOn...)
		_, err := topoSortSteps(temp)
		if err != nil {
			return nil, err
		}
		event, err := stateEvent(EventStepDependsOnChanged, StepDependsOnChanged{StepID: stepID, DependsOn: append([]string(nil), action.DependsOn...)})
		if err != nil {
			return nil, err
		}
		return []capability.StateEvent{event}, nil
	case ActionSetStepParent:
		if err := requireCreated(*created); err != nil {
			return nil, err
		}
		stepID := requireStepID(action)
		if _, ok := findStep(*plan, stepID); !ok {
			return nil, fmt.Errorf("planner: step %q not found", stepID)
		}
		if action.ParentID == stepID {
			return nil, fmt.Errorf("planner: step cannot be its own parent")
		}
		if action.ParentID != "" {
			if _, ok := findStep(*plan, action.ParentID); !ok {
				return nil, fmt.Errorf("planner: parent step %q not found", action.ParentID)
			}
		}
		idx, _ := findStep(*plan, stepID)
		temp := cloneSteps(plan.Steps)
		temp[idx].ParentID = action.ParentID
		_, err := topoSortSteps(temp)
		if err != nil {
			return nil, err
		}
		event, err := stateEvent(EventStepParentChanged, StepParentChanged{StepID: stepID, ParentID: action.ParentID})
		if err != nil {
			return nil, err
		}
		return []capability.StateEvent{event}, nil
	case ActionSetCurrentStep:
		if err := requireCreated(*created); err != nil {
			return nil, err
		}
		stepID := action.StepID
		if stepID != "" {
			if _, ok := findStep(*plan, stepID); !ok {
				return nil, fmt.Errorf("planner: step %q not found", stepID)
			}
		}
		event, err := stateEvent(EventCurrentStepChanged, CurrentStepChanged{StepID: stepID})
		if err != nil {
			return nil, err
		}
		return []capability.StateEvent{event}, nil
	default:
		return nil, fmt.Errorf("planner: unsupported action %q", action.Action)
	}
}

func requireStepID(action Action) string {
	if action.StepID != "" {
		return action.StepID
	}
	if action.Step != nil {
		return action.Step.ID
	}
	return ""
}

func summarizeActions(actions []Action) string {
	if len(actions) == 1 {
		return "Applied 1 planner action."
	}
	return fmt.Sprintf("Applied %d planner actions.", len(actions))
}

func marshalResult(result ApplyActionsResult) ([]byte, error) {
	return json.Marshal(result)
}

func cloneSteps(steps []Step) []Step {
	cloned := append([]Step(nil), steps...)
	for i := range cloned {
		cloned[i].DependsOn = append([]string(nil), cloned[i].DependsOn...)
	}
	return cloned
}
