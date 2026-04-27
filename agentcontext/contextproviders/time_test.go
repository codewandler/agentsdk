package contextproviders

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
)

func TestTimeProviderBucketsCurrentTime(t *testing.T) {
	now := time.Date(2026, 4, 27, 10, 11, 45, 0, time.UTC)
	provider := Time(time.Minute, WithClock(func() time.Time { return now }), WithLocation(time.UTC))

	context, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Key() != "time" {
		t.Fatalf("key = %q", provider.Key())
	}
	if got, want := len(context.Fragments), 1; got != want {
		t.Fatalf("fragments = %d, want %d", got, want)
	}
	fragment := context.Fragments[0]
	if fragment.Key != "time/current" {
		t.Fatalf("fragment key = %q", fragment.Key)
	}
	if !strings.Contains(fragment.Content, "current_time: 2026-04-27T10:11:00Z") {
		t.Fatalf("content = %q", fragment.Content)
	}
	if fragment.CachePolicy.MaxAge != time.Minute || fragment.CachePolicy.Scope != agentcontext.CacheTurn {
		t.Fatalf("cache policy = %#v", fragment.CachePolicy)
	}
}

func TestTimeProviderFingerprintChangesOnlyAfterBucket(t *testing.T) {
	now := time.Date(2026, 4, 27, 10, 11, 45, 0, time.UTC)
	provider := Time(time.Minute, WithClock(func() time.Time { return now }), WithLocation(time.UTC))

	first, ok, err := provider.StateFingerprint(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || first == "" {
		t.Fatalf("fingerprint = %q ok=%v", first, ok)
	}

	now = now.Add(10 * time.Second)
	second, ok, err := provider.StateFingerprint(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || second != first {
		t.Fatalf("same bucket fingerprint = %q, want %q", second, first)
	}

	now = now.Add(20 * time.Second)
	third, ok, err := provider.StateFingerprint(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || third == first {
		t.Fatalf("next bucket fingerprint = %q, want different from %q", third, first)
	}
}

func TestTimeProviderSupportsCustomKey(t *testing.T) {
	provider := Time(time.Minute, WithTimeKey("time/custom"))
	if provider.Key() != "time/custom" {
		t.Fatalf("key = %q", provider.Key())
	}
}
