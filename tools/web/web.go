// Package web provides the web_fetch and web_search tools.
package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"

	"github.com/codewandler/agentcore/tool"
	"github.com/codewandler/agentcore/interfaces"
	"github.com/codewandler/agentcore/internal/humanize"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	defaultFetchTimeout = 10 * time.Second
	maxFetchTimeout     = 60 * time.Second
	maxResponseSize     = 5 * 1024 * 1024 // 5 MB
	maxRedirects        = 10
	userAgent           = "flai/1.0"

	defaultSearchResults = 5
	maxSearchResults     = 10
)

var allowedMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodPost:    true,
	http.MethodPut:     true,
	http.MethodPatch:   true,
	http.MethodDelete:  true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// ── Parameter types ───────────────────────────────────────────────────────────

// WebFetchParams defines parameters for web_fetch.
type WebFetchParams struct {
	URL     string            `json:"url" jsonschema:"description=URL to fetch,required"`
	Method  string            `json:"method,omitempty" jsonschema:"description=HTTP method (default GET)"`
	Headers map[string]string `json:"headers,omitempty" jsonschema:"description=Custom HTTP headers"`
	Timeout int               `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds (default 10 max 60)"`
	Body    string            `json:"body,omitempty" jsonschema:"description=Request body for POST/PUT/PATCH"`
}

// WebSearchParams defines parameters for web_search.
type WebSearchParams struct {
	Query      string `json:"query" jsonschema:"description=Search query,required"`
	MaxResults int    `json:"max_results,omitempty" jsonschema:"description=Number of results to return (default 5 max 10)"`
}

// ── Tools factory ─────────────────────────────────────────────────────────────

// Tools returns the web tools.
// If provider is nil, web_search is omitted from the returned slice.
func Tools(provider interfaces.WebSearchProvider) []tool.Tool {
	tools := []tool.Tool{webFetch()}
	if provider != nil {
		tools = append(tools, webSearch(provider))
	}
	return tools
}

// ── web_fetch ─────────────────────────────────────────────────────────────────

func webFetch() tool.Tool {
	return tool.New("web_fetch",
		"Fetch content from a URL. Returns status, headers, and body. HTML is automatically converted to Markdown.",
		func(ctx tool.Ctx, p WebFetchParams) (tool.Result, error) {
			if strings.TrimSpace(p.URL) == "" {
				return tool.Error("url cannot be empty"), nil
			}
			parsed, err := url.Parse(p.URL)
			if err != nil {
				return tool.Errorf("invalid url: %v", err), nil
			}
			if parsed.Scheme != "http" && parsed.Scheme != "https" {
				return tool.Error("only http and https URLs are supported"), nil
			}

			method := strings.ToUpper(strings.TrimSpace(p.Method))
			if method == "" {
				method = http.MethodGet
			}
			if !allowedMethods[method] {
				return tool.Errorf("unsupported HTTP method: %s", method), nil
			}

			timeout := time.Duration(p.Timeout) * time.Second
			if timeout <= 0 {
				timeout = defaultFetchTimeout
			}
			if timeout > maxFetchTimeout {
				timeout = maxFetchTimeout
			}

			client := &http.Client{
				Timeout: timeout,
				CheckRedirect: func(_ *http.Request, via []*http.Request) error {
					if len(via) >= maxRedirects {
						return fmt.Errorf("too many redirects (%d)", maxRedirects)
					}
					return nil
				},
			}

			var bodyReader io.Reader
			if p.Body != "" {
				bodyReader = strings.NewReader(p.Body)
			}
			req, err := http.NewRequestWithContext(ctx, method, p.URL, bodyReader)
			if err != nil {
				return nil, fmt.Errorf("create request: %w", err)
			}
			req.Header.Set("User-Agent", userAgent)
			for k, v := range p.Headers {
				req.Header.Set(k, v)
			}

			start := time.Now()
			resp, err := client.Do(req)
			if err != nil {
				return tool.Errorf("request failed: %v", err), nil
			}
			defer resp.Body.Close()
			dur := time.Since(start)

			limited := io.LimitReader(resp.Body, int64(maxResponseSize)+1)
			rawBody, err := io.ReadAll(limited)
			if err != nil {
				return nil, fmt.Errorf("read response: %w", err)
			}
			truncated := len(rawBody) > maxResponseSize
			if truncated {
				rawBody = rawBody[:maxResponseSize]
			}

			ct := resp.Header.Get("Content-Type")
			bodyStr := string(rawBody)
			if isHTML(ct) {
				if md, err := htmltomarkdown.ConvertString(bodyStr); err == nil {
					bodyStr = md
				}
			}

			finalURL := resp.Request.URL.String()
			return &fetchResult{
				URL:         p.URL,
				FinalURL:    finalURL,
				Status:      resp.Status,
				ContentType: simplifyMIME(ct),
				Size:        len(rawBody),
				Duration:    dur,
				Truncated:   truncated,
				Body:        bodyStr,
			}, nil
		},
	)
}

