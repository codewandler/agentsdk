// Package phone provides the phone tool for SIP call origination.
//
// Each dial operation creates an independent SIP user agent and transport
// (via diago/sipgo), so multiple concurrent calls are fully isolated.
package phone

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"

	"github.com/emiago/diago"
	"github.com/emiago/sipgo"
	"github.com/emiago/sipgo/sip"
)

// Dialer abstracts SIP call origination so the tool can be tested without
// a live SIP stack.
type Dialer interface {
	// Dial places an outbound SIP call. The returned Call stays alive until
	// Hangup is called or the remote side terminates.
	Dial(ctx context.Context, addr, transport, number string) (Call, error)
}

// Call represents an active outbound SIP call.
type Call interface {
	// Hangup terminates the call and releases all resources (UA, transport).
	Hangup()
	// Done returns a channel that closes when the call ends (remote hangup
	// or local hangup).
	Done() <-chan struct{}
}

// ── Live SIP implementation ───────────────────────────────────────────────────

// sipDialer is the production Dialer backed by diago/sipgo.
type sipDialer struct {
	log *slog.Logger
}

func newSIPDialer(log *slog.Logger) *sipDialer {
	if log == nil {
		log = slog.Default()
	}
	return &sipDialer{log: log}
}

func (d *sipDialer) Dial(ctx context.Context, addr, transport, number string) (Call, error) {
	ua, err := sipgo.NewUA(sipgo.WithUserAgent("agentsdk-phone"))
	if err != nil {
		return nil, fmt.Errorf("create SIP UA: %w", err)
	}

	dg := diago.NewDiago(ua,
		diago.WithLogger(d.log),
		diago.WithTransport(diago.Transport{
			Transport:      transport,
			RewriteContact: true,
		}),
	)

	// Start the diago server so the transport is active.
	bgCtx, bgCancel := context.WithCancel(context.Background())
	if err := dg.ServeBackground(bgCtx, func(_ *diago.DialogServerSession) {
		d.log.Debug("phone: unexpected inbound SIP call, ignoring")
	}); err != nil {
		bgCancel()
		_ = ua.Close()
		return nil, fmt.Errorf("start SIP transport: %w", err)
	}

	host, port := parseHostPort(addr)
	recipient := sip.Uri{
		User: number,
		Host: host,
		Port: port,
	}

	d.log.Info("phone: SIP INVITE", "to", number, "addr", addr)

	dialog, err := dg.Invite(ctx, recipient, diago.InviteOptions{
		Transport: transport,
	})
	if err != nil {
		bgCancel()
		_ = ua.Close()
		return nil, fmt.Errorf("SIP INVITE to %s: %w", number, err)
	}

	d.log.Info("phone: call established", "to", number)

	// Start media echo to keep the RTP path alive.
	go func() {
		if err := dialog.Media().Echo(); err != nil {
			d.log.Debug("phone: media echo stopped", "to", number, "error", err)
		}
	}()

	// When the dialog ends, cancel the background context.
	done := make(chan struct{})
	go func() {
		<-dialog.Context().Done()
		bgCancel()
		close(done)
	}()

	return &sipCall{
		dialog:   dialog,
		ua:       ua,
		bgCancel: bgCancel,
		done:     done,
		log:      d.log,
	}, nil
}

// sipCall is a live SIP call owning its own UA and transport.
type sipCall struct {
	dialog   *diago.DialogClientSession
	ua       *sipgo.UserAgent
	bgCancel context.CancelFunc
	done     chan struct{}
	log      *slog.Logger

	once sync.Once
}

func (c *sipCall) Hangup() {
	c.once.Do(func() {
		if c.dialog != nil {
			_ = c.dialog.Hangup(context.Background())
		}
		if c.bgCancel != nil {
			c.bgCancel()
		}
		if c.ua != nil {
			_ = c.ua.Close()
		}
	})
}

func (c *sipCall) Done() <-chan struct{} {
	return c.done
}

// parseHostPort splits "host:port" into host and port number.
func parseHostPort(addr string) (string, int) {
	parts := strings.SplitN(addr, ":", 2)
	if len(parts) == 2 {
		p, _ := strconv.Atoi(parts[1])
		return parts[0], p
	}
	return addr, 5060
}
