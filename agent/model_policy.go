package agent

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/compatibility"
)

//go:embed evidence/agentic_coding.json
var embeddedAgenticCodingEvidence []byte

// ModelUseCase identifies a compatibility use case for model routing.
// The canonical definition is in [agentconfig.ModelUseCase].
type ModelUseCase = agentconfig.ModelUseCase

const (
	ModelUseCaseAgenticCoding = agentconfig.ModelUseCaseAgenticCoding
	ModelUseCaseSummarization = agentconfig.ModelUseCaseSummarization
)

// ModelPolicy configures model compatibility routing.
// The canonical definition is in [agentconfig.ModelPolicy].
type ModelPolicy = agentconfig.ModelPolicy

// ParseModelUseCase delegates to [agentconfig.ParseModelUseCase].
func ParseModelUseCase(value string) (ModelUseCase, error) {
	return agentconfig.ParseModelUseCase(value)
}

// ParseSourceAPI delegates to [agentconfig.ParseSourceAPI].
func ParseSourceAPI(value string) (adapt.ApiKind, error) {
	return agentconfig.ParseSourceAPI(value)
}

// FormatSourceAPI delegates to [agentconfig.FormatSourceAPI].
func FormatSourceAPI(api adapt.ApiKind) string {
	return agentconfig.FormatSourceAPI(api)
}

func LoadCompatibilityEvidence(policy ModelPolicy) (adapterconfig.CompatibilityEvidence, string, error) {
	useCase, err := policy.LLMUseCase()
	if err != nil {
		return adapterconfig.CompatibilityEvidence{}, "", err
	}
	if useCase == "" {
		useCase = compatibility.UseCaseAgenticCoding
	}
	if policy.EvidencePath != "" {
		evidence, err := adapterconfig.LoadCompatibilityEvidence(policy.EvidencePath)
		return evidence, policy.EvidencePath, err
	}
	switch useCase {
	case compatibility.UseCaseAgenticCoding:
		var evidence adapterconfig.CompatibilityEvidence
		if err := json.NewDecoder(bytes.NewReader(embeddedAgenticCodingEvidence)).Decode(&evidence); err != nil {
			return adapterconfig.CompatibilityEvidence{}, "", fmt.Errorf("decode embedded compatibility evidence for %q: %w", useCase, err)
		}
		return evidence, "embedded:agentic_coding", nil
	default:
		return adapterconfig.CompatibilityEvidence{}, "", fmt.Errorf("no embedded compatibility evidence for use case %q; pass --compat-evidence", useCase)
	}
}

type modelCompatibilityState struct {
	UseCase        compatibility.UseCase
	Status         compatibility.Status
	SourceAPI      adapt.ApiKind
	ProviderAPI    adapt.ApiKind
	EvidenceSource string
	Pinned         bool
	Diagnostic     string

	MissingRequired   []compatibility.Feature
	UntestedRequired  []compatibility.Feature
	DegradedPreferred []compatibility.Feature
}

func (s modelCompatibilityState) configured() bool {
	return s.UseCase != "" || s.Status != "" || s.Diagnostic != ""
}

func modelCompatibilityFromEvaluation(e compatibility.Evaluation, evidenceSource string, pinned bool) modelCompatibilityState {
	return modelCompatibilityState{
		UseCase:           e.UseCase,
		Status:            e.Status,
		SourceAPI:         e.Candidate.SourceAPI,
		ProviderAPI:       e.Candidate.ProviderAPI,
		EvidenceSource:    evidenceSource,
		Pinned:            pinned,
		MissingRequired:   append([]compatibility.Feature(nil), e.MissingRequired...),
		UntestedRequired:  append([]compatibility.Feature(nil), e.UntestedRequired...),
		DegradedPreferred: append([]compatibility.Feature(nil), e.DegradedPreferred...),
	}
}

func featureNames(features []compatibility.Feature) string {
	if len(features) == 0 {
		return ""
	}
	names := make([]string, 0, len(features))
	for _, feature := range features {
		names = append(names, string(feature))
	}
	return strings.Join(names, ",")
}

func modelPolicyLookupNames(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	names := []string{model}
	if slash := strings.LastIndex(model, "/"); slash >= 0 && slash < len(model)-1 {
		names = append(names, model[slash+1:])
	}
	return names
}
