// Package websearch defines provider contracts for web search tools.
package websearch

import "context"

// Provider defines the interface for web search implementations.
type Provider interface {
	// Name returns the provider name.
	Name() string

	// Search performs a web search with the given options.
	Search(ctx context.Context, query string, options Options) ([]Result, error)
}

// Options configures a web search.
type Options struct {
	MaxResults int
}

// Result represents a single web search result.
type Result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Snippet     string `json:"snippet"`
	Content     string `json:"content"`
}
