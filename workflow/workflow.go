// Package workflow provides minimal orchestration over action references.
package workflow

import (
	"fmt"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/thread"
)

// ActionRef identifies an action used by a workflow. Resolution is owned by the
// executor so workflows can stay declarative and serializable.
type ActionRef = action.Ref

// Step is one workflow node.
type Step struct {
	ID        string
	Action    ActionRef
	Input     any
	DependsOn []string
}

// Definition describes a workflow graph. The initial implementation executes a
// topologically ordered DAG and passes dependency outputs as step inputs.
type Definition struct {
	Name        string
	Description string
	Steps       []Step
}

// Result is the structured workflow execution result.
type Result struct {
	StepResults map[string]action.Result
	Data        any
}

const (
	EventStarted       thread.EventKind = "workflow.started"
	EventStepStarted   thread.EventKind = "workflow.step_started"
	EventStepCompleted thread.EventKind = "workflow.step_completed"
	EventStepFailed    thread.EventKind = "workflow.step_failed"
	EventCompleted     thread.EventKind = "workflow.completed"
	EventFailed        thread.EventKind = "workflow.failed"
)

// EventDefinitions returns persistent thread-event definitions for workflow
// execution events. Live workflow events use the same concrete payload structs;
// persistence adapters choose the matching Event* kind when appending to a
// thread log.
func EventDefinitions() []thread.EventDefinition {
	return []thread.EventDefinition{
		thread.DefineEvent[Started](EventStarted),
		thread.DefineEvent[StepStarted](EventStepStarted),
		thread.DefineEvent[StepCompleted](EventStepCompleted),
		thread.DefineEvent[StepFailed](EventStepFailed),
		thread.DefineEvent[Completed](EventCompleted),
		thread.DefineEvent[Failed](EventFailed),
	}
}

type Started struct {
	WorkflowName string `json:"workflow_name"`
}

type StepStarted struct {
	WorkflowName string `json:"workflow_name"`
	StepID       string `json:"step_id"`
	ActionName   string `json:"action_name"`
}

type StepCompleted struct {
	WorkflowName string `json:"workflow_name"`
	StepID       string `json:"step_id"`
	ActionName   string `json:"action_name"`
	Data         any    `json:"data,omitempty"`
}

type StepFailed struct {
	WorkflowName string `json:"workflow_name"`
	StepID       string `json:"step_id"`
	ActionName   string `json:"action_name"`
	Error        string `json:"error"`
}

type Completed struct {
	WorkflowName string `json:"workflow_name"`
	Data         any    `json:"data,omitempty"`
}

type Failed struct {
	WorkflowName string `json:"workflow_name"`
	Error        string `json:"error"`
}

// EventHandler receives concrete workflow event payloads as they occur.
type EventHandler func(action.Ctx, action.Event)

// Resolver resolves action references at execution time.
type Resolver interface {
	ResolveAction(action.Ctx, ActionRef) (action.Action, bool)
}

// ResolverFunc adapts a function into Resolver.
type ResolverFunc func(action.Ctx, ActionRef) (action.Action, bool)

func (f ResolverFunc) ResolveAction(ctx action.Ctx, ref ActionRef) (action.Action, bool) {
	return f(ctx, ref)
}

// RegistryResolver resolves workflow action refs from an action.Registry.
type RegistryResolver struct {
	Registry *action.Registry
}

func (r RegistryResolver) ResolveAction(_ action.Ctx, ref ActionRef) (action.Action, bool) {
	if r.Registry == nil {
		return nil, false
	}
	return r.Registry.Get(ref.Name)
}

// Executor executes workflows over resolved actions.
type Executor struct {
	Resolver Resolver
	OnEvent  EventHandler
}

