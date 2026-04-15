package skill

// SkillMetadata is the typed, versioned convention for SKILL.md frontmatter.
type SkillMetadata struct {
	Name          string      `yaml:"name"`
	Description   string      `yaml:"description"`
	Risk          string      `yaml:"risk,omitempty"`
	Compatibility string      `yaml:"compatibility,omitempty"`
	Triggers      []string    `yaml:"triggers,omitempty"`
	Domain        string      `yaml:"domain,omitempty"`
	Role          string      `yaml:"role,omitempty"`
	Coder         *CoderBlock `yaml:"coder,omitempty"`
	// When declares conditions for automatic loading of this skill (Approach A).
	// If non-nil and all conditions match at startup, the skill is auto-loaded
	// without any parent skill needing to enumerate it.
	When  *WhenEntry     `yaml:"when,omitempty"`
	Extra map[string]any `yaml:"-"`
}

// CoderBlock holds coder-agent-specific frontmatter conventions.
type CoderBlock struct {
	Kind     string        `yaml:"kind,omitempty"`
	Triggers []string      `yaml:"triggers,omitempty"`
	Ensure   []EnsureEntry `yaml:"ensure,omitempty"`
	When     []WhenEntry   `yaml:"when,omitempty"`
	Examples []string      `yaml:"examples,omitempty"`
}

// EnsureEntry declares a required file path, tool, or skill dependency.
type EnsureEntry struct {
	Path  string `yaml:"path,omitempty"`
	Tool  string `yaml:"tool,omitempty"`
	Skill string `yaml:"skill,omitempty"`
}

// WhenEntry declares a condition under which the skill or reference is relevant.
// All specified fields must match (AND logic). Unset fields are ignored.
type WhenEntry struct {
	OS     string   `yaml:"os,omitempty"`
	Env    string   `yaml:"env,omitempty"`
	Tools  []string `yaml:"tools,omitempty"`
	Skills []string `yaml:"skills,omitempty"`
	// Language matches if the detected language equals this value (case-insensitive).
	Language string `yaml:"language,omitempty"`
	// Languages matches if any detected language equals one of these values (OR logic).
	Languages []string `yaml:"languages,omitempty"`
	// Files lists glob patterns relative to workdir. All must match at least one file.
	Files []string `yaml:"files,omitempty"`
	// Refs declares specific reference paths to auto-load within named skills.
	// Keys are skill names; values are ref paths relative to the skill root.
	// Applied after Skills are loaded. Does NOT affect condition evaluation.
	Refs map[string][]string `yaml:"refs,omitempty"`
}

// IsZero reports whether all fields of the entry are empty.
func (e WhenEntry) IsZero() bool {
	return e.OS == "" && e.Env == "" &&
		len(e.Tools) == 0 && len(e.Skills) == 0 &&
		e.Language == "" && len(e.Languages) == 0 &&
		len(e.Files) == 0 && len(e.Refs) == 0
}
