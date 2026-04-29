package toolmw

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/codewandler/agentsdk/tool"
	"github.com/invopop/jsonschema"
	"github.com/stretchr/testify/require"
)

// ── Test helpers ──────────────────────────────────────────────────────────────

type riskTestTool struct {
	name   string
	intent tool.Intent
}

func (t *riskTestTool) Name() string                { return t.name }
func (t *riskTestTool) Description() string         { return "test" }
func (t *riskTestTool) Guidance() string             { return "" }
func (t *riskTestTool) Schema() *jsonschema.Schema   { return nil }
func (t *riskTestTool) Execute(_ tool.Ctx, _ json.RawMessage) (tool.Result, error) { return tool.Text("ok"), nil }
func (t *riskTestTool) DeclareIntent(_ tool.Ctx, _ json.RawMessage) (tool.Intent, error) {
	return t.intent, nil
}

type riskTestCtx struct{ context.Context }

func (c riskTestCtx) WorkDir() string       { return "/tmp/project" }
func (c riskTestCtx) AgentID() string       { return "test" }
func (c riskTestCtx) SessionID() string     { return "sess" }
func (c riskTestCtx) Extra() map[string]any { return nil }

func riskCtx() tool.Ctx { return riskTestCtx{Context: context.Background()} }

func riskCtxWithApprover(approve bool) tool.Ctx {
	ctx := tool.CtxWithApprover(context.Background(), func(_ tool.Ctx, _ tool.Intent, _ any) (bool, error) {
		return approve, nil
	})
	return riskTestCtx{Context: ctx}
}

// staticAssessor always returns the same assessment.
type staticAssessor struct {
	assessment Assessment
	err        error
}

func (a *staticAssessor) Assess(_ tool.Ctx, _ tool.Intent) (Assessment, error) {
	return a.assessment, a.err
}

// ── RiskGate: allow ──────────────────────────────────────────────────────────

func TestRiskGate_Allow(t *testing.T) {
	base := &riskTestTool{
		name:   "file_read",
		intent: tool.Intent{Tool: "file_read", ToolClass: "filesystem_read", Confidence: "high"},
	}

	gate := tool.Apply(base, NewRiskGate(&staticAssessor{
		assessment: Assessment{Decision: Decision{Action: ActionAllow}},
	}))

	res, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ok", res.String())
}

// ── RiskGate: reject ─────────────────────────────────────────────────────────

func TestRiskGate_Reject(t *testing.T) {
	base := &riskTestTool{
		name:   "file_delete",
		intent: tool.Intent{Tool: "file_delete", ToolClass: "filesystem_delete", Confidence: "high"},
	}

	gate := tool.Apply(base, NewRiskGate(&staticAssessor{
		assessment: Assessment{Decision: Decision{Action: ActionReject, Rationale: "deleting system file"}},
	}))

	res, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err) // result, not error
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "rejected")
	require.Contains(t, res.String(), "deleting system file")
}

// ── RiskGate: error action ────────────────────────────────────────────────────

func TestRiskGate_ErrorAction(t *testing.T) {
	base := &riskTestTool{
		name:   "bash",
		intent: tool.Intent{Tool: "bash", ToolClass: "command_execution", Confidence: "low"},
	}

	gate := tool.Apply(base, NewRiskGate(&staticAssessor{
		assessment: Assessment{Decision: Decision{Action: ActionError, Rationale: "analysis pipeline failed"}},
	}))

	res, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err) // result, not error
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "assessment error")
	require.Contains(t, res.String(), "analysis pipeline failed")
}

// ── RiskGate: requires approval — approved ───────────────────────────────────

func TestRiskGate_RequiresApproval_Approved(t *testing.T) {
	base := &riskTestTool{
		name:   "file_write",
		intent: tool.Intent{Tool: "file_write", ToolClass: "filesystem_write", Confidence: "high"},
	}

	gate := tool.Apply(base, NewRiskGate(&staticAssessor{
		assessment: Assessment{Decision: Decision{Action: ActionRequiresApproval, Rationale: "writing outside workspace"}},
	}))

	res, err := gate.Execute(riskCtxWithApprover(true), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ok", res.String())
}

// ── RiskGate: requires approval — denied ─────────────────────────────────────

func TestRiskGate_RequiresApproval_Denied(t *testing.T) {
	base := &riskTestTool{
		name:   "file_write",
		intent: tool.Intent{Tool: "file_write", ToolClass: "filesystem_write", Confidence: "high"},
	}

	gate := tool.Apply(base, NewRiskGate(&staticAssessor{
		assessment: Assessment{Decision: Decision{Action: ActionRequiresApproval, Rationale: "writing outside workspace"}},
	}))

	res, err := gate.Execute(riskCtxWithApprover(false), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "denied by user")
}

// ── RiskGate: requires approval — no approver ────────────────────────────────

func TestRiskGate_RequiresApproval_NoApprover(t *testing.T) {
	base := &riskTestTool{
		name:   "file_write",
		intent: tool.Intent{Tool: "file_write", ToolClass: "filesystem_write", Confidence: "high"},
	}

	gate := tool.Apply(base, NewRiskGate(&staticAssessor{
		assessment: Assessment{Decision: Decision{Action: ActionRequiresApproval, Rationale: "no approver"}},
	}))

	// No approver in context → deny.
	res, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "no approver configured")
}

