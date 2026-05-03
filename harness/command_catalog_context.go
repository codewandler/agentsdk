package harness

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/llmadapter/unified"
)

const (
	AgentCommandCatalogProviderKey agentcontext.ProviderKey = "agent_command_catalog"
	AgentCommandCatalogFragmentKey agentcontext.FragmentKey = "agent_command_catalog/session_command"
)

// formatAgentCommandCatalog renders agent-callable session commands as compact
// model context for the generic session_command tool.
func formatAgentCommandCatalog(catalog []CommandCatalogEntry) string {
	if len(catalog) == 0 {
		return "No agent-callable session commands are available."
	}
	entries := append([]CommandCatalogEntry(nil), catalog...)
	sort.SliceStable(entries, func(i, j int) bool {
		return strings.Join(entries[i].Descriptor.Path, " ") < strings.Join(entries[j].Descriptor.Path, " ")
	})

	var b strings.Builder
	b.WriteString("Available session commands for the session_command tool.\n")
	b.WriteString("Call the tool with a command path array and an input object matching the selected command.\n")
	b.WriteString("\nCommands:\n")
	for _, entry := range entries {
		desc := entry.Descriptor
		fmt.Fprintf(&b, "- %s", strings.Join(desc.Path, " "))
		if desc.Description != "" {
			fmt.Fprintf(&b, ": %s", desc.Description)
		}
		b.WriteByte('\n')
		writeAgentCommandCatalogInput(&b, desc)
		writeAgentCommandCatalogOutput(&b, desc.Output)
	}
	return strings.TrimRight(b.String(), "\n")
}

func (s *Session) agentCommandCatalogContextProvider() agentcontext.Provider {
	return agentCommandCatalogProvider{Session: s}
}

type agentCommandCatalogProvider struct {
	Session *Session
}

func (p agentCommandCatalogProvider) Key() agentcontext.ProviderKey {
	return AgentCommandCatalogProviderKey
}

func (p agentCommandCatalogProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	content := formatAgentCommandCatalog(p.Session.CommandCatalog(CommandCatalogAgentCallable()))
	return agentcontext.ProviderContext{
		Fragments: []agentcontext.ContextFragment{{
			Key:         AgentCommandCatalogFragmentKey,
			Role:        unified.RoleUser,
			Content:     content,
			Authority:   agentcontext.AuthorityDeveloper,
			CachePolicy: agentcontext.CachePolicy{Stable: true, Scope: agentcontext.CacheThread},
		}},
		Fingerprint: agentcontext.ProviderFingerprint([]agentcontext.ContextFragment{{
			Key:     AgentCommandCatalogFragmentKey,
			Content: content,
		}}),
	}, nil
}

func (p agentCommandCatalogProvider) StateFingerprint(ctx context.Context, req agentcontext.Request) (string, bool, error) {
	providerContext, err := p.GetContext(ctx, req)
	if err != nil {
		return "", false, err
	}
	return providerContext.Fingerprint, true, nil
}

func writeAgentCommandCatalogInput(b *strings.Builder, desc command.Descriptor) {
	if len(desc.Input.Fields) == 0 {
		b.WriteString("  input: {}\n")
		return
	}
	b.WriteString("  input:\n")
	fields := append([]command.InputFieldDescriptor(nil), desc.Input.Fields...)
	sort.SliceStable(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for _, field := range fields {
		fmt.Fprintf(b, "  - %s: %s", field.Name, field.Type)
		if field.Required {
			b.WriteString(" required")
		} else {
			b.WriteString(" optional")
		}
		if field.Variadic {
			b.WriteString(" variadic")
		}
		if field.Source != "" {
			fmt.Fprintf(b, " source=%s", field.Source)
		}
		if len(field.EnumValues) > 0 {
			fmt.Fprintf(b, " enum=%s", strings.Join(field.EnumValues, "|"))
		}
		if field.Description != "" {
			fmt.Fprintf(b, " - %s", field.Description)
		}
		b.WriteByte('\n')
	}
}

func writeAgentCommandCatalogOutput(b *strings.Builder, output command.OutputDescriptor) {
	if output.Kind == "" && output.Description == "" && output.Schema.Type == "" {
		return
	}
	b.WriteString("  output:")
	if output.Kind != "" {
		fmt.Fprintf(b, " %s", output.Kind)
	}
	if output.Schema.Type != "" {
		fmt.Fprintf(b, " schema=%s", output.Schema.Type)
	}
	if len(output.MediaTypes) > 0 {
		fmt.Fprintf(b, " media=%s", strings.Join(output.MediaTypes, "|"))
	}
	if output.Description != "" {
		fmt.Fprintf(b, " - %s", output.Description)
	}
	b.WriteByte('\n')
}
