package toolmw

import "github.com/codewandler/agentsdk/safety"

// SafetyAssessment converts the legacy tool middleware assessment shape into the
// surface-neutral safety assessment shape. Existing tool middleware remains the
// compatibility layer; new action/workflow policy code should use safety.*.
func SafetyAssessment(assessment Assessment) safety.Assessment {
	dims := make([]safety.Dimension, 0, len(assessment.Dimensions))
	for _, dim := range assessment.Dimensions {
		dims = append(dims, safety.Dimension{Name: dim.Name, Severity: dim.Severity, Reason: dim.Reason})
	}
	return safety.Assessment{
		Decision: safety.Decision{
			Action:    safetyAction(assessment.Decision.Action),
			Reasons:   append([]string(nil), assessment.Decision.Reasons...),
			Rationale: assessment.Decision.Rationale,
		},
		Dimensions:  dims,
		Confidence:  assessment.Confidence,
		Explanation: assessment.Explanation,
	}
}

// ToolAssessment converts a surface-neutral safety assessment into the legacy
// tool middleware shape used by RiskGate.
func ToolAssessment(assessment safety.Assessment) Assessment {
	dims := make([]Dimension, 0, len(assessment.Dimensions))
	for _, dim := range assessment.Dimensions {
		dims = append(dims, Dimension{Name: dim.Name, Severity: dim.Severity, Reason: dim.Reason})
	}
	return Assessment{
		Decision: Decision{
			Action:    toolAction(assessment.Decision.Action),
			Reasons:   append([]string(nil), assessment.Decision.Reasons...),
			Rationale: assessment.Decision.Rationale,
		},
		Dimensions:  dims,
		Confidence:  assessment.Confidence,
		Explanation: assessment.Explanation,
	}
}

func safetyAction(action Action) safety.DecisionAction {
	switch action {
	case ActionAllow:
		return safety.DecisionAllow
	case ActionRequiresApproval:
		return safety.DecisionRequiresApproval
	case ActionReject:
		return safety.DecisionReject
	case ActionError:
		return safety.DecisionError
	default:
		return safety.DecisionAllow
	}
}

func toolAction(action safety.DecisionAction) Action {
	switch action {
	case safety.DecisionAllow:
		return ActionAllow
	case safety.DecisionRequiresApproval:
		return ActionRequiresApproval
	case safety.DecisionReject:
		return ActionReject
	case safety.DecisionError:
		return ActionError
	default:
		return ActionAllow
	}
}
