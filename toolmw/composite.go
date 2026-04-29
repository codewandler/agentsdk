package toolmw

import (
	"github.com/codewandler/agentsdk/tool"
)

// CompositeAssessor routes to the appropriate assessor based on intent.
// It checks assessors in order and uses the first one that accepts the intent.
// If no assessor accepts, it falls back to the Default assessor.
type CompositeAssessor struct {
	// Assessors are checked in order. Each can optionally implement
	// IntentAcceptor to indicate whether it handles a given intent.
	Assessors []IntentAssessor

	// Default is used when no assessor in the list accepts the intent.
	// If nil, returns an allow decision.
	Default IntentAssessor
}

// IntentAcceptor is an optional interface an IntentAssessor can implement
// to indicate whether it handles a given intent. Assessors that don't
// implement this are assumed to accept all intents.
type IntentAcceptor interface {
	AcceptsIntent(intent tool.Intent) bool
}

func (a *CompositeAssessor) Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error) {
	for _, assessor := range a.Assessors {
		if acceptor, ok := assessor.(IntentAcceptor); ok {
			if !acceptor.AcceptsIntent(intent) {
				continue
			}
		}
		return assessor.Assess(ctx, intent)
	}
	if a.Default != nil {
		return a.Default.Assess(ctx, intent)
	}
	return Assessment{Decision: Decision{Action: ActionAllow}}, nil
}

// Compile-time check.
var _ IntentAssessor = (*CompositeAssessor)(nil)
