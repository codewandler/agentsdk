package daemon

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"github.com/codewandler/agentsdk/harness"
)

const Mode = "harness.service"

// Config contains process-level daemon/service settings. The daemon package is
// intentionally a thin wrapper over harness.Service: harness remains the
// runtime/session owner, while this package owns service-mode conventions such
// as storage paths and graceful shutdown.
type Config struct {
	Service     *harness.Service
	SessionsDir string
	ConfigPath  string
}

// Host is the service-mode wrapper used by CLIs, tests, and future HTTP/SSE or
// trigger hosts. It does not introduce a second app/runtime/plugin system.
type Host struct {
	service     *harness.Service
	sessionsDir string
	configPath  string

	mu     sync.Mutex
	closed bool
}

func New(cfg Config) (*Host, error) {
	if cfg.Service == nil {
		return nil, fmt.Errorf("daemon: harness service is required")
	}
	sessionsDir, err := absClean(cfg.SessionsDir)
	if err != nil {
		return nil, fmt.Errorf("daemon: sessions dir: %w", err)
	}
	configPath, err := absClean(cfg.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("daemon: config path: %w", err)
	}
	return &Host{service: cfg.Service, sessionsDir: sessionsDir, configPath: configPath}, nil
}

func (h *Host) Service() *harness.Service {
	if h == nil {
		return nil
	}
	return h.service
}

func (h *Host) StoragePaths() StoragePaths {
	if h == nil {
		return StoragePaths{}
	}
	return StoragePaths{SessionsDir: h.sessionsDir, ConfigPath: h.configPath}
}

type StoragePaths struct {
	SessionsDir string
	ConfigPath  string
}

func (h *Host) OpenSession(ctx context.Context, req harness.SessionOpenRequest) (*harness.Session, error) {
	if h == nil || h.service == nil {
		return nil, fmt.Errorf("daemon: harness service is required")
	}
	if h.isClosed() {
		return nil, fmt.Errorf("daemon: host is closed")
	}
	if strings.TrimSpace(req.StoreDir) == "" {
		req.StoreDir = h.sessionsDir
	}
	return h.service.OpenSession(ctx, req)
}

func (h *Host) ResumeSession(ctx context.Context, req harness.SessionOpenRequest) (*harness.Session, error) {
	if strings.TrimSpace(req.Resume) == "" {
		return nil, fmt.Errorf("daemon: resume session is required")
	}
	if strings.TrimSpace(req.StoreDir) == "" {
		req.StoreDir = h.sessionsDir
	}
	return h.OpenSession(ctx, req)
}

func (h *Host) Sessions() []harness.SessionSummary {
	if h == nil || h.service == nil {
		return nil
	}
	return h.service.Sessions()
}

func (h *Host) Status() Status {
	if h == nil || h.service == nil {
		return Status{ServiceStatus: harness.ServiceStatus{Mode: Mode, Health: "unavailable", Closed: true}}
	}
	serviceStatus := h.service.Status()
	serviceStatus.Mode = Mode
	if h.isClosed() {
		serviceStatus.Health = "closed"
		serviceStatus.Closed = true
	}
	return Status{ServiceStatus: serviceStatus, Storage: h.StoragePaths()}
}

type Status struct {
	harness.ServiceStatus
	Storage StoragePaths
}

func (h *Host) Shutdown(ctx context.Context) error {
	if h == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return nil
	}
	h.closed = true
	h.mu.Unlock()
	if h.service != nil {
		return h.service.Close()
	}
	return nil
}

func (h *Host) isClosed() bool {
	if h == nil {
		return true
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.closed
}

func absClean(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	return filepath.Abs(path)
}
