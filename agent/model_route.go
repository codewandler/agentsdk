package agent

import (
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/conversation"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/compatibility"
	"github.com/codewandler/llmadapter/unified"
)

// modelRoute holds model client resolution, routing identity, and compatibility
// policy state. It is owned by Instance and populated during initRuntime.
type modelRoute struct {
	client            unified.Client
	autoMux           func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error)
	autoResult        adapterconfig.AutoResult
	providerIdentity  conversation.ProviderIdentity
	resolvedProvider  string
	resolvedModel     string
	sourceAPI         adapt.ApiKind
	sourceAPIExplicit bool
	modelPolicy       ModelPolicy
	compatibility     modelCompatibilityState
	contextWindow     int
}

func (r *modelRoute) autoMuxSourceAPI() adapt.ApiKind {
	if r.modelPolicy.Configured() {
		if r.modelPolicy.SourceAPI != "" {
			return r.modelPolicy.SourceAPI
		}
		if !r.sourceAPIExplicit {
			return ""
		}
	}
	return r.sourceAPI
}

func (r *modelRoute) policySourceAPI() adapt.ApiKind {
	if r.modelPolicy.SourceAPI != "" {
		return r.modelPolicy.SourceAPI
	}
	if r.sourceAPIExplicit {
		return r.sourceAPI
	}
	if r.modelPolicy.Configured() {
		return ""
	}
	return r.sourceAPI
}

func (r *modelRoute) applyModelPolicyWithoutAutoResult() error {
	useCase, err := r.modelPolicy.LLMUseCase()
	if err != nil {
		return err
	}
	if useCase == "" {
		return nil
	}
	if r.modelPolicy.ApprovedOnly {
		return fmt.Errorf("agent: approved-only model policy requires auto mux routing")
	}
	r.compatibility = modelCompatibilityState{
		UseCase:    useCase,
		Status:     compatibility.StatusUnavailable,
		Diagnostic: "custom client has no llmadapter route config",
	}
	return nil
}

func (r *modelRoute) applyModelPolicy(model string) error {
	if !r.modelPolicy.Configured() {
		return nil
	}
	useCase, err := r.modelPolicy.LLMUseCase()
	if err != nil {
		return err
	}
	if useCase == "" {
		return nil
	}
	sourceAPI := r.policySourceAPI()
	if r.modelPolicy.ApprovedOnly {
		return r.applyApprovedOnlyModelPolicy(useCase, sourceAPI, model)
	}
	return r.applyEvaluationModelPolicy(useCase, sourceAPI, model)
}

func (r *modelRoute) applyApprovedOnlyModelPolicy(useCase compatibility.UseCase, sourceAPI adapt.ApiKind, model string) error {
	evidence, evidenceSource, err := LoadCompatibilityEvidence(r.modelPolicy)
	if err != nil {
		return err
	}
	selection, err := selectModelForPolicy(r.autoResult, model, sourceAPI, adapterconfig.UseCaseSelectionOptions{
		UseCase:       useCase,
		Evidence:      evidence,
		AllowDegraded: r.modelPolicy.AllowDegraded,
		AllowUntested: r.modelPolicy.AllowUntested,
	})
	if err != nil {
		return err
	}
	pinnedConfig, err := pinnedConfigForSelection(r.autoResult.Config, selection, model)
	if err != nil {
		return err
	}
	client, err := adapterconfig.NewMuxClient(pinnedConfig, adapterconfig.WithSourceAPI(selection.Resolution.SourceAPI), adapterconfig.WithFallback(false))
	if err != nil {
		return err
	}
	r.client = client
	r.autoResult.Config = pinnedConfig
	r.autoResult.Client = client
	r.sourceAPI = selection.Resolution.SourceAPI
	r.sourceAPIExplicit = true
	r.compatibility = modelCompatibilityFromEvaluation(selection.Evaluation, evidenceSource, true)
	r.compatibility.SourceAPI = selection.Resolution.SourceAPI
	r.compatibility.ProviderAPI = selection.Resolution.ProviderAPI
	return nil
}

