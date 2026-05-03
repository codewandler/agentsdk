package harness

import "github.com/codewandler/agentsdk/command"

// CommandCatalogOption filters or configures command catalog projection.
type CommandCatalogOption func(*commandCatalogOptions)

type commandCatalogOptions struct {
	agentCallableOnly bool
	userCallableOnly  bool
}

// CommandCatalogAgentCallable limits the catalog to commands explicitly allowed
// for agent/tool invocation.
func CommandCatalogAgentCallable() CommandCatalogOption {
	return func(opts *commandCatalogOptions) { opts.agentCallableOnly = true }
}

// CommandCatalogUserCallable limits the catalog to commands available in
// user-facing command surfaces.
func CommandCatalogUserCallable() CommandCatalogOption {
	return func(opts *commandCatalogOptions) { opts.userCallableOnly = true }
}

// CommandCatalogEntry describes one executable command together with its input
// schema projection. The descriptor remains the source of truth; InputSchema is
// derived from Descriptor.Input.
type CommandCatalogEntry struct {
	Descriptor   command.Descriptor `json:"descriptor"`
	InputSchema  command.JSONSchema `json:"inputSchema"`
	OutputSchema command.JSONSchema `json:"outputSchema,omitempty"`
}

// Internal control commands are omitted from catalogs; they remain directly
// executable through session slash-command routing but are not projected to
// descriptor-backed API/tool catalogs.
// CommandCatalog returns a flattened catalog of executable commands exposed by
// the session. Non-executable namespace nodes are omitted, but executable parent
// nodes with subcommands would be included.
func (s *Session) CommandCatalog(opts ...CommandCatalogOption) []CommandCatalogEntry {
	if s == nil {
		return nil
	}
	commands, err := s.Commands()
	if err != nil {
		return nil
	}
	return commandCatalogFromDescriptors(commands.Descriptors(), opts...)
}

func commandCatalogFromDescriptors(descriptors []command.Descriptor, opts ...CommandCatalogOption) []CommandCatalogEntry {
	catalogOptions := commandCatalogOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&catalogOptions)
		}
	}
	var out []CommandCatalogEntry
	for _, desc := range descriptors {
		appendCommandCatalogEntries(&out, desc, catalogOptions)
	}
	return out
}

func appendCommandCatalogEntries(out *[]CommandCatalogEntry, desc command.Descriptor, opts commandCatalogOptions) {
	if desc.Executable && commandCatalogKeeps(desc, opts) {
		*out = append(*out, CommandCatalogEntry{
			Descriptor:   desc,
			InputSchema:  command.CommandInputSchema(desc),
			OutputSchema: desc.Output.Schema,
		})
	}
	for _, sub := range desc.Subcommands {
		appendCommandCatalogEntries(out, sub, opts)
	}
}

func commandCatalogKeeps(desc command.Descriptor, opts commandCatalogOptions) bool {
	if desc.Policy.Internal {
		return false
	}
	if opts.agentCallableOnly && !desc.AgentCallable() {
		return false
	}
	if opts.userCallableOnly && !desc.UserCallable() {
		return false
	}
	return true
}
