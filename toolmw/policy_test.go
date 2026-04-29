package toolmw

import (
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

func TestPolicyAssessor_OpaqueIntent(t *testing.T) {
	a := NewPolicyAssessor()
	assessment, err := a.Assess(riskCtx(), tool.Intent{
		Tool:       "mystery",
		Opaque:     true,
		Confidence: "low",
	})
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Contains(t, assessment.Decision.Reasons, "opaque_intent")
}

func TestPolicyAssessor_NoOperations(t *testing.T) {
	a := NewPolicyAssessor()
	assessment, err := a.Assess(riskCtx(), tool.Intent{
		Tool:       "skill",
		ToolClass:  "agent_control",
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionAllow, assessment.Decision.Action)
}

func TestPolicyAssessor_ReadWorkspace_Allow(t *testing.T) {
	a := NewPolicyAssessor()
	assessment, err := a.Assess(riskCtx(), tool.Intent{
		Tool:      "file_read",
		ToolClass: "filesystem_read",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/tmp/project/main.go", Locality: "workspace"},
			Operation: "read",
			Certain:   true,
		}},
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionAllow, assessment.Decision.Action)
	require.Len(t, assessment.Dimensions, 1)
	require.Equal(t, 1, assessment.Dimensions[0].Severity) // read(1) + workspace(0) = 1
}

func TestPolicyAssessor_WriteSystem_RequiresApproval(t *testing.T) {
	a := NewPolicyAssessor()
	assessment, err := a.Assess(riskCtx(), tool.Intent{
		Tool:      "file_write",
		ToolClass: "filesystem_write",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/etc/crontab", Locality: "system"},
			Operation: "write",
			Certain:   true,
		}},
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Equal(t, 7, assessment.Dimensions[0].Severity) // write(4) + system(3) = 7
}

func TestPolicyAssessor_DeleteSecret_Reject(t *testing.T) {
	a := NewPolicyAssessor()
	assessment, err := a.Assess(riskCtx(), tool.Intent{
		Tool:      "file_delete",
		ToolClass: "filesystem_delete",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/home/user/.ssh/id_rsa", Locality: "secret"},
			Operation: "delete",
			Certain:   true,
		}},
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionReject, assessment.Decision.Action)
	require.Equal(t, 10, assessment.Dimensions[0].Severity) // delete(6) + secret(5) = 11, capped to 10
}

func TestPolicyAssessor_NetworkRead_Allow(t *testing.T) {
	a := NewPolicyAssessor()
	assessment, err := a.Assess(riskCtx(), tool.Intent{
		Tool:      "web_fetch",
		ToolClass: "network_access",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "url", Value: "https://example.com", Locality: "network"},
			Operation: "network_read",
			Certain:   true,
		}},
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionAllow, assessment.Decision.Action)
	require.Equal(t, 4, assessment.Dimensions[0].Severity) // network_read(2) + network(2) = 4
}

func TestPolicyAssessor_NetworkWrite_RequiresApproval(t *testing.T) {
	a := NewPolicyAssessor()
	assessment, err := a.Assess(riskCtx(), tool.Intent{
		Tool:      "web_fetch",
		ToolClass: "network_access",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "url", Value: "https://api.prod/deploy", Locality: "network"},
			Operation: "network_write",
			Certain:   true,
		}},
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Equal(t, 7, assessment.Dimensions[0].Severity) // network_write(5) + network(2) = 7
}

func TestPolicyAssessor_MultipleOps_MaxSeverityWins(t *testing.T) {
	a := NewPolicyAssessor()
	assessment, err := a.Assess(riskCtx(), tool.Intent{
		Tool:      "file_edit",
		ToolClass: "filesystem_write",
		Operations: []tool.IntentOperation{
			{
				Resource:  tool.IntentResource{Category: "file", Value: "src/main.go", Locality: "workspace"},
				Operation: "write",
				Certain:   true,
			},
			{
				Resource:  tool.IntentResource{Category: "file", Value: "/etc/passwd", Locality: "system"},
				Operation: "write",
				Certain:   true,
			},
		},
		Confidence: "high",
	})
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Len(t, assessment.Dimensions, 2)
	// First op: write(4) + workspace(0) = 4
	require.Equal(t, 4, assessment.Dimensions[0].Severity)
	// Second op: write(4) + system(3) = 7 → drives the decision
	require.Equal(t, 7, assessment.Dimensions[1].Severity)
}

func TestScoreSeverity(t *testing.T) {
	tests := []struct {
		op       string
		locality string
		want     int
	}{
		{"read", "workspace", 1},
		{"read", "secret", 6},
		{"write", "workspace", 4},
		{"write", "system", 7},
		{"delete", "workspace", 6},
		{"delete", "secret", 10}, // 6+5=11, capped
		{"execute", "workspace", 5},
		{"network_read", "network", 4},
		{"network_write", "network", 7},
		{"device_write", "system", 10}, // 8+3=11, capped
		{"unknown_op", "workspace", 0},
		{"read", "unknown_locality", 1},
	}
	for _, tt := range tests {
		t.Run(tt.op+"/"+tt.locality, func(t *testing.T) {
			got := scoreSeverity(tt.op, tt.locality)
			require.Equal(t, tt.want, got)
		})
	}
}