// Execute runs def and returns a workflow result in action.Result.Data. Execution
// stops at the first failed or unresolved step; partial step results are kept in
// Result.StepResults.
func (e Executor) Execute(ctx action.Ctx, def Definition, input any) action.Result {
	var events []action.Event
	emit := func(event action.Event) {
		events = append(events, event)
		if e.OnEvent != nil {
			e.OnEvent(ctx, event)
		}
	}
	fail := func(err error, data any) action.Result {
		emit(Failed{WorkflowName: def.Name, Error: err.Error()})
		return action.Result{Data: data, Error: err, Events: events}
	}

	if e.Resolver == nil {
		return fail(fmt.Errorf("workflow %q has no action resolver", def.Name), nil)
	}
	ordered, err := validateAndOrder(def.Steps)
	if err != nil {
		return fail(err, nil)
	}
	emit(Started{WorkflowName: def.Name})
	results := make(map[string]action.Result, len(ordered))
	var last any = input
	for _, step := range ordered {
		a, ok := e.Resolver.ResolveAction(ctx, step.Action)
		if !ok || a == nil {
			return fail(fmt.Errorf("workflow %q step %q action %q not found", def.Name, step.ID, step.Action.Name), Result{StepResults: results, Data: last})
		}
		stepInput := stepInput(step, input, results)
		emit(StepStarted{WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name})
		res := a.Execute(ctx, stepInput)
		results[step.ID] = res
		events = append(events, res.Events...)
		if res.Error != nil {
			err := fmt.Errorf("workflow %q step %q failed: %w", def.Name, step.ID, res.Error)
			emit(StepFailed{WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, Error: res.Error.Error()})
			return fail(err, Result{StepResults: results, Data: last})
		}
		last = res.Data
		emit(StepCompleted{WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, Data: res.Data})
	}
	emit(Completed{WorkflowName: def.Name, Data: last})
	return action.Result{Data: Result{StepResults: results, Data: last}, Events: events}
}

func stepInput(step Step, initial any, results map[string]action.Result) any {
	if step.Input != nil {
		return step.Input
	}
	switch len(step.DependsOn) {
	case 0:
		return initial
	case 1:
		return results[step.DependsOn[0]].Data
	default:
		in := make(map[string]any, len(step.DependsOn))
		for _, dep := range step.DependsOn {
			in[dep] = results[dep].Data
		}
		return in
	}
}

// Validate checks workflow definition invariants that are independent of action
// resolution.
func Validate(def Definition) error {
	_, err := validateAndOrder(def.Steps)
	if err != nil {
		return err
	}
	return nil
}

// ValidateActions checks that each workflow action reference resolves in reg.
func ValidateActions(def Definition, reg *action.Registry) error {
	if err := Validate(def); err != nil {
		return err
	}
	for _, step := range def.Steps {
		if _, ok := reg.Get(step.Action.Name); !ok {
			return fmt.Errorf("workflow %q step %q action %q not found", def.Name, step.ID, step.Action.Name)
		}
	}
	return nil
}

func validateAndOrder(steps []Step) ([]Step, error) {
	byID := make(map[string]Step, len(steps))
	for _, step := range steps {
		if step.ID == "" {
			return nil, fmt.Errorf("workflow step id is required")
		}
		if step.Action.Name == "" {
			return nil, fmt.Errorf("workflow step %q action name is required", step.ID)
		}
		if _, exists := byID[step.ID]; exists {
			return nil, fmt.Errorf("duplicate workflow step %q", step.ID)
		}
		byID[step.ID] = step
	}
	var ordered []Step
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) error
	visit = func(id string) error {
		if visited[id] {
			return nil
		}
		if visiting[id] {
			return fmt.Errorf("workflow contains dependency cycle at step %q", id)
		}
		step, ok := byID[id]
		if !ok {
			return fmt.Errorf("workflow step %q not found", id)
		}
		visiting[id] = true
		for _, dep := range step.DependsOn {
			if _, ok := byID[dep]; !ok {
				return fmt.Errorf("workflow step %q depends on unknown step %q", id, dep)
			}
			if err := visit(dep); err != nil {
				return err
			}
		}
		visiting[id] = false
		visited[id] = true
		ordered = append(ordered, step)
		return nil
	}
	for _, step := range steps {
		if err := visit(step.ID); err != nil {
			return nil, err
		}
	}
	return ordered, nil
}

// WorkflowAction exposes a workflow definition as an action.
type WorkflowAction struct {
	Definition Definition
	Executor   Executor
}

func (a WorkflowAction) Spec() action.Spec {
	return action.Spec{Name: a.Definition.Name, Description: a.Definition.Description}
}

func (a WorkflowAction) Execute(ctx action.Ctx, input any) action.Result {
	return a.Executor.Execute(ctx, a.Definition, input)
}

var _ action.Action = WorkflowAction{}
