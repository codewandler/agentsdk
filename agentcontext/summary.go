package agentcontext

import (
	"fmt"
	"sort"
	"strings"
)

// FormatRenderRecords renders the last committed provider records for humans.
func FormatRenderRecords(records map[ProviderKey]ProviderRenderRecord) string {
	if len(records) == 0 {
		return "context: no render state"
	}
	providers := make([]ProviderKey, 0, len(records))
	for key := range records {
		providers = append(providers, key)
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i] < providers[j] })

	var b strings.Builder
	b.WriteString("context:\n")
	for _, providerKey := range providers {
		record := records[providerKey]
		fmt.Fprintf(&b, "- provider: %s\n", providerKey)
		if record.Fingerprint != "" {
			fmt.Fprintf(&b, "  fingerprint: %s\n", record.Fingerprint)
		}
		if record.Snapshot != nil && record.Snapshot.Fingerprint != "" {
			fmt.Fprintf(&b, "  snapshot: %s\n", record.Snapshot.Fingerprint)
		}
		if len(record.Fragments) == 0 {
			b.WriteString("  fragments: none\n")
			continue
		}
		b.WriteString("  fragments:\n")
		keys := make([]FragmentKey, 0, len(record.Fragments))
		for key := range record.Fragments {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
		for _, key := range keys {
			fragment := record.Fragments[key]
			status := "active"
			if fragment.Removed {
				status = "removed"
			}
			fmt.Fprintf(&b, "  - %s [%s]", key, status)
			if fragment.Fingerprint != "" {
				fmt.Fprintf(&b, " fp=%s", fragment.Fingerprint)
			}
			b.WriteByte('\n')
			content := strings.TrimSpace(fragment.Fragment.Content)
			if content != "" && !fragment.Removed {
				writeIndented(&b, content, "    ")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// LastRenderState renders the manager's last committed records for humans.
func (m *Manager) LastRenderState() string {
	if m == nil {
		return "context: unavailable"
	}
	return FormatRenderRecords(m.Records())
}

func writeIndented(b *strings.Builder, text, prefix string) {
	for _, line := range strings.Split(text, "\n") {
		fmt.Fprintf(b, "%s%s\n", prefix, line)
	}
}
