package browserplugin

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/chromedp/chromedp"
)

// Session represents a single browser session with its own isolated context.
type Session struct {
	ID          string
	allocCtx    context.Context
	allocCancel context.CancelFunc
	browserCtx  context.Context
	browserStop context.CancelFunc
	createdAt   time.Time
	lastUsedAt  time.Time
	mu          sync.Mutex
}

// withTimeout returns a child context of the browser context with the given
// deadline. The caller must call the returned cancel function.
func (s *Session) withTimeout(timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(s.browserCtx, timeout)
}

// touch updates the last-used timestamp.
func (s *Session) touch() {
	s.mu.Lock()
	s.lastUsedAt = time.Now()
	s.mu.Unlock()
}

// idleSince returns the duration since last use.
func (s *Session) idleSince() time.Duration {
	s.mu.Lock()
	d := time.Since(s.lastUsedAt)
	s.mu.Unlock()
	return d
}

// SessionManager manages browser sessions with lifecycle controls.
type SessionManager struct {
	mu       sync.Mutex
	sessions map[string]*Session
	config   Config
	done     chan struct{}
	once     sync.Once
}

// NewSessionManager creates a manager and starts the idle reaper.
func NewSessionManager(cfg Config) *SessionManager {
	m := &SessionManager{
		sessions: make(map[string]*Session),
		config:   cfg,
		done:     make(chan struct{}),
	}
	go m.reapLoop()
	return m
}

// Resolve returns the session for the given ID, or creates one if the first
// operation is an open. Returns an error if neither condition is met.
func (m *SessionManager) Resolve(sessionID string, hasOpen bool) (*Session, error) {
	if hasOpen && sessionID == "" {
		return m.Create(m.config.Headless)
	}
	if sessionID == "" {
		return nil, fmt.Errorf("session_id required or first operation must be open")
	}
	return m.Get(sessionID)
}

// Create launches or attaches a new browser session.
func (m *SessionManager) Create(headless bool) (*Session, error) {
	m.mu.Lock()
	if m.config.MaxSessions > 0 && len(m.sessions) >= m.config.MaxSessions {
		m.mu.Unlock()
		return nil, fmt.Errorf("max sessions reached (%d)", m.config.MaxSessions)
	}
	m.mu.Unlock()

	id := generateSessionID()

	var allocCtx context.Context
	var allocCancel context.CancelFunc

	switch m.config.Mode {
	case ModeAttach:
		if m.config.RemoteURL == "" {
			return nil, fmt.Errorf("attach mode requires RemoteURL")
		}
		allocCtx, allocCancel = chromedp.NewRemoteAllocator(context.Background(), m.config.RemoteURL)

	default: // ModeLaunch
		opts := append(chromedp.DefaultExecAllocatorOptions[:],
			chromedp.Flag("headless", headless),
			chromedp.Flag("disable-gpu", true),
			chromedp.Flag("no-first-run", true),
			chromedp.Flag("disable-extensions", true),
			chromedp.Flag("disable-default-apps", true),
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.UserAgent("Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/137.0.0.0 Safari/537.36"),
			chromedp.WindowSize(1280, 720),
		)
		if m.config.ChromePath != "" {
			opts = append(opts, chromedp.ExecPath(m.config.ChromePath))
		} else if envPath := os.Getenv("CHROME_PATH"); envPath != "" {
			opts = append(opts, chromedp.ExecPath(envPath))
		}
		allocCtx, allocCancel = chromedp.NewExecAllocator(context.Background(), opts...)
	}

	browserCtx, browserStop := chromedp.NewContext(allocCtx)

	// Force browser start so we fail fast on misconfiguration.
	if err := chromedp.Run(browserCtx); err != nil {
		browserStop()
		allocCancel()
		return nil, fmt.Errorf("failed to start browser: %w", err)
	}

	now := time.Now()
	sess := &Session{
		ID:          id,
		allocCtx:    allocCtx,
		allocCancel: allocCancel,
		browserCtx:  browserCtx,
		browserStop: browserStop,
		createdAt:   now,
		lastUsedAt:  now,
	}

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	return sess, nil
}

// Get retrieves an existing session by ID.
func (m *SessionManager) Get(id string) (*Session, error) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	m.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("session not found: %s", id)
	}
	sess.touch()
	return sess, nil
}

// Close terminates a session and releases its resources.
func (m *SessionManager) Close(id string) error {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("session not found: %s", id)
	}
	sess.browserStop()
	sess.allocCancel()
	return nil
}

// CloseAll terminates all sessions and stops the reaper.
func (m *SessionManager) CloseAll() {
	m.once.Do(func() { close(m.done) })

	m.mu.Lock()
	sessions := make([]*Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.sessions = make(map[string]*Session)
	m.mu.Unlock()

	for _, s := range sessions {
		s.browserStop()
		s.allocCancel()
	}
}

// reapLoop periodically checks for idle sessions and closes them.
func (m *SessionManager) reapLoop() {
	interval := m.config.IdleTimeout / 2
	if interval < time.Second {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.done:
			return
		case <-ticker.C:
			m.reapIdle()
		}
	}
}

func (m *SessionManager) reapIdle() {
	m.mu.Lock()
	var expired []string
	for id, sess := range m.sessions {
		if sess.idleSince() > m.config.IdleTimeout {
			expired = append(expired, id)
		}
	}
	m.mu.Unlock()

	for _, id := range expired {
		_ = m.Close(id)
	}
}

func generateSessionID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return "sess_" + hex.EncodeToString(b)
}
