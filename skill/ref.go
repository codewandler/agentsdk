package skill

import "strings"

// RefMetadata is the minimal frontmatter schema for reference files (references/*.md).
// Intentionally minimal — only what the agent needs to decide when to load the reference.
// Does NOT include name, description, role, risk, or other skill-level fields.
type RefMetadata struct {
	// Trigger is a comma-separated list of trigger phrases (LLM-driven loading).
	// Use for 1-3 short triggers. For more, prefer Triggers (list form).
	Trigger string `yaml:"trigger,omitempty"`

	// Triggers is the list form of trigger phrases.
	// Takes precedence over Trigger if both are present.
	Triggers []string `yaml:"triggers,omitempty"`

	// When specifies conditions for automatic loading (detector-driven).
	// If non-nil and all conditions evaluate to true, the ref is auto-loaded at startup.
	// Orthogonal to Trigger/Triggers: both can be set simultaneously.
	When *WhenEntry `yaml:"when,omitempty"`
}

// AllTriggers returns the full deduplicated list of triggers.
// Triggers list takes precedence; Trigger string is split by comma and appended.
func (r *RefMetadata) AllTriggers() []string {
	if len(r.Triggers) > 0 {
		seen := make(map[string]bool)
		var out []string
		for _, t := range r.Triggers {
			if t := strings.TrimSpace(t); t != "" && !seen[t] {
				seen[t] = true
				out = append(out, t)
			}
		}
		return out
	}
	if r.Trigger == "" {
		return nil
	}
	var out []string
	seen := make(map[string]bool)
	for _, part := range strings.Split(r.Trigger, ",") {
		if t := strings.TrimSpace(part); t != "" && !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}
