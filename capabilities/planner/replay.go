package planner

import (
	"context"
	"fmt"

	"github.com/codewandler/agentsdk/capability"
)

func (p *Planner) ApplyEvent(ctx context.Context, event capability.StateEvent) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	plan := clonePlan(p.plan)
	created := p.created
	if err := applyEventTo(&plan, &created, event); err != nil {
		return err
	}
	p.plan = plan
	p.created = created
	return nil
}

func applyEventTo(plan *Plan, created *bool, event capability.StateEvent) error {
	switch event.Name {
	case EventPlanCreated:
		payload, err := decodeStateEvent[PlanCreated](event)
		if err != nil {
			return err
		}
		if payload.PlanID == "" {
			return fmt.Errorf("planner: plan_created missing plan_id")
		}
		*plan = Plan{ID: payload.PlanID, Title: payload.Title}
		*created = true
	case EventStepAdded:
		if err := requireCreated(*created); err != nil {
			return err
		}
		payload, err := decodeStateEvent[StepAdded](event)
		if err != nil {
			return err
		}
		if payload.Step.ID == "" {
			return fmt.Errorf("planner: step_added missing step id")
		}
		if payload.Step.Status == "" {
			payload.Step.Status = StepPending
		}
		if !validStatus(payload.Step.Status) {
			return fmt.Errorf("planner: invalid step status %q", payload.Step.Status)
		}
		if _, ok := findStep(*plan, payload.Step.ID); ok {
			return fmt.Errorf("planner: step %q already exists", payload.Step.ID)
		}
		plan.Steps = append(plan.Steps, payload.Step)
		sorted, err := topoSortSteps(plan.Steps)
		if err != nil {
			return err
		}
		plan.Steps = sorted
	case EventStepRemoved:
		if err := requireCreated(*created); err != nil {
			return err
		}
		payload, err := decodeStateEvent[StepRemoved](event)
		if err != nil {
			return err
		}
		index, ok := findStep(*plan, payload.StepID)
		if !ok {
			return fmt.Errorf("planner: step %q not found", payload.StepID)
		}
		plan.Steps = append(plan.Steps[:index], plan.Steps[index+1:]...)
		if plan.CurrentStepID == payload.StepID {
			plan.CurrentStepID = ""
		}
		for i := range plan.Steps {
			plan.Steps[i].DependsOn = filterString(plan.Steps[i].DependsOn, payload.StepID)
		}
		plan.Steps, _ = topoSortSteps(plan.Steps)
	case EventStepTitleChanged:
		if err := requireCreated(*created); err != nil {
			return err
		}
		payload, err := decodeStateEvent[StepTitleChanged](event)
		if err != nil {
			return err
		}
		index, ok := findStep(*plan, payload.StepID)
		if !ok {
			return fmt.Errorf("planner: step %q not found", payload.StepID)
		}
		plan.Steps[index].Title = payload.Title
	case EventStepStatusChanged:
		if err := requireCreated(*created); err != nil {
			return err
		}
		payload, err := decodeStateEvent[StepStatusChanged](event)
		if err != nil {
			return err
		}
		if !validStatus(payload.Status) {
			return fmt.Errorf("planner: invalid step status %q", payload.Status)
		}
		index, ok := findStep(*plan, payload.StepID)
		if !ok {
			return fmt.Errorf("planner: step %q not found", payload.StepID)
		}
		plan.Steps[index].Status = payload.Status
	case EventStepDependsOnChanged:
		if err := requireCreated(*created); err != nil {
			return err
		}
		payload, err := decodeStateEvent[StepDependsOnChanged](event)
		if err != nil {
			return err
		}
		index, ok := findStep(*plan, payload.StepID)
		if !ok {
			return fmt.Errorf("planner: step %q not found", payload.StepID)
		}
		plan.Steps[index].DependsOn = append([]string(nil), payload.DependsOn...)
		sorted, err := topoSortSteps(plan.Steps)
		if err != nil {
			return err
		}
		plan.Steps = sorted
	case EventStepParentChanged:
		if err := requireCreated(*created); err != nil {
			return err
		}
		payload, err := decodeStateEvent[StepParentChanged](event)
		if err != nil {
			return err
		}
		index, ok := findStep(*plan, payload.StepID)
		if !ok {
			return fmt.Errorf("planner: step %q not found", payload.StepID)
		}
		plan.Steps[index].ParentID = payload.ParentID
		sorted, err := topoSortSteps(plan.Steps)
		if err != nil {
			return err
		}
		plan.Steps = sorted
	case EventCurrentStepChanged:
		if err := requireCreated(*created); err != nil {
			return err
		}
		payload, err := decodeStateEvent[CurrentStepChanged](event)
		if err != nil {
			return err
		}
		if payload.StepID != "" {
			if _, ok := findStep(*plan, payload.StepID); !ok {
				return fmt.Errorf("planner: step %q not found", payload.StepID)
			}
		}
		plan.CurrentStepID = payload.StepID
	default:
		return fmt.Errorf("planner: unsupported event %q", event.Name)
	}
	return nil
}

func findStep(plan Plan, id string) (int, bool) {
	for i, step := range plan.Steps {
		if step.ID == id {
			return i, true
		}
	}
	return -1, false
}

func filterString(ss []string, exclude string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != exclude {
			out = append(out, s)
		}
	}
	return out
}
