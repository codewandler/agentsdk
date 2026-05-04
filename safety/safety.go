// Package safety defines surface-neutral safety and approval policy primitives.
//
// The package intentionally sits above action and below harness/channel code:
// actions expose intent, safety evaluates that intent, and harness/channel layers
// decide how approvals are presented to users. Existing tool middleware can adapt
// these primitives without moving terminal risk logging opportunistically.
package safety

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/codewandler/agentsdk/action"
)

// DecisionAction is the normalized safety gate outcome.
type DecisionAction string

const (
	DecisionAllow            DecisionAction = "allow"
	DecisionRequiresApproval DecisionAction = "requires_approval"
	DecisionReject           DecisionAction = "reject"
	DecisionError            DecisionAction = "error"
)

// Dimension is one scored safety/risk dimension.
type Dimension struct {
	Name     string `json:"name"`
	Severity int    `json:"severity"` // 0-10
	Reason   string `json:"reason,omitempty"`
}

// Decision is the policy decision for one intent.
type Decision struct {
	Action    DecisionAction `json:"action"`
	Reasons   []string       `json:"reasons,omitempty"`
	Rationale string         `json:"rationale,omitempty"`
}

// Assessment is the result of evaluating an action intent.
type Assessment struct {
	Decision    Decision    `json:"decision"`
	Dimensions  []Dimension `json:"dimensions,omitempty"`
	Confidence  string      `json:"confidence,omitempty"`
	Explanation string      `json:"explanation,omitempty"`
}

// Assessor evaluates surface-neutral action intents.
type Assessor interface {
	Assess(ctx action.Ctx, intent action.Intent) (Assessment, error)
}

// Approver is supplied by a harness/session/channel boundary when human or host
// policy approval is possible.
type Approver func(ctx action.Ctx, request ApprovalRequest) (bool, error)

// ApprovalRequest is the structured payload passed to an approver.
type ApprovalRequest struct {
	Intent     action.Intent `json:"intent"`
	Assessment Assessment    `json:"assessment"`
}

// EventType classifies safety/audit publications.
type EventType string

const (
	EventAssessed EventType = "safety.assessed"
	EventApproved EventType = "safety.approved"
	EventRejected EventType = "safety.rejected"
	EventDenied   EventType = "safety.denied"
	EventErrored  EventType = "safety.errored"
	EventAllowed  EventType = "safety.allowed"
)

// Event is a durable/auditable safety publication. It is intentionally usable as
// an action.Event so action, workflow, harness, and channel layers can persist or
// stream it without depending on tool-specific middleware.
type Event struct {
	ID         string        `json:"id,omitempty"`
	Type       EventType     `json:"type"`
	At         time.Time     `json:"at"`
	Intent     action.Intent `json:"intent"`
	Assessment Assessment    `json:"assessment"`
	Approved   bool          `json:"approved,omitempty"`
	Error      string        `json:"error,omitempty"`
}

// AuditStore records safety decisions. Thread-backed stores can implement this
// interface later; InMemoryAuditStore gives tests and embedded hosts a concrete
// baseline now.
type AuditStore interface {
	AppendSafetyEvent(ctx context.Context, event Event) error
	ListSafetyEvents(ctx context.Context) ([]Event, error)
}

type InMemoryAuditStore struct {
	mu     sync.Mutex
	events []Event
}

func NewInMemoryAuditStore() *InMemoryAuditStore { return &InMemoryAuditStore{} }

func (s *InMemoryAuditStore) AppendSafetyEvent(ctx context.Context, event Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if event.At.IsZero() {
		event.At = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, cloneEvent(event))
	return nil
}

func (s *InMemoryAuditStore) ListSafetyEvents(ctx context.Context) ([]Event, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, len(s.events))
	for i, event := range s.events {
		out[i] = cloneEvent(event)
	}
	return out, nil
}

// Gate is action middleware that assesses intent, applies approval policy, and
// records audit events before allowing an action to execute.
type Gate struct {
	action.HooksBase
	Assessor Assessor
	Approver Approver
	Audit    AuditStore
	FailOpen bool
	Now      func() time.Time
}

