// Package skill defines the interfaces and value types for the flai skill system.
// Skills are reusable markdown instruction carriers. This package has no external
// dependencies so any package can import it without pulling in runtime machinery.
package skill

import (
	"context"
	"errors"
	"io"
)

// ErrNotLoaded is returned by Unload when the requested skill is not currently active.
var ErrNotLoaded = errors.New("skill not loaded")

// ── Core types ────────────────────────────────────────────────────────────────

// SkillFrontmatter holds the structured metadata from a SKILL.md frontmatter block.
// Parsing is performed by the runtime layer; this type carries no YAML struct tags.
type SkillFrontmatter struct {
	Name          string
	Description   string
	Compatibility string
	// Triggers is a comma-separated list of keywords (legacy flat field).
	// Prefer Parsed.Coder.Triggers for typed access.
	Triggers string
	Domain   string
	Role     string
	Metadata map[string]any
	// Parsed holds the fully typed metadata from SkillMetadata.
	// Always non-nil after ParseFrontmatter succeeds.
	Parsed *SkillMetadata
}

// File represents a single file belonging to a skill.
type File interface {
	// Path returns the path relative to the skill root directory,
	// e.g. "SKILL.md" or "references/review.md".
	Path() string
	// Open returns a reader for the file content.
	// The caller must close the returned ReadCloser.
	Open() (io.ReadCloser, error)
	// RefFrontmatter returns parsed YAML frontmatter from the file.
	// Returns nil if no frontmatter is present.
	RefFrontmatter() (*RefMetadata, error)
}

// Skill represents a discovered skill and its metadata.
type Skill interface {
	Name() string
	Description() string
	Frontmatter() SkillFrontmatter
	// Source returns a label for where the skill was loaded from, e.g. "embedded" or a dir path.
	Source() string
	MainFile() File
	// ListReferences returns all files other than SKILL.md in the skill directory.
	ListReferences() []File
}

// Loader discovers skills from a single source (directory, embedded FS, etc.).
type Loader interface {
	// Load returns all skills discoverable from this source.
	// Missing or empty sources return (nil, nil).
	Load() ([]Skill, error)
	// Source returns a human-readable label for this loader.
	Source() string
}

// ── Registry ──────────────────────────────────────────────────────────────────

// Registry manages skill discovery and load/unload state.
// All methods must be safe for concurrent use.
type Registry interface {
	Scan()
	List() []Skill
	Get(name string) (Skill, bool)
	IsLoaded(name string) bool
	Load(name string) error
	Unload(name string) error
	LoadedNames() []string
	LoadPaths(skillName string, paths []string) error
	UnloadPaths(skillName string) error
	LoadedPaths(skillName string) []File
	LoadWithDependencies(name string) error
}

// DynamicRegistry extends Registry with dynamic loading from arbitrary sources.
type DynamicRegistry interface {
	Registry
	// AddLoader adds a skill loader to the registry. The loader's skills become
	// discoverable via List() and can be loaded via Load(). Call Scan() to refresh.
	AddLoader(loader Loader)
	LoadFrom(ctx context.Context, source string) (name string, err error)
}

// RegistryKey is the Extra() key under which skill tools look up the Registry.
const RegistryKey = "flai.skill_registry"
