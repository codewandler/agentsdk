package shell

import (
	"context"

	"github.com/codewandler/agentsdk/action"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
	"github.com/stretchr/testify/require"
)

// ── test helpers ──────────────────────────────────────────────────────────────

type shellTestCtx struct {
	action.BaseCtx
	workDir string
	extras  map[string]any
}

func (c shellTestCtx) WorkDir() string       { return c.workDir }
func (c shellTestCtx) AgentID() string       { return "test-agent" }
func (c shellTestCtx) SessionID() string     { return "sess" }
func (c shellTestCtx) Extra() map[string]any { return c.extras }

func shellCtx() tool.Ctx {
	return shellTestCtx{BaseCtx: action.BaseCtx{Context: context.Background()}, workDir: "/tmp/project", extras: map[string]any{}}
}

func shellCtxWithExtras(extras map[string]any) tool.Ctx {
	return shellTestCtx{BaseCtx: action.BaseCtx{Context: context.Background()}, workDir: "/tmp/project", extras: extras}
}

// declareIntent extracts the intent from a tool built with the given options.
func declareIntent(t *testing.T, ctx tool.Ctx, input string, opts ...Option) tool.Intent {
	t.Helper()
	tools := Tools(opts...)
	require.Len(t, tools, 1)
	inner := tool.Innermost(tools[0])
	provider, ok := inner.(tool.IntentProvider)
	require.True(t, ok, "bash tool should implement IntentProvider")
	intent, err := provider.DeclareIntent(ctx, json.RawMessage(input))
	require.NoError(t, err)
	return intent
}

// ── Summary field ─────────────────────────────────────────────────────────────

func TestBashIntent_SummaryPopulated(t *testing.T) {
	intent := declareIntent(t, shellCtx(), `{"cmd": "echo hello"}`)
	require.Equal(t, "echo hello", intent.Summary)
}

func TestBashIntent_SummaryMultiCommand(t *testing.T) {
	intent := declareIntent(t, shellCtx(), `{"cmd": ["echo a", "echo b"]}`)
	require.Equal(t, "echo a; echo b", intent.Summary)
}

// ── Opaque without analyzer ───────────────────────────────────────────────────

func TestBashIntent_OpaqueWithoutAnalyzer(t *testing.T) {
	intent := declareIntent(t, shellCtx(), `{"cmd": "ls -la"}`)
	require.True(t, intent.Opaque)
	require.Equal(t, "low", intent.Confidence)
	require.Equal(t, "command_execution", intent.ToolClass)
	require.Equal(t, "ls -la", intent.Summary)
}

func TestBashIntent_EmptyCommand(t *testing.T) {
	intent := declareIntent(t, shellCtx(), `{"cmd": []}`)
	require.True(t, intent.Opaque)
	require.Equal(t, "low", intent.Confidence)
	require.Empty(t, intent.Summary)
}

// ── With analyzer ─────────────────────────────────────────────────────────────

func TestBashIntent_WithAnalyzer_SingleCommand(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	intent := declareIntent(t, shellCtx(), `{"cmd": "ls -la"}`, WithRiskAnalyzer(analyzer))

	require.False(t, intent.Opaque)
	require.Equal(t, "command_execution", intent.ToolClass)
	require.Equal(t, "ls -la", intent.Summary)
	require.NotEmpty(t, intent.Confidence)

	// Extra should carry the assessment for CmdRiskAssessor reuse.
	_, ok := intent.Extra.(*cmdrisk.Assessment)
	require.True(t, ok, "Extra should be *cmdrisk.Assessment")
}

func TestBashIntent_WithAnalyzer_MultiCommand_AssessedIndividually(t *testing.T) {
	analyzer := cmdrisk.New(cmdrisk.Config{})
	intent := declareIntent(t, shellCtx(),
		`{"cmd": ["echo hello", "cat /etc/passwd"]}`,
		WithRiskAnalyzer(analyzer))

	require.False(t, intent.Opaque)
	require.Equal(t, "echo hello; cat /etc/passwd", intent.Summary)

	// Should have targets from both commands.
	assessment, ok := intent.Extra.(*cmdrisk.Assessment)
	require.True(t, ok)
	// The merged assessment should contain targets from both commands.
	require.NotEmpty(t, assessment.Targets)
}

// ── Context threading ─────────────────────────────────────────────────────────

func TestBuildRiskContext_Defaults(t *testing.T) {
	ctx := shellCtx()
	riskCtx := buildRiskContext(ctx, "/tmp/project")

	require.Equal(t, cmdrisk.EnvironmentDeveloperWorkstation, riskCtx.Environment)
	require.Equal(t, cmdrisk.CommandOriginMachineGenerated, riskCtx.Trust.CommandOrigin)
	require.True(t, riskCtx.Interactive)
	require.False(t, riskCtx.Sandboxed)
	require.Equal(t, []string{"/tmp/project"}, riskCtx.Asset.WorkspacePathPrefixes)
}

