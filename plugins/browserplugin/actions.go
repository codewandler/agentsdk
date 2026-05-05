package browserplugin

import "github.com/codewandler/agentsdk/action"

// ── Action input/output types ─────────────────────────────────────────────────

// OpenInput is the input for browser.open.
type OpenInput struct {
	Headless bool `json:"headless,omitempty"`
}

// OpenOutput is the output for browser.open.
type OpenOutput struct {
	SessionID string `json:"session_id"`
}

// NavigateInput is the input for browser.navigate.
type NavigateInput struct {
	SessionID string `json:"session_id"`
	URL       string `json:"url"`
}

// NavigateOutput is the output for browser.navigate.
type NavigateOutput struct {
	SessionID string `json:"session_id"`
	Title     string `json:"title"`
	URL       string `json:"url"`
}

// ClickInput is the input for browser.click.
type ClickInput struct {
	SessionID string `json:"session_id"`
	Selector  string `json:"selector"`
}

// ClickOutput is the output for browser.click.
type ClickOutput struct {
	Navigated bool   `json:"navigated,omitempty"`
	NewURL    string `json:"new_url,omitempty"`
}

// TypeInput is the input for browser.type.
type TypeInput struct {
	SessionID string `json:"session_id"`
	Selector  string `json:"selector"`
	Text      string `json:"text"`
	Clear     bool   `json:"clear,omitempty"`
	Submit    bool   `json:"submit,omitempty"`
}

// TypeOutput is the output for browser.type.
type TypeOutput struct{}

// SelectInput is the input for browser.select.
type SelectInput struct {
	SessionID string   `json:"session_id"`
	Selector  string   `json:"selector"`
	Values    []string `json:"values"`
}

// SelectOutput is the output for browser.select.
type SelectOutput struct{}

// ReadInput is the input for browser.read.
type ReadInput struct {
	SessionID string `json:"session_id"`
	Selector  string `json:"selector,omitempty"`
}

// ReadOutput is the output for browser.read.
type ReadOutput struct {
	Text string `json:"text"`
}

// ScreenshotInput is the input for browser.screenshot.
type ScreenshotInput struct {
	SessionID string `json:"session_id"`
	Selector  string `json:"selector,omitempty"`
	FullPage  bool   `json:"full_page,omitempty"`
}

// ScreenshotOutput is the output for browser.screenshot.
type ScreenshotOutput struct {
	Path string `json:"path"`
}

// EvaluateInput is the input for browser.evaluate.
type EvaluateInput struct {
	SessionID  string `json:"session_id"`
	Expression string `json:"expression"`
}

// EvaluateOutput is the output for browser.evaluate.
type EvaluateOutput struct {
	Result any `json:"result"`
}

// WaitInput is the input for browser.wait.
type WaitInput struct {
	SessionID string `json:"session_id"`
	Selector  string `json:"selector,omitempty"`
	TimeoutMs int    `json:"timeout_ms,omitempty"`
}

// WaitOutput is the output for browser.wait.
type WaitOutput struct{}

// HistoryInput is the input for browser.back and browser.forward.
type HistoryInput struct {
	SessionID string `json:"session_id"`
}

// HistoryOutput is the output for browser.back and browser.forward.
type HistoryOutput struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// CloseInput is the input for browser.close.
type CloseInput struct {
	SessionID string `json:"session_id"`
}

// CloseOutput is the output for browser.close.
type CloseOutput struct {
	Closed bool `json:"closed"`
}

// ── Action registration ───────────────────────────────────────────────────────

// actions returns all browser actions. Called once during plugin construction.
func (p *Plugin) actions() []action.Action {
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

		action.NewTyped(action.Spec{
			Name:        "browser.type",
			Description: "Type text into an input element.",
		}, p.executeType),

		action.NewTyped(action.Spec{
			Name:        "browser.select",
			Description: "Select option(s) in a dropdown element.",
		}, p.executeSelect),

		action.NewTyped(action.Spec{
			Name:        "browser.read",
			Description: "Read text content from the page or a specific element.",
		}, p.executeRead),

		action.NewTyped(action.Spec{
			Name:        "browser.screenshot",
			Description: "Take a screenshot of the page or a specific element.",
		}, p.executeScreenshot),

		action.NewTyped(action.Spec{
			Name:        "browser.evaluate",
			Description: "Execute JavaScript in the page context and return the result.",
		}, p.executeEvaluate),

		action.NewTyped(action.Spec{
			Name:        "browser.wait",
			Description: "Wait for an element to become visible.",
		}, p.executeWait),

		action.NewTyped(action.Spec{
			Name:        "browser.back",
			Description: "Navigate back in browser history.",
		}, p.executeBack),

		action.NewTyped(action.Spec{
			Name:        "browser.forward",
			Description: "Navigate forward in browser history.",
		}, p.executeForward),

		action.NewTyped(action.Spec{
			Name:        "browser.close",
			Description: "Close a browser session and release resources.",
		}, p.executeClose),
	}
}
