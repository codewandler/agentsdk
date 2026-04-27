package contextproviders

import (
	"context"
	"fmt"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
)

type TimeOption func(*TimeProvider)

type TimeProvider struct {
	key      agentcontext.ProviderKey
	clock    func() time.Time
	interval time.Duration
	location *time.Location
}

func Time(interval time.Duration, opts ...TimeOption) *TimeProvider {
	p := &TimeProvider{
		key:      "time",
		clock:    time.Now,
		interval: interval,
		location: time.Local,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

func WithTimeKey(key agentcontext.ProviderKey) TimeOption {
	return func(p *TimeProvider) { p.key = key }
}

func WithClock(clock func() time.Time) TimeOption {
	return func(p *TimeProvider) {
		if clock != nil {
			p.clock = clock
		}
	}
}

func WithLocation(location *time.Location) TimeOption {
	return func(p *TimeProvider) {
		if location != nil {
			p.location = location
		}
	}
}

func (p *TimeProvider) Key() agentcontext.ProviderKey {
	if p == nil || p.key == "" {
		return "time"
	}
	return p.key
}

func (p *TimeProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	content := p.content()
	return agentcontext.ProviderContext{
		Fragments: []agentcontext.ContextFragment{{
			Key:       "time/current",
			Role:      unified.RoleUser,
			Content:   content,
			Authority: agentcontext.AuthorityUser,
			CachePolicy: agentcontext.CachePolicy{
				MaxAge: p.bucketInterval(),
				Scope:  agentcontext.CacheTurn,
			},
		}},
		Fingerprint: contentFingerprint("time", content),
	}, nil
}

func (p *TimeProvider) StateFingerprint(ctx context.Context, _ agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return contentFingerprint("time", p.content()), true, nil
}

func (p *TimeProvider) content() string {
	now := p.now()
	return fmt.Sprintf("current_time: %s", now.Format(time.RFC3339))
}

func (p *TimeProvider) now() time.Time {
	var now time.Time
	if p != nil && p.clock != nil {
		now = p.clock()
	} else {
		now = time.Now()
	}
	location := time.Local
	if p != nil && p.location != nil {
		location = p.location
	}
	now = now.In(location)
	interval := p.bucketInterval()
	if interval <= 0 {
		return now
	}
	return now.Truncate(interval)
}

func (p *TimeProvider) bucketInterval() time.Duration {
	if p == nil || p.interval <= 0 {
		return time.Minute
	}
	return p.interval
}
