// Package workflow provides minimal orchestration over action references.
package workflow

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

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
	Version     string
	Steps       []Step
}

// Result is the structured workflow execution result.
type Result struct {
	RunID       RunID
	StepResults map[string]action.Result
	Data        any
}

const (
	EventQueued        thread.EventKind = "workflow.queued"
	EventStarted       thread.EventKind = "workflow.started"
	EventStepStarted   thread.EventKind = "workflow.step_started"
	EventStepCompleted thread.EventKind = "workflow.step_completed"
	EventStepFailed    thread.EventKind = "workflow.step_failed"
	EventCompleted     thread.EventKind = "workflow.completed"
	EventFailed        thread.EventKind = "workflow.failed"
	EventCanceled      thread.EventKind = "workflow.canceled"
)

// EventDefinitions returns persistent thread-event definitions for workflow
// execution events. Live workflow events use the same concrete payload structs;
// persistence adapters choose the matching Event* kind when appending to a
// thread log.
func EventDefinitions() []thread.EventDefinition {
	return []thread.EventDefinition{
		thread.DefineEvent[Queued](EventQueued),
		thread.DefineEvent[Started](EventStarted),
		thread.DefineEvent[StepStarted](EventStepStarted),
		thread.DefineEvent[StepCompleted](EventStepCompleted),
		thread.DefineEvent[StepFailed](EventStepFailed),
		thread.DefineEvent[Completed](EventCompleted),
		thread.DefineEvent[Failed](EventFailed),
		thread.DefineEvent[Canceled](EventCanceled),
	}
}

type Queued struct {
	RunID             RunID       `json:"run_id"`
	WorkflowName      string      `json:"workflow_name"`
	Metadata          RunMetadata `json:"metadata,omitempty"`
	Input             ValueRef    `json:"input,omitempty"`
	DefinitionHash    string      `json:"definition_hash,omitempty"`
	DefinitionVersion string      `json:"definition_version,omitempty"`
	At                time.Time   `json:"at,omitempty"`
}

type Started struct {
	RunID             RunID       `json:"run_id"`
	WorkflowName      string      `json:"workflow_name"`
	Metadata          RunMetadata `json:"metadata,omitempty"`
	Input             ValueRef    `json:"input,omitempty"`
	DefinitionHash    string      `json:"definition_hash,omitempty"`
	DefinitionVersion string      `json:"definition_version,omitempty"`
	At                time.Time   `json:"at,omitempty"`
}

type StepStarted struct {
	RunID         RunID     `json:"run_id"`
	WorkflowName  string    `json:"workflow_name"`
	StepID        string    `json:"step_id"`
	ActionName    string    `json:"action_name"`
	ActionVersion string    `json:"action_version,omitempty"`
	Attempt       int       `json:"attempt"`
	At            time.Time `json:"at,omitempty"`
}

type StepCompleted struct {
	RunID         RunID     `json:"run_id"`
	WorkflowName  string    `json:"workflow_name"`
	StepID        string    `json:"step_id"`
	ActionName    string    `json:"action_name"`
	ActionVersion string    `json:"action_version,omitempty"`
	Attempt       int       `json:"attempt"`
	Output        ValueRef  `json:"output,omitempty"`
	At            time.Time `json:"at,omitempty"`
}

type StepFailed struct {
	RunID         RunID     `json:"run_id"`
	WorkflowName  string    `json:"workflow_name"`
	StepID        string    `json:"step_id"`
	ActionName    string    `json:"action_name"`
	ActionVersion string    `json:"action_version,omitempty"`
	Attempt       int       `json:"attempt"`
	Error         string    `json:"error"`
	At            time.Time `json:"at,omitempty"`
}

type Completed struct {
	RunID        RunID     `json:"run_id"`
	WorkflowName string    `json:"workflow_name"`
	Output       ValueRef  `json:"output,omitempty"`
	At           time.Time `json:"at,omitempty"`
}

type Failed struct {
	RunID        RunID     `json:"run_id"`
	WorkflowName string    `json:"workflow_name"`
	Error        string    `json:"error"`
	At           time.Time `json:"at,omitempty"`
}

type Canceled struct {
	RunID        RunID     `json:"run_id"`
	WorkflowName string    `json:"workflow_name"`
	Reason       string    `json:"reason,omitempty"`
	At           time.Time `json:"at,omitempty"`
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
	Resolver          Resolver
	OnEvent           EventHandler
	RunID             RunID
	NewRunID          func() RunID
	Now               func() time.Time
	Metadata          RunMetadata
	Input             ValueRef
	DefinitionHash    string
	DefinitionVersion string
}

type ExecuteOption func(*Executor)

func NewExecutor(resolver Resolver, opts ...ExecuteOption) Executor {
	executor := Executor{Resolver: resolver}
	for _, opt := range opts {
		if opt != nil {
			opt(&executor)
		}
	}
	return executor
}

func WithEventHandler(handler EventHandler) ExecuteOption {
	return func(e *Executor) {
		if handler == nil {
			return
		}
		previous := e.OnEvent
		e.OnEvent = func(ctx action.Ctx, event action.Event) {
			if previous != nil {
				previous(ctx, event)
			}
			handler(ctx, event)
		}
	}
}

