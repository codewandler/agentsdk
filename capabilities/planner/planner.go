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
	ID        string     `json:"id,omitempty"`
	ParentID  string     `json:"parent_id,omitempty"`
	DependsOn []string   `json:"depends_on,omitempty"`
	Title     string     `json:"title,omitempty"`
	Status    StepStatus `json:"status,omitempty"`
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
	for i := range plan.Steps {
		plan.Steps[i].DependsOn = append([]string(nil), plan.Steps[i].DependsOn...)
	}
	return plan
}

func topoSortSteps(steps []Step) ([]Step, error) {
	if len(steps) == 0 {
		return nil, nil
	}
	byID := make(map[string]Step, len(steps))
	for _, s := range steps {
		byID[s.ID] = s
	}

	inDegree := make(map[string]int, len(steps))
	for id := range byID {
		inDegree[id] = 0
	}
	adj := make(map[string][]string, len(steps))
	for _, s := range steps {
		for _, dep := range s.DependsOn {
			if _, ok := byID[dep]; !ok {
				return nil, fmt.Errorf("planner: dependency %q not found", dep)
			}
			adj[dep] = append(adj[dep], s.ID)
			inDegree[s.ID]++
		}
		if s.ParentID != "" {
			if _, ok := byID[s.ParentID]; !ok {
				return nil, fmt.Errorf("planner: parent %q not found", s.ParentID)
			}
		}
	}

	ready := make([]string, 0, len(steps))
	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}
	sort.Strings(ready)

	ordered := make([]Step, 0, len(steps))
	for len(ready) > 0 {
		sort.Strings(ready)
		id := ready[0]
		ready = ready[1:]
		ordered = append(ordered, byID[id])
		for _, next := range adj[id] {
			inDegree[next]--
			if inDegree[next] == 0 {
				ready = append(ready, next)
			}
		}
	}

	if len(ordered) != len(steps) {
		return nil, fmt.Errorf("planner: cycle detected in step dependencies")
	}
	return ordered, nil
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
