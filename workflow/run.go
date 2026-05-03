package workflow

import (
	"fmt"
	"sync/atomic"
	"time"

	gonanoid "github.com/matoous/go-nanoid/v2"
)

// RunID identifies one execution of a workflow definition.
type RunID string

var runSeq uint64

// NewRunID returns a workflow run identifier. Future durable harnesses may
// supply their own IDs through Executor.RunID or Executor.NewRunID.
func NewRunID() RunID {
	id, err := gonanoid.New(12)
	if err != nil {
		return RunID(fmt.Sprintf("run_%d", atomic.AddUint64(&runSeq, 1)))
	}
	return RunID("run_" + id)
}

// RunStatus is the materialized status of a workflow run.
type RunStatus string

const (
	RunQueued    RunStatus = "queued"
	RunRunning   RunStatus = "running"
	RunSucceeded RunStatus = "succeeded"
	RunFailed    RunStatus = "failed"
	RunCanceled  RunStatus = "canceled"
)

// StepStatus is the materialized status of a workflow step.
type StepStatus string

const (
	StepQueued        StepStatus = "queued"
	StepRunning       StepStatus = "running"
	StepSucceeded     StepStatus = "succeeded"
	StepFailedStatus  StepStatus = "failed"
	StepCanceled      StepStatus = "canceled"
	StepSkippedStatus StepStatus = "skipped"
)

// RunState is the materialized state of one workflow execution.
type RunState struct {
	ID                RunID
	WorkflowName      string
	Status            RunStatus
	StartedAt         time.Time
	CompletedAt       time.Time
	Duration          time.Duration
	Steps             map[string]StepState
	Output            ValueRef
	Error             string
	Metadata          RunMetadata
	Input             ValueRef
	DefinitionHash    string
	DefinitionVersion string
}

// RunSummary is the compact read-model view for listing workflow executions.
type RunSummary struct {
	ID                RunID
	WorkflowName      string
	Status            RunStatus
	StartedAt         time.Time
	CompletedAt       time.Time
	Duration          time.Duration
	Error             string
	Metadata          RunMetadata
	Input             ValueRef
	DefinitionHash    string
	DefinitionVersion string
}

// RunMetadata describes the harness/session source that invoked a workflow run.
type RunMetadata struct {
	SessionID   string   `json:"session_id,omitempty"`
	AgentName   string   `json:"agent_name,omitempty"`
	ThreadID    string   `json:"thread_id,omitempty"`
	BranchID    string   `json:"branch_id,omitempty"`
	Trigger     string   `json:"trigger,omitempty"`
	CommandPath []string `json:"command_path,omitempty"`
}

// StepState is the materialized state of one workflow step.
type StepState struct {
	ID             string
	ActionName     string
	ActionVersion  string
	IdempotencyKey string
	Status         StepStatus
	Attempt        int
	Attempts       []AttemptState
	Output         ValueRef
	Error          string
}

// AttemptState is the materialized state of one step attempt.
type AttemptState struct {
	Attempt int
	Status  StepStatus
	Output  ValueRef
	Error   string
}

// Projector materializes workflow run state from concrete workflow event
// payloads. Non-workflow events are ignored so action-emitted events can share
// the same action.Result.Events slice.
type Projector struct{}

// Project materializes states for all workflow runs represented in events.
func (Projector) Project(events []any) (map[RunID]RunState, error) {
	states := map[RunID]RunState{}
	for _, event := range events {
		if err := applyEvent(states, event); err != nil {
			return nil, err
		}
	}
	return states, nil
}

// ProjectRun materializes the state for runID from events.
func (p Projector) ProjectRun(events []any, runID RunID) (RunState, bool, error) {
	states, err := p.Project(events)
	if err != nil {
		return RunState{}, false, err
	}
	state, ok := states[runID]
	return state, ok, nil
}

