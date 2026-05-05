// Package browserplugin provides browser automation via the Chrome DevTools
// Protocol. It exposes a single "browser" tool backed by core actions, plus a
// context provider that renders the current page's interactable elements.
package browserplugin

import "time"

// Mode determines how the plugin connects to Chrome.
type Mode string

const (
	// ModeLaunch starts a new Chrome process managed by the plugin.
	ModeLaunch Mode = "launch"
	// ModeAttach connects to an already-running Chrome via its remote debugging URL.
	ModeAttach Mode = "attach"
)

// Config holds browser plugin settings.
type Config struct {
	// Mode selects launch (start new Chrome) or attach (connect to existing).
	// Default: ModeLaunch.
	Mode Mode

	// RemoteURL is the WebSocket debugger URL for attach mode
	// (e.g. "ws://localhost:9222").
	RemoteURL string

	// ChromePath overrides automatic Chrome/Chromium detection.
	// When empty, the plugin checks CHROME_PATH env then falls back to
	// chromedp's built-in path resolution.
	ChromePath string

	// Headless controls whether launched Chrome runs headless.
	// Default: true. Ignored in attach mode.
	Headless bool

	// IdleTimeout is how long a session can be unused before the reaper
	// closes it automatically. Default: 10 minutes.
	IdleTimeout time.Duration

	// MaxSessions caps concurrent browser sessions. Default: 3.
	MaxSessions int

	// OpTimeout is the maximum time any single operation may take.
	// Default: 30 seconds. The wait operation uses its own timeout_ms
	// parameter but is still capped by this value.
	OpTimeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Mode:        ModeLaunch,
		Headless:    true,
		IdleTimeout: 10 * time.Minute,
		MaxSessions: 3,
		OpTimeout:   30 * time.Second,
	}
}

// Option configures a Plugin.
type Option func(*Config)

// WithMode sets the connection mode (launch or attach).
func WithMode(mode Mode) Option {
	return func(c *Config) { c.Mode = mode }
}

// WithRemoteURL sets the WebSocket URL for attach mode.
func WithRemoteURL(url string) Option {
	return func(c *Config) { c.RemoteURL = url }
}

// WithChromePath sets an explicit Chrome/Chromium binary path.
func WithChromePath(path string) Option {
	return func(c *Config) { c.ChromePath = path }
}

// WithHeadless controls headless mode for launched browsers.
func WithHeadless(headless bool) Option {
	return func(c *Config) { c.Headless = headless }
}

// WithIdleTimeout sets the session idle timeout.
func WithIdleTimeout(d time.Duration) Option {
	return func(c *Config) { c.IdleTimeout = d }
}

// WithMaxSessions sets the maximum concurrent sessions.
func WithMaxSessions(n int) Option {
	return func(c *Config) { c.MaxSessions = n }
}

// WithOpTimeout sets the per-operation timeout.
func WithOpTimeout(d time.Duration) Option {
	return func(c *Config) { c.OpTimeout = d }
}