// ── RiskGate: assessment error — fail closed ─────────────────────────────────

func TestRiskGate_AssessmentError_FailClosed(t *testing.T) {
	base := &riskTestTool{
		name:   "test",
		intent: tool.Intent{Tool: "test", Confidence: "high"},
	}

	gate := tool.Apply(base, NewRiskGate(&staticAssessor{
		err: errors.New("assessor broke"),
	}))

	res, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "assessment error")
}

// ── RiskGate: assessment error — fail open ───────────────────────────────────

func TestRiskGate_AssessmentError_FailOpen(t *testing.T) {
	base := &riskTestTool{
		name:   "test",
		intent: tool.Intent{Tool: "test", Confidence: "high"},
	}

	rg := &RiskGate{
		Assessor: &staticAssessor{err: errors.New("assessor broke")},
		FailOpen: true,
	}
	gate := tool.Apply(base, tool.HooksMiddleware(rg))

	res, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ok", res.String()) // passed through
}

// ── RiskGate: approver on middleware takes precedence ─────────────────────────

func TestRiskGate_MiddlewareApproverTakesPrecedence(t *testing.T) {
	base := &riskTestTool{
		name:   "test",
		intent: tool.Intent{Tool: "test", Confidence: "high"},
	}

	middlewareApproverCalled := false
	rg := &RiskGate{
		Assessor: &staticAssessor{
			assessment: Assessment{Decision: Decision{Action: ActionRequiresApproval}},
		},
		Approver: func(_ tool.Ctx, _ tool.Intent, _ any) (bool, error) {
			middlewareApproverCalled = true
			return true, nil
		},
	}
	gate := tool.Apply(base, tool.HooksMiddleware(rg))

	// Context has a different approver that would deny.
	res, err := gate.Execute(riskCtxWithApprover(false), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ok", res.String())
	require.True(t, middlewareApproverCalled)
}


// ── RiskGate: approver receives assessment as detail ──────────────────────────

func TestRiskGate_ApproverReceivesAssessment(t *testing.T) {
	expectedAssessment := Assessment{
		Decision:   Decision{Action: ActionRequiresApproval, Rationale: "test"},
		Dimensions: []Dimension{{Name: "write:file", Severity: 5}},
		Confidence: "high",
	}

	base := &riskTestTool{
		name:   "test",
		intent: tool.Intent{Tool: "test", ToolClass: "filesystem_write", Confidence: "high"},
	}

	var receivedDetail any
	rg := &RiskGate{
		Assessor: &staticAssessor{assessment: expectedAssessment},
		Approver: func(_ tool.Ctx, _ tool.Intent, detail any) (bool, error) {
			receivedDetail = detail
			return true, nil
		},
	}
	gate := tool.Apply(base, tool.HooksMiddleware(rg))

	_, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)

	// The approver should receive the Assessment as the detail parameter.
	assessment, ok := receivedDetail.(Assessment)
	require.True(t, ok, "detail should be Assessment, got %T", receivedDetail)
	require.Equal(t, expectedAssessment.Decision.Action, assessment.Decision.Action)
	require.Len(t, assessment.Dimensions, 1)
	require.Equal(t, "write:file", assessment.Dimensions[0].Name)
}

// ── Integration: RiskGate + PolicyAssessor + real DeclareIntent ─────────────

func TestRiskGate_Integration_PolicyAssessor(t *testing.T) {
	// A tool that declares a write to a system file.
	base := &riskTestTool{
		name: "file_write",
		intent: tool.Intent{
			Tool:      "file_write",
			ToolClass: "filesystem_write",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "file", Value: "/etc/crontab", Locality: "system"},
				Operation: "write",
				Certain:   true,
			}},
			Confidence: "high",
		},
	}

	// Wire RiskGate with real PolicyAssessor.
	gate := tool.Apply(base, NewRiskGate(NewPolicyAssessor()))

	// write(4) + system(3) = 7 → requires_approval.
	// No approver → denied.
	res, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "no approver configured")

	// With approver that approves → passes through.
	res, err = gate.Execute(riskCtxWithApprover(true), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ok", res.String())
}

func TestRiskGate_Integration_WorkspaceRead_AutoAllow(t *testing.T) {
	// A tool that declares a read from workspace — should auto-allow.
	base := &riskTestTool{
		name: "file_read",
		intent: tool.Intent{
			Tool:      "file_read",
			ToolClass: "filesystem_read",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "file", Value: "src/main.go", Locality: "workspace"},
				Operation: "read",
				Certain:   true,
			}},
			Confidence: "high",
		},
	}

	gate := tool.Apply(base, NewRiskGate(NewPolicyAssessor()))

	// read(1) + workspace(0) = 1 → allow. No approver needed.
	res, err := gate.Execute(riskCtx(), json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Equal(t, "ok", res.String())
}
