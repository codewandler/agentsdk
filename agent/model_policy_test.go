package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/compatibility"
	"github.com/codewandler/modeldb"
	"github.com/stretchr/testify/require"
)

func TestParseSourceAPI(t *testing.T) {
	got, err := ParseSourceAPI("auto")
	require.NoError(t, err)
	require.Empty(t, got)

	got, err = ParseSourceAPI("openai.chat.completions")
	require.NoError(t, err)
	require.Equal(t, adapt.ApiOpenAIChatCompletions, got)

	_, err = ParseSourceAPI("bad")
	require.Error(t, err)
}

func TestLoadEmbeddedAgenticCodingEvidence(t *testing.T) {
	evidence, source, err := LoadCompatibilityEvidence(ModelPolicy{UseCase: ModelUseCaseAgenticCoding})
	require.NoError(t, err)
	require.Equal(t, "embedded:agentic_coding", source)
	require.Equal(t, compatibility.UseCaseAgenticCoding, evidence.UseCase)
	require.NotEmpty(t, evidence.Rows)
}

func TestApprovedOnlyModelPolicyPinsSelectedRoute(t *testing.T) {
	cfg := modelPolicyTestConfig(t)
	a, err := New(
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: "haiku", MaxTokens: 1000}),
		WithAutoMux(func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
			return adapterconfig.AutoResult{
				Client: runnertest.NewClient(runnertest.TextStream("unused")),
				Config: cfg,
			}, nil
		}),
		WithModelPolicy(ModelPolicy{
			UseCase:      ModelUseCaseAgenticCoding,
			ApprovedOnly: true,
			EvidencePath: modelPolicyEvidencePath(t, compatibility.StatusApproved),
		}),
	)
	require.NoError(t, err)
	require.Equal(t, "anthropic", a.route.resolvedProvider)
	require.Equal(t, "claude-haiku-test", a.route.resolvedModel)
	require.Equal(t, adapt.ApiAnthropicMessages, a.route.sourceAPI)
	require.Equal(t, compatibility.StatusApproved, a.route.compatibility.Status)
	require.True(t, a.route.compatibility.Pinned)
	require.Contains(t, a.ParamsSummary(), "compatibility: approved")
	require.Len(t, a.route.autoResult.Config.Routes, 1)
	require.False(t, a.route.autoResult.Config.Routes[0].DynamicModels)
}

func TestApprovedOnlyModelPolicyPinsQualifiedRequestModel(t *testing.T) {
	cfg := modelPolicyTestConfig(t)
	a, err := New(
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: "provider/haiku", MaxTokens: 1000}),
		WithAutoMux(func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
			return adapterconfig.AutoResult{
				Client: runnertest.NewClient(runnertest.TextStream("unused")),
				Config: cfg,
			}, nil
		}),
		WithModelPolicy(ModelPolicy{
			UseCase:      ModelUseCaseAgenticCoding,
			ApprovedOnly: true,
			EvidencePath: modelPolicyEvidencePath(t, compatibility.StatusApproved),
		}),
	)
	require.NoError(t, err)
	require.Equal(t, "provider/haiku", a.route.autoResult.Config.Routes[0].Model)
	require.Equal(t, "claude-haiku-test", a.route.autoResult.Config.Routes[0].NativeModel)
	require.Equal(t, compatibility.StatusApproved, a.route.compatibility.Status)
}

func TestEvaluationOnlyModelPolicyUsesEvidenceForQualifiedModel(t *testing.T) {
	cfg := modelPolicyTestConfig(t)
	a, err := New(
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: "provider/haiku", MaxTokens: 1000}),
		WithAutoMux(func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
			return adapterconfig.AutoResult{
				Client: runnertest.NewClient(runnertest.TextStream("unused")),
				Config: cfg,
			}, nil
		}),
		WithModelPolicy(ModelPolicy{
			UseCase:      ModelUseCaseAgenticCoding,
			EvidencePath: modelPolicyEvidencePath(t, compatibility.StatusApproved),
		}),
	)
	require.NoError(t, err)
	require.Equal(t, compatibility.StatusApproved, a.route.compatibility.Status)
	require.Contains(t, a.ParamsSummary(), "compatibility: approved")
}

func TestApprovedOnlyModelPolicyFailsClosed(t *testing.T) {
	cfg := modelPolicyTestConfig(t)
	_, err := New(
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: "haiku", MaxTokens: 1000}),
		WithAutoMux(func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
			return adapterconfig.AutoResult{Client: runnertest.NewClient(), Config: cfg}, nil
		}),
		WithModelPolicy(ModelPolicy{
			UseCase:      ModelUseCaseAgenticCoding,
			ApprovedOnly: true,
			EvidencePath: modelPolicyEvidencePath(t, compatibility.StatusUntested),
		}),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "no candidates approved")
}

