# DESIGN: Browser Control Plugin

Status: Draft  
Date: 2026-05-05

## Summary

A `plugins/browserplugin` that provides browser automation via the Chrome
DevTools Protocol (CDP), using `github.com/chromedp/chromedp` as the Go-native
CDP client. No Node.js, no Playwright, no Selenium — just the user's existing
Chrome/Chromium and a pure-Go WebSocket connection.

**Key architectural choice:** each browser operation is a core `action.Action`
(typed, reusable from YAML workflows and Go code). A single `browser` tool
wraps these actions behind a batched operations array (oneOf discriminated
union), following the `tools/phone` pattern.

## Goals

- Zero external runtime dependencies (pure Go, no sidecar processes)
- Leverage the user's installed Chrome/Chromium
- **Actions-first**: each operation is a standalone `action.Action`, reusable
  from the tool, YAML pipelines, and programmatic Go callers
- Single `browser` tool with batched operations (thin dispatch adapter)
- Maintain session state across invocations (session_id routing)
- Provide a **context provider** with a structured view of interactable elements
- Graceful lifecycle: idle timeout, explicit close, plugin-shutdown cleanup

## Non-Goals

- Cross-browser support (Firefox, Safari) — CDP is Chromium-only
- Full test-framework features (assertions, retries, test reporters)
- Downloading/managing browser binaries (user provides Chrome)
- Replacing `web_fetch` — this is for interactive pages, not simple HTTP GETs

---

## Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                         browserplugin                              │
├───────────────────────────────────────────────────────────────────┤
│  Plugin (Plugin + ActionsPlugin + CatalogToolsPlugin +            │
│          AgentContextPlugin)                                      │
│                                                                   │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │  Core Actions (action.Action)                               │  │
│  │    browser.open, browser.navigate, browser.click,           │  │
│  │    browser.type, browser.select, browser.read,              │  │
│  │    browser.screenshot, browser.evaluate, browser.wait,      │  │
│  │    browser.back, browser.forward, browser.close             │  │
│  └──────────────────────────┬──────────────────────────────────┘  │
│                             │                                     │
│                      called by both                                │
│                      ┌──────┴──────┐                              │
│                      ▼             ▼                               │
│  ┌──────────────────────┐  ┌─────────────────────────────┐       │
│  │  "browser" tool      │  │  YAML workflows / Go code   │       │
│  │  (operations array)  │  │  (direct action invocation)  │       │
│  └──────────────────────┘  └─────────────────────────────┘       │
│                                                                   │
│  SessionManager (stateful, thread-safe)                           │
│  ContextProvider (page element tree)                              │
└───────────────────────────┬───────────────────────────────────────┘
                            │ WebSocket (CDP)
                            ▼
┌───────────────────────────────────────────────────────────────────┐
│  Chrome / Chromium                                                │
│  (launched headless by plugin, or attached via ws URL)            │
└───────────────────────────────────────────────────────────────────┘
```

### Plugin Interface Compliance

```go
var (
    _ app.Plugin              = (*Plugin)(nil)
    _ app.ActionsPlugin       = (*Plugin)(nil)
    _ app.CatalogToolsPlugin  = (*Plugin)(nil)
    _ app.AgentContextPlugin  = (*Plugin)(nil)
)
```

---

## Core Actions (action.Action layer)

Each browser operation is implemented as a standalone `action.Action` registered
via `ActionsPlugin.Actions()`. These are the **reusable primitives** — callable
from the tool, from YAML workflow steps, or programmatically.

### Action Catalog

| Action Name | Input Type | Output Type | Description |
|-------------|-----------|-------------|-------------|
| `browser.open` | `OpenInput` | `OpenOutput` | Launch/attach browser, create session |
| `browser.navigate` | `NavigateInput` | `NavigateOutput` | Navigate to URL, wait for load |
| `browser.click` | `ClickInput` | `ClickOutput` | Click element by selector |
| `browser.type` | `TypeInput` | `TypeOutput` | Type text into input element |
| `browser.select` | `SelectInput` | `SelectOutput` | Select option(s) in dropdown |
| `browser.read` | `ReadInput` | `ReadOutput` | Extract text from page/element |
| `browser.screenshot` | `ScreenshotInput` | `ScreenshotOutput` | Capture viewport/element as PNG |
| `browser.evaluate` | `EvaluateInput` | `EvaluateOutput` | Execute JS, return result |
| `browser.wait` | `WaitInput` | `WaitOutput` | Wait for selector/condition |
| `browser.back` | `HistoryInput` | `HistoryOutput` | Navigate back |
| `browser.forward` | `HistoryInput` | `HistoryOutput` | Navigate forward |
| `browser.close` | `CloseInput` | `CloseOutput` | Destroy session |

### Action Input/Output Types

Every action input carries `SessionID` explicitly, making actions self-contained
and independently invocable (no ambient state required from the tool layer).

```go
type NavigateInput struct {
    SessionID string `json:"session_id"`
    URL       string `json:"url"`
}

