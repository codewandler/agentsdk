// Package workflow provides minimal orchestration over action references.
package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/thread"
)

// ActionRef identifies an action used by a workflow. Resolution is owned by the
// executor so workflows can stay declarative and serializable.
type ActionRef = action.Ref

// Step is one workflow node.
type Step struct {
	ID             string
	Action         ActionRef
	Input          any
	InputMap       map[string]string
	InputTemplate  any
	DependsOn      []string
	When           Condition
	Retry          RetryPolicy
	Timeout        time.Duration
	ErrorPolicy    StepErrorPolicy
	IdempotencyKey string
}

// Condition controls whether a step should execute after dependencies finish.
type Condition struct {
	StepID string
	Equals any
	Exists bool
	Not    bool
}

// RetryPolicy controls retry attempts for one step. MaxAttempts includes the
// first attempt; zero means one attempt.
type RetryPolicy struct {
	MaxAttempts int
	Backoff     time.Duration
}

// StepErrorPolicy controls how the executor handles terminal step failures.
type StepErrorPolicy string

const (
	StepErrorFail     StepErrorPolicy = ""
	StepErrorContinue StepErrorPolicy = "continue"
)

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
	EventStepSkipped   thread.EventKind = "workflow.step_skipped"
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
		thread.DefineEvent[StepSkipped](EventStepSkipped),
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
	RunID          RunID     `json:"run_id"`
	WorkflowName   string    `json:"workflow_name"`
	StepID         string    `json:"step_id"`
	ActionName     string    `json:"action_name"`
	ActionVersion  string    `json:"action_version,omitempty"`
	Attempt        int       `json:"attempt"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	At             time.Time `json:"at,omitempty"`
}

type StepCompleted struct {
	RunID          RunID     `json:"run_id"`
	WorkflowName   string    `json:"workflow_name"`
	StepID         string    `json:"step_id"`
	ActionName     string    `json:"action_name"`
	ActionVersion  string    `json:"action_version,omitempty"`
	Attempt        int       `json:"attempt"`
	Output         ValueRef  `json:"output,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	At             time.Time `json:"at,omitempty"`
}

