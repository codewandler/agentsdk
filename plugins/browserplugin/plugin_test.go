//go:build integration

package browserplugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Interface compliance ──────────────────────────────────────────────────────

func TestPluginInterfaces(t *testing.T) {
	_ = app.Plugin(New())
	_ = app.ActionsPlugin(New())
	_ = app.CatalogToolsPlugin(New())
	_ = app.AgentContextPlugin(New())
}

// ── Unit tests (no Chrome needed) ─────────────────────────────────────────────

func TestPluginName(t *testing.T) {
	p := New()
	assert.Equal(t, "browser", p.Name())
}

func TestPluginActions(t *testing.T) {
	p := New()
	actions := p.Actions()
	require.Len(t, actions, 12)

	names := make([]string, len(actions))
	for i, a := range actions {
		names[i] = a.Spec().Name
	}
	assert.Contains(t, names, "browser.open")
	assert.Contains(t, names, "browser.navigate")
	assert.Contains(t, names, "browser.click")
	assert.Contains(t, names, "browser.type")
	assert.Contains(t, names, "browser.select")
	assert.Contains(t, names, "browser.read")
	assert.Contains(t, names, "browser.screenshot")
	assert.Contains(t, names, "browser.evaluate")
	assert.Contains(t, names, "browser.wait")
	assert.Contains(t, names, "browser.back")
	assert.Contains(t, names, "browser.forward")
	assert.Contains(t, names, "browser.close")
}

func TestPluginCatalogTools(t *testing.T) {
	p := New()
	tools := p.CatalogTools()
	require.Len(t, tools, 1)
	assert.Equal(t, "browser", tools[0].Name())
}

// ── Integration tests (require Chrome in PATH) ───────────────────────────────

func testServer() *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>Test Page</title></head>
<body>
  <h1>Hello Browser</h1>
  <nav>
    <a href="/about" id="about-link">About</a>
    <button id="btn-action" onclick="document.getElementById('output').textContent='clicked'">Click Me</button>
  </nav>
  <input type="text" id="search" placeholder="Search..." />
  <div id="output">initial</div>
</body></html>`)
	})
	mux.HandleFunc("/about", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, `<!DOCTYPE html>