func TestBuildRiskContext_FromExtras(t *testing.T) {
	ctx := shellCtxWithExtras(map[string]any{
		"cmdrisk.environment":             "ci",
		"cmdrisk.command_origin":          "user_authored",
		"cmdrisk.interactive":             false,
		"cmdrisk.sandboxed":               true,
		"cmdrisk.sensitive_path_prefixes": []string{"/etc"},
		"cmdrisk.secret_path_prefixes":    []string{"/home/user/.ssh"},
		"cmdrisk.trusted_source_hosts":    []string{"github.com"},
		"cmdrisk.trusted_url_domains":     []string{"example.com"},
	})
	riskCtx := buildRiskContext(ctx, "/workspace")

	require.Equal(t, cmdrisk.Environment("ci"), riskCtx.Environment)
	require.Equal(t, cmdrisk.CommandOrigin("user_authored"), riskCtx.Trust.CommandOrigin)
	require.False(t, riskCtx.Interactive)
	require.True(t, riskCtx.Sandboxed)
	require.Equal(t, []string{"/etc"}, riskCtx.Asset.SensitivePathPrefixes)
	require.Equal(t, []string{"/home/user/.ssh"}, riskCtx.Asset.SecretPathPrefixes)
	require.Equal(t, []string{"github.com"}, riskCtx.Trust.TrustedSourceHosts)
	require.Equal(t, []string{"example.com"}, riskCtx.Trust.TrustedURLDomains)
}

func TestBuildRiskContext_UsesAgentIDAsUser(t *testing.T) {
	ctx := shellCtx()
	riskCtx := buildRiskContext(ctx, "/tmp/project")
	require.Equal(t, "test-agent", riskCtx.User)
}

// ── Merge logic ───────────────────────────────────────────────────────────────

func TestMergeAssessments_NilPrev(t *testing.T) {
	next := &cmdrisk.Assessment{Command: "echo hello", Confidence: cmdrisk.ConfidenceHigh}
	result := mergeAssessments(nil, next)
	require.Equal(t, next, result)
}

func TestMergeAssessments_TakesStricterDecision(t *testing.T) {
	prev := &cmdrisk.Assessment{
		Command:    "echo hello",
		Confidence: cmdrisk.ConfidenceHigh,
		Decision:   cmdrisk.Decision{Action: cmdrisk.ActionAllow},
	}
	next := &cmdrisk.Assessment{
		Command:    "rm -rf /",
		Confidence: cmdrisk.ConfidenceHigh,
		Decision:   cmdrisk.Decision{Action: cmdrisk.ActionReject, Rationale: "destructive"},
	}
	result := mergeAssessments(prev, next)
	require.Equal(t, cmdrisk.ActionReject, result.Decision.Action)
	require.Equal(t, "destructive", result.Decision.Rationale)
}

func TestMergeAssessments_TakesLowerConfidence(t *testing.T) {
	prev := &cmdrisk.Assessment{
		Command:    "echo hello",
		Confidence: cmdrisk.ConfidenceHigh,
		Decision:   cmdrisk.Decision{Action: cmdrisk.ActionAllow},
	}
	next := &cmdrisk.Assessment{
		Command:    "some-unknown-cmd",
		Confidence: cmdrisk.ConfidenceLow,
		Decision:   cmdrisk.Decision{Action: cmdrisk.ActionAllow},
	}
	result := mergeAssessments(prev, next)
	require.Equal(t, cmdrisk.ConfidenceLow, result.Confidence)
}

func TestMergeAssessments_MergesBehaviors(t *testing.T) {
	prev := &cmdrisk.Assessment{
		Command:    "echo hello",
		Behaviors:  []string{"data_only"},
		Decision:   cmdrisk.Decision{Action: cmdrisk.ActionAllow},
		Confidence: cmdrisk.ConfidenceHigh,
	}
	next := &cmdrisk.Assessment{
		Command:    "cat /etc/passwd",
		Behaviors:  []string{"filesystem_read", "data_only"},
		Decision:   cmdrisk.Decision{Action: cmdrisk.ActionAllow},
		Confidence: cmdrisk.ConfidenceHigh,
	}
	result := mergeAssessments(prev, next)
	require.Equal(t, []string{"data_only", "filesystem_read"}, result.Behaviors)
}

func TestMergeAssessments_MergesTargets(t *testing.T) {
	prev := &cmdrisk.Assessment{
		Command:    "echo hello",
		Targets:    []cmdrisk.Target{{Category: "file", Value: "stdout"}},
		Decision:   cmdrisk.Decision{Action: cmdrisk.ActionAllow},
		Confidence: cmdrisk.ConfidenceHigh,
	}
	next := &cmdrisk.Assessment{
		Command:    "cat /etc/passwd",
		Targets:    []cmdrisk.Target{{Category: "file", Value: "/etc/passwd"}},
		Decision:   cmdrisk.Decision{Action: cmdrisk.ActionAllow},
		Confidence: cmdrisk.ConfidenceHigh,
	}
	result := mergeAssessments(prev, next)
	require.Len(t, result.Targets, 2)
}

func TestMergeRiskDimensions_MaxSeverityPerName(t *testing.T) {
	a := []cmdrisk.RiskDimension{
		{Name: "scope", Severity: 3, Reason: "low"},
		{Name: "trust", Severity: 5, Reason: "moderate"},
	}
	b := []cmdrisk.RiskDimension{
		{Name: "scope", Severity: 7, Reason: "high"},
		{Name: "reversibility", Severity: 2, Reason: "easy"},
	}
	result := mergeRiskDimensions(a, b)
	require.Len(t, result, 3)

	byName := map[string]cmdrisk.RiskDimension{}
	for _, d := range result {
		byName[d.Name] = d
	}
	require.Equal(t, 7, byName["scope"].Severity)
	require.Equal(t, "high", byName["scope"].Reason)
	require.Equal(t, 5, byName["trust"].Severity)
	require.Equal(t, 2, byName["reversibility"].Severity)
}

func TestAppendUnique(t *testing.T) {
	result := appendUnique([]string{"a", "b"}, "b", "c", "a", "d")
	require.Equal(t, []string{"a", "b", "c", "d"}, result)
}