type StepFailed struct {
	RunID          RunID     `json:"run_id"`
	WorkflowName   string    `json:"workflow_name"`
	StepID         string    `json:"step_id"`
	ActionName     string    `json:"action_name"`
	ActionVersion  string    `json:"action_version,omitempty"`
	Attempt        int       `json:"attempt"`
	Error          string    `json:"error"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	At             time.Time `json:"at,omitempty"`
}
type StepSkipped struct {
	RunID          RunID     `json:"run_id"`
	WorkflowName   string    `json:"workflow_name"`
	StepID         string    `json:"step_id"`
	ActionName     string    `json:"action_name"`
	ActionVersion  string    `json:"action_version,omitempty"`
	Reason         string    `json:"reason,omitempty"`
	IdempotencyKey string    `json:"idempotency_key,omitempty"`
	At             time.Time `json:"at,omitempty"`
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
	MaxConcurrency    int
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

func WithMaxConcurrency(max int) ExecuteOption {
	return func(e *Executor) { e.MaxConcurrency = max }
}

// Execute runs def and returns a workflow result in action.Result.Data. The
// default MaxConcurrency=1 preserves sequential topological pipeline semantics;
// higher values execute independent ready steps concurrently.
func (e Executor) Execute(ctx action.Ctx, def Definition, input any) action.Result {
	runID := e.runID()
	now := e.nowFunc()
	var (
		mu     sync.Mutex
		events []action.Event
	)
	emit := func(event action.Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
		if e.OnEvent != nil {
			e.OnEvent(ctx, event)
		}
	}
	appendEvents := func(extra []action.Event) {
		if len(extra) == 0 {
			return
		}
		mu.Lock()
		events = append(events, extra...)
		mu.Unlock()
	}
	resultEvents := func() []action.Event {
		mu.Lock()
		defer mu.Unlock()
		return append([]action.Event(nil), events...)
	}
	cancel := func(reason string, data any) action.Result {
		emit(Canceled{RunID: runID, WorkflowName: def.Name, Reason: reason, At: now()})
		return action.Result{Data: data, Error: ctx.Err(), Events: resultEvents()}
	}
	fail := func(err error, data any) action.Result {
		emit(Failed{RunID: runID, WorkflowName: def.Name, Error: err.Error(), At: now()})
		return action.Result{Data: data, Error: err, Events: resultEvents()}
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

	results, last, err := e.executeSteps(ctx, def, ordered, input, runID, now, emit, appendEvents)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return cancel(ctxErr.Error(), Result{RunID: runID, StepResults: results, Data: last})
		}
		return fail(err, Result{RunID: runID, StepResults: results, Data: last})
	}
	emit(Completed{RunID: runID, WorkflowName: def.Name, Output: valueRefFor(last), At: now()})
	return action.Result{Data: Result{RunID: runID, StepResults: results, Data: last}, Events: resultEvents()}
}

func (e Executor) runID() RunID {
	if e.RunID != "" {
		return e.RunID
	}
	if e.NewRunID != nil {
		return e.NewRunID()
	}
	return NewRunID()
}

func (e Executor) nowFunc() func() time.Time {
	if e.Now != nil {
		return e.Now
	}
	return time.Now
}

func (e Executor) executeSteps(ctx action.Ctx, def Definition, ordered []Step, input any, runID RunID, now func() time.Time, emit func(action.Event), appendEvents func([]action.Event)) (map[string]action.Result, any, error) {
	if e.MaxConcurrency <= 1 {
		return e.executeSequential(ctx, def, ordered, input, runID, now, emit, appendEvents)
	}
	return e.executeParallel(ctx, def, ordered, input, runID, now, emit, appendEvents)
}

func (e Executor) executeSequential(ctx action.Ctx, def Definition, ordered []Step, input any, runID RunID, now func() time.Time, emit func(action.Event), appendEvents func([]action.Event)) (map[string]action.Result, any, error) {
	results := make(map[string]action.Result, len(ordered))
	var last any = input
	for _, step := range ordered {
		res, skipped, err := e.executeStep(ctx, def, step, input, results, runID, now, emit, appendEvents)
		results[step.ID] = res
		if err != nil {
			if step.ErrorPolicy == StepErrorContinue {
				continue
			}
			return results, last, err
		}
		if !skipped {
			last = res.Data
		}
	}
	return results, last, nil
}

func (e Executor) executeParallel(ctx action.Ctx, def Definition, ordered []Step, input any, runID RunID, now func() time.Time, emit func(action.Event), appendEvents func([]action.Event)) (map[string]action.Result, any, error) {
	results := make(map[string]action.Result, len(ordered))
	done := map[string]bool{}
	limit := e.MaxConcurrency
	if limit <= 0 {
		limit = 1
	}
	var last any = input
	for len(done) < len(ordered) {
		if err := ctx.Err(); err != nil {
			return results, last, err
		}
		ready := readySteps(ordered, done)
		if len(ready) == 0 {
			return results, last, fmt.Errorf("workflow %q has no runnable steps; dependency state is incomplete", def.Name)
		}
		if len(ready) > limit {
			ready = ready[:limit]
		}
		type outcome struct {
			step    Step
			result  action.Result
			skipped bool
			err     error
		}
		outcomes := make([]outcome, len(ready))
		var wg sync.WaitGroup
		for i, step := range ready {
			wg.Add(1)
			go func(i int, step Step) {
				defer wg.Done()
				res, skipped, err := e.executeStep(ctx, def, step, input, resultsSnapshot(results), runID, now, emit, appendEvents)
				outcomes[i] = outcome{step: step, result: res, skipped: skipped, err: err}
			}(i, step)
		}
		wg.Wait()
		for _, out := range outcomes {
			results[out.step.ID] = out.result
			done[out.step.ID] = true
			if out.err != nil && out.step.ErrorPolicy != StepErrorContinue {
				return results, last, out.err
			}
			if out.err == nil && !out.skipped {
				last = out.result.Data
			}
		}
	}
	return results, last, nil
}

func readySteps(ordered []Step, done map[string]bool) []Step {
	ready := make([]Step, 0)
	for _, step := range ordered {
		if done[step.ID] {
			continue
		}
		depsDone := true
		for _, dep := range step.DependsOn {
			if !done[dep] {
				depsDone = false
				break
			}
		}
		if depsDone {
			ready = append(ready, step)
		}
	}
	return ready
}

func resultsSnapshot(results map[string]action.Result) map[string]action.Result {
	out := make(map[string]action.Result, len(results))
	for key, value := range results {
		out[key] = value
	}
	return out
}

func (e Executor) executeStep(ctx action.Ctx, def Definition, step Step, initial any, results map[string]action.Result, runID RunID, now func() time.Time, emit func(action.Event), appendEvents func([]action.Event)) (action.Result, bool, error) {
	if err := ctx.Err(); err != nil {
		return action.Result{Error: err}, false, err
	}
	if !conditionMatches(step.When, results) {
		emit(StepSkipped{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, IdempotencyKey: step.IdempotencyKey, Reason: "condition false", At: now()})
		return action.Result{}, true, nil
	}
	a, ok := e.Resolver.ResolveAction(ctx, step.Action)
	if !ok || a == nil {
		err := fmt.Errorf("workflow %q step %q action %q not found", def.Name, step.ID, step.Action.Name)
		return action.Result{Error: err}, false, err
	}
	stepInput := stepInput(step, initial, results)
	if err := validateActionValue(a.Spec().Input, stepInput, "input"); err != nil {
		err = fmt.Errorf("workflow %q step %q invalid input: %w", def.Name, step.ID, err)
		emit(StepFailed{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, Attempt: 1, Error: err.Error(), IdempotencyKey: step.IdempotencyKey, At: now()})
		return action.Result{Error: err}, false, err
	}
	maxAttempts := step.Retry.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	var last action.Result
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		stepCtx := action.Ctx(ctx)
		cancel := func() {}
		if step.Timeout > 0 {
			deadlineCtx, deadlineCancel := context.WithTimeout(ctx, step.Timeout)
			stepCtx = action.NewCtx(deadlineCtx,
				action.WithOutput(ctx.Output()),
				action.WithEmit(ctx.Emit),
			)
			cancel = deadlineCancel
		}
		emit(StepStarted{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, Attempt: attempt, IdempotencyKey: step.IdempotencyKey, At: now()})
		last = a.Execute(stepCtx, stepInput)
		cancel()
		appendEvents(last.Events)
		lastErr = last.Error
		if lastErr == nil && step.Timeout > 0 && stepCtx.Err() != nil {
			lastErr = stepCtx.Err()
			last.Error = lastErr
		}
		if lastErr == nil {
			if err := validateActionValue(a.Spec().Output, last.Data, "output"); err != nil {
				lastErr = fmt.Errorf("workflow %q step %q invalid output: %w", def.Name, step.ID, err)
				last.Error = lastErr
			}
		}
		if lastErr == nil {
			emit(StepCompleted{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, Attempt: attempt, Output: valueRefFor(last.Data), IdempotencyKey: step.IdempotencyKey, At: now()})
			return last, false, nil
		}
		emit(StepFailed{RunID: runID, WorkflowName: def.Name, StepID: step.ID, ActionName: step.Action.Name, ActionVersion: step.Action.Version, Attempt: attempt, Error: lastErr.Error(), IdempotencyKey: step.IdempotencyKey, At: now()})
		if err := ctx.Err(); err != nil {
			return last, false, err
		}
		if attempt < maxAttempts && step.Retry.Backoff > 0 {
			select {
			case <-ctx.Done():
				return last, false, ctx.Err()
			case <-time.After(step.Retry.Backoff):
			}
		}
	}
	return last, false, fmt.Errorf("workflow %q step %q failed: %w", def.Name, step.ID, lastErr)
}

func stepInput(step Step, initial any, results map[string]action.Result) any {
	if step.InputMap != nil {
		out := make(map[string]any, len(step.InputMap))
		for key, expr := range step.InputMap {
			out[key] = resolveInputExpression(expr, initial, results)
		}
		return out
	}
	if step.InputTemplate != nil {
		return renderInputTemplate(step.InputTemplate, initial, results)
	}
	if step.Input != nil {
		return renderInputTemplate(step.Input, initial, results)
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

func conditionMatches(condition Condition, results map[string]action.Result) bool {
	if condition.StepID == "" {
		return true
	}
	res, ok := results[condition.StepID]
	matched := ok
	if condition.Exists {
		matched = ok && res.Data != nil
	}
	if condition.Equals != nil {
		matched = ok && reflect.DeepEqual(res.Data, condition.Equals)
	}
	if condition.Not {
		return !matched
	}
	return matched
}

func renderInputTemplate(template any, initial any, results map[string]action.Result) any {
	switch v := template.(type) {
	case string:
		return renderTemplateString(v, initial, results)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = renderInputTemplate(item, initial, results)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = renderInputTemplate(item, initial, results)
		}
		return out
	default:
		return template
	}
}

func renderTemplateString(template string, initial any, results map[string]action.Result) any {
	trimmed := strings.TrimSpace(template)
	if strings.HasPrefix(trimmed, "{{") && strings.HasSuffix(trimmed, "}}") && strings.Count(trimmed, "{{") == 1 {
		return resolveInputExpression(strings.TrimSuffix(strings.TrimPrefix(trimmed, "{{"), "}}"), initial, results)
	}
	out := template
	for out != "" {
		start := strings.Index(out, "{{")
		if start < 0 {
			return out
		}
		endRel := strings.Index(out[start+2:], "}}")
		if endRel < 0 {
			return out
		}
		end := start + 2 + endRel
		expr := strings.TrimSpace(out[start+2 : end])
		value := fmt.Sprint(resolveInputExpression(expr, initial, results))
		out = out[:start] + value + out[end+2:]
	}
	return out
}

func resolveInputExpression(expr string, initial any, results map[string]action.Result) any {
	expr = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(expr, "$"), "."))
	switch expr {
	case "input":
		return initial
	case "":
		return nil
	}
	parts := strings.Split(expr, ".")
	if len(parts) >= 3 && parts[0] == "steps" {
		res, ok := results[parts[1]]
		if !ok {
			return nil
		}
		switch parts[2] {
		case "output", "data":
			return valuePath(res.Data, parts[3:])
		case "error":
			if res.Error == nil {
				return nil
			}
			return res.Error.Error()
		}
	}
	return nil
}

func valuePath(value any, path []string) any {
	cur := value
	for _, part := range path {
		switch v := cur.(type) {
		case map[string]any:
			cur = v[part]
		case map[string]string:
			cur = v[part]
		default:
			rv := reflect.ValueOf(cur)
			if rv.Kind() == reflect.Struct {
				field := rv.FieldByName(part)
				if field.IsValid() && field.CanInterface() {
					cur = field.Interface()
					continue
				}
			}
			return nil
		}
	}
	return cur
}

func valueRefFor(value any) ValueRef {
	if ref, ok := value.(ValueRef); ok {
		return ref
	}
	return InlineValue(value)
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