func WithRunID(runID RunID) ExecuteOption {
	return func(e *Executor) { e.RunID = runID }
}

func WithRunMetadata(metadata RunMetadata) ExecuteOption {
	return func(e *Executor) { e.Metadata = metadata }
}

func WithInputRef(input ValueRef) ExecuteOption {
	return func(e *Executor) { e.Input = input }
}

func WithDefinitionIdentity(hash, version string) ExecuteOption {
	return func(e *Executor) {
		e.DefinitionHash = hash
		e.DefinitionVersion = version
	}
}

// Execute runs def and returns a workflow result in action.Result.Data. Execution
// stops at the first failed or unresolved step; partial step results are kept in
// Result.StepResults.
func (e Executor) Execute(ctx action.Ctx, def Definition, input any) action.Result {
	runID := e.RunID
	if runID == "" {
		if e.NewRunID != nil {
			runID = e.NewRunID()
		} else {
			runID = NewRunID()
		}
	}
	now := e.Now
	if now == nil {
		now = time.Now
	}
	var events []action.Event
	emit := func(event action.Event) {
		events = append(events, event)
		if e.OnEvent != nil {
			e.OnEvent(ctx, event)
		}
	}
	cancel := func(reason string, data any) action.Result {
		emit(Canceled{RunID: runID, WorkflowName: def.Name, Reason: reason, At: now()})
		return action.Result{Data: data, Error: ctx.Err(), Events: events}
	}
	fail := func(err error, data any) action.Result {
		emit(Failed{RunID: runID, WorkflowName: def.Name, Error: err.Error(), At: now()})
		return action.Result{Data: data, Error: err, Events: events}
	}

	if err := ctx.Err(); err != nil {
		return cancel(err.Error(), nil)
	}
	if e.Resolver == nil {
		return fail(fmt.Errorf("workflow %q has no action resolver", def.Name), nil)
	}
	ordered, err := validateAndOrder(def.Steps)
	if err != nil {
		return fail(err, nil)
	}
	definitionHash := e.DefinitionHash
	if definitionHash == "" {
		definitionHash = DefinitionHash(def)
	}
	definitionVersion := e.DefinitionVersion
	if definitionVersion == "" {
		definitionVersion = def.Version
	}
	emit(Started{RunID: runID, WorkflowName: def.Name, Metadata: e.Metadata, Input: e.Input, DefinitionHash: definitionHash, DefinitionVersion: definitionVersion, At: now()})
	results := make(map[string]action.Result, len(ordered))
	var last any = input
	for _, step := range ordered {
		if err := ctx.Err(); err != nil {
			return cancel(err.Error(), Result{RunID: runID, StepResults: results, Data: last})
		}
		a, ok := e.Resolver.ResolveAction(ctx, step.Action)
		if !ok || a == nil {
			return fail(fmt.Errorf("workflow %q step %q action %q not found", def.Name, step.ID, step.Action.Name), Result{RunID: runID, StepResults: results, Data: last})
		}
		stepInput := stepInput(step, input, results)
		if err := validateActionValue(a.Spec().Input, stepInput, "input"); err != nil {
			return fail(fmt.Errorf("workflow %q step %q invalid input: %w", def.Name, step.ID, err), Result{RunID: runID, StepResults: results, Data: last})
		}
		attempt := 1
		emit(StepStarted{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, Attempt: attempt, At: now()})
		res := a.Execute(ctx, stepInput)
		results[step.ID] = res
		events = append(events, res.Events...)
		if res.Error != nil {
			emit(StepFailed{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, Attempt: attempt, Error: res.Error.Error(), At: now()})
			if err := ctx.Err(); err != nil {
				return cancel(err.Error(), Result{RunID: runID, StepResults: results, Data: last})
			}
			err := fmt.Errorf("workflow %q step %q failed: %w", def.Name, step.ID, res.Error)
			return fail(err, Result{RunID: runID, StepResults: results, Data: last})
		}
		if err := validateActionValue(a.Spec().Output, res.Data, "output"); err != nil {
			emit(StepFailed{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, Attempt: attempt, Error: err.Error(), At: now()})
			return fail(fmt.Errorf("workflow %q step %q invalid output: %w", def.Name, step.ID, err), Result{RunID: runID, StepResults: results, Data: last})
		}
		last = res.Data
		emit(StepCompleted{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, Attempt: attempt, Output: InlineValue(res.Data), At: now()})
	}
	emit(Completed{RunID: runID, WorkflowName: def.Name, Output: InlineValue(last), At: now()})
	return action.Result{Data: Result{RunID: runID, StepResults: results, Data: last}, Events: events}
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

func DefinitionHash(def Definition) string {
	canonical := struct {
		Name    string
		Version string
		Steps   []Step
	}{Name: def.Name, Version: def.Version, Steps: def.Steps}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validateActionValue(t action.Type, value any, role string) error {
	if t.IsZero() || t.Schema == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%s is not JSON serializable: %w", role, err)
	}
	if err := t.ValidateJSON(raw); err != nil {
		return fmt.Errorf("%s does not match schema: %w", role, err)
	}
	return nil
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