type NavigateOutput struct {
    SessionID string `json:"session_id"`
    Title     string `json:"title"`
    URL       string `json:"url"`
    Status    int    `json:"status"`
}

type ClickInput struct {
    SessionID string `json:"session_id"`
    Selector  string `json:"selector"`
}

type ClickOutput struct {
    Navigated bool   `json:"navigated,omitempty"`
    NewURL    string `json:"new_url,omitempty"`
}

// ... same pattern for all actions
```

### Action Registration

```go
func (p *Plugin) Actions() []action.Action {
    return []action.Action{
        action.NewTyped(action.Spec{
            Name:        "browser.open",
            Description: "Open a new browser session.",
        }, p.executeOpen),

        action.NewTyped(action.Spec{
            Name:        "browser.navigate",
            Description: "Navigate to a URL and wait for page load.",
        }, p.executeNavigate),

        action.NewTyped(action.Spec{
            Name:        "browser.click",
            Description: "Click an element by CSS selector.",
        }, p.executeClick),

        // ... one per operation
    }
}
```

### YAML Workflow Usage

Because actions are registered in the app's action registry, they become
addressable in declarative workflows:

```yaml
steps:
  - action: browser.open
    input:
      headless: true
    output: session

  - action: browser.navigate
    input:
      session_id: "{{ session.session_id }}"
      url: "https://example.com/status"

  - action: browser.read
    input:
      session_id: "{{ session.session_id }}"
      selector: ".deploy-status"
    output: status_text

  - action: browser.close
    input:
      session_id: "{{ session.session_id }}"
```

---

## Tool Design: Single `browser` Tool with Operations

The tool is a **thin dispatch adapter** over the action layer. It handles
session ID resolution, sequential dispatch, and result aggregation. Zero browser
logic lives here.

### Top-Level Params

```go
// BrowserParams is the top-level input for the browser tool.
type BrowserParams struct {
    SessionID  string             `json:"session_id,omitempty" jsonschema:"description=Browser session ID. Omit on first call (open creates one). Required for all other operations."`
    Operations []BrowserOperation `json:"operations" jsonschema:"description=Browser operations to perform in sequence.,required"`
}
```

### Discriminated Union (oneOf)

```go
// BrowserOperation is a discriminated union — exactly one field must be set.
type BrowserOperation struct {
    Open       *OpenOp       `json:"open,omitempty"       jsonschema:"description=Open a new browser session."`
    Navigate   *NavigateOp   `json:"navigate,omitempty"   jsonschema:"description=Navigate to a URL."`
    Click      *ClickOp      `json:"click,omitempty"      jsonschema:"description=Click an element."`
    Type       *TypeOp       `json:"type,omitempty"       jsonschema:"description=Type text into an element."`
    Select     *SelectOp     `json:"select,omitempty"     jsonschema:"description=Select option(s) in a select element."`
    Read       *ReadOp       `json:"read,omitempty"       jsonschema:"description=Read text content from the page or an element."`
    Screenshot *ScreenshotOp `json:"screenshot,omitempty" jsonschema:"description=Take a screenshot of the page or an element."`
    Evaluate   *EvaluateOp   `json:"evaluate,omitempty"   jsonschema:"description=Execute JavaScript in the page context."`
    Wait       *WaitOp       `json:"wait,omitempty"       jsonschema:"description=Wait for an element or condition."`
    Back       *BackOp       `json:"back,omitempty"       jsonschema:"description=Navigate back in history."`
    Forward    *ForwardOp    `json:"forward,omitempty"    jsonschema:"description=Navigate forward in history."`
    Close      *CloseOp      `json:"close,omitempty"      jsonschema:"description=Close the browser session."`
}
```

### Operation Structs

These are the JSON-facing types for the tool schema. Each maps 1:1 to a core
action's input (minus `session_id`, which the tool injects from the top-level
param).

```go
type OpenOp struct {
    Headless bool `json:"headless,omitempty" jsonschema:"description=Run browser in headless mode (default true)."`
}

