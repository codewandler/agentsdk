package runtime

import (
	"fmt"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
)

type AutoClientFunc func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error)

func DefaultAutoOptions(model string, sourceAPI adapt.ApiKind) adapterconfig.AutoOptions {
	opts := adapterconfig.AutoOptions{
		EnableEnv:         true,
		EnableLocalClaude: true,
		EnableLocalCodex:  true,
		UseModelDB:        true,
		DynamicModels:     true,
		SourceAPI:         sourceAPI,
		ModelDBAliases:    defaultModelDBAliases(),
	}
	if model != "" {
		opts.Intents = []adapterconfig.AutoIntent{{
			Name:      model,
			SourceAPI: sourceAPI,
		}}
	}
	return opts
}

func defaultModelDBAliases() []adapterconfig.ModelDBAliasConfig {
	return []adapterconfig.ModelDBAliasConfig{
		{Name: "opus", ServiceID: "anthropic", WireModelID: "claude-opus-4-6"},
		{Name: "opus", ServiceID: "openrouter", WireModelID: "anthropic/claude-opus-4.6"},
		{Name: "qwen3-coder", ServiceID: "openrouter", WireModelID: "qwen/qwen3-coder"},
		{Name: "qwen3-coder-next", ServiceID: "openrouter", WireModelID: "qwen/qwen3-coder-next"},
	}
}

func AutoMuxClient(model string, sourceAPI adapt.ApiKind, autoMux AutoClientFunc) (adapterconfig.AutoResult, error) {
	if autoMux == nil {
		autoMux = adapterconfig.AutoMuxClient
	}
	result, err := autoMux(DefaultAutoOptions(model, sourceAPI))
	if err != nil {
		return adapterconfig.AutoResult{}, fmt.Errorf("auto-detect llmadapter providers: %w", err)
	}
	return result, nil
}

func RouteIdentity(result adapterconfig.AutoResult, sourceAPI adapt.ApiKind, model string) (conversation.ProviderIdentity, adapterconfig.AutoRouteSummary, bool) {
	summary, ok := result.RouteSummary(sourceAPI, model)
	if !ok {
		return conversation.ProviderIdentity{}, adapterconfig.AutoRouteSummary{}, false
	}
	return RouteSummaryIdentity(summary), summary, true
}

func RouteSummaryIdentity(summary adapterconfig.AutoRouteSummary) conversation.ProviderIdentity {
	return conversation.ProviderIdentity{
		ProviderName: summary.Provider,
		APIKind:      string(summary.ProviderAPI),
		NativeModel:  summary.NativeModel,
	}
}
