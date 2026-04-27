package planner

import (
	"encoding/json"
	"fmt"

	"github.com/codewandler/agentsdk/capability"
)

const (
	EventPlanCreated          = "plan_created"
	EventStepAdded            = "step_added"
	EventStepRemoved          = "step_removed"
	EventStepTitleChanged     = "step_title_changed"
	EventStepStatusChanged    = "step_status_changed"
	EventStepDependsOnChanged = "step_depends_on_changed"
	EventStepParentChanged    = "step_parent_changed"
	EventCurrentStepChanged   = "current_step_changed"
)

type PlanCreated struct {
	PlanID string `json:"plan_id"`
	Title  string `json:"title,omitempty"`
}

type StepAdded struct {
	Step Step `json:"step"`
}

type StepRemoved struct {
	StepID string `json:"step_id"`
}

type StepTitleChanged struct {
	StepID string `json:"step_id"`
	Title  string `json:"title"`
}

type StepStatusChanged struct {
	StepID string     `json:"step_id"`
	Status StepStatus `json:"status"`
}

type StepDependsOnChanged struct {
	StepID    string   `json:"step_id"`
	DependsOn []string `json:"depends_on"`
}

type StepParentChanged struct {
	StepID   string `json:"step_id"`
	ParentID string `json:"parent_id"`
}

type CurrentStepChanged struct {
	StepID string `json:"step_id,omitempty"`
}

func stateEvent(name string, body any) (capability.StateEvent, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return capability.StateEvent{}, err
	}
	return capability.StateEvent{Name: name, Body: raw}, nil
}

func decodeStateEvent[T any](event capability.StateEvent) (T, error) {
	var out T
	if err := json.Unmarshal(event.Body, &out); err != nil {
		return out, fmt.Errorf("planner: decode %s: %w", event.Name, err)
	}
	return out, nil
}
