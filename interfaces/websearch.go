package interfaces

import "context"

// WebSearchProvider defines the interface for web search implementations.
// Implementations (e.g., Tavily adapter) are provided separately.
type WebSearchProvider interface {
	// Name returns the provider name.
	Name() string
	
	// Search performs a web search with the given options.
	Search(ctx context.Context, query string, options SearchOptions) ([]Result, error)
}

// SearchOptions configures a web search.
type SearchOptions struct {
	MaxResults int
	// Other options can be added by implementations
}

// Result represents a single web search result.
type Result struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Snippet     string `json:"snippet"`
	Content     string `json:"content"`
}
