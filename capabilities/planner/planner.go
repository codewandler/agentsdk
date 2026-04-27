package planner

import (
	"context"
	"fmt"
	"sort"

	"github.com/codewandler/agentsdk/capability"
)

const CapabilityName = "planner"

type StepStatus string

const (
	StepPending    StepStatus = "pending"
	StepInProgress StepStatus = "in_progress"
	StepCompleted  StepStatus = "completed"
)

type Plan struct {
	ID            string `json:"id,omitempty"`
	Title         string `json:"title,omitempty"`
	CurrentStepID string `json:"current_step_id,omitempty"`
	Steps         []Step `json:"steps,omitempty"`
}

type Step struct {
	ID     string     `json:"id,omitempty"`
	Order  int        `json:"order"`
	Title  string     `json:"title,omitempty"`
	Status StepStatus `json:"status,omitempty"`
}

type Planner struct {
	spec    capability.AttachSpec
	runtime capability.Runtime
	plan    Plan
	created bool
}

func New(spec capability.AttachSpec, runtime capability.Runtime) *Planner {
	if spec.CapabilityName == "" {
		spec.CapabilityName = CapabilityName
	}
	return &Planner{spec: spec, runtime: runtime}
}

func (p *Planner) Name() string { return CapabilityName }

func (p *Planner) InstanceID() string { return p.spec.InstanceID }

func (p *Planner) State(context.Context) (Plan, error) {
	return clonePlan(p.plan), nil
}

func clonePlan(plan Plan) Plan {
	plan.Steps = append([]Step(nil), plan.Steps...)
	return plan
}

func normalizeSteps(plan *Plan) {
	sort.SliceStable(plan.Steps, func(i, j int) bool {
		if plan.Steps[i].Order == plan.Steps[j].Order {
			return plan.Steps[i].ID < plan.Steps[j].ID
		}
		return plan.Steps[i].Order < plan.Steps[j].Order
	})
}

func validStatus(status StepStatus) bool {
	switch status {
	case StepPending, StepInProgress, StepCompleted:
		return true
	default:
		return false
	}
}

func requireCreated(created bool) error {
	if !created {
		return fmt.Errorf("planner: plan has not been created")
	}
	return nil
}