// ── web_search ────────────────────────────────────────────────────────────────

func webSearch(provider interfaces.WebSearchProvider) tool.Tool {
	desc := fmt.Sprintf("Search the web using %s. Returns titles, URLs, and snippets.", provider.Name())
	return tool.New("web_search", desc,
		func(ctx tool.Ctx, p WebSearchParams) (tool.Result, error) {
			if strings.TrimSpace(p.Query) == "" {
				return tool.Error("query cannot be empty"), nil
			}
			n := p.MaxResults
			if n < 1 {
				n = defaultSearchResults
			}
			if n > maxSearchResults {
				n = maxSearchResults
			}
			results, err := provider.Search(ctx, p.Query, interfaces.SearchOptions{MaxResults: n})
			if err != nil {
				return tool.Errorf("search failed: %v", err), nil
			}
			return &searchResult{Query: p.Query, Results: results}, nil
		},
	)
}

// ── Result types ──────────────────────────────────────────────────────────────

type fetchResult struct {
	URL         string        `json:"url"`
	FinalURL    string        `json:"final_url,omitempty"`
	Status      string        `json:"status"`
	ContentType string        `json:"content_type,omitempty"`
	Size        int           `json:"size"`
	Duration    time.Duration `json:"-"`
	Truncated   bool          `json:"truncated,omitempty"`
	Body        string        `json:"body"`
}

func (r *fetchResult) IsError() bool { return false }
func (r *fetchResult) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Status: %s | Type: %s | Size: %s | Time: %dms",
		r.Status, r.ContentType, humanize.Size(int64(r.Size)), r.Duration.Milliseconds())
	if r.FinalURL != "" && r.FinalURL != r.URL {
		fmt.Fprintf(&sb, " | Redirected: %s", r.FinalURL)
	}
	if r.Truncated {
		sb.WriteString(" | [truncated]")
	}
	sb.WriteString("\n\n")
	sb.WriteString(r.Body)
	return sb.String()
}
func (r *fetchResult) MarshalJSON() ([]byte, error) {
	type wire fetchResult
	return json.Marshal((*wire)(r))
}

type searchResult struct {
	Query   string             `json:"query"`
	Results []interfaces.Result `json:"results"`
}

func (r *searchResult) IsError() bool { return false }
func (r *searchResult) String() string {
	if len(r.Results) == 0 {
		return fmt.Sprintf("No results found for %q.", r.Query)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Search: %q — %d result(s)\n\n", r.Query, len(r.Results))
	for i, res := range r.Results {
		fmt.Fprintf(&sb, "%d. %s\n   %s\n", i+1, res.Title, res.URL)
		if res.Snippet != "" {
			fmt.Fprintf(&sb, "   %s\n", res.Snippet)
		}
		sb.WriteByte('\n')
	}
	return strings.TrimSpace(sb.String())
}
func (r *searchResult) MarshalJSON() ([]byte, error) {
	type wire searchResult
	return json.Marshal((*wire)(r))
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func isHTML(ct string) bool {
	lower := strings.ToLower(ct)
	return strings.Contains(lower, "text/html") || strings.Contains(lower, "application/xhtml")
}

func simplifyMIME(ct string) string {
	if idx := strings.Index(ct, ";"); idx >= 0 {
		return strings.TrimSpace(ct[:idx])
	}
	return ct
}