func (r *modelRoute) applyEvaluationModelPolicy(useCase compatibility.UseCase, sourceAPI adapt.ApiKind, model string) error {
	evidenceDiagnostic := ""
	if evidence, evidenceSource, err := LoadCompatibilityEvidence(r.modelPolicy); err == nil {
		selection, err := selectModelForPolicy(r.autoResult, model, sourceAPI, adapterconfig.UseCaseSelectionOptions{
			UseCase:       useCase,
			Evidence:      evidence,
			AllowDegraded: true,
			AllowUntested: true,
		})
		if err == nil {
			r.compatibility = modelCompatibilityFromEvaluation(selection.Evaluation, evidenceSource, false)
			r.compatibility.SourceAPI = selection.Resolution.SourceAPI
			r.compatibility.ProviderAPI = selection.Resolution.ProviderAPI
			return nil
		}
	} else {
		evidenceDiagnostic = err.Error()
	}
	evaluations, err := adapterconfig.EvaluateCompatibilityCandidates(r.autoResult.Config, model, sourceAPI, useCase)
	if err != nil {
		r.compatibility = modelCompatibilityState{
			UseCase:    useCase,
			Status:     compatibility.StatusUnavailable,
			Diagnostic: err.Error(),
		}
		return nil
	}
	if len(evaluations) == 0 {
		r.compatibility = modelCompatibilityState{
			UseCase:    useCase,
			Status:     compatibility.StatusUnavailable,
			Diagnostic: "no compatibility candidates",
		}
		return nil
	}
	r.compatibility = modelCompatibilityFromEvaluation(evaluations[0], "", false)
	r.compatibility.Diagnostic = evidenceDiagnostic
	return nil
}

func (r *modelRoute) resolveRouteIdentity(model string) {
	r.providerIdentity = conversation.ProviderIdentity{}
	r.resolvedProvider = ""
	r.resolvedModel = ""
	identity, summary, ok := agentruntime.RouteIdentity(r.autoResult, r.sourceAPI, model)
	if !ok {
		return
	}
	r.resolvedProvider = summary.Provider
	r.resolvedModel = summary.NativeModel
	r.contextWindow = summary.ContextWindow
	r.providerIdentity = identity
}

func selectModelForPolicy(result adapterconfig.AutoResult, model string, sourceAPI adapt.ApiKind, opts adapterconfig.UseCaseSelectionOptions) (adapterconfig.UseCaseModelSelection, error) {
	var lastErr error
	for _, candidate := range modelPolicyLookupNames(model) {
		selection, err := result.SelectModelForUseCase(candidate, sourceAPI, opts)
		if err == nil {
			return selection, nil
		}
		lastErr = err
	}
	return adapterconfig.UseCaseModelSelection{}, lastErr
}

func (r *modelRoute) compatibilitySummary() string {
	if !r.compatibility.configured() {
		return ""
	}
	state := r.compatibility
	parts := []string{}
	if state.SourceAPI != "" {
		parts = append(parts, "source_api: "+string(state.SourceAPI))
	}
	if state.ProviderAPI != "" {
		parts = append(parts, "provider_api: "+string(state.ProviderAPI))
	}
	if state.UseCase != "" {
		parts = append(parts, "use_case: "+string(state.UseCase))
	}
	if state.Status != "" {
		parts = append(parts, "compatibility: "+string(state.Status))
	}
	if missing := featureNames(state.MissingRequired); missing != "" {
		parts = append(parts, "missing_required: "+missing)
	}
	if untested := featureNames(state.UntestedRequired); untested != "" {
		parts = append(parts, "untested_required: "+untested)
	}
	if degraded := featureNames(state.DegradedPreferred); degraded != "" {
		parts = append(parts, "degraded_preferred: "+degraded)
	}
	if state.Diagnostic != "" {
		parts = append(parts, "reason: "+state.Diagnostic)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, "  ")
}
