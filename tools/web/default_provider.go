package web

import (
	"os"
	"strings"

	"github.com/codewandler/agentsdk/tools/web/tavily"
	"github.com/codewandler/agentsdk/websearch"
)

const envWebSearchProvider = "WEBSEARCH_PROVIDER"

// DefaultSearchProviderFromEnv returns the configured default web search provider.
//
// Current policy:
//   - WEBSEARCH_PROVIDER unset or "tavily": use Tavily when TAVILY_API_KEY is set
//   - WEBSEARCH_PROVIDER "none": disable web search
//   - unknown provider values disable web search for now
func DefaultSearchProviderFromEnv() websearch.Provider {
	provider := strings.ToLower(strings.TrimSpace(os.Getenv(envWebSearchProvider)))
	if provider == "none" {
		return nil
	}
	if provider != "" && provider != "tavily" {
		return nil
	}

	key := strings.TrimSpace(os.Getenv("TAVILY_API_KEY"))
	if key == "" {
		return nil
	}
	return tavily.New(tavily.WithAPIKey(key))
}
