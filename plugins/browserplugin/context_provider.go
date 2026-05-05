package browserplugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/chromedp/chromedp"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
)

// browserContextProvider injects the current page's interactable elements
// into the agent's context window when a browser session is active.
type browserContextProvider struct {
	sessions *SessionManager
}

var (
	_ agentcontext.Provider              = (*browserContextProvider)(nil)
	_ agentcontext.FingerprintingProvider = (*browserContextProvider)(nil)
)

func (p *browserContextProvider) Key() agentcontext.ProviderKey {
	return "browser"
}

func (p *browserContextProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	sess := p.mruSession()
	if sess == nil {
		return agentcontext.ProviderContext{}, nil
	}

	// Get current URL and title.
	url, title := p.pageInfo(sess)
	if url == "" {
		// Page not loaded yet or session is dead.
		content := fmt.Sprintf("[browser: %s | loading...]", sess.ID)
		return agentcontext.ProviderContext{
			Fragments: []agentcontext.ContextFragment{{
				Key:       "browser/elements",
				Role:      unified.RoleUser,
				Content:   content,
				Authority: agentcontext.AuthorityTool,
				CachePolicy: agentcontext.CachePolicy{
					Scope: agentcontext.CacheTurn,
				},
			}},
			Fingerprint: fingerprint("browser", content),
		}, nil
	}

	// Extract interactable elements.
	elements, err := extractElements(sess.browserCtx)
	if err != nil {
		// Degrade gracefully: show URL without element tree.
		content := fmt.Sprintf("[browser: %s | %s | %q]\n(element extraction failed: %v)", sess.ID, url, title, err)
		return agentcontext.ProviderContext{
			Fragments: []agentcontext.ContextFragment{{
				Key:       "browser/elements",
				Role:      unified.RoleUser,
				Content:   content,
				Authority: agentcontext.AuthorityTool,
				CachePolicy: agentcontext.CachePolicy{
					Scope: agentcontext.CacheTurn,
				},
			}},
			Fingerprint: fingerprint("browser", content),
		}, nil
	}

	content := renderElementTree(elements, sess.ID, url, title, len(elements))
	fp := fingerprint("browser", content)

	return agentcontext.ProviderContext{
		Fragments: []agentcontext.ContextFragment{{
			Key:       "browser/elements",
			Role:      unified.RoleUser,
			Content:   content,
			Authority: agentcontext.AuthorityTool,
			CachePolicy: agentcontext.CachePolicy{
				Scope: agentcontext.CacheTurn,
			},
		}},
		Fingerprint: fp,
	}, nil
}

func (p *browserContextProvider) StateFingerprint(ctx context.Context, _ agentcontext.Request) (string, bool, error) {
	sess := p.mruSession()
	if sess == nil {
		return fingerprint("browser", ""), true, nil
	}

	// Cheap fingerprint: URL + element count via JS.
	var result string
	err := chromedp.Run(sess.browserCtx,
		chromedp.Evaluate(`document.URL + "|" + document.querySelectorAll("*").length`, &result),
	)
	if err != nil {
		return "", false, nil // can't fingerprint, force re-render
	}

	return fingerprint("browser", sess.ID+"|"+result), true, nil
}

// mruSession returns the most-recently-used session, or nil if none.
func (p *browserContextProvider) mruSession() *Session {
	p.sessions.mu.Lock()
	defer p.sessions.mu.Unlock()

	var best *Session
	for _, s := range p.sessions.sessions {
		if best == nil || s.lastUsedAt.After(best.lastUsedAt) {
			best = s
		}
	}
	return best
}

// pageInfo retrieves the current URL and title from a session.
func (p *browserContextProvider) pageInfo(sess *Session) (url, title string) {
	_ = chromedp.Run(sess.browserCtx,
		chromedp.Location(&url),
		chromedp.Title(&title),
	)
	return url, title
}

func fingerprint(kind, content string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + content))
	return "sha256:" + hex.EncodeToString(sum[:])
}
