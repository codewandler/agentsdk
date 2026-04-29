package toolmw

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
	"github.com/stretchr/testify/require"
)

func TestCmdRiskAssessor_ReusesPrecomputed(t *testing.T) {
	precomputed := &cmdrisk.Assessment{
		Command:    "rm -rf /",
		Confidence: cmdrisk.ConfidenceHigh,
		Decision: cmdrisk.Decision{
			Action:    cmdrisk.ActionReject,
			Reasons:   []string{"destructive"},
			Rationale: "deletes entire filesystem",
		},
		RiskDimensions: []cmdrisk.RiskDimension{
			{Name: "destructiveness", Severity: 10, Reason: "recursive delete from root"},
		},
		Behaviors: []string{"filesystem_delete"},
	}

	assessor := NewCmdRiskAssessor(nil) // analyzer not needed when Extra is set
	intent := tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		Extra:     precomputed,
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	require.Equal(t, ActionReject, assessment.Decision.Action)
	require.Equal(t, "deletes entire filesystem", assessment.Decision.Rationale)
	require.Len(t, assessment.Dimensions, 1)
	require.Equal(t, 10, assessment.Dimensions[0].Severity)
	require.Equal(t, "high", assessment.Confidence)
}

func TestCmdRiskAssessor_OpaqueWithoutAnalyzer(t *testing.T) {
	assessor := NewCmdRiskAssessor(nil)
	intent := tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		Opaque:    true,
		// No Extra, no Summary, no analyzer — opaque fallback.
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Contains(t, assessment.Decision.Reasons, "opaque_intent")
}

func TestCmdRiskAssessor_FallbackUsesAnalyzerWithSummary(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	assessor := NewCmdRiskAssessor(analyzer)

	// No Extra, but Summary carries the command string → analyzer runs.
	intent := tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		Summary:   "ls -la",
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	require.NotContains(t, assessment.Decision.Reasons, "opaque_intent")
	require.NotEmpty(t, assessment.Confidence)
}

func TestCmdRiskAssessor_FallbackNoAnalyzerWithSummary(t *testing.T) {
	// Has Summary but no analyzer → no_analyzer fallback.
	assessor := NewCmdRiskAssessor(nil)
	intent := tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		Summary:   "ls -la",
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Contains(t, assessment.Decision.Reasons, "no_analyzer")
}

func TestCmdRiskAssessor_MapsAllActions(t *testing.T) {
	tests := []struct {
		action cmdrisk.Action
		want   Action
	}{
		{cmdrisk.ActionAllow, ActionAllow},
		{cmdrisk.ActionRequiresApproval, ActionRequiresApproval},
		{cmdrisk.ActionReject, ActionReject},
		{cmdrisk.ActionError, ActionError},
	}

	assessor := NewCmdRiskAssessor(nil)
	for _, tt := range tests {
		t.Run(string(tt.action), func(t *testing.T) {
			assessment, err := assessor.Assess(riskCtx(), tool.Intent{
				Extra: &cmdrisk.Assessment{
					Decision: cmdrisk.Decision{Action: tt.action},
				},
			})
			require.NoError(t, err)
			require.Equal(t, tt.want, assessment.Decision.Action)
		})
	}
}

func TestCmdRiskAssessor_WithRealAnalyzer(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})

	// Use the real analyzer to assess a command.
	assessment, err := analyzer.Assess(t.Context(), cmdrisk.Request{
		Command: "ls -la",
		Context: cmdrisk.Context{
			Environment: cmdrisk.EnvironmentDeveloperWorkstation,
			Interactive: true,
			Trust: cmdrisk.TrustContext{
				CommandOrigin: cmdrisk.CommandOriginMachineGenerated,
			},
		},
	})
	require.NoError(t, err)

	// Map through our assessor — verify the bridge works end-to-end.
	assessor := NewCmdRiskAssessor(analyzer)
	result, err := assessor.Assess(riskCtx(), tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		Extra:     &assessment,
	})
	require.NoError(t, err)
	require.Contains(t, []Action{ActionAllow, ActionRequiresApproval, ActionReject, ActionError}, result.Decision.Action)
	require.NotEmpty(t, result.Confidence)
}

func TestCmdRiskAssessor_FallbackReadsContextExtras(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	assessor := NewCmdRiskAssessor(analyzer)

	ctx := riskCtxWithExtras(map[string]any{
		"cmdrisk.environment":    "ci",
		"cmdrisk.command_origin": "machine_generated",
		"cmdrisk.interactive":    false,
	})

	intent := tool.Intent{
		Tool:      "bash",
		ToolClass: "command_execution",
		Summary:   "echo hello",
	}

	assessment, err := assessor.Assess(ctx, intent)
	require.NoError(t, err)
	require.NotContains(t, assessment.Decision.Reasons, "opaque_intent")
}