type NavigateOp struct {
    URL string `json:"url" jsonschema:"description=URL to navigate to.,required"`
}

type ClickOp struct {
    Selector string `json:"selector" jsonschema:"description=CSS selector of the element to click.,required"`
}

type TypeOp struct {
    Selector string `json:"selector" jsonschema:"description=CSS selector of the input element.,required"`
    Text     string `json:"text"     jsonschema:"description=Text to type.,required"`
    Clear    bool   `json:"clear,omitempty"  jsonschema:"description=Clear the field before typing."`
    Submit   bool   `json:"submit,omitempty" jsonschema:"description=Press Enter after typing."`
}

type SelectOp struct {
    Selector string   `json:"selector" jsonschema:"description=CSS selector of the select element.,required"`
    Values   []string `json:"values"   jsonschema:"description=Option value(s) to select.,required"`
}

type ReadOp struct {
    Selector string `json:"selector,omitempty" jsonschema:"description=CSS selector to read from. Omit for full page text."`
}

type ScreenshotOp struct {
    Selector string `json:"selector,omitempty"  jsonschema:"description=CSS selector to screenshot. Omit for full viewport."`
    FullPage bool   `json:"full_page,omitempty" jsonschema:"description=Capture the full scrollable page."`
}

type EvaluateOp struct {
    Expression string `json:"expression" jsonschema:"description=JavaScript expression to evaluate.,required"`
}

