package agentdir

import (
	"path/filepath"
	"strings"

	"github.com/codewandler/agentsdk/agentconfig"
)

type ManifestModelPolicy struct {
	UseCase       string `json:"use_case"`
	SourceAPI     string `json:"source_api"`
	ApprovedOnly  *bool  `json:"approved_only"`
	AllowDegraded *bool  `json:"allow_degraded"`
	AllowUntested *bool  `json:"allow_untested"`
	EvidencePath  string `json:"evidence_path"`
}

func (p ManifestModelPolicy) AgentPolicy(baseDir string) (agentconfig.ModelPolicy, bool, error) {
	configured := strings.TrimSpace(p.UseCase) != "" ||
		strings.TrimSpace(p.SourceAPI) != "" ||
		p.ApprovedOnly != nil ||
		p.AllowDegraded != nil ||
		p.AllowUntested != nil ||
		strings.TrimSpace(p.EvidencePath) != ""
	if !configured {
		return agentconfig.ModelPolicy{}, false, nil
	}
	var out agentconfig.ModelPolicy
	if strings.TrimSpace(p.UseCase) != "" {
		useCase, err := agentconfig.ParseModelUseCase(p.UseCase)
		if err != nil {
			return agentconfig.ModelPolicy{}, false, err
		}
		out.UseCase = useCase
	}
	if strings.TrimSpace(p.SourceAPI) != "" {
		sourceAPI, err := agentconfig.ParseSourceAPI(p.SourceAPI)
		if err != nil {
			return agentconfig.ModelPolicy{}, false, err
		}
		out.SourceAPI = sourceAPI
	}
	if p.ApprovedOnly != nil {
		out.ApprovedOnly = *p.ApprovedOnly
	}
	if p.AllowDegraded != nil {
		out.AllowDegraded = *p.AllowDegraded
	}
	if p.AllowUntested != nil {
		out.AllowUntested = *p.AllowUntested
	}
	if strings.TrimSpace(p.EvidencePath) != "" {
		out.EvidencePath = p.EvidencePath
		if !filepath.IsAbs(out.EvidencePath) {
			out.EvidencePath = filepath.Join(baseDir, out.EvidencePath)
		}
	}
	return out, true, nil
}
