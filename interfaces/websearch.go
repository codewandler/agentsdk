package interfaces

import "github.com/codewandler/agentsdk/websearch"

// WebSearchProvider defines the interface for web search implementations.
//
// Deprecated: use websearch.Provider.
type WebSearchProvider = websearch.Provider

// SearchOptions configures a web search.
//
// Deprecated: use websearch.Options.
type SearchOptions = websearch.Options

// Result represents a single web search result.
//
// Deprecated: use websearch.Result.
type Result = websearch.Result