// ── Structured intent (AssessIntent path) ─────────────────────────────────────

func TestCmdRiskAssessor_StructuredIntent_ReadWorkspace(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	assessor := NewCmdRiskAssessor(analyzer)

	intent := tool.Intent{
		Tool:      "file_read",
		ToolClass: "filesystem_read",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/tmp/project/main.go", Locality: "workspace"},
			Operation: "filesystem_read",
			Certain:   true,
		}},
		Confidence: "high",
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	require.Equal(t, ActionAllow, assessment.Decision.Action)
	require.NotEmpty(t, assessment.Dimensions, "should have risk dimensions from cmdrisk")
}

func TestCmdRiskAssessor_StructuredIntent_WriteSystemFile(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	assessor := NewCmdRiskAssessor(analyzer)

	intent := tool.Intent{
		Tool:      "file_write",
		ToolClass: "filesystem_write",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/etc/hosts", Locality: "system"},
			Operation: "filesystem_write",
			Certain:   true,
		}},
		Confidence: "high",
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	// Writing to /etc/hosts → requires approval (absolute path, outside workspace).
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.NotEmpty(t, assessment.Dimensions)
}

func TestCmdRiskAssessor_StructuredIntent_PrivilegedAmplifies(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	assessor := NewCmdRiskAssessor(analyzer)

	ctx := riskCtxWithExtras(map[string]any{
		"cmdrisk.is_privileged": true,
	})

	intent := tool.Intent{
		Tool:      "file_write",
		ToolClass: "filesystem_write",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/etc/hosts"},
			Operation: "filesystem_write",
			Certain:   true,
		}},
		Confidence: "high",
	}

	assessment, err := assessor.Assess(ctx, intent)
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	// Should mention privilege in reasons.
	require.Contains(t, assessment.Decision.Reasons, "context:is_privileged")
}

func TestCmdRiskAssessor_StructuredIntent_DeleteSecretPath(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	assessor := NewCmdRiskAssessor(analyzer)

	ctx := riskCtxWithExtras(map[string]any{
		"cmdrisk.secret_path_prefixes": []string{"/home/user/.ssh"},
	})

	intent := tool.Intent{
		Tool:      "file_delete",
		ToolClass: "filesystem_delete",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/home/user/.ssh/id_rsa"},
			Operation: "filesystem_delete",
			Certain:   true,
		}},
		Confidence: "high",
	}

	assessment, err := assessor.Assess(ctx, intent)
	require.NoError(t, err)
	require.NotEqual(t, ActionAllow, assessment.Decision.Action)
	// Should have elevated data_sensitivity dimension.
	dimByName := map[string]Dimension{}
	for _, d := range assessment.Dimensions {
		dimByName[d.Name] = d
	}
	require.Greater(t, dimByName["data_sensitivity"].Severity, 0)
}

func TestCmdRiskAssessor_StructuredIntent_NetworkFetch(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	assessor := NewCmdRiskAssessor(analyzer)

	intent := tool.Intent{
		Tool:      "web_fetch",
		ToolClass: "network_access",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "url", Value: "https://example.com/api"},
			Operation: "network_fetch",
			Certain:   true,
		}},
		Confidence: "high",
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	// Network fetch without trusted source → requires approval.
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
}

func TestCmdRiskAssessor_StructuredIntent_NoAnalyzer_FallsBack(t *testing.T) {
	// Without analyzer, structured intents get no_analyzer fallback.
	assessor := NewCmdRiskAssessor(nil)

	intent := tool.Intent{
		Tool:      "file_read",
		ToolClass: "filesystem_read",
		Operations: []tool.IntentOperation{{
			Resource:  tool.IntentResource{Category: "file", Value: "/tmp/project/main.go"},
			Operation: "filesystem_read",
			Certain:   true,
		}},
		Confidence: "high",
	}

	assessment, err := assessor.Assess(riskCtx(), intent)
	require.NoError(t, err)
	require.Equal(t, ActionRequiresApproval, assessment.Decision.Action)
	require.Contains(t, assessment.Decision.Reasons, "no_analyzer")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func riskCtxWithExtras(extras map[string]any) tool.Ctx {
	return riskTestCtxWithExtras{riskTestCtx{Context: context.Background()}, extras}
}

type riskTestCtxWithExtras struct {
	riskTestCtx
	extras map[string]any
}

func (c riskTestCtxWithExtras) Extra() map[string]any { return c.extras }
