package contextproviders

import (
	"github.com/codewandler/agentsdk/agentcontext"
)

// AgentsMarkdownOption configures the AGENTS.md file-backed context provider.
type AgentsMarkdownOption func(*FileProvider)

// AgentsMarkdown renders discovered AGENTS.md-style project instruction files as
// diffable context fragments that are re-read from disk on each render.
func AgentsMarkdown(paths []string, opts ...AgentsMarkdownOption) agentcontext.Provider {
	files := make([]FileSpec, 0, len(paths))
	for _, path := range paths {
		files = append(files, FileSpec{
			Path:      path,
			Key:       agentcontext.FragmentKey("agents_md/" + sanitizeKey(path)),
			Optional:  true,
			Authority: agentcontext.AuthorityUser,
		})
	}
	providerOpts := make([]FileProviderOption, 0, len(opts))
	for _, opt := range opts {
		if opt != nil {
			providerOpts = append(providerOpts, FileProviderOption(opt))
		}
	}
	return FileContext("agents_markdown", files, providerOpts...)
}
