package toolmw

import (
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
)

// CmdRiskAssessor is the unified risk assessor for all tools. It bridges
// cmdrisk's risk scoring, policy engine, and allowance system into the
// agentsdk middleware layer.
//
// Assessment strategy by intent type:
//  1. Pre-computed cmdrisk.Assessment in Intent.Extra → reuse directly (bash DeclareIntent path).
//  2. Structured intent with Operations → call AssessIntent for full multi-dimensional scoring.
//  3. Command string in Intent.Summary → call Assess to parse and analyze the shell command.
//  4. Opaque intent → conservative requires_approval fallback.
type CmdRiskAssessor struct {
	Analyzer *cmdrisk.Analyzer
}

// NewCmdRiskAssessor creates a CmdRiskAssessor with the given analyzer.
func NewCmdRiskAssessor(analyzer *cmdrisk.Analyzer) *CmdRiskAssessor {
	return &CmdRiskAssessor{Analyzer: analyzer}
}

func (a *CmdRiskAssessor) Assess(ctx tool.Ctx, intent tool.Intent) (Assessment, error) {
	// 1. Reuse pre-computed assessment if available (from bash DeclareIntent).
	if ca, ok := intent.Extra.(*cmdrisk.Assessment); ok {
		return mapCmdRiskAssessment(*ca), nil
	}

	if a.Analyzer == nil {
		return opaqueAssessment(intent), nil
	}

	// 2. Structured intent with operations → AssessIntent (full risk pipeline).
	if len(intent.Operations) > 0 {
		ops := make([]cmdrisk.IntentOperation, 0, len(intent.Operations))
		for _, op := range intent.Operations {
			ops = append(ops, cmdrisk.IntentOperation{
				Behavior: op.Operation,
				Target:   op.Resource.Value,
				Category: op.Resource.Category,
				Certain:  op.Certain,
			})
		}
		assessment, err := a.Analyzer.AssessIntent(ctx, cmdrisk.IntentRequest{
			Context:    buildCmdRiskContext(ctx),
			Operations: ops,
		})
		if err == nil {
			return mapCmdRiskAssessment(assessment), nil
		}
		// On error, fall through to opaque.
	}

	// 3. Command string in Summary → parse and analyze.
	if intent.Summary != "" {
		assessment, err := a.Analyzer.Assess(ctx, cmdrisk.Request{
			Command: intent.Summary,
			Context: buildCmdRiskContext(ctx),
		})
		if err == nil {
			return mapCmdRiskAssessment(assessment), nil
		}
	}

	// 4. Opaque fallback.
	return opaqueAssessment(intent), nil
}

func opaqueAssessment(intent tool.Intent) Assessment {
	if intent.Opaque || (len(intent.Operations) == 0 && intent.Summary == "") {
		return Assessment{
			Decision:   Decision{Action: ActionRequiresApproval, Reasons: []string{"opaque_intent"}, Rationale: "tool intent could not be determined"},
			Confidence: string(cmdrisk.ConfidenceLow),
		}
	}
	return Assessment{
		Decision:   Decision{Action: ActionRequiresApproval, Reasons: []string{"no_analyzer"}, Rationale: "no risk analyzer configured"},
		Confidence: intent.Confidence,
	}
}

func mapCmdRiskAssessment(ca cmdrisk.Assessment) Assessment {
	action := ActionAllow
	switch ca.Decision.Action {
	case cmdrisk.ActionRequiresApproval:
		action = ActionRequiresApproval
	case cmdrisk.ActionReject:
		action = ActionReject
	case cmdrisk.ActionError:
		action = ActionError
	}

	dims := make([]Dimension, 0, len(ca.RiskDimensions))
	for _, d := range ca.RiskDimensions {
		dims = append(dims, Dimension{
			Name:     d.Name,
			Severity: d.Severity,
			Reason:   d.Reason,
		})
	}

	return Assessment{
		Decision:    Decision{Action: action, Reasons: ca.Decision.Reasons, Rationale: ca.Decision.Rationale},
		Dimensions:  dims,
		Confidence:  string(ca.Confidence),
		Explanation: ca.Explanation.Summary,
	}
}

// buildCmdRiskContext constructs a cmdrisk.Context from the tool execution
// context. It reads environment and trust metadata from ctx.Extra() when
// available, falling back to safe defaults.
func buildCmdRiskContext(ctx tool.Ctx) cmdrisk.Context {
	extra := ctx.Extra()

	env := cmdrisk.EnvironmentDeveloperWorkstation
	if v, ok := extra["cmdrisk.environment"].(string); ok {
		env = cmdrisk.Environment(v)
	}

	origin := cmdrisk.CommandOriginMachineGenerated
	if v, ok := extra["cmdrisk.command_origin"].(string); ok {
		origin = cmdrisk.CommandOrigin(v)
	}

	interactive := true
	if v, ok := extra["cmdrisk.interactive"].(bool); ok {
		interactive = v
	}

	sandboxed := false
	if v, ok := extra["cmdrisk.sandboxed"].(bool); ok {
		sandboxed = v
	}

	privileged := false
	if v, ok := extra["cmdrisk.is_privileged"].(bool); ok {
		privileged = v
	}

	riskCtx := cmdrisk.Context{
		Environment:  env,
		User:         ctx.AgentID(),
		IsPrivileged: privileged,
		Interactive:  interactive,
		Sandboxed:    sandboxed,
		Asset: cmdrisk.AssetContext{
			WorkspacePathPrefixes: []string{ctx.WorkDir()},
		},
		Trust: cmdrisk.TrustContext{
			CommandOrigin: origin,
		},
	}

	if prefixes, ok := extra["cmdrisk.sensitive_path_prefixes"].([]string); ok {
		riskCtx.Asset.SensitivePathPrefixes = prefixes
	}
	if prefixes, ok := extra["cmdrisk.secret_path_prefixes"].([]string); ok {
		riskCtx.Asset.SecretPathPrefixes = prefixes
	}
	if prefixes, ok := extra["cmdrisk.workspace_path_prefixes"].([]string); ok {
		riskCtx.Asset.WorkspacePathPrefixes = prefixes
	}
	if hosts, ok := extra["cmdrisk.trusted_source_hosts"].([]string); ok {
		riskCtx.Trust.TrustedSourceHosts = hosts
	}
	if domains, ok := extra["cmdrisk.trusted_url_domains"].([]string); ok {
		riskCtx.Trust.TrustedURLDomains = domains
	}
	if allowances, ok := extra["cmdrisk.allowances"].(cmdrisk.AllowanceSet); ok {
		riskCtx.Allowances = allowances
	}

	return riskCtx
}

// Compile-time check.
var _ IntentAssessor = (*CmdRiskAssessor)(nil)
