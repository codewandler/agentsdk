package browserplugin

import (
	"context"
	"fmt"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/tool"
)

// ── Tool-level parameter types ────────────────────────────────────────────────

// BrowserParams is the top-level input for the browser tool.
type BrowserParams struct {
	SessionID  string             `json:"session_id,omitempty" jsonschema:"description=Browser session ID. Omit on first call (open creates one). Required for all other operations."`
	Operations []BrowserOperation `json:"operations" jsonschema:"description=Browser operations to perform in sequence.,required"`
}

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

// ── Operation structs ─────────────────────────────────────────────────────────

// OpenOp opens a new browser session.
type OpenOp struct {
	Headless *bool `json:"headless,omitempty" jsonschema:"description=Run browser in headless mode (default true)."`
}

// NavigateOp navigates to a URL.
type NavigateOp struct {
	URL string `json:"url" jsonschema:"description=URL to navigate to.,required"`
}

// ClickOp clicks an element.
type ClickOp struct {
	Selector string `json:"selector" jsonschema:"description=CSS selector of the element to click.,required"`
}

// TypeOp types text into an element.
type TypeOp struct {
	Selector string `json:"selector" jsonschema:"description=CSS selector of the input element.,required"`
	Text     string `json:"text"     jsonschema:"description=Text to type.,required"`
	Clear    bool   `json:"clear,omitempty"  jsonschema:"description=Clear the field before typing."`
	Submit   bool   `json:"submit,omitempty" jsonschema:"description=Press Enter after typing."`
}

// SelectOp selects option(s) in a dropdown.
type SelectOp struct {
	Selector string   `json:"selector" jsonschema:"description=CSS selector of the select element.,required"`
	Values   []string `json:"values"   jsonschema:"description=Option value(s) to select.,required"`
}

// ReadOp reads text content.
type ReadOp struct {
	Selector string `json:"selector,omitempty" jsonschema:"description=CSS selector to read from. Omit for full page text."`
}

// ScreenshotOp takes a screenshot.
type ScreenshotOp struct {
	Selector string `json:"selector,omitempty"  jsonschema:"description=CSS selector to screenshot. Omit for full viewport."`
	FullPage bool   `json:"full_page,omitempty" jsonschema:"description=Capture the full scrollable page."`
}

// EvaluateOp executes JavaScript.
type EvaluateOp struct {
	Expression string `json:"expression" jsonschema:"description=JavaScript expression to evaluate.,required"`
}

// WaitOp waits for an element.
type WaitOp struct {
	Selector  string `json:"selector,omitempty"   jsonschema:"description=CSS selector to wait for."`
	TimeoutMs int    `json:"timeout_ms,omitempty" jsonschema:"description=Max wait time in milliseconds (default 5000)."`
}

// BackOp navigates back.
type BackOp struct{}

// ForwardOp navigates forward.
type ForwardOp struct{}

// CloseOp closes the session.
type CloseOp struct{}

// ── Tool construction ─────────────────────────────────────────────────────────

func (p *Plugin) browserTool() tool.Tool {
	return tool.New("browser",
		"Control a browser via Chrome DevTools Protocol. Supports batched operations: open, navigate, click, type, select, read, screenshot, evaluate, wait, back, forward, close.",
		func(ctx tool.Ctx, params BrowserParams) (tool.Result, error) {
			return p.executeBrowser(ctx, params)
		},
		tool.WithGuidance[BrowserParams](
			"Each element in the operations array must have EXACTLY ONE field set (e.g. {\"navigate\": {\"url\": \"...\"}}).\n"+
				"Do NOT set multiple operation fields in the same array element.\n"+
				"The first call should include an {\"open\": {}} operation. Subsequent calls reuse the returned session_id.\n"+
				"Always close sessions when done to free resources.\n"+
				"Example: {\"operations\": [{\"open\": {}}, {\"navigate\": {\"url\": \"https://example.com\"}}, {\"read\": {\"selector\": \"h1\"}}]}"),
	)
}

// ── Dispatch logic ────────────────────────────────────────────────────────────

func (p *Plugin) executeBrowser(_ tool.Ctx, params BrowserParams) (tool.Result, error) {
	if len(params.Operations) == 0 {
		return nil, fmt.Errorf("at least one operation is required")
	}

	// Validate: each operation must have exactly one field set.
	for i, op := range params.Operations {
		if n := opFieldCount(op); n == 0 {
			return nil, fmt.Errorf("operation[%d]: no operation field set", i)
		} else if n > 1 {
			return nil, fmt.Errorf("operation[%d]: exactly one operation field must be set, got %d (set only one of: open, navigate, click, type, select, read, screenshot, evaluate, wait, back, forward, close)", i, n)
		}
	}

	// Determine if first op is open.
	hasOpen := params.Operations[0].Open != nil

	session, err := p.sessions.Resolve(params.SessionID, hasOpen)
	if err != nil {
		return nil, err
	}

	rb := tool.NewResult()
	for i, op := range params.Operations {
		result := p.dispatchToAction(session, op)
		if result.IsError() {
			rb.WithError()
			rb.Textf("operation[%d] %s error: %v", i, opName(op), result.Err())
			break
		}
		if len(params.Operations) > 1 {
			rb.Textf("## [%d] %s", i+1, opName(op))
		}
		rb.Text(formatActionResult(op, result))

		// If open just created a session, capture it for subsequent ops.
		if op.Open != nil {
			if out, ok := result.Data.(OpenOutput); ok {
				session, _ = p.sessions.Get(out.SessionID)
			}
		}
	}
	if session != nil {
		rb.Textf("\nsession_id: %s", session.ID)
	}
	return rb.Build(), nil
}

