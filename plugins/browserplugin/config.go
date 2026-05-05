// Package browserplugin provides browser automation via the Chrome DevTools
// Protocol. It exposes a single "browser" tool backed by core actions, plus a
// context provider that renders the current page's interactable elements.
package browserplugin

import (
	"os"
	"path/filepath"
	"runtime"
	"time"
)

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

	// UserDataDir is the default Chrome profile directory for sessions.
	// Defaults to the user's Chrome profile directory if it exists,
	// giving access to existing cookies, logins, and consent state.
	// Set to empty string to use ephemeral profiles.
	// Note: Chrome locks its profile — if Chrome is already running,
	// the session will use a copy or fail. chromedp handles this gracefully.
	UserDataDir string
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		Mode:        ModeLaunch,
		Headless:    true,
		IdleTimeout: 10 * time.Minute,
		MaxSessions: 3,
		OpTimeout:   30 * time.Second,
		UserDataDir: defaultUserDataDir(),
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

// WithUserDataDir sets the default Chrome profile directory.
// Set to empty string to use ephemeral (temp) profiles.
func WithUserDataDir(dir string) Option {
	return func(c *Config) { c.UserDataDir = dir }
}

// defaultUserDataDir returns the user's Chrome profile directory if it exists.
func defaultUserDataDir() string {
	var candidates []string
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		candidates = []string{
			filepath.Join(home, "Library", "Application Support", "Google", "Chrome"),
			filepath.Join(home, "Library", "Application Support", "Chromium"),
		}
	case "windows":
		candidates = []string{
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Chromium", "User Data"),
		}
	default: // linux, freebsd, etc.
		home, _ := os.UserHomeDir()
		candidates = []string{
			filepath.Join(home, ".config", "google-chrome"),
			filepath.Join(home, ".config", "chromium"),
		}
	}
	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}
	return "" // no Chrome profile found, use ephemeral
}
