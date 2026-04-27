package contextproviders

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
)

type EnvironmentOption func(*EnvironmentProvider)

type EnvironmentProvider struct {
	key        agentcontext.ProviderKey
	workDir    string
	hostname   string
	kernel     string
	readKernel func() string
}

func Environment(opts ...EnvironmentOption) *EnvironmentProvider {
	p := &EnvironmentProvider{
		key:        "environment",
		readKernel: defaultKernelVersion,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

func WithEnvironmentKey(key agentcontext.ProviderKey) EnvironmentOption {
	return func(p *EnvironmentProvider) { p.key = key }
}

func WithWorkDir(workDir string) EnvironmentOption {
	return func(p *EnvironmentProvider) { p.workDir = workDir }
}

func WithHostname(hostname string) EnvironmentOption {
	return func(p *EnvironmentProvider) { p.hostname = hostname }
}

func WithKernelVersion(version string) EnvironmentOption {
	return func(p *EnvironmentProvider) { p.kernel = version }
}

func WithKernelVersionFunc(fn func() string) EnvironmentOption {
	return func(p *EnvironmentProvider) { p.readKernel = fn }
}

func (p *EnvironmentProvider) Key() agentcontext.ProviderKey {
	if p == nil || p.key == "" {
		return "environment"
	}
	return p.key
}

func (p *EnvironmentProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	content := p.content()
	return agentcontext.ProviderContext{
		Fragments: []agentcontext.ContextFragment{{
			Key:       "environment/system",
			Role:      unified.RoleUser,
			Content:   content,
			Authority: agentcontext.AuthorityUser,
			CachePolicy: agentcontext.CachePolicy{
				Stable: true,
				Scope:  agentcontext.CacheThread,
			},
		}},
		Fingerprint: contentFingerprint("environment", content),
	}, nil
}

func (p *EnvironmentProvider) StateFingerprint(ctx context.Context, _ agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return contentFingerprint("environment", p.content()), true, nil
}

func (p *EnvironmentProvider) content() string {
	var b strings.Builder
	writeLine(&b, "working_directory", p.resolvedWorkDir())
	writeLine(&b, "os", goruntime.GOOS)
	writeLine(&b, "arch", goruntime.GOARCH)
	writeLine(&b, "kernel", p.resolvedKernel())
	writeLine(&b, "hostname", p.resolvedHostname())
	return b.String()
}

func (p *EnvironmentProvider) resolvedWorkDir() string {
	if p != nil && p.workDir != "" {
		if abs, err := filepath.Abs(p.workDir); err == nil {
			return abs
		}
		return p.workDir
	}
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func (p *EnvironmentProvider) resolvedHostname() string {
	if p != nil && p.hostname != "" {
		return p.hostname
	}
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	return hostname
}

func (p *EnvironmentProvider) resolvedKernel() string {
	if p != nil {
		if p.kernel != "" {
			return p.kernel
		}
		if p.readKernel != nil {
			return strings.TrimSpace(p.readKernel())
		}
	}
	return defaultKernelVersion()
}

func defaultKernelVersion() string {
	if goruntime.GOOS != "linux" {
		return ""
	}
	b, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func writeLine(b *strings.Builder, key string, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteByte('\n')
	}
	fmt.Fprintf(b, "%s: %s", key, value)
}
