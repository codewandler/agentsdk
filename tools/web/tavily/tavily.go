// Package tavily provides a Tavily web search provider.
package tavily

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/codewandler/agentsdk/websearch"
)

const (
	apiURL         = "https://api.tavily.com/search"
	defaultTimeout = 30 * time.Second
	defaultResults = 5
	maxResults     = 10
)

// Provider implements websearch.Provider using the Tavily API.
type Provider struct {
	apiKey      string
	searchDepth string // "basic" or "advanced"
	httpClient  *http.Client
}

// Option configures the Provider.
type Option func(*Provider)

// WithAPIKey sets the Tavily API key.
func WithAPIKey(key string) Option {
	return func(p *Provider) { p.apiKey = key }
}

// WithSearchDepth sets the search depth ("basic" or "advanced").
// Advanced is more thorough but slower and uses more API credits.
func WithSearchDepth(depth string) Option {
	return func(p *Provider) { p.searchDepth = depth }
}

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(p *Provider) { p.httpClient = c }
}

// New creates a Tavily search provider.
func New(opts ...Option) *Provider {
	p := &Provider{
		searchDepth: "basic",
		httpClient:  &http.Client{Timeout: defaultTimeout},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name satisfies websearch.Provider.
func (p *Provider) Name() string { return "tavily" }

// Search satisfies websearch.Provider.
func (p *Provider) Search(ctx context.Context, query string, opts websearch.Options) ([]websearch.Result, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("tavily: API key not configured (set TAVILY_API_KEY)")
	}
	if query == "" {
		return nil, fmt.Errorf("tavily: query cannot be empty")
	}

	n := opts.MaxResults
	if n < 1 {
		n = defaultResults
	}
	if n > maxResults {
		n = maxResults
	}

	body, err := json.Marshal(searchRequest{
		APIKey:            p.apiKey,
		Query:             query,
		SearchDepth:       p.searchDepth,
		MaxResults:        n,
		IncludeAnswer:     false,
		IncludeRawContent: false,
	})
	if err != nil {
		return nil, fmt.Errorf("tavily: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tavily: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tavily: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("tavily: API returned %d: %s", resp.StatusCode, string(b))
	}

	var sr searchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("tavily: decode response: %w", err)
	}

	results := make([]websearch.Result, len(sr.Results))
	for i, r := range sr.Results {
		results[i] = websearch.Result{Title: r.Title, URL: r.URL, Snippet: r.Content}
	}
	return results, nil
}

var _ websearch.Provider = (*Provider)(nil)

type searchRequest struct {
	APIKey            string `json:"api_key"`
	Query             string `json:"query"`
	SearchDepth       string `json:"search_depth,omitempty"`
	MaxResults        int    `json:"max_results,omitempty"`
	IncludeAnswer     bool   `json:"include_answer"`
	IncludeRawContent bool   `json:"include_raw_content"`
}

type searchResponse struct {
	Results []searchResult `json:"results"`
}

type searchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}
