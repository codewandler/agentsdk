package shell

import (
	"strings"

	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
)

// bashIntent returns a DeclareIntent option for the bash tool.
// If analyzer is non-nil, it uses cmdrisk for full command analysis.
// Otherwise, it returns an opaque intent.
func bashIntent(analyzer *cmdrisk.Analyzer) tool.TypedToolOption[BashParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p BashParams) (tool.Intent, error) {
		commands := []string(p.Cmd)
		summary := strings.Join(commands, "; ")
		if len(commands) == 0 {
			return tool.Intent{
				Tool:       "bash",
				ToolClass:  "command_execution",
				Opaque:     true,
				Confidence: "low",
				Behaviors:  []string{"command_execution"},
			}, nil
		}

		if analyzer == nil {
			return tool.Intent{
				Tool:       "bash",
				ToolClass:  "command_execution",
				Summary:    summary,
				Opaque:     true,
				Confidence: "low",
				Behaviors:  []string{"command_execution"},
			}, nil
		}

		workDir := p.Workdir
		if workDir == "" {
			workDir = ctx.WorkDir()
		}

		riskCtx := buildRiskContext(ctx, workDir)

		// Assess each command individually so the risk model matches the
		// actual execution semantics (each command runs in its own bash -c).
		// Merge the results into a single worst-case assessment.
		var merged *cmdrisk.Assessment
		for _, cmd := range commands {
			assessment, err := analyzer.Assess(ctx, cmdrisk.Request{
				Command: cmd,
				Context: riskCtx,
			})
			if err != nil {
				return tool.Intent{
					Tool:       "bash",
					ToolClass:  "command_execution",
					Summary:    summary,
					Opaque:     true,
					Confidence: "low",
					Behaviors:  []string{"command_execution"},
				}, nil
			}
			merged = mergeAssessments(merged, &assessment)
		}

		// Map cmdrisk targets → IntentOperations.
		ops := make([]tool.IntentOperation, 0, len(merged.Targets))
		for _, target := range merged.Targets {
			ops = append(ops, tool.IntentOperation{
				Resource: tool.IntentResource{
					Category: target.Category,
					Value:    target.Value,
					Locality: target.Locality,
				},
				Operation: target.Role,
				Certain:   target.Certain,
			})
		}

		return tool.Intent{
			Tool:       "bash",
			ToolClass:  "command_execution",
			Summary:    summary,
			Operations: ops,
			Behaviors:  merged.Behaviors,
			Confidence: string(merged.Confidence),
			Extra:      merged, // CmdRiskAssessor reuses this — no double work
		}, nil
	})
}

// buildRiskContext constructs a cmdrisk.Context from the tool execution context
// instead of hardcoding values. It reads environment and trust metadata from
// ctx.Extra() when available, falling back to safe defaults.
func buildRiskContext(ctx tool.Ctx, workDir string) cmdrisk.Context {
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

	riskCtx := cmdrisk.Context{
		Environment: env,
		User:        ctx.AgentID(),
		Interactive: interactive,
		Sandboxed:   sandboxed,
		Asset: cmdrisk.AssetContext{
			WorkspacePathPrefixes: []string{workDir},
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

// mergeAssessments combines two assessments into a worst-case composite.
// The first call should pass nil for prev.
func mergeAssessments(prev, next *cmdrisk.Assessment) *cmdrisk.Assessment {
	if prev == nil {
		return next
	}

	// Use the stricter decision.
	decision := prev.Decision
	if actionSeverity(next.Decision.Action) > actionSeverity(prev.Decision.Action) {
		decision = next.Decision
	}

	// Use the lower confidence.
	confidence := prev.Confidence
	if confidenceSeverity(next.Confidence) > confidenceSeverity(prev.Confidence) {
		confidence = next.Confidence
	}

	// Merge slices.
	behaviors := appendUnique(prev.Behaviors, next.Behaviors...)
	derivedFlags := appendUnique(prev.DerivedFlags, next.DerivedFlags...)
	constraints := appendUnique(prev.PreservedConstraints, next.PreservedConstraints...)

	return &cmdrisk.Assessment{
		Command:              prev.Command + "; " + next.Command,
		Context:              prev.Context,
		Classification:       prev.Classification, // first command's classification
		Facts:                append(append([]cmdrisk.Fact(nil), prev.Facts...), next.Facts...),
		Behaviors:            behaviors,
		Targets:              append(append([]cmdrisk.Target(nil), prev.Targets...), next.Targets...),
		AllowanceMatches:     append(append([]cmdrisk.AllowanceMatch(nil), prev.AllowanceMatches...), next.AllowanceMatches...),
		PreservedConstraints: constraints,
		RiskDimensions:       mergeRiskDimensions(prev.RiskDimensions, next.RiskDimensions),
		DerivedFlags:         derivedFlags,
		Confidence:           confidence,
		Decision:             decision,
		Explanation:          prev.Explanation, // keep first; full detail is in Extra
	}
}

func actionSeverity(a cmdrisk.Action) int {
	switch a {
	case cmdrisk.ActionAllow:
		return 0
	case cmdrisk.ActionRequiresApproval:
		return 1
	case cmdrisk.ActionReject:
		return 2
	case cmdrisk.ActionError:
		return 3
	default:
		return 4
	}
}

func confidenceSeverity(c cmdrisk.Confidence) int {
	switch c {
	case cmdrisk.ConfidenceHigh:
		return 0
	case cmdrisk.ConfidenceModerate:
		return 1
	case cmdrisk.ConfidenceLow:
		return 2
	default:
		return 3
	}
}

// mergeRiskDimensions takes the max severity per dimension name.
func mergeRiskDimensions(a, b []cmdrisk.RiskDimension) []cmdrisk.RiskDimension {
	byName := map[string]cmdrisk.RiskDimension{}
	var order []string
	for _, d := range a {
		byName[d.Name] = d
		order = append(order, d.Name)
	}
	for _, d := range b {
		if existing, ok := byName[d.Name]; ok {
			if d.Severity > existing.Severity {
				byName[d.Name] = d
			}
		} else {
			byName[d.Name] = d
			order = append(order, d.Name)
		}
	}
	out := make([]cmdrisk.RiskDimension, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out
}

func appendUnique(base []string, items ...string) []string {
	seen := make(map[string]struct{}, len(base))
	for _, s := range base {
		seen[s] = struct{}{}
	}
	out := append([]string(nil), base...)
	for _, s := range items {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
