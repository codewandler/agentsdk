package toolmw

import (
	"testing"

	"github.com/codewandler/agentsdk/safety"
	"github.com/stretchr/testify/require"
)

func TestSafetyAssessmentRoundTrip(t *testing.T) {
	assessment := Assessment{
		Decision:    Decision{Action: ActionRequiresApproval, Reasons: []string{"outside_workspace"}, Rationale: "writes outside workspace"},
		Dimensions:  []Dimension{{Name: "write:file", Severity: 7, Reason: "system path"}},
		Confidence:  "high",
		Explanation: "summary",
	}

	safetyAssessment := SafetyAssessment(assessment)
	require.Equal(t, safety.DecisionRequiresApproval, safetyAssessment.Decision.Action)
	require.Equal(t, []string{"outside_workspace"}, safetyAssessment.Decision.Reasons)
	require.Equal(t, "write:file", safetyAssessment.Dimensions[0].Name)

	roundTrip := ToolAssessment(safetyAssessment)
	require.Equal(t, assessment, roundTrip)

	// Conversion must defensively clone mutable slices.
	safetyAssessment.Decision.Reasons[0] = "mutated"
	safetyAssessment.Dimensions[0].Name = "mutated"
	require.Equal(t, []string{"outside_workspace"}, roundTrip.Decision.Reasons)
	require.Equal(t, "write:file", roundTrip.Dimensions[0].Name)
}
