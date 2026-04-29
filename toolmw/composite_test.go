package toolmw

import (
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

// selectiveAssessor only handles intents with a specific ToolClass.
type selectiveAssessor struct {
	toolClass  string
	assessment Assessment
}

func (a *selectiveAssessor) AcceptsIntent(intent tool.Intent) bool {
	return intent.ToolClass == a.toolClass
}

func (a *selectiveAssessor) Assess(_ tool.Ctx, _ tool.Intent) (Assessment, error) {
	return a.assessment, nil
}

func TestCompositeAssessor_RoutesToFirst(t *testing.T) {
	comp := &CompositeAssessor{
		Assessors: []IntentAssessor{
			&selectiveAssessor{
				toolClass:  "filesystem_write",
				assessment: Assessment{Decision: Decision{Action: ActionRequiresApproval, Rationale: "write detected"}},
			},
			NewPolicyAssessor(),
		},
	}

	assessment, err := comp.Assess(riskCtx(), tool.Intent{
		Tool:      "file_write",
		ToolClass: "filesystem_write",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/tmp/x", Locality: "workspace"},
			Operation: "write",
			Certain:   true,
		}},
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Equal(t, "write detected", assessment.Decision.Rationale)
}

func TestCompositeAssessor_SkipsNonAccepting(t *testing.T) {
	comp := &CompositeAssessor{
		Assessors: []IntentAssessor{
			&selectiveAssessor{
				toolClass:  "filesystem_write",
				assessment: Assessment{Decision: Decision{Action: ActionReject}},
			},
		},
		Default: NewPolicyAssessor(),
	}

	// filesystem_read doesn't match the selective assessor → falls through to default.
	assessment, err := comp.Assess(riskCtx(), tool.Intent{
		Tool:      "file_read",
		ToolClass: "filesystem_read",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/tmp/x", Locality: "workspace"},
			Operation: "read",
			Certain:   true,
		}},
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionAllow, assessment.Decision.Action) // PolicyAssessor: read+workspace = 1 → allow
}

func TestCompositeAssessor_NoAssessors_NoDefault(t *testing.T) {
	comp := &CompositeAssessor{}

	assessment, err := comp.Assess(riskCtx(), tool.Intent{Tool: "test"})
	require.NoError(t, err)
	require.Equal(t, ActionAllow, assessment.Decision.Action)
}

func TestCompositeAssessor_AssessorWithoutAcceptor(t *testing.T) {
	// An assessor that doesn't implement IntentAcceptor accepts everything.
	comp := &CompositeAssessor{
		Assessors: []IntentAssessor{
			&staticAssessor{
				assessment: Assessment{Decision: Decision{Action: ActionRequiresApproval, Rationale: "catch-all"}},
			},
		},
	}

	assessment, err := comp.Assess(riskCtx(), tool.Intent{Tool: "anything"})
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
}
