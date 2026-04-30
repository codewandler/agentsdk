// Package standard assembles common agentsdk tool bundles.
package standard

import (
	"github.com/codewandler/agentsdk/activation"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/filesystem"
	"github.com/codewandler/agentsdk/tools/git"
	"github.com/codewandler/agentsdk/tools/jsonquery"
	"github.com/codewandler/agentsdk/tools/notify"
	"github.com/codewandler/agentsdk/tools/phone"
	"github.com/codewandler/agentsdk/tools/shell"
	"github.com/codewandler/agentsdk/tools/skills"
	"github.com/codewandler/agentsdk/tools/todo"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
	"github.com/codewandler/agentsdk/tools/turn"
	"github.com/codewandler/agentsdk/tools/vision"
	"github.com/codewandler/agentsdk/tools/web"
	"github.com/codewandler/agentsdk/websearch"
	"github.com/codewandler/cmdrisk"
)

// Options configures a standard tool bundle.
type Options struct {
	WebSearchProvider websearch.Provider

	// RiskAnalyzer is the cmdrisk analyzer used by the bash tool for
	// command risk assessment. When nil, a default analyzer is created
	// automatically (cmdrisk.New with default policy — no external deps).
	// Set to an explicit analyzer to customise the cmdrisk policy.
	RiskAnalyzer *cmdrisk.Analyzer

	// NoDefaultRiskAnalyzer disables automatic creation of a default
	// cmdrisk analyzer. When true and RiskAnalyzer is nil, the bash tool
	// falls back to opaque intent (no command analysis).
	NoDefaultRiskAnalyzer bool

	IncludeGit            bool
	IncludeNotify         bool
	IncludeTodo           bool
	IncludeToolManagement bool
	IncludeTurnDone       bool
	IncludeVision         bool

	// PhoneConfig configures the phone tool for SIP call origination.
	// When non-nil, the phone tool is included. If SIPAddr is empty,
	// dial operations must provide sip_endpoint.
	PhoneConfig *phone.Config
}

// Toolset groups a standard tool bundle with the activation manager that owns
// its active/inactive state.
type Toolset struct {
	tools      []tool.Tool
	activation *activation.Manager
}

// NewToolset returns a standard tool bundle with all tools initially active.
func NewToolset(opts Options) *Toolset {
	return NewToolsetFromTools(Tools(opts)...)
}

// NewToolsetFromTools returns an activation-backed toolset for an explicit list
// of tools.
func NewToolsetFromTools(tools ...tool.Tool) *Toolset {
	return &Toolset{
		tools:      append([]tool.Tool(nil), tools...),
		activation: activation.New(tools...),
	}
}

// DefaultToolset returns the default lightweight terminal-agent toolset.
func DefaultToolset() *Toolset {
	return NewToolset(DefaultOptions())
}

// Tools returns all tools in the bundle.
func (s *Toolset) Tools() []tool.Tool {
	if s == nil {
		return nil
	}
	return append([]tool.Tool(nil), s.tools...)
}

// Activation returns the activation manager for the bundle.
func (s *Toolset) Activation() *activation.Manager {
	if s == nil {
		return nil
	}
	return s.activation
}

// ActiveTools returns the currently active tools in bundle order.
func (s *Toolset) ActiveTools() []tool.Tool {
	if s == nil || s.activation == nil {
		return nil
	}
	return s.activation.ActiveTools()
}

// Tools returns the common coding-agent tools plus optional extras.
func Tools(opts Options) []tool.Tool {
	var out []tool.Tool
	out = append(out, shell.Tools(shellOpts(opts)...)...)
	out = append(out, filesystem.Tools()...)
	out = append(out, jsonquery.Tools()...)
	out = append(out, web.Tools(opts.WebSearchProvider)...)
	out = append(out, skills.Tools()...)
	if opts.IncludeGit {
		out = append(out, git.Tools()...)
	}
	if opts.IncludeNotify {
		out = append(out, notify.Tools()...)
	}
	if opts.IncludeTodo {
		out = append(out, todo.Tools()...)
	}
	if opts.IncludeToolManagement {
		out = append(out, toolmgmt.Tools()...)
	}
	if opts.IncludeTurnDone {
		out = append(out, turn.Tools()...)
	}
	if opts.IncludeVision {
		out = append(out, vision.Tools(vision.ClientFromEnv())...)
	}
	if opts.PhoneConfig != nil {
		out = append(out, phone.Tools(*opts.PhoneConfig)...)
	}
	return out
}

// DefaultOptions returns the default bundle options used by lightweight terminal agents.
func DefaultOptions() Options {
	return Options{
		WebSearchProvider:     web.DefaultSearchProviderFromEnv(),
		IncludeGit:            true,
		IncludeToolManagement: true,
	}
}

// DefaultTools returns the default bundle used by lightweight terminal agents.
func DefaultTools() []tool.Tool {
	return Tools(DefaultOptions())
}

// CatalogOptions returns the full standard tool catalog for app/resource
// selection. It is intentionally broader than DefaultOptions: explicit agent
// specs may opt into optional tools without making every default agent start
// with those tools active.
func CatalogOptions() Options {
	opts := DefaultOptions()
	opts.IncludeGit = true
	opts.IncludeNotify = true
	opts.IncludeTodo = true
	opts.IncludeTurnDone = true
	opts.IncludeVision = true
	opts.PhoneConfig = &phone.Config{}
	return opts
}

// CatalogTools returns all standard tools that app/resource agents may select.
// If web search is not configured, the catalog still contains web_search as a
// call-time configuration error so resource bundles can start consistently.
func CatalogTools() []tool.Tool {
	opts := CatalogOptions()
	tools := Tools(opts)
	if opts.WebSearchProvider == nil {
		tools = append(tools, web.SearchTool(nil))
	}
	return tools
}

// shellOpts returns shell.Option values derived from the standard Options.
func shellOpts(opts Options) []shell.Option {
	analyzer := opts.RiskAnalyzer
	if analyzer == nil && !opts.NoDefaultRiskAnalyzer {
		analyzer = cmdrisk.New(cmdrisk.Config{})
	}
	if analyzer == nil {
		return nil
	}
	return []shell.Option{shell.WithRiskAnalyzer(analyzer)}
}
