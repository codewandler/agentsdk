package tavily

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/websearch"
	"github.com/stretchr/testify/require"
)

// redirectTransport rewrites every outbound request so it hits the given test
// server instead of the real Tavily API URL.
type redirectTransport struct {
	to string // base URL of the httptest.Server, e.g. "http://127.0.0.1:PORT"
}

func (r *redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req2 := req.Clone(req.Context())
	req2.URL.Host = strings.TrimPrefix(r.to, "http://")
	req2.URL.Scheme = "http"
	return http.DefaultTransport.RoundTrip(req2)
}

func TestProvider_Search_NoAPIKey(t *testing.T) {
	p := New()
	_, err := p.Search(context.Background(), "golang", websearch.Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "API key not configured")
}

func TestProvider_Search_EmptyQuery(t *testing.T) {
	p := New(WithAPIKey("test-key"))
	_, err := p.Search(context.Background(), "", websearch.Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "query cannot be empty")
}

func TestProvider_Search_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(w, `{"results":[{"title":"Go Blog","url":"https://go.dev/blog","content":"Latest Go news"}]}`)
	}))
	defer ts.Close()

	p := New(
		WithAPIKey("test-key"),
		WithHTTPClient(&http.Client{Transport: &redirectTransport{to: ts.URL}}),
	)

	results, err := p.Search(context.Background(), "golang", websearch.Options{MaxResults: 3})
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Equal(t, "Go Blog", results[0].Title)
	require.Equal(t, "https://go.dev/blog", results[0].URL)
	require.Equal(t, "Latest Go news", results[0].Snippet)
}

func TestProvider_Search_Non200Response(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = fmt.Fprint(w, "rate limited")
	}))
	defer ts.Close()

	p := New(
		WithAPIKey("test-key"),
		WithHTTPClient(&http.Client{Transport: &redirectTransport{to: ts.URL}}),
	)

	_, err := p.Search(context.Background(), "golang", websearch.Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "429")
	require.Contains(t, err.Error(), "rate limited")
}

func TestProvider_Search_InvalidJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, "{bad json}")
	}))
	defer ts.Close()

	p := New(
		WithAPIKey("test-key"),
		WithHTTPClient(&http.Client{Transport: &redirectTransport{to: ts.URL}}),
	)

	_, err := p.Search(context.Background(), "golang", websearch.Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "decode response")
}

func TestProvider_Search_MaxResultsClamped(t *testing.T) {
	var capturedMaxResults int

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body searchRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		capturedMaxResults = body.MaxResults

		w.Header().Set("Content-Type", "application/json")
		resp := searchResponse{Results: []searchResult{{Title: "t", URL: "u", Content: "c"}}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	p := New(
		WithAPIKey("test-key"),
		WithHTTPClient(&http.Client{Transport: &redirectTransport{to: ts.URL}}),
	)

	_, err := p.Search(context.Background(), "golang", websearch.Options{MaxResults: 99})
	require.NoError(t, err)
	require.Equal(t, 10, capturedMaxResults)
}
