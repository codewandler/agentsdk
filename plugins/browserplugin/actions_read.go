package browserplugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/chromedp/chromedp"

	"github.com/codewandler/agentsdk/action"
)

func (p *Plugin) executeRead(_ action.Ctx, input ReadInput) (ReadOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return ReadOutput{}, err
	}

	ctx, cancel := p.opContext(sess)
	defer cancel()

	var text string
	if input.Selector != "" {
		err = chromedp.Run(ctx,
			chromedp.Text(input.Selector, &text, chromedp.NodeVisible),
		)
	} else {
		err = chromedp.Run(ctx,
			chromedp.Text("body", &text, chromedp.NodeVisible),
		)
	}
	if err != nil {
		return ReadOutput{}, fmt.Errorf("read failed: %w", err)
	}
	return ReadOutput{Text: text}, nil
}

func (p *Plugin) executeScreenshot(_ action.Ctx, input ScreenshotInput) (ScreenshotOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return ScreenshotOutput{}, err
	}

	ctx, cancel := p.opContext(sess)
	defer cancel()

	var buf []byte
	switch {
	case input.Selector != "":
		err = chromedp.Run(ctx,
			chromedp.WaitVisible(input.Selector),
			chromedp.Screenshot(input.Selector, &buf),
		)
	case input.FullPage:
		err = chromedp.Run(ctx,
			chromedp.FullScreenshot(&buf, 90),
		)
	default:
		err = chromedp.Run(ctx,
			chromedp.CaptureScreenshot(&buf),
		)
	}
	if err != nil {
		return ScreenshotOutput{}, fmt.Errorf("screenshot failed: %w", err)
	}

	f, err := os.CreateTemp("", "browser-screenshot-*.png")
	if err != nil {
		return ScreenshotOutput{}, fmt.Errorf("create temp file: %w", err)
	}
	if _, err := f.Write(buf); err != nil {
		f.Close()
		return ScreenshotOutput{}, fmt.Errorf("write screenshot: %w", err)
	}
	f.Close()

	absPath, _ := filepath.Abs(f.Name())
	return ScreenshotOutput{Path: absPath}, nil
}

func (p *Plugin) executeEvaluate(_ action.Ctx, input EvaluateInput) (EvaluateOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return EvaluateOutput{}, err
	}
	if input.Expression == "" {
		return EvaluateOutput{}, fmt.Errorf("expression is required")
	}

	ctx, cancel := p.opContext(sess)
	defer cancel()

	var result any
	err = chromedp.Run(ctx,
		chromedp.Evaluate(input.Expression, &result),
	)
	if err != nil {
		return EvaluateOutput{}, fmt.Errorf("evaluate failed: %w", err)
	}

	raw, _ := json.Marshal(result)
	var normalized any
	_ = json.Unmarshal(raw, &normalized)

	return EvaluateOutput{Result: normalized}, nil
}
