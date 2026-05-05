package browserplugin

import (
	"fmt"

	"github.com/chromedp/chromedp"

	"github.com/codewandler/agentsdk/action"
)

func (p *Plugin) executeOpen(_ action.Ctx, input OpenInput) (OpenOutput, error) {
	headless := input.Headless
	// Default to config headless if not explicitly set (struct zero = false,
	// but OpenOp.Headless defaults to true via tool layer).
	sess, err := p.sessions.Create(headless)
	if err != nil {
		return OpenOutput{}, err
	}
	return OpenOutput{SessionID: sess.ID}, nil
}

func (p *Plugin) executeNavigate(_ action.Ctx, input NavigateInput) (NavigateOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return NavigateOutput{}, err
	}

	if input.URL == "" {
		return NavigateOutput{}, fmt.Errorf("url is required")
	}

	var title, location string
	err = chromedp.Run(sess.browserCtx,
		chromedp.Navigate(input.URL),
		chromedp.Location(&location),
		chromedp.Title(&title),
	)
	if err != nil {
		return NavigateOutput{}, fmt.Errorf("navigation failed: %w", err)
	}

	return NavigateOutput{
		SessionID: sess.ID,
		Title:     title,
		URL:       location,
	}, nil
}

func (p *Plugin) executeBack(_ action.Ctx, input HistoryInput) (HistoryOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return HistoryOutput{}, err
	}

	var title, location string
	err = chromedp.Run(sess.browserCtx,
		chromedp.NavigateBack(),
		chromedp.Location(&location),
		chromedp.Title(&title),
	)
	if err != nil {
		return HistoryOutput{}, fmt.Errorf("back navigation failed: %w", err)
	}

	return HistoryOutput{URL: location, Title: title}, nil
}

func (p *Plugin) executeForward(_ action.Ctx, input HistoryInput) (HistoryOutput, error) {
	sess, err := p.sessions.Get(input.SessionID)
	if err != nil {
		return HistoryOutput{}, err
	}

	var title, location string
	err = chromedp.Run(sess.browserCtx,
		chromedp.NavigateForward(),
		chromedp.Location(&location),
		chromedp.Title(&title),
	)
	if err != nil {
		return HistoryOutput{}, fmt.Errorf("forward navigation failed: %w", err)
	}

	return HistoryOutput{URL: location, Title: title}, nil
}

func (p *Plugin) executeClose(_ action.Ctx, input CloseInput) (CloseOutput, error) {
	if err := p.sessions.Close(input.SessionID); err != nil {
		return CloseOutput{}, err
	}
	return CloseOutput{Closed: true}, nil
}