<html><head><title>About</title></head>
<body><h1>About Page</h1><a href="/" id="home-link">Home</a></body></html>`)
	})
	return httptest.NewServer(mux)
}

func TestOpenAndClose(t *testing.T) {
	p := New(WithHeadless(true))
	defer p.Shutdown()

	out, err := p.executeOpen(nil, OpenInput{Headless: true})
	require.NoError(t, err)
	assert.NotEmpty(t, out.SessionID)

	// Session exists.
	_, err = p.sessions.Get(out.SessionID)
	require.NoError(t, err)

	// Close it.
	closeOut, err := p.executeClose(nil, CloseInput{SessionID: out.SessionID})
	require.NoError(t, err)
	assert.True(t, closeOut.Closed)

	// Session gone.
	_, err = p.sessions.Get(out.SessionID)
	assert.Error(t, err)
}

func TestNavigateAndRead(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	p := New(WithHeadless(true))
	defer p.Shutdown()

	openOut, err := p.executeOpen(nil, OpenInput{Headless: true})
	require.NoError(t, err)

	navOut, err := p.executeNavigate(nil, NavigateInput{SessionID: openOut.SessionID, URL: srv.URL})
	require.NoError(t, err)
	assert.Equal(t, "Test Page", navOut.Title)
	assert.Contains(t, navOut.URL, srv.URL)

	readOut, err := p.executeRead(nil, ReadInput{SessionID: openOut.SessionID, Selector: "h1"})
	require.NoError(t, err)
	assert.Equal(t, "Hello Browser", readOut.Text)
}

func TestClickAndEvaluate(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	p := New(WithHeadless(true))
	defer p.Shutdown()

	openOut, err := p.executeOpen(nil, OpenInput{Headless: true})
	require.NoError(t, err)
	sid := openOut.SessionID

	_, err = p.executeNavigate(nil, NavigateInput{SessionID: sid, URL: srv.URL})
	require.NoError(t, err)

	_, err = p.executeClick(nil, ClickInput{SessionID: sid, Selector: "#btn-action"})
	require.NoError(t, err)

	readOut, err := p.executeRead(nil, ReadInput{SessionID: sid, Selector: "#output"})
	require.NoError(t, err)
	assert.Equal(t, "clicked", readOut.Text)
}

func TestTypeAction(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	p := New(WithHeadless(true))
	defer p.Shutdown()

	openOut, err := p.executeOpen(nil, OpenInput{Headless: true})
	require.NoError(t, err)
	sid := openOut.SessionID

	_, err = p.executeNavigate(nil, NavigateInput{SessionID: sid, URL: srv.URL})
	require.NoError(t, err)

	_, err = p.executeType(nil, TypeInput{SessionID: sid, Selector: "#search", Text: "hello world"})
	require.NoError(t, err)

	evalOut, err := p.executeEvaluate(nil, EvaluateInput{
		SessionID:  sid,
		Expression: `document.getElementById("search").value`,
	})
	require.NoError(t, err)
	assert.Equal(t, "hello world", evalOut.Result)
}

func TestToolDispatch(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	p := New(WithHeadless(true))
	defer p.Shutdown()

	headless := true
	params := BrowserParams{
		Operations: []BrowserOperation{
			{Open: &OpenOp{Headless: &headless}},
			{Navigate: &NavigateOp{URL: srv.URL}},
			{Read: &ReadOp{Selector: "h1"}},
		},
	}

	result, err := p.executeBrowser(nil, params)
	require.NoError(t, err)
	require.NotNil(t, result)

	content := resultText(t, result)
	assert.Contains(t, content, "Hello Browser")
	assert.Contains(t, content, "session_id: sess_")
}

func TestContextProvider(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	p := New(WithHeadless(true))
	defer p.Shutdown()

	openOut, err := p.executeOpen(nil, OpenInput{Headless: true})
	require.NoError(t, err)

	_, err = p.executeNavigate(nil, NavigateInput{SessionID: openOut.SessionID, URL: srv.URL})
	require.NoError(t, err)

	provider := &browserContextProvider{sessions: p.sessions}
	ctx := provider.sessions.sessions[openOut.SessionID].browserCtx

	_ = ctx // just to confirm session is alive

	pc, err := provider.GetContext(nil, agentcontext.Request{})
	require.NoError(t, err)
	require.NotEmpty(t, pc.Fragments)

	content := pc.Fragments[0].Content
	assert.Contains(t, content, openOut.SessionID)
	assert.Contains(t, content, "Test Page")
	// Should contain some interactive elements.
	assert.Contains(t, content, "link")
}

func TestIdleReaper(t *testing.T) {
	p := New(WithHeadless(true), WithIdleTimeout(500*time.Millisecond))
	defer p.Shutdown()

	openOut, err := p.executeOpen(nil, OpenInput{Headless: true})
	require.NoError(t, err)

	// Wait for reaper to fire.
	time.Sleep(1500 * time.Millisecond)

	_, err = p.sessions.Get(openOut.SessionID)
	assert.Error(t, err, "session should have been reaped")
}

func TestErrorOnBadSelector(t *testing.T) {
	srv := testServer()
	defer srv.Close()

	p := New(WithHeadless(true))
	defer p.Shutdown()

	openOut, err := p.executeOpen(nil, OpenInput{Headless: true})
	require.NoError(t, err)

	_, err = p.executeNavigate(nil, NavigateInput{SessionID: openOut.SessionID, URL: srv.URL})
	require.NoError(t, err)

	_, err = p.executeWait(nil, WaitInput{
		SessionID: openOut.SessionID,
		Selector:  "#nonexistent",
		TimeoutMs: 500,
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "wait timeout")
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func resultText(t *testing.T, r tool.Result) string {
	t.Helper()
	raw, err := json.Marshal(r)
	require.NoError(t, err)
	return string(raw)
}

// Ensure action.Ctx accepts nil (context.Background is used internally).
var _ action.Ctx = (*nilCtx)(nil)

type nilCtx struct{}

func (*nilCtx) Deadline() (interface{}, bool) { return nil, false }
func (*nilCtx) Done() <-chan struct{}          { return nil }
func (*nilCtx) Err() error                    { return nil }
func (*nilCtx) Value(any) any                 { return nil }
