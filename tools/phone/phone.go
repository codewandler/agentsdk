package phone

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/codewandler/agentsdk/tool"
)

// ── Configuration ─────────────────────────────────────────────────────────────

// Config configures the phone tool. SIPAddr is required.
type Config struct {
	// SIPAddr is the SIP endpoint in "host:port" form (e.g. "asterisk.dev.internal:5062").
	SIPAddr string

	// Transport is "tcp" or "udp". Default: "tcp".
	Transport string

	// Log is the logger for SIP operations. Default: slog.Default().
	Log *slog.Logger

	// Dialer overrides the default SIP dialer (for testing).
	Dialer Dialer
}

func (c Config) transport() string {
	if c.Transport != "" {
		return c.Transport
	}
	return "tcp"
}

func (c Config) dialer() Dialer {
	if c.Dialer != nil {
		return c.Dialer
	}
	return newSIPDialer(c.Log)
}

// ── Parameter types (oneOf operations) ────────────────────────────────────────

// PhoneParams defines the parameters for the phone tool.
type PhoneParams struct {
	Operations []PhoneOperation `json:"operations" jsonschema:"description=Phone operations to perform.,required"`
}

// PhoneOperation is a discriminated union — exactly one field must be set.
type PhoneOperation struct {
	Dial   *DialOp   `json:"dial,omitempty" jsonschema:"description=Place an outbound SIP call."`
	Hangup *HangupOp `json:"hangup,omitempty" jsonschema:"description=Hang up an active call."`
	Status *StatusOp `json:"status,omitempty" jsonschema:"description=List active calls and their state."`
}

// DialOp places an outbound call.
type DialOp struct {
	Number  string `json:"number" jsonschema:"description=Phone number or SIP address to dial.,required"`
	Timeout int    `json:"timeout,omitempty" jsonschema:"description=Dial timeout in seconds (default 30)."`
}

// HangupOp terminates an active call.
type HangupOp struct {
	CallID string `json:"call_id" jsonschema:"description=Call ID to hang up (from dial result).,required"`
}

// StatusOp lists active calls. No parameters.
type StatusOp struct{}

// ── Call registry ─────────────────────────────────────────────────────────────

type activeCall struct {
	ID        string
	Number    string
	StartedAt time.Time
	State     string // "ringing", "active", "ended"
	call      Call
}

type callRegistry struct {
	mu    sync.Mutex
	calls map[string]*activeCall
	seq   int
}

func newRegistry() *callRegistry {
	return &callRegistry{calls: make(map[string]*activeCall)}
}

func (r *callRegistry) add(number string, call Call) *activeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	id := fmt.Sprintf("call-%d", r.seq)
	ac := &activeCall{
		ID:        id,
		Number:    number,
		StartedAt: time.Now(),
		State:     "active",
		call:      call,
	}
	r.calls[id] = ac

	// Watch for remote hangup.
	go func() {
		<-call.Done()
		r.mu.Lock()
		ac.State = "ended"
		r.mu.Unlock()
	}()

	return ac
}

func (r *callRegistry) get(id string) (*activeCall, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ac, ok := r.calls[id]
	return ac, ok
}

func (r *callRegistry) remove(id string) (*activeCall, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	ac, ok := r.calls[id]
	if ok {
		delete(r.calls, id)
	}
	return ac, ok
}

func (r *callRegistry) active() []*activeCall {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*activeCall
	for _, ac := range r.calls {
		out = append(out, ac)
	}
	return out
}

// ── Tool construction ─────────────────────────────────────────────────────────

const defaultDialTimeout = 30

// Tools returns the phone tool configured with the given SIP settings.
// Returns nil if cfg.SIPAddr is empty.
func Tools(cfg Config) []tool.Tool {
	if cfg.SIPAddr == "" {
		return nil
	}

	registry := newRegistry()
	dialer := cfg.dialer()
	sipAddr := cfg.SIPAddr
	transport := cfg.transport()

	return []tool.Tool{
		tool.New("phone",
			"Place and manage SIP phone calls. Supports dialing numbers, hanging up active calls, and listing call status.",
			func(ctx tool.Ctx, p PhoneParams) (tool.Result, error) {
				if len(p.Operations) == 0 {
					return nil, fmt.Errorf("at least one operation is required")
				}
				return executeOps(ctx, p.Operations, registry, dialer, sipAddr, transport)
			},
			phoneIntent(sipAddr),
		),
	}
}

// ── Operation dispatch ────────────────────────────────────────────────────────

func executeOps(
	ctx tool.Ctx,
	ops []PhoneOperation,
	registry *callRegistry,
	dialer Dialer,
	sipAddr, transport string,
) (tool.Result, error) {
	var parts []string
	anyError := false

	for _, op := range ops {
		var text string
		var err error

		switch {
		case op.Dial != nil:
			text, err = executeDial(ctx, op.Dial, registry, dialer, sipAddr, transport)
		case op.Hangup != nil:
			text, err = executeHangup(op.Hangup, registry)
		case op.Status != nil:
			text, err = executeStatus(registry)
		default:
			err = fmt.Errorf("operation must have exactly one of: dial, hangup, status")
		}

		if err != nil {
			parts = append(parts, fmt.Sprintf("error: %v", err))
			anyError = true
		} else {
			parts = append(parts, text)
		}
	}

	b := tool.NewResult()
	if anyError {
		b.WithError()
	}
	b.Text(strings.Join(parts, "\n\n"))
	return b.Build(), nil
}

func executeDial(
	ctx tool.Ctx,
	op *DialOp,
	registry *callRegistry,
	dialer Dialer,
	sipAddr, transport string,
) (string, error) {
	if op.Number == "" {
		return "", fmt.Errorf("dial: number is required")
	}

	timeout := op.Timeout
	if timeout < 1 {
		timeout = defaultDialTimeout
	}

	dialCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	call, err := dialer.Dial(dialCtx, sipAddr, transport, op.Number)
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", op.Number, err)
	}

	ac := registry.add(op.Number, call)
	return fmt.Sprintf("Call started: %s\nNumber: %s\nState: %s", ac.ID, ac.Number, ac.State), nil
}

func executeHangup(op *HangupOp, registry *callRegistry) (string, error) {
	if op.CallID == "" {
		return "", fmt.Errorf("hangup: call_id is required")
	}

	ac, ok := registry.remove(op.CallID)
	if !ok {
		return "", fmt.Errorf("hangup: unknown call %q", op.CallID)
	}

	duration := time.Since(ac.StartedAt).Truncate(time.Second)
	ac.call.Hangup()
	return fmt.Sprintf("Call %s ended (duration: %s)", ac.ID, duration), nil
}

func executeStatus(registry *callRegistry) (string, error) {
	calls := registry.active()
	if len(calls) == 0 {
		return "No active calls.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Active calls: %d\n", len(calls))
	for _, ac := range calls {
		dur := time.Since(ac.StartedAt).Truncate(time.Second)
		fmt.Fprintf(&sb, "  %-10s %-20s %-8s %s\n", ac.ID, ac.Number, ac.State, dur)
	}
	return sb.String(), nil
}
