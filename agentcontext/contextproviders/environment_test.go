package contextproviders

import (
	"context"
	"runtime"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
)

func TestEnvironmentProviderRendersStableSystemContext(t *testing.T) {
	provider := Environment(
		WithWorkDir("."),
		WithHostname("host-a"),
		WithKernelVersion("kernel-a"),
	)

	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Key() != "environment" {
		t.Fatalf("key = %q", provider.Key())
	}
	if got, want := len(providerContext.Fragments), 1; got != want {
		t.Fatalf("fragments = %d, want %d", got, want)
	}
	fragment := providerContext.Fragments[0]
	if fragment.Key != "environment/system" {
		t.Fatalf("fragment key = %q", fragment.Key)
	}
	for _, want := range []string{
		"working_directory:",
		"os: " + runtime.GOOS,
		"arch: " + runtime.GOARCH,
		"kernel: kernel-a",
		"hostname: host-a",
	} {
		if !strings.Contains(fragment.Content, want) {
			t.Fatalf("content missing %q: %s", want, fragment.Content)
		}
	}
	if !fragment.CachePolicy.Stable || fragment.CachePolicy.Scope != agentcontext.CacheThread {
		t.Fatalf("cache policy = %#v", fragment.CachePolicy)
	}
	if providerContext.Fingerprint == "" {
		t.Fatal("missing provider fingerprint")
	}
	fingerprint, ok, err := provider.StateFingerprint(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || fingerprint != providerContext.Fingerprint {
		t.Fatalf("state fingerprint = %q ok=%v, want %q", fingerprint, ok, providerContext.Fingerprint)
	}
}

func TestEnvironmentProviderSupportsCustomKey(t *testing.T) {
	provider := Environment(WithEnvironmentKey("env/custom"), WithHostname("host-a"))
	if provider.Key() != "env/custom" {
		t.Fatalf("key = %q", provider.Key())
	}
}
