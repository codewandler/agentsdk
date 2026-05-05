package browserplugin

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"

	"github.com/codewandler/agentsdk/action"
)

func (p *Plugin) executeOpen(_ action.Ctx, input OpenInput) (OpenOutput, error) {
	sess, err := p.sessions.Create(CreateOpts{
		Headless:    input.Headless,
		UserDataDir: input.UserDataDir,
	})
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

	ctx, cancel := p.opContext(sess)
	defer cancel()

	var title, location string
	err = chromedp.Run(ctx,
		chromedp.Navigate(input.URL),
		chromedp.Location(&location),
		chromedp.Title(&title),
	)
	if err != nil {
		return NavigateOutput{}, fmt.Errorf("navigation failed: %w", err)
	}

	// Auto-dismiss common cookie consent banners.
	dismissCookieConsent(ctx)

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

	ctx, cancel := p.opContext(sess)
	defer cancel()

	var title, location string
	err = chromedp.Run(ctx,
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

	ctx, cancel := p.opContext(sess)
	defer cancel()

	var title, location string
	err = chromedp.Run(ctx,
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

// opContext returns a child of the session's browser context with the
// configured operation timeout. chromedp finds its internal state via context
// value chain, so this correctly scopes the deadline without losing the
// browser connection.
func (p *Plugin) opContext(sess *Session) (context.Context, context.CancelFunc) {
	return context.WithTimeout(sess.browserCtx, p.sessions.config.OpTimeout)
}

// dismissCookieConsent attempts to click common cookie consent "accept" buttons.
// It's best-effort — failures are silently ignored.
func dismissCookieConsent(ctx context.Context) {
	// Common selectors for cookie consent accept buttons across popular
	// consent frameworks (Google, OneTrust, CookieBot, etc.).
	const js = `(function() {
		const selectors = [
			'button#L2AGLb',                          // Google
			'button#W0wltc',                          // Google (reject-ish but dismisses)
			'[aria-label="Accept all"]',
			'[aria-label="Alle akzeptieren"]',
			'button.accept-all',
			'#onetrust-accept-btn-handler',           // OneTrust
			'#CybotCookiebotDialogBodyLevelButtonLevelOptinAllowAll', // CookieBot
			'.cc-accept',                             // Cookie Consent (Osano)
			'[data-testid="cookie-policy-manage-dialog-btn-accept-all"]',
			'button[data-cookiefirst-action="accept"]',
		];
		for (const sel of selectors) {
			const btn = document.querySelector(sel);
			if (btn && btn.offsetParent !== null) {
				btn.click();
				return sel;
			}
		}
		return null;
	})()`
	var result any
	// Ignore errors — this is best-effort.
	_ = chromedp.Run(ctx, chromedp.Evaluate(js, &result))
}