func TestEvaluationOnlyModelPolicyUsesEvidenceWithoutPinning(t *testing.T) {
	cfg := modelPolicyTestConfig(t)
	a, err := New(
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: "haiku", MaxTokens: 1000}),
		WithAutoMux(func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
			return adapterconfig.AutoResult{
				Client: runnertest.NewClient(runnertest.TextStream("unused")),
				Config: cfg,
			}, nil
		}),
		WithModelPolicy(ModelPolicy{
			UseCase:      ModelUseCaseAgenticCoding,
			EvidencePath: modelPolicyEvidencePath(t, compatibility.StatusApproved),
		}),
	)
	require.NoError(t, err)
	require.Equal(t, compatibility.StatusApproved, a.route.compatibility.Status)
	require.False(t, a.route.compatibility.Pinned)
	require.Contains(t, a.ParamsSummary(), "compatibility: approved")
}

func TestEvaluationOnlyModelPolicyRecordsMissingEvidenceDiagnostic(t *testing.T) {
	cfg := modelPolicyTestConfig(t)
	a, err := New(
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: "haiku", MaxTokens: 1000}),
		WithAutoMux(func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
			return adapterconfig.AutoResult{
				Client: runnertest.NewClient(runnertest.TextStream("unused")),
				Config: cfg,
			}, nil
		}),
		WithModelPolicy(ModelPolicy{UseCase: ModelUseCaseSummarization}),
	)
	require.NoError(t, err)
	require.Contains(t, a.route.compatibility.Diagnostic, "no embedded compatibility evidence")
	require.Contains(t, a.ParamsSummary(), "compatibility:")
}

func TestEvaluationOnlyModelPolicyDoesNotFailCustomClient(t *testing.T) {
	a, err := New(
		WithClient(runnertest.NewClient()),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: "test/model", MaxTokens: 1000}),
		WithModelPolicy(ModelPolicy{UseCase: ModelUseCaseAgenticCoding}),
	)
	require.NoError(t, err)
	require.Equal(t, compatibility.StatusUnavailable, a.route.compatibility.Status)
	require.Contains(t, a.ParamsSummary(), "custom client")
}

func modelPolicyEvidencePath(t *testing.T, status compatibility.Status) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evidence.json")
	evidence := adapterconfig.CompatibilityEvidence{
		UseCase: compatibility.UseCaseAgenticCoding,
		Rows: []adapterconfig.CompatibilityRowEvidence{{
			PublicModel:      "haiku",
			NativeModel:      "claude-haiku-test",
			Provider:         "anthropic",
			ProviderAPI:      adapt.ApiAnthropicMessages,
			Status:           status,
			Text:             "live",
			Tools:            "live",
			ToolContinuation: "live",
			StructuredOutput: "live",
			Reasoning:        "live",
			PromptCaching:    "live",
			Usage:            "live",
			CacheAccounting:  "live",
		}},
	}
	data, err := json.Marshal(evidence)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}

func modelPolicyTestConfig(t *testing.T) adapterconfig.Config {
	t.Helper()
	catalog := modeldb.NewCatalog()
	key := modeldb.ModelKey{Creator: "anthropic", Family: "claude", Series: "haiku", Version: "test"}
	catalog.Services["anthropic"] = modeldb.Service{ID: "anthropic", Name: "Anthropic"}
	catalog.Models[key] = modeldb.ModelRecord{
		Key:     key,
		Name:    "Claude Haiku Test",
		Aliases: []string{"haiku"},
	}
	catalog.Offerings[modeldb.OfferingRef{ServiceID: "anthropic", WireModelID: "claude-haiku-test"}] = modeldb.Offering{
		ServiceID:   "anthropic",
		WireModelID: "claude-haiku-test",
		ModelKey:    key,
		Aliases:     []string{"haiku"},
		Exposures: []modeldb.OfferingExposure{{
			APIType: modeldb.APITypeAnthropicMessages,
			ExposedCapabilities: &modeldb.Capabilities{
				Streaming:        true,
				ToolUse:          true,
				StructuredOutput: true,
				Reasoning:        &modeldb.ReasoningCapability{Available: true},
				Caching:          &modeldb.CachingCapability{Available: true},
			},
		}},
	}
	path := filepath.Join(t.TempDir(), "catalog.json")
	require.NoError(t, modeldb.SaveJSON(path, catalog))
	cfg := adapterconfig.Config{
		ModelDB: adapterconfig.ModelDBConfig{CatalogPath: path},
		Providers: []adapterconfig.ProviderConfig{{
			Name:             "anthropic",
			Type:             "anthropic",
			APIKey:           "test",
			ModelDBServiceID: "anthropic",
		}},
		Routes: []adapterconfig.RouteConfig{{
			SourceAPI:    adapt.ApiAnthropicMessages,
			Model:        "haiku",
			Provider:     "anthropic",
			ProviderAPI:  adapt.ApiAnthropicMessages,
			ModelDBModel: "haiku",
			Weight:       100,
		}},
	}
	adapterconfig.ApplyDefaults(&cfg)
	return cfg
}
