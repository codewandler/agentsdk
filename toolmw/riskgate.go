package toolmw

import (
	"encoding/json"
	"fmt"

	"github.com/codewandler/agentsdk/tool"
)

// IntentAssessor evaluates an Intent and returns a risk assessment.
type IntentAssessor interface {
	Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error)
}

// Assessment is the result of risk evaluation.
type Assessment struct {
	Decision    Decision    `json:"decision"`
	Dimensions  []Dimension `json:"dimensions,omitempty"`
	Confidence  string      `json:"confidence"`
	Explanation string      `json:"explanation,omitempty"`
}

// Decision is the gate outcome.
type Decision struct {
	Action    Action   `json:"action"`
	Reasons   []string `json:"reasons,omitempty"`
	Rationale string   `json:"rationale,omitempty"`
}

// Action is the risk gate decision.
type Action string

const (
	ActionAllow            Action = "allow"
	ActionRequiresApproval Action = "requires_approval"
	ActionReject           Action = "reject"
	ActionError            Action = "error"
)

// Dimension is a single scored risk dimension.
type Dimension struct {
	Name     string `json:"name"`
	Severity int    `json:"severity"` // 0-10
	Reason   string `json:"reason,omitempty"`
}

// RiskGate is a middleware that extracts intent, assesses risk, and gates
// on approval before allowing tool execution.
//
// Flow:
//  1. ExtractIntent from the inner tool
//  2. Assessor.Assess(intent) → Assessment
//  3. Gate on Assessment.Decision.Action:
//     - allow → pass through
//     - requires_approval → call Approver
//     - reject → return denial result
type RiskGate struct {
	tool.HooksBase

	// Assessor evaluates the intent and returns a risk decision.
	Assessor IntentAssessor

	// Approver is optional. If nil, the RiskGate looks for an Approver
	// in the tool.Ctx via tool.ApproverFrom(ctx). This allows the
	// app/runtime layer to inject the approver once, rather than
	// wiring it into every middleware instance.
	Approver tool.Approver

	// FailOpen controls behavior when assessment fails. If true,
	// assessment errors allow the tool call through. If false (default),
	// assessment errors deny the call. Default: fail closed.
	FailOpen bool
}

// NewRiskGate creates a Middleware from a RiskGate.
func NewRiskGate(assessor IntentAssessor) tool.Middleware {
	return tool.HooksMiddleware(&RiskGate{Assessor: assessor})
}

func (m *RiskGate) OnInput(ctx tool.Ctx, inner tool.Tool, input json.RawMessage, state tool.CallState) (json.RawMessage, tool.Result, error) {
	if m.Assessor == nil {
		return input, nil, nil // no assessor configured — pass through
	}

	// 1. Extract intent (uses IntentProvider if available).
	intent := tool.ExtractIntent(inner, ctx, input)
	state["intent"] = intent

	// 2. Assess risk.
	assessment, err := m.Assessor.Assess(ctx, intent)
	if err != nil {
		if m.FailOpen {
			return input, nil, nil
		}
		return nil, tool.Errorf("[risk gate] assessment error: %v", err), nil
	}
	state["assessment"] = assessment

	// 3. Gate on decision.
	switch assessment.Decision.Action {
	case ActionAllow:
		return input, nil, nil

	case ActionRequiresApproval:
		approver := m.resolveApprover(ctx)
		if approver == nil {
			return nil, tool.Errorf("[risk gate] approval required but no approver configured: %s",
				assessment.Decision.Rationale), nil
		}
		approved, err := approver(ctx, intent, assessment)
		if err != nil {
			return nil, tool.Errorf("[risk gate] approval error: %v", err), nil
		}
		if !approved {
			return nil, tool.Errorf("[risk gate] denied by user: %s",
				assessment.Decision.Rationale), nil
		}
		return input, nil, nil

	case ActionReject:
		return nil, tool.Errorf("[risk gate] rejected: %s",
			assessment.Decision.Rationale), nil

	case ActionError:
		return nil, tool.Errorf("[risk gate] assessment error: %s",
			assessment.Decision.Rationale), nil

	default:
		return input, nil, nil
	}
}

// OnResult can log the outcome for audit.
func (m *RiskGate) OnResult(ctx tool.Ctx, inner tool.Tool, input json.RawMessage, res tool.Result, err error, state tool.CallState) (tool.Result, error) {
	// Future: audit logging of intent + assessment + outcome.
	return res, err
}

func (m *RiskGate) resolveApprover(ctx tool.Ctx) tool.Approver {
	if m.Approver != nil {
		return m.Approver
	}
	return tool.ApproverFrom(ctx)
}

// summarizeDimensions returns a human-readable summary of risk dimensions.
func summarizeDimensions(dims []Dimension) string {
	if len(dims) == 0 {
		return "no risk dimensions"
	}
	var maxSev int
	var reasons []string
	for _, d := range dims {
		if d.Severity > maxSev {
			maxSev = d.Severity
		}
		if d.Severity >= 5 && d.Reason != "" {
			reasons = append(reasons, d.Reason)
		}
	}
	if len(reasons) == 0 {
		return fmt.Sprintf("max severity %d/10", maxSev)
	}
	return fmt.Sprintf("max severity %d/10: %s", maxSev, reasons[0])
}
