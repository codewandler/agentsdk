package toolmw

import (
	"fmt"

	"github.com/codewandler/agentsdk/tool"
)

// PolicyAssessor scores risk based on operation×locality for structured tools.
// It uses a simple additive model: each operation's base weight plus the
// resource locality weight determines severity. The maximum severity across
// all operations drives the decision.
type PolicyAssessor struct{}

// NewPolicyAssessor creates a PolicyAssessor.
func NewPolicyAssessor() *PolicyAssessor {
	return &PolicyAssessor{}
}

func (a *PolicyAssessor) Assess(_ tool.Ctx, intent tool.Intent) (Assessment, error) {
	if intent.Opaque {
		return Assessment{
			Decision:   Decision{Action: ActionRequiresApproval, Reasons: []string{"opaque_intent"}, Rationale: "tool intent could not be determined"},
			Confidence: "low",
		}, nil
	}

	if len(intent.Operations) == 0 {
		return Assessment{
			Decision:   Decision{Action: ActionAllow},
			Confidence: intent.Confidence,
		}, nil
	}

	var (
		maxSeverity int
		reasons     []string
		dims        []Dimension
	)

	for _, op := range intent.Operations {
		severity := scoreSeverity(op.Operation, op.Resource.Locality)
		dim := Dimension{
			Name:     op.Operation + ":" + op.Resource.Category,
			Severity: severity,
			Reason:   fmt.Sprintf("%s %s (%s)", op.Operation, op.Resource.Value, op.Resource.Locality),
		}
		dims = append(dims, dim)
		if severity > maxSeverity {
			maxSeverity = severity
		}
		if severity >= 7 {
			reasons = append(reasons, dim.Reason)
		}
	}

	action := ActionAllow
	if maxSeverity >= 8 {
		action = ActionReject
	} else if maxSeverity >= 5 {
		action = ActionRequiresApproval
	}

	return Assessment{
		Decision:   Decision{Action: action, Reasons: reasons, Rationale: summarizeDimensions(dims)},
		Dimensions: dims,
		Confidence: intent.Confidence,
	}, nil
}

// scoreSeverity returns 0-10 based on operation × locality.
//
// Operation weights reflect inherent risk:
//
//	read=1, network_read=2, write=4, network_write=5,
//	delete=6, execute=5, persistence_modify=7, device_write=8
//
// Locality weights reflect resource sensitivity:
//
//	workspace=0, unknown=1, network=2, system=3, sensitive=4, secret=5
// Package-level weight tables — allocated once, read-only.
var opWeight = map[string]int{
	"read":               1,
	"network_read":       2,
	"write":              4,
	"network_write":      5,
	"delete":             6,
	"execute":            5,
	"persistence_modify": 7,
	"device_write":       8,
}

var localityWeight = map[string]int{
	"workspace": 0,
	"unknown":   1,
	"network":   2,
	"system":    3,
	"sensitive": 4,
	"secret":    5,
}

func scoreSeverity(operation, locality string) int {
	score := opWeight[operation] + localityWeight[locality]
	if score > 10 {
		score = 10
	}
	return score
}

// Compile-time check.
var _ IntentAssessor = (*PolicyAssessor)(nil)
