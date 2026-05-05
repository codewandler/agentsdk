package browserplugin

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/chromedp/chromedp/kb"

	"github.com/codewandler/agentsdk/action"
)

func (p *Plugin) executeClick(_ action.Ctx, input ClickInput) (ClickOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return ClickOutput{}, err
	}
	if input.Selector == "" {
		return ClickOutput{}, fmt.Errorf("selector is required")
	}

	var beforeURL string
	err = chromedp.Run(sess.browserCtx,
		chromedp.Location(&beforeURL),
		chromedp.WaitVisible(input.Selector),
		chromedp.Click(input.Selector),
		chromedp.Sleep(100*time.Millisecond), // brief settle for potential navigation
	)
	if err != nil {
		return ClickOutput{}, fmt.Errorf("click failed: %w", err)
	}

	var afterURL string
	_ = chromedp.Run(sess.browserCtx, chromedp.Location(&afterURL))

	out := ClickOutput{}
	if afterURL != beforeURL {
		out.Navigated = true
		out.NewURL = afterURL
	}
	return out, nil
}

func (p *Plugin) executeType(_ action.Ctx, input TypeInput) (TypeOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return TypeOutput{}, err
	}
	if input.Selector == "" {
		return TypeOutput{}, fmt.Errorf("selector is required")
	}
	if input.Text == "" && !input.Clear {
		return TypeOutput{}, fmt.Errorf("text is required")
	}

	tasks := chromedp.Tasks{
		chromedp.WaitVisible(input.Selector),
	}
	if input.Clear {
		tasks = append(tasks,
			chromedp.Clear(input.Selector),
		)
	}
	if input.Text != "" {
		tasks = append(tasks,
			chromedp.SendKeys(input.Selector, input.Text),
		)
	}
	if input.Submit {
		tasks = append(tasks,
			chromedp.SendKeys(input.Selector, kb.Enter),
		)
	}

	if err := chromedp.Run(sess.browserCtx, tasks); err != nil {
		return TypeOutput{}, fmt.Errorf("type failed: %w", err)
	}
	return TypeOutput{}, nil
}

func (p *Plugin) executeSelect(_ action.Ctx, input SelectInput) (SelectOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return SelectOutput{}, err
	}
	if input.Selector == "" {
		return SelectOutput{}, fmt.Errorf("selector is required")
	}
	if len(input.Values) == 0 {
		return SelectOutput{}, fmt.Errorf("at least one value is required")
	}

	err = chromedp.Run(sess.browserCtx,
		chromedp.WaitVisible(input.Selector),
		chromedp.SetValue(input.Selector, input.Values[0]),
	)
	if err != nil {
		return SelectOutput{}, fmt.Errorf("select failed: %w", err)
	}
	return SelectOutput{}, nil
}

func (p *Plugin) executeWait(_ action.Ctx, input WaitInput) (WaitOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return WaitOutput{}, err
	}
	if input.Selector == "" {
		return WaitOutput{}, fmt.Errorf("selector is required")
	}

	timeout := 5 * time.Second
	if input.TimeoutMs > 0 {
		timeout = time.Duration(input.TimeoutMs) * time.Millisecond
	}

	ctx, cancel := context.WithTimeout(sess.browserCtx, timeout)
	defer cancel()

	if err := chromedp.Run(ctx, chromedp.WaitVisible(input.Selector)); err != nil {
		return WaitOutput{}, fmt.Errorf("wait timeout: element %q not visible within %v: %w", input.Selector, timeout, err)
	}
	return WaitOutput{}, nil
}
