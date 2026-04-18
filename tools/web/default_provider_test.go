package web

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultSearchProviderFromEnv_DefaultsToTavilyWhenAPIKeyPresent(t *testing.T) {
	t.Setenv("WEBSEARCH_PROVIDER", "")
	t.Setenv("TAVILY_API_KEY", "test-key")

	p := DefaultSearchProviderFromEnv()
	require.NotNil(t, p)
	require.Equal(t, "tavily", p.Name())
}

func TestDefaultSearchProviderFromEnv_ExplicitTavily(t *testing.T) {
	t.Setenv("WEBSEARCH_PROVIDER", "tavily")
	t.Setenv("TAVILY_API_KEY", "test-key")

	p := DefaultSearchProviderFromEnv()
	require.NotNil(t, p)
	require.Equal(t, "tavily", p.Name())
}

func TestDefaultSearchProviderFromEnv_NoneDisablesProvider(t *testing.T) {
	t.Setenv("WEBSEARCH_PROVIDER", "none")
	t.Setenv("TAVILY_API_KEY", "test-key")

	p := DefaultSearchProviderFromEnv()
	require.Nil(t, p)
}

func TestDefaultSearchProviderFromEnv_UnknownProviderDisablesProvider(t *testing.T) {
	t.Setenv("WEBSEARCH_PROVIDER", "duckduckgo")
	t.Setenv("TAVILY_API_KEY", "test-key")

	p := DefaultSearchProviderFromEnv()
	require.Nil(t, p)
}

func TestDefaultSearchProviderFromEnv_MissingAPIKeyDisablesProvider(t *testing.T) {
	t.Setenv("WEBSEARCH_PROVIDER", "")
	t.Setenv("TAVILY_API_KEY", "")

	p := DefaultSearchProviderFromEnv()
	require.Nil(t, p)
}
