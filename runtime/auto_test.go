package runtime

import (
	"errors"
	"testing"

	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/stretchr/testify/require"
)

func TestDefaultAutoOptionsUsesRequestedModelIntent(t *testing.T) {
	opts := DefaultAutoOptions("haiku", adapt.ApiOpenAIResponses)

	require.True(t, opts.EnableEnv)
	require.True(t, opts.EnableLocalClaude)
	require.True(t, opts.EnableLocalCodex)
	require.True(t, opts.UseModelDB)
	require.True(t, opts.DynamicModels)
	require.Equal(t, adapt.ApiOpenAIResponses, opts.SourceAPI)
	require.Contains(t, opts.ModelDBAliases, adapterconfig.ModelDBAliasConfig{
		Name:        "opus",
		ServiceID:   "anthropic",
		WireModelID: "claude-opus-4-6",
	})
	require.Contains(t, opts.ModelDBAliases, adapterconfig.ModelDBAliasConfig{
		Name:        "opus",
		ServiceID:   "openrouter",
		WireModelID: "anthropic/claude-opus-4.6",
	})
	require.Len(t, opts.Intents, 1)
	require.Equal(t, "haiku", opts.Intents[0].Name)
	require.Equal(t, adapt.ApiOpenAIResponses, opts.Intents[0].SourceAPI)
}

func TestAutoMuxClientWrapsDefaultOptions(t *testing.T) {
	var got adapterconfig.AutoOptions
	result, err := AutoMuxClient("sonnet", adapt.ApiAnthropicMessages, func(opts adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
		got = opts
		return adapterconfig.AutoResult{}, nil
	})

	require.NoError(t, err)
	require.Equal(t, adapterconfig.AutoResult{}, result)
	require.Equal(t, "sonnet", got.Intents[0].Name)
	require.Equal(t, adapt.ApiAnthropicMessages, got.SourceAPI)
}

func TestAutoMuxClientWrapsError(t *testing.T) {
	_, err := AutoMuxClient("sonnet", adapt.ApiAnthropicMessages, func(opts adapterconfig.AutoOptions) (adapterconfig.AutoResult, error) {
		return adapterconfig.AutoResult{}, errors.New("boom")
	})

	require.ErrorContains(t, err, "auto-detect llmadapter providers: boom")
}

func TestRouteIdentityFromAutoResult(t *testing.T) {
	result := adapterconfig.AutoResult{
		Config: adapterconfig.Config{
			Providers: []adapterconfig.ProviderConfig{{Name: "openai_responses", Type: "openai_responses"}},
			Routes: []adapterconfig.RouteConfig{{
				SourceAPI:   adapt.ApiOpenAIResponses,
				Model:       "default",
				Provider:    "openai_responses",
				ProviderAPI: adapt.ApiOpenAIResponses,
				NativeModel: "gpt-5.4",
			}},
		},
		Enabled: []adapterconfig.AutoProvider{{Name: "openai_responses", Type: "openai_responses"}},
	}

	identity, summary, ok := RouteIdentity(result, adapt.ApiOpenAIResponses, "default")

	require.True(t, ok)
	require.Equal(t, "openai_responses", summary.Provider)
	require.Equal(t, "openai_responses", identity.ProviderName)
	require.Equal(t, string(adapt.ApiOpenAIResponses), identity.APIKind)
	require.Equal(t, "gpt-5.4", identity.NativeModel)
}