func NewGate(assessor Assessor) action.Middleware {
	return action.HooksMiddleware(&Gate{Assessor: assessor})
}

func (g *Gate) OnInput(ctx action.Ctx, inner action.Action, input any, state action.CallState) (any, action.Result, bool) {
	if g.Assessor == nil {
		return input, action.Result{}, false
	}
	intent := action.ExtractIntent(inner, ctx, input)
	state["safety.intent"] = intent
	assessment, err := g.Assessor.Assess(ctx, intent)
	if err != nil {
		if g.FailOpen {
			g.append(ctx, Event{Type: EventErrored, Intent: intent, Error: err.Error()})
			return input, action.Result{}, false
		}
		event := g.append(ctx, Event{Type: EventErrored, Intent: intent, Error: err.Error()})
		return input, action.Failed(fmt.Errorf("safety assessment error: %w", err), event), true
	}
	state["safety.assessment"] = assessment
	g.append(ctx, Event{Type: EventAssessed, Intent: intent, Assessment: assessment})

	switch assessment.Decision.Action {
	case DecisionAllow:
		event := g.append(ctx, Event{Type: EventAllowed, Intent: intent, Assessment: assessment})
		state["safety.event"] = event
		return input, action.Result{}, false
	case DecisionRequiresApproval:
		if g.Approver == nil {
			event := g.append(ctx, Event{Type: EventDenied, Intent: intent, Assessment: assessment, Error: "approval required but no approver configured"})
			return input, action.Failed(errors.New("safety approval required but no approver configured"), event), true
		}
		approved, err := g.Approver(ctx, ApprovalRequest{Intent: intent, Assessment: assessment})
		if err != nil {
			event := g.append(ctx, Event{Type: EventErrored, Intent: intent, Assessment: assessment, Error: err.Error()})
			return input, action.Failed(fmt.Errorf("safety approval error: %w", err), event), true
		}
		if !approved {
			event := g.append(ctx, Event{Type: EventRejected, Intent: intent, Assessment: assessment})
			return input, action.Failed(errors.New("safety approval rejected"), event), true
		}
		event := g.append(ctx, Event{Type: EventApproved, Intent: intent, Assessment: assessment, Approved: true})
		state["safety.event"] = event
		return input, action.Result{}, false
	case DecisionReject:
		event := g.append(ctx, Event{Type: EventRejected, Intent: intent, Assessment: assessment})
		return input, action.Failed(fmt.Errorf("safety rejected: %s", assessment.Decision.Rationale), event), true
	case DecisionError:
		event := g.append(ctx, Event{Type: EventErrored, Intent: intent, Assessment: assessment, Error: assessment.Decision.Rationale})
		return input, action.Failed(fmt.Errorf("safety decision error: %s", assessment.Decision.Rationale), event), true
	default:
		return input, action.Result{}, false
	}
}

func (g *Gate) OnResult(ctx action.Ctx, inner action.Action, input any, result action.Result, state action.CallState) action.Result {
	if result.IsError() {
		return result
	}
	if event, ok := state["safety.event"].(Event); ok {
		result.Events = append([]action.Event{event}, result.Events...)
	}
	return result
}

func (g *Gate) append(ctx context.Context, event Event) Event {
	if event.At.IsZero() {
		if g.Now != nil {
			event.At = g.Now()
		} else {
			event.At = time.Now().UTC()
		}
	}
	if g.Audit != nil {
		_ = g.Audit.AppendSafetyEvent(ctx, event)
	}
	return event
}

func cloneEvent(event Event) Event {
	event.Intent.Operations = append([]action.IntentOperation(nil), event.Intent.Operations...)
	event.Intent.Behaviors = append([]string(nil), event.Intent.Behaviors...)
	event.Assessment.Decision.Reasons = append([]string(nil), event.Assessment.Decision.Reasons...)
	event.Assessment.Dimensions = append([]Dimension(nil), event.Assessment.Dimensions...)
	return event
}
