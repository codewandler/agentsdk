package contextproviders

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
)

// Cmd describes one shell command whose output becomes a context fragment line.
type Cmd struct {
	// Key is the label rendered before the output (e.g. "branch").
	Key string
	// Command is the executable name.
	Command string
	// Args are the command arguments.
	Args []string
	// Optional marks the command as non-fatal: if it fails the key is
	// silently omitted instead of failing the provider.
	Optional bool
	// TrimOutput controls whether leading/trailing whitespace is stripped
	// from the command output. Default true when zero value.
	TrimOutput *bool
}

// CmdProviderOption configures a CmdProvider.
type CmdProviderOption func(*CmdProvider)

// CmdProvider runs a list of commands and renders their output as key/value
// lines in a single context fragment. Commands that produce empty output are
// omitted. The provider is useful for lightweight system/environment probes
// where each command returns a short value.
type CmdProvider struct {
	key       agentcontext.ProviderKey
	fragment  agentcontext.FragmentKey
	role      unified.Role
	authority agentcontext.FragmentAuthority
	cache     agentcontext.CachePolicy
	workDir   string
	timeout   time.Duration
	cmds      []Cmd
	runCmd    func(ctx context.Context, workDir string, name string, args ...string) (string, error)
}

// CmdContext creates a provider that runs commands and renders their output as
// key/value context lines.
func CmdContext(key agentcontext.ProviderKey, fragment agentcontext.FragmentKey, cmds []Cmd, opts ...CmdProviderOption) *CmdProvider {
	p := &CmdProvider{
		key:       key,
		fragment:  fragment,
		role:      unified.RoleUser,
		authority: agentcontext.AuthorityUser,
		cache:     agentcontext.CachePolicy{Scope: agentcontext.CacheTurn},
		timeout:   5 * time.Second,
		cmds:      append([]Cmd(nil), cmds...),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// WithCmdWorkDir sets the working directory for all commands.
func WithCmdWorkDir(dir string) CmdProviderOption {
	return func(p *CmdProvider) { p.workDir = dir }
}

// WithCmdTimeout sets the per-command timeout.
func WithCmdTimeout(timeout time.Duration) CmdProviderOption {
	return func(p *CmdProvider) { p.timeout = timeout }
}

// WithCmdRole sets the fragment role.
func WithCmdRole(role unified.Role) CmdProviderOption {
	return func(p *CmdProvider) { p.role = role }
}

// WithCmdAuthority sets the fragment authority.
func WithCmdAuthority(authority agentcontext.FragmentAuthority) CmdProviderOption {
	return func(p *CmdProvider) { p.authority = authority }
}

// WithCmdCache sets the fragment cache policy.
func WithCmdCache(cache agentcontext.CachePolicy) CmdProviderOption {
	return func(p *CmdProvider) { p.cache = cache }
}

// WithCmdRunner overrides the command runner for testing.
func WithCmdRunner(run func(ctx context.Context, workDir string, name string, args ...string) (string, error)) CmdProviderOption {
	return func(p *CmdProvider) { p.runCmd = run }
}

func (p *CmdProvider) Key() agentcontext.ProviderKey {
	if p == nil || p.key == "" {
		return "cmd"
	}
	return p.key
}

func (p *CmdProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	content, err := p.render(ctx)
	if err != nil {
		return agentcontext.ProviderContext{}, err
	}
	fp := contentFingerprint(string(p.key), content)
	if content == "" {
		return agentcontext.ProviderContext{Fingerprint: fp}, nil
	}
	return agentcontext.ProviderContext{
		Fragments: []agentcontext.ContextFragment{{
			Key:         p.fragment,
			Role:        p.role,
			Content:     content,
			Authority:   p.authority,
			CachePolicy: p.cache,
		}},
		Fingerprint: fp,
	}, nil
}

func (p *CmdProvider) StateFingerprint(ctx context.Context, _ agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	content, err := p.render(ctx)
	if err != nil {
		return "", false, err
	}
	return contentFingerprint(string(p.key), content), true, nil
}

func (p *CmdProvider) render(ctx context.Context) (string, error) {
	if p == nil || len(p.cmds) == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, cmd := range p.cmds {
		out, err := p.run(ctx, cmd)
		if err != nil {
			if cmd.Optional {
				continue
			}
			return "", fmt.Errorf("cmd provider %s: %s: %w", p.key, cmd.Key, err)
		}
		if cmd.TrimOutput == nil || *cmd.TrimOutput {
			out = strings.TrimSpace(out)
		}
		if out == "" {
			continue
		}
		writeLine(&b, cmd.Key, out)
	}
	return b.String(), nil
}

func (p *CmdProvider) run(ctx context.Context, cmd Cmd) (string, error) {
	if p.runCmd != nil {
		return p.runCmd(ctx, p.workDir, cmd.Command, cmd.Args...)
	}
	timeout := p.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(cmdCtx, cmd.Command, cmd.Args...)
	if p.workDir != "" {
		c.Dir = p.workDir
	}
	out, err := c.CombinedOutput()
	if err != nil {
		if errors.Is(cmdCtx.Err(), context.DeadlineExceeded) {
			return "", cmdCtx.Err()
		}
		return "", fmt.Errorf("%s %s: %w: %s", cmd.Command, strings.Join(cmd.Args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
