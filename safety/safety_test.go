package safety

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/stretchr/testify/require"
)

type staticAssessor struct {
	assessment Assessment
	err        error
}

func (a staticAssessor) Assess(action.Ctx, action.Intent) (Assessment, error) {
	return a.assessment, a.err
}

type intentAction struct {
	executed bool
	intent   action.Intent
}

func (a *intentAction) Spec() action.Spec { return action.Spec{Name: "write_config"} }
func (a *intentAction) Execute(action.Ctx, any) action.Result {
	a.executed = true
	return action.OK("done")
}
func (a *intentAction) DeclareIntent(action.Ctx, any) (action.Intent, error) {
	return a.intent, nil
}

func TestGateAllowsAndPublishesAuditEvent(t *testing.T) {
	store := NewInMemoryAuditStore()
	base := &intentAction{intent: action.Intent{Class: "filesystem_read", Confidence: "high"}}
	gate := action.Apply(base, action.HooksMiddleware(&Gate{
		Assessor: staticAssessor{assessment: Assessment{Decision: Decision{Action: DecisionAllow}, Confidence: "high"}},
		Audit:    store,
		Now:      fixedNow,
	}))

	result := gate.Execute(action.NewCtx(context.Background()), map[string]any{"path": "README.md"})

	require.False(t, result.IsError())
	require.True(t, base.executed)
	require.Len(t, result.Events, 1)
	event, ok := result.Events[0].(Event)
	require.True(t, ok)
	require.Equal(t, EventAllowed, event.Type)
	require.Equal(t, "write_config", event.Intent.Action)
	events, err := store.ListSafetyEvents(action.NewCtx(context.Background()))
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, EventAssessed, events[0].Type)
	require.Equal(t, EventAllowed, events[1].Type)
}

func TestGateRequiresApprovalAtBoundary(t *testing.T) {
	base := &intentAction{intent: action.Intent{Class: "filesystem_write", Confidence: "high"}}
	var request ApprovalRequest
	gate := action.Apply(base, action.HooksMiddleware(&Gate{
		Assessor: staticAssessor{assessment: Assessment{Decision: Decision{Action: DecisionRequiresApproval, Rationale: "writes outside workspace"}}},
		Approver: func(ctx action.Ctx, req ApprovalRequest) (bool, error) {
			request = req
			return true, nil
		},
		Now: fixedNow,
	}))

	result := gate.Execute(action.NewCtx(context.Background()), nil)

	require.False(t, result.IsError())
	require.True(t, base.executed)
	require.Equal(t, DecisionRequiresApproval, request.Assessment.Decision.Action)
	require.Equal(t, "write_config", request.Intent.Action)
	require.Len(t, result.Events, 1)
	require.Equal(t, EventApproved, result.Events[0].(Event).Type)
}

func TestGateDeniesApprovalWhenNoApprover(t *testing.T) {
	base := &intentAction{intent: action.Intent{Class: "filesystem_write", Confidence: "high"}}
	gate := action.Apply(base, action.HooksMiddleware(&Gate{
		Assessor: staticAssessor{assessment: Assessment{Decision: Decision{Action: DecisionRequiresApproval}}},
		Now:      fixedNow,
	}))

	result := gate.Execute(action.NewCtx(context.Background()), nil)

	require.True(t, result.IsError())
	require.False(t, base.executed)
	require.ErrorContains(t, result.Err(), "approval required")
	require.Len(t, result.Events, 1)
	require.Equal(t, EventDenied, result.Events[0].(Event).Type)
}

func TestGateRejectsBeforeExecution(t *testing.T) {
	base := &intentAction{intent: action.Intent{Class: "command_execution", Confidence: "high"}}
	gate := action.Apply(base, action.HooksMiddleware(&Gate{
		Assessor: staticAssessor{assessment: Assessment{Decision: Decision{Action: DecisionReject, Rationale: "destructive"}}},
		Now:      fixedNow,
	}))

	result := gate.Execute(action.NewCtx(context.Background()), nil)

	require.True(t, result.IsError())
	require.False(t, base.executed)
	require.ErrorContains(t, result.Err(), "destructive")
	require.Equal(t, EventRejected, result.Events[0].(Event).Type)
}

func TestGateFailsClosedOnAssessmentError(t *testing.T) {
	base := &intentAction{intent: action.Intent{Class: "unknown", Opaque: true}}
	gate := action.Apply(base, action.HooksMiddleware(&Gate{
		Assessor: staticAssessor{err: errors.New("analyzer unavailable")},
		Now:      fixedNow,
	}))

	result := gate.Execute(action.NewCtx(context.Background()), nil)

	require.True(t, result.IsError())
	require.False(t, base.executed)
	require.ErrorContains(t, result.Err(), "analyzer unavailable")
	require.Equal(t, EventErrored, result.Events[0].(Event).Type)
}

func TestInMemoryAuditStoreClonesEvents(t *testing.T) {
	store := NewInMemoryAuditStore()
	event := Event{
		Type:   EventAssessed,
		Intent: action.Intent{Operations: []action.IntentOperation{{Operation: "write"}}, Behaviors: []string{"filesystem_write"}},
		Assessment: Assessment{
			Decision:   Decision{Reasons: []string{"outside_workspace"}},
			Dimensions: []Dimension{{Name: "scope", Severity: 7}},
		},
	}
	require.NoError(t, store.AppendSafetyEvent(action.NewCtx(context.Background()), event))
	event.Intent.Operations[0].Operation = "mutated"
	event.Assessment.Dimensions[0].Name = "mutated"

	events, err := store.ListSafetyEvents(action.NewCtx(context.Background()))
	require.NoError(t, err)
	require.Equal(t, "write", events[0].Intent.Operations[0].Operation)
	require.Equal(t, "scope", events[0].Assessment.Dimensions[0].Name)
	events[0].Assessment.Dimensions[0].Name = "changed"

	events, err = store.ListSafetyEvents(action.NewCtx(context.Background()))
	require.NoError(t, err)
	require.Equal(t, "scope", events[0].Assessment.Dimensions[0].Name)
}

func fixedNow() time.Time { return time.Date(2026, 5, 4, 8, 0, 0, 0, time.UTC) }