type WaitOp struct {
    Selector  string `json:"selector,omitempty"   jsonschema:"description=CSS selector to wait for."`
    TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"description=Max wait time in milliseconds (default 5000)."`
}

type BackOp struct{}

type ForwardOp struct{}

type CloseOp struct{}
```

### Tool → Action Dispatch

```go
func (p *Plugin) executeBrowser(ctx tool.Ctx, params BrowserParams) (tool.Result, error) {
    if len(params.Operations) == 0 {
        return nil, fmt.Errorf("at least one operation is required")
    }

    session, err := p.sessions.Resolve(params.SessionID, params.Operations)
    if err != nil {
        return nil, err
    }

    actx := actionCtxFrom(ctx, session)

    rb := tool.NewResult()
    for i, op := range params.Operations {
        result := p.dispatchToAction(actx, session, op)
        if result.IsError() {
            rb.WithError()
            rb.Textf("operation[%d] error: %v", i, result.Err())
            break
        }
        if len(params.Operations) > 1 {
            rb.Textf("## [%d] %s", i+1, opName(op))
        }
        rb.Text(formatActionResult(result))
    }
    rb.Textf("\nsession_id: %s", session.ID)
    return rb.Build(), nil
}

func (p *Plugin) dispatchToAction(ctx action.Ctx, session *Session, op BrowserOperation) action.Result {
    switch {
    case op.Open != nil:
        return p.openAction.Execute(ctx, OpenInput{Headless: op.Open.Headless})
    case op.Navigate != nil:
        return p.navigateAction.Execute(ctx, NavigateInput{
            SessionID: session.ID, URL: op.Navigate.URL,
        })
    case op.Click != nil:
        return p.clickAction.Execute(ctx, ClickInput{
            SessionID: session.ID, Selector: op.Click.Selector,
        })
    // ... etc
    default:
        return action.Failed(fmt.Errorf("operation must have exactly one field set"))
    }
}
```

Operations execute **sequentially**. The agent can batch related steps
(navigate + wait + read) in one round-trip to reduce turn count.

### Example Invocations

**First call — open + navigate:**
```json
{
  "operations": [
    {"open": {"headless": true}},
    {"navigate": {"url": "https://example.com/dashboard"}}
  ]
}
```

**Subsequent call — interact:**
```json
{
  "session_id": "sess_7f3a",
  "operations": [
    {"click": {"selector": "a.env-link[data-env=\"prod\"]"}},
    {"wait": {"selector": ".deploy-status"}},
    {"read": {"selector": ".deploy-status"}}
  ]
}
```

**Close:**
```json
{
  "session_id": "sess_7f3a",
  "operations": [{"close": {}}]
}
```

### Error Semantics

On first error, execution **stops**. Partial results up to the failure are
returned.

| Condition | Behavior |
|-----------|----------|
| No session_id + no `open` op | Error: "session_id required or first operation must be open" |
| Session not found | Error: "session not found: {id}" |
| Selector not found | Error: "element not found: {selector}" |
| Navigation timeout | Error: "navigation timeout: {url}" |
| Chrome crashed | Error + mark session dead + cleanup |
| Multiple `open` ops | Error: "open must be the first and only open operation" |

---

## Session Management

### Session struct

```go
type Session struct {
    ID          string
    allocCtx    context.Context
    allocCancel context.CancelFunc
    browserCtx  context.Context
    cancelFn    context.CancelFunc
    createdAt   time.Time
    lastUsedAt  time.Time
    mu          sync.Mutex
}
```

### SessionManager

```go
type SessionManager struct {
    mu       sync.Mutex
    sessions map[string]*Session
    config   Config
    reaper   *time.Ticker
}

type Config struct {
    Mode        Mode          // Launch | Attach
    RemoteURL   string        // ws://localhost:9222 (attach mode)
    ChromePath  string        // explicit path, or auto-detect
    Headless    bool          // default: true
    IdleTimeout time.Duration // default: 10m
    MaxSessions int           // default: 3
}
```

### Resolve Logic

```go
func (m *SessionManager) Resolve(sessionID string, ops []BrowserOperation) (*Session, error) {
    // If first op is Open → create new session (sessionID must be empty)
    // If sessionID provided → look up existing session
    // Otherwise → error
}
```

### Lifecycle

| Event | Behavior |
|-------|----------|
| `open` operation | Create session, return ID in output |
| Operations with `session_id` | Route to existing session, update `lastUsedAt` |
| `close` operation | Tear down session, kill Chrome context |
| Idle timeout fires | Auto-close, log warning |
| Plugin shutdown | Close all sessions |

---

## Context Provider: Interactable Elements Tree

The plugin implements `app.AgentContextPlugin` and returns a provider that
injects a compact page-element tree when a session is active.

### Provider Identity

```go
func (p *browserContextProvider) Key() agentcontext.ProviderKey {
    return "browser"
}
```

### Extraction Strategy

1. **Accessibility tree** — `Accessibility.getFullAXTree` via CDP gives
   semantic roles, names, states without DOM traversal.
2. **Resolve selectors** — for each interactive node, compute a minimal stable
   selector: prefer `#id` → `[aria-label="..."]` → shortest unique CSS path.
3. **Filter** — keep interactive roles: `link`, `button`, `textbox`,
   `combobox`, `checkbox`, `radio`, `menuitem`, `tab`, `searchbox`. Include
   headings for structural context.
4. **Truncate** — cap at 80 elements, viewport-visible first. Add
   `(+N more below fold)` when truncated.

### Output Format

```
[browser: sess_7f3a | https://example.com/dashboard | "Dashboard - App"]

nav#main-nav
  [1] link "Home" → a@href="/"
  [2] link "Settings" → a@href="/settings"
  [3] button "Notifications" → button#notif-btn

main
  [4] searchbox "Search..." → input#search-box
  [5] button "Search" → button.search-submit
  [6] link "Item 1" → a@href="/items/1"
  [7] link "Item 2" → a@href="/items/2"
  [8] button "Load more" → button.load-more

footer
  [9] link "Help" → a@href="/help"
  [10] link "Logout" → a@href="/logout"

(+12 more below fold)
```

Numbered indices are visual aids. The selector after `→` is what the agent
passes to `click`, `type`, `read`, etc.

### Fingerprinting

Implements `agentcontext.FingerprintingProvider` to skip re-rendering when the
page hasn't changed between turns:

```go
func (p *browserContextProvider) StateFingerprint(ctx context.Context, req Request) (string, bool, error) {
    // Hash of: session_id + active URL + DOM element count
    // Cheap CDP call: Runtime.evaluate("document.URL + document.querySelectorAll('*').length")
}
```

### Behavior Matrix

| Condition | Context output |
|-----------|---------------|
| No active session | Empty (nothing injected) |
| Session exists, page loaded | Element tree |
| Session exists, page loading | `[browser: sess_7f3a | loading...]` |
| Multiple sessions | MRU session tree + one-line summary of others |

---

## Chrome Discovery & Launch

### Auto-detection (launch mode)

1. `Config.ChromePath` if set
2. `CHROME_PATH` env var
3. chromedp's built-in detection (well-known paths per OS)

### Launch flags

```go
opts := append(chromedp.DefaultExecAllocatorOptions[:],
    chromedp.Flag("headless", true),
    chromedp.Flag("disable-gpu", true),
    chromedp.Flag("no-first-run", true),
    chromedp.Flag("disable-extensions", true),
    chromedp.Flag("disable-default-apps", true),
    chromedp.WindowSize(1280, 720),
)
```

### Attach mode

```go
allocCtx, cancel := chromedp.NewRemoteAllocator(ctx, config.RemoteURL)
```

User starts Chrome: `google-chrome --remote-debugging-port=9222`

---

## Implementation Plan

### Phase 1: Core (MVP)

```
plugins/browserplugin/
├── plugin.go            // Plugin struct, Name(), Actions(), CatalogTools(), AgentContextProviders()
├── config.go            // Config, Mode, defaults, chrome detection
├── session.go           // Session, SessionManager, lifecycle, reaper
│
│   # Core actions (grouped by domain, not one-per-file)
├── actions.go           // Action registration (Actions() method, action.NewTyped calls)
├── actions_nav.go       // open, navigate, back, forward, close implementations
├── actions_interact.go  // click, type, select, wait implementations
├── actions_read.go      // read, screenshot, evaluate implementations
│
│   # Tool adapter (delegates to actions)
├── tool.go              // "browser" tool: BrowserParams, BrowserOperation, dispatch loop
│
│   # Context provider
├── context_provider.go  // AgentContextPlugin impl, element tree extraction
├── elements.go          // Accessibility tree → compact representation
│
└── plugin_test.go       // Integration tests (require Chrome in PATH)
```

Each `actions_*.go` file groups 3-4 related action implementations (~80-100
lines per file). If any single action grows complex, it can be split out later.

### Phase 2: Robustness

- **Tab management** — `browser.new_tab`, `browser.switch_tab`, `browser.list_tabs`
- **Network event capture** — surface failed requests in action output
- **Cookie/storage** — `browser.get_cookies`, `browser.set_cookie`
- **Download detection** — capture file downloads, return path
- **Scroll** — `browser.scroll` (up/down/to element)

### Phase 3: Agent UX Polish

- **Auto-summary** — navigate output includes title + meta description
- **Selector suggestions on failure** — return nearby candidates from a11y tree
- **Form detection** — `browser.fill_form` composite action: `{fields: {name: value}}`

---

## Dependencies

| Package | Purpose | Weight |
|---------|---------|--------|
| `github.com/chromedp/chromedp` | CDP client, browser lifecycle | Pure Go, ~3 packages |
| `github.com/chromedp/cdproto` | Generated CDP protocol types | Auto-pulled |

No CGO. No external binaries. Static binary.

---

## Security Considerations

| Risk | Mitigation |
|------|-----------|
| CDP = full browser control | Sessions auto-expire; MaxSessions cap |
| Remote debugging port | Bind `127.0.0.1` only (chromedp default) |
| Arbitrary JS via `evaluate` | Opt-in; hosts can exclude from tool guidance |
| Data exfiltration via screenshots | Temp dir, cleaned on session close |
| Resource exhaustion | MaxSessions=3, idle reaper, process kill on shutdown |
| Sandbox escape | `--no-sandbox` only when explicitly configured |

---

## Integration with Existing Ecosystem

| Component | Relationship |
|-----------|-------------|
| `visionplugin` | `screenshot` produces PNG path → agent passes to `vision` tool |
| `web_fetch` | Complementary: `web_fetch` for static/API; `browser` for JS-rendered pages |
| `toolmgmt` | Browser is a **catalog tool** — agent activates when needed |
| `tools/phone` | **Pattern reference** — same single-tool + operations + registry design |
| `action` package | Browser ops are first-class actions, registered in app action registry |
| YAML workflows | Actions addressable as `browser.*` steps in declarative pipelines |

---

## Decisions

| # | Question | Decision |
|---|----------|----------|
| 1 | Internal architecture | **Actions-first**: each operation is a core `action.Action`. Tool is a thin dispatch adapter. |
| 2 | Tool surface | Single `browser` tool with `operations[]` discriminated union (phone pattern) |
| 3 | Why actions? | Reusable from YAML workflows, programmatic Go, and the tool. Single implementation, multiple surfaces. |
| 4 | File organization | Group actions by domain (nav, interact, read) — not one file per action. Split only when complexity demands. |
| 5 | Launch vs attach | Support both. Default: launch-headless. Attach via config. |
| 6 | Selector strategy | `#id` → `[aria-label]` → shortest unique CSS. No XPath. |
| 7 | Context budget | 80 interactable elements, viewport-first. `read` op for deep-dive. |
| 8 | Multi-tab in context | Active tab only + tab list header. |
| 9 | Auth flows | Agent composes from primitives. No magic. |
| 10 | Default vs catalog | **Catalog** — opt-in. Browser is heavy. |
| 11 | Error handling in batch | Stop on first error, return partial results. |
| 12 | Session creation | Explicit `open` operation (not auto-create). |
| 13 | Plugin interfaces | `ActionsPlugin` + `CatalogToolsPlugin` + `AgentContextPlugin` |

---

## Example Agent Interaction

```
Agent: I need to check the deployment status on our internal dashboard.

→ browser({operations: [{open: {headless: true}}, {navigate: {url: "https://internal.example.com/deploy"}}]})
← ## [1] open
  Session created.
  ## [2] navigate
  Navigated to "Deployments" (https://internal.example.com/deploy, status 200)

  session_id: sess_7f3a

[Context provider injects element tree into next turn's context]

Agent sees in context:
  [browser: sess_7f3a | https://internal.example.com/deploy | "Deployments"]
  [1] link "Production" → a.env-link[data-env="prod"]
  [2] link "Staging" → a.env-link[data-env="staging"]
  [3] button "Refresh" → button#refresh

→ browser({session_id: "sess_7f3a", operations: [{click: {selector: "a.env-link[data-env=\"prod\"]"}}, {wait: {selector: ".deploy-status"}}, {read: {selector: ".deploy-status"}}]})
← ## [1] click
  Clicked a.env-link[data-env="prod"]. Page navigated to /deploy/prod.
  ## [2] wait
  Element .deploy-status visible.
  ## [3] read
  Production: v2.3.1 healthy (3/3 replicas)

  session_id: sess_7f3a

→ browser({session_id: "sess_7f3a", operations: [{close: {}}]})
← Session sess_7f3a closed.
```
