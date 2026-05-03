package trigger

import (
	"context"
	"time"
)

type EventID string
type RuleID string
type SourceID string

type Event struct {
	ID                EventID
	Type              string
	SourceID          SourceID
	Subject           string
	At                time.Time
	Data              map[string]any
	CorrelationID     string
	CausedByRunID     string
	CausedBySessionID string
}

const EventTypeInterval = "timer.interval"

type SessionMode string

const (
	SessionShared         SessionMode = "shared"
	SessionTriggerOwned   SessionMode = "trigger_owned"
	SessionEphemeral      SessionMode = "ephemeral"
	SessionResumeOrCreate SessionMode = "resume_or_create"
)

type TargetKind string

const (
	TargetWorkflow    TargetKind = "workflow"
	TargetAgentPrompt TargetKind = "agent_prompt"
	TargetAction      TargetKind = "action"
)

type Target struct {
	Kind         TargetKind
	WorkflowName string
	AgentName    string
	Prompt       string
	ActionName   string
	Input        any
	IncludeEvent bool
}

func (t Target) Name() string {
	switch t.Kind {
	case TargetWorkflow:
		return t.WorkflowName
	case TargetAgentPrompt:
		return t.AgentName
	case TargetAction:
		return t.ActionName
	default:
		return ""
	}
}

type SessionPolicy struct {
	Mode      SessionMode
	Name      string
	AgentName string
}

type OverlapPolicy string

const (
	OverlapSkipIfRunning OverlapPolicy = "skip_if_running"
)

type JobPolicy struct {
	Overlap OverlapPolicy
	Timeout time.Duration
}

type Rule struct {
	ID      RuleID
	Source  Source
	Matcher Matcher
	Target  Target
	Session SessionPolicy
	Policy  JobPolicy
}

func (r Rule) Normalized() Rule {
	if r.Matcher == nil {
		r.Matcher = MatchAll{}
	}
	if r.Policy.Overlap == "" {
		r.Policy.Overlap = OverlapSkipIfRunning
	}
	if r.Session.Mode == "" {
		r.Session.Mode = SessionTriggerOwned
	}
	return r
}

func (r Rule) EventType() string {
	if source, ok := r.Source.(interface{ EventType() string }); ok {
		return source.EventType()
	}
	return ""
}

type Execution struct {
	Rule  Rule
	Event Event
}

type ExecutionResult struct {
	TargetKind    TargetKind
	TargetName    string
	SessionName   string
	SessionID     string
	WorkflowRunID string
	Output        any
}

type Executor interface {
	ExecuteTrigger(context context.Context, execution Execution) (ExecutionResult, error)
}