func applyEvent(states map[RunID]RunState, event any) error {
	switch e := event.(type) {
	case Queued:
		state := stateFor(states, e.RunID)
		state.ID = e.RunID
		state.WorkflowName = e.WorkflowName
		state.Status = RunQueued
		state.Metadata = e.Metadata
		state.Input = e.Input
		state.DefinitionHash = e.DefinitionHash
		state.DefinitionVersion = e.DefinitionVersion
		state.StartedAt = time.Time{}
		state.CompletedAt = time.Time{}
		state.Duration = 0
		states[e.RunID] = state
	case Started:
		state := stateFor(states, e.RunID)
		state.ID = e.RunID
		state.WorkflowName = e.WorkflowName
		state.Status = RunRunning
		state.StartedAt = e.At
		state.CompletedAt = time.Time{}
		state.Duration = 0
		if !e.Metadata.IsZero() {
			state.Metadata = e.Metadata
		}
		if !emptyValueRef(e.Input) {
			state.Input = e.Input
		}
		if e.DefinitionHash != "" {
			state.DefinitionHash = e.DefinitionHash
		}
		if e.DefinitionVersion != "" {
			state.DefinitionVersion = e.DefinitionVersion
		}
		states[e.RunID] = state
	case StepStarted:
		state := stateFor(states, e.RunID)
		step := state.Steps[e.StepID]
		step.ID = e.StepID
		step.ActionName = e.ActionName
		step.ActionVersion = e.ActionVersion
		step.Status = StepRunning
		step.IdempotencyKey = e.IdempotencyKey
		step.Attempt = normalizeAttempt(e.Attempt)
		step.Error = ""
		step = upsertAttempt(step, AttemptState{Attempt: step.Attempt, Status: StepRunning})
		state.Steps[e.StepID] = step
		states[e.RunID] = state
	case StepCompleted:
		state := stateFor(states, e.RunID)
		step := state.Steps[e.StepID]
		step.ID = e.StepID
		step.ActionName = e.ActionName
		step.ActionVersion = e.ActionVersion
		step.Status = StepSucceeded
		step.IdempotencyKey = e.IdempotencyKey
		step.Attempt = normalizeAttempt(e.Attempt)
		step.Output = e.Output
		step.Error = ""
		step = upsertAttempt(step, AttemptState{Attempt: step.Attempt, Status: StepSucceeded, Output: e.Output})
		state.Steps[e.StepID] = step
		states[e.RunID] = state
	case StepFailed:
		state := stateFor(states, e.RunID)
		step := state.Steps[e.StepID]
		step.ID = e.StepID
		step.ActionName = e.ActionName
		step.ActionVersion = e.ActionVersion
		step.IdempotencyKey = e.IdempotencyKey
		step.Status = StepFailedStatus
		step.Attempt = normalizeAttempt(e.Attempt)
		step.Error = e.Error
		step = upsertAttempt(step, AttemptState{Attempt: step.Attempt, Status: StepFailedStatus, Error: e.Error})
		state.Steps[e.StepID] = step
		states[e.RunID] = state
	case StepSkipped:
		state := stateFor(states, e.RunID)
		step := state.Steps[e.StepID]
		step.ID = e.StepID
		step.ActionName = e.ActionName
		step.ActionVersion = e.ActionVersion
		step.IdempotencyKey = e.IdempotencyKey
		step.Status = StepSkippedStatus
		step.Error = e.Reason
		state.Steps[e.StepID] = step
		states[e.RunID] = state
	case Completed:
		state := stateFor(states, e.RunID)
		state.ID = e.RunID
		state.WorkflowName = e.WorkflowName
		state.Status = RunSucceeded
		state.CompletedAt = e.At
		state.Duration = runDuration(state.StartedAt, state.CompletedAt)
		state.Output = e.Output
		state.Error = ""
		states[e.RunID] = state
	case Failed:
		state := stateFor(states, e.RunID)
		state.ID = e.RunID
		state.WorkflowName = e.WorkflowName
		state.Status = RunFailed
		state.CompletedAt = e.At
		state.Duration = runDuration(state.StartedAt, state.CompletedAt)
		state.Error = e.Error
		states[e.RunID] = state
	case Canceled:
		state := stateFor(states, e.RunID)
		state.ID = e.RunID
		state.WorkflowName = e.WorkflowName
		state.Status = RunCanceled
		state.CompletedAt = e.At
		state.Duration = runDuration(state.StartedAt, state.CompletedAt)
		state.Error = e.Reason
		states[e.RunID] = state
	case *Queued, *Started, *StepStarted, *StepCompleted, *StepFailed, *StepSkipped, *Completed, *Failed, *Canceled:
		return fmt.Errorf("workflow: pointer events are not supported")
	default:
		return nil
	}
	return nil
}

func runDuration(startedAt, completedAt time.Time) time.Duration {
	if startedAt.IsZero() || completedAt.IsZero() || completedAt.Before(startedAt) {
		return 0
	}
	return completedAt.Sub(startedAt)
}

func normalizeAttempt(attempt int) int {
	if attempt <= 0 {
		return 1
	}
	return attempt
}

func upsertAttempt(step StepState, attempt AttemptState) StepState {
	attempt.Attempt = normalizeAttempt(attempt.Attempt)
	for i, existing := range step.Attempts {
		if existing.Attempt == attempt.Attempt {
			step.Attempts[i] = attempt
			return step
		}
	}
	step.Attempts = append(step.Attempts, attempt)
	return step
}

func stateFor(states map[RunID]RunState, runID RunID) RunState {
	state := states[runID]
	if state.Steps == nil {
		state.Steps = map[string]StepState{}
	}
	return state
}

func (m RunMetadata) IsZero() bool {
	return m.SessionID == "" && m.AgentName == "" && m.ThreadID == "" && m.BranchID == "" && m.Trigger == "" && len(m.CommandPath) == 0
}

func emptyValueRef(value ValueRef) bool {
	return value.ID == "" && value.MediaType == "" && value.ExternalURI == "" && value.Inline == nil && !value.Redacted
}
