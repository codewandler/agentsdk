package web

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codewandler/core/tool"
	"github.com/codewandler/flai/core/websearch"
)

// ── helpers ───────────────────────────────────────────────────────────────────

type testCtx struct{ context.Context }

func (c *testCtx) WorkDir() string       { return "." }
func (c *testCtx) AgentID() string       { return "test" }
func (c *testCtx) SessionID() string     { return "test" }
func (c *testCtx) Extra() map[string]any { return nil }

func tctx() tool.Ctx { return &testCtx{Context: context.Background()} }

func toJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

// ── web_fetch tests ───────────────────────────────────────────────────────────

func TestWebFetch_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("hello world"))
	}))
	defer srv.Close()

	tl := webFetch()
	res, err := tl.Execute(tctx(), toJSON(t, WebFetchParams{URL: srv.URL}))
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "hello world")
	require.Contains(t, res.String(), "200")
}

func TestWebFetch_HTMLConvertedToMarkdown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body><h1>Title</h1><p>A paragraph.</p></body></html>"))
	}))
	defer srv.Close()

	tl := webFetch()
	res, err := tl.Execute(tctx(), toJSON(t, WebFetchParams{URL: srv.URL}))
	require.NoError(t, err)
	require.False(t, res.IsError())
	// HTML should be converted — heading content must be present
	require.Contains(t, res.String(), "Title")
}

func TestWebFetch_CustomHeaders(t *testing.T) {
	var receivedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tl := webFetch()
	res, err := tl.Execute(tctx(), toJSON(t, WebFetchParams{
		URL:     srv.URL,
		Headers: map[string]string{"Authorization": "Bearer token123"},
	}))
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, "Bearer token123", receivedAuth)
}

func TestWebFetch_PostWithBody(t *testing.T) {
	var receivedBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		b, _ := io.ReadAll(r.Body)
		receivedBody = string(b)
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("created"))
	}))
	defer srv.Close()

	tl := webFetch()
	res, err := tl.Execute(tctx(), toJSON(t, WebFetchParams{
		URL:    srv.URL,
		Method: "POST",
		Body:   `{"name":"test"}`,
	}))
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Equal(t, `{"name":"test"}`, receivedBody)
}

func TestWebFetch_FollowsRedirect(t *testing.T) {
	var finalHit bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			finalHit = true
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("arrived"))
			return
		}
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer srv.Close()

	tl := webFetch()
	res, err := tl.Execute(tctx(), toJSON(t, WebFetchParams{URL: srv.URL + "/start"}))
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.True(t, finalHit)
	require.Contains(t, res.String(), "arrived")
}

func TestWebFetch_EmptyURLReturnsError(t *testing.T) {
	tl := webFetch()
	res, err := tl.Execute(tctx(), toJSON(t, WebFetchParams{URL: ""}))
	require.NoError(t, err)
	require.True(t, res.IsError())
}

func TestWebFetch_NonHTTPSchemeReturnsError(t *testing.T) {
	tl := webFetch()
	res, err := tl.Execute(tctx(), toJSON(t, WebFetchParams{URL: "ftp://example.com/file"}))
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, strings.ToLower(res.String()), "http")
}

func TestWebFetch_Non2xxStatusNotAnError(t *testing.T) {
	// web_fetch returns the response even for 4xx — callers inspect the status field.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	tl := webFetch()
	res, err := tl.Execute(tctx(), toJSON(t, WebFetchParams{URL: srv.URL}))
	require.NoError(t, err)
	require.False(t, res.IsError(), "non-2xx status should not be a tool error")
	require.Contains(t, res.String(), "404")
}

// ── web_search tests ──────────────────────────────────────────────────────────

func TestWebSearch_ReturnsFormattedResults(t *testing.T) {
	provider := &stubProvider{results: []websearch.Result{
		{Title: "Go Tutorial", URL: "https://go.dev/doc", Snippet: "Learn Go"},
		{Title: "Effective Go", URL: "https://go.dev/doc/effective_go", Snippet: "Best practices"},
	}}

	tl := webSearch(provider)
	res, err := tl.Execute(tctx(), toJSON(t, WebSearchParams{Query: "golang tutorial", MaxResults: 2}))
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	require.Contains(t, output, "Go Tutorial")
	require.Contains(t, output, "https://go.dev/doc")
	require.Contains(t, output, "Effective Go")
}

func TestWebSearch_EmptyQueryReturnsError(t *testing.T) {
	tl := webSearch(&stubProvider{})
	res, err := tl.Execute(tctx(), toJSON(t, WebSearchParams{Query: ""}))
	require.NoError(t, err)
	require.True(t, res.IsError())
}

func TestWebSearch_NoResultsGraceful(t *testing.T) {
	tl := webSearch(&stubProvider{results: nil})
	res, err := tl.Execute(tctx(), toJSON(t, WebSearchParams{Query: "xyzzy"}))
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Contains(t, strings.ToLower(res.String()), "no results")
}

func TestTools_NilProviderOmitsWebSearch(t *testing.T) {
	tools := Tools(nil)
	require.Len(t, tools, 1)
	require.Equal(t, "web_fetch", tools[0].Name())
}

func TestTools_WithProviderIncludesWebSearch(t *testing.T) {
	tools := Tools(&stubProvider{})
	require.Len(t, tools, 2)
	names := []string{tools[0].Name(), tools[1].Name()}
	require.Contains(t, names, "web_fetch")
	require.Contains(t, names, "web_search")
}

// ── stub provider ─────────────────────────────────────────────────────────────

type stubProvider struct {
	results []websearch.Result
	err     error
}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) Search(_ context.Context, _ string, _ websearch.SearchOptions) ([]websearch.Result, error) {
	return s.results, s.err
}
