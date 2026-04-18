package web

import (
	"os"
	"strings"

	"github.com/codewandler/agentcore/interfaces"
	"github.com/codewandler/agentcore/tools/web/tavily"
)

const envWebSearchProvider = "WEBSEARCH_PROVIDER"

// DefaultSearchProviderFromEnv returns the configured default web search provider.
//
// Current policy:
//   - WEBSEARCH_PROVIDER unset or "tavily": use Tavily when TAVILY_API_KEY is set
//   - WEBSEARCH_PROVIDER "none": disable web search
//   - unknown provider values disable web search for now
func DefaultSearchProviderFromEnv() interfaces.WebSearchProvider {
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
