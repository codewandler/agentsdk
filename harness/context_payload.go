package harness

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/command"
)

type ContextStatePayload struct {
	State ContextState `json:"state"`
	Text  string       `json:"text,omitempty"`
}

func (p ContextStatePayload) Display(mode command.DisplayMode) (string, error) {
	if mode == command.DisplayJSON {
		data, err := json.MarshalIndent(p, "", "  ")
		if err != nil {
			return "", err
		}
		return string(data), nil
	}
	if strings.TrimSpace(p.Text) != "" {
		return p.Text, nil
	}
	return formatContextState(p.State), nil
}

func formatContextState(state ContextState) string {
	if state.Text != "" && len(state.Snapshot.Providers) == 0 {
		return state.Text
	}
	if len(state.Snapshot.Providers) == 0 {
		return "context: no render state"
	}
	var b strings.Builder
	if state.Agent != "" {
		fmt.Fprintf(&b, "context for agent %q:\n", state.Agent)
	} else {
		b.WriteString("context:\n")
	}
	for _, provider := range state.Snapshot.Providers {
		desc := provider.Descriptor
		fmt.Fprintf(&b, "- provider: %s\n", desc.Key)
		if desc.Description != "" {
			fmt.Fprintf(&b, "  description: %s\n", desc.Description)
		}
		if desc.Lifecycle != "" {
			fmt.Fprintf(&b, "  lifecycle: %s\n", desc.Lifecycle)
		}
		if provider.Fingerprint != "" {
			fmt.Fprintf(&b, "  fingerprint: %s\n", provider.Fingerprint)
		}
		if len(provider.Fragments) == 0 {
			b.WriteString("  fragments: none\n")
			continue
		}
		b.WriteString("  fragments:\n")
		for _, fragment := range provider.Fragments {
			status := "active"
			if fragment.Removed {
				status = "removed"
			}
			fmt.Fprintf(&b, "  - %s [%s]", fragment.Key, status)
			if fragment.Fingerprint != "" {
				fmt.Fprintf(&b, " fp=%s", fragment.Fingerprint)
			}
			if fragment.Authority != "" {
				fmt.Fprintf(&b, " authority=%s", fragment.Authority)
			}
			b.WriteByte('\n')
			if !fragment.Removed && strings.TrimSpace(fragment.Content) != "" {
				writeContextIndented(&b, fragment.Content, "    ")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func writeContextIndented(b *strings.Builder, text, prefix string) {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		fmt.Fprintf(b, "%s%s\n", prefix, line)
	}
}

var _ command.Displayable = ContextStatePayload{}