// dispatchToAction maps a BrowserOperation to the corresponding action call.
func (p *Plugin) dispatchToAction(session *Session, op BrowserOperation) action.Result {
	ctx := context.Background()

	sid := ""
	if session != nil {
		sid = session.ID
	}

	switch {
	case op.Open != nil:
		headless := p.sessions.config.Headless
		if op.Open.Headless != nil {
			headless = *op.Open.Headless
		}
		out, err := p.executeOpen(ctx, OpenInput{Headless: headless})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Navigate != nil:
		out, err := p.executeNavigate(ctx, NavigateInput{SessionID: sid, URL: op.Navigate.URL})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Click != nil:
		out, err := p.executeClick(ctx, ClickInput{SessionID: sid, Selector: op.Click.Selector})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Type != nil:
		out, err := p.executeType(ctx, TypeInput{
			SessionID: sid,
			Selector:  op.Type.Selector,
			Text:      op.Type.Text,
			Clear:     op.Type.Clear,
			Submit:    op.Type.Submit,
		})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Select != nil:
		out, err := p.executeSelect(ctx, SelectInput{
			SessionID: sid,
			Selector:  op.Select.Selector,
			Values:    op.Select.Values,
		})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Read != nil:
		out, err := p.executeRead(ctx, ReadInput{SessionID: sid, Selector: op.Read.Selector})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Screenshot != nil:
		out, err := p.executeScreenshot(ctx, ScreenshotInput{
			SessionID: sid,
			Selector:  op.Screenshot.Selector,
			FullPage:  op.Screenshot.FullPage,
		})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Evaluate != nil:
		out, err := p.executeEvaluate(ctx, EvaluateInput{SessionID: sid, Expression: op.Evaluate.Expression})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Wait != nil:
		out, err := p.executeWait(ctx, WaitInput{
			SessionID: sid,
			Selector:  op.Wait.Selector,
			TimeoutMs: op.Wait.TimeoutMs,
		})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Back != nil:
		out, err := p.executeBack(ctx, HistoryInput{SessionID: sid})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Forward != nil:
		out, err := p.executeForward(ctx, HistoryInput{SessionID: sid})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	case op.Close != nil:
		out, err := p.executeClose(ctx, CloseInput{SessionID: sid})
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(out)

	default:
		return action.Failed(fmt.Errorf("operation must have exactly one field set"))
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// opFieldCount returns how many operation fields are non-nil.
func opFieldCount(op BrowserOperation) int {
	n := 0
	if op.Open != nil {
		n++
	}
	if op.Navigate != nil {
		n++
	}
	if op.Click != nil {
		n++
	}
	if op.Type != nil {
		n++
	}
	if op.Select != nil {
		n++
	}
	if op.Read != nil {
		n++
	}
	if op.Screenshot != nil {
		n++
	}
	if op.Evaluate != nil {
		n++
	}
	if op.Wait != nil {
		n++
	}
	if op.Back != nil {
		n++
	}
	if op.Forward != nil {
		n++
	}
	if op.Close != nil {
		n++
	}
	return n
}

// opName returns a human-readable name for the operation.
func opName(op BrowserOperation) string {
	switch {
	case op.Open != nil:
		return "open"
	case op.Navigate != nil:
		return "navigate"
	case op.Click != nil:
		return "click"
	case op.Type != nil:
		return "type"
	case op.Select != nil:
		return "select"
	case op.Read != nil:
		return "read"
	case op.Screenshot != nil:
		return "screenshot"
	case op.Evaluate != nil:
		return "evaluate"
	case op.Wait != nil:
		return "wait"
	case op.Back != nil:
		return "back"
	case op.Forward != nil:
		return "forward"
	case op.Close != nil:
		return "close"
	default:
		return "unknown"
	}
}

// formatActionResult converts an action result's data to a readable string.
func formatActionResult(op BrowserOperation, r action.Result) string {
	switch {
	case op.Open != nil:
		if out, ok := r.Data.(OpenOutput); ok {
			return fmt.Sprintf("Session created: %s", out.SessionID)
		}
	case op.Navigate != nil:
		if out, ok := r.Data.(NavigateOutput); ok {
			return fmt.Sprintf("Navigated to %q (%s)", out.Title, out.URL)
		}
	case op.Click != nil:
		if out, ok := r.Data.(ClickOutput); ok {
			if out.Navigated {
				return fmt.Sprintf("Clicked. Page navigated to %s", out.NewURL)
			}
			return "Clicked."
		}
	case op.Type != nil:
		return "Typed."
	case op.Select != nil:
		return "Selected."
	case op.Read != nil:
		if out, ok := r.Data.(ReadOutput); ok {
			return out.Text
		}
	case op.Screenshot != nil:
		if out, ok := r.Data.(ScreenshotOutput); ok {
			return fmt.Sprintf("Screenshot saved: %s", out.Path)
		}
	case op.Evaluate != nil:
		if out, ok := r.Data.(EvaluateOutput); ok {
			return fmt.Sprintf("%v", out.Result)
		}
	case op.Wait != nil:
		return "Element visible."
	case op.Back != nil:
		if out, ok := r.Data.(HistoryOutput); ok {
			return fmt.Sprintf("Back to %q (%s)", out.Title, out.URL)
		}
	case op.Forward != nil:
		if out, ok := r.Data.(HistoryOutput); ok {
			return fmt.Sprintf("Forward to %q (%s)", out.Title, out.URL)
		}
	case op.Close != nil:
		return "Session closed."
	}
	return fmt.Sprintf("%v", r.Data)
}
