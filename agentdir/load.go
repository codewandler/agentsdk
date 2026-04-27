// Package agentdir loads filesystem-described agent resources.
package agentdir

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/capability"
	cmdmarkdown "github.com/codewandler/agentsdk/command/markdown"
	md "github.com/codewandler/agentsdk/markdown"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/llmadapter/unified"
)

type AgentFrontmatter struct {
	Name         string         `yaml:"name"`
	Description  string         `yaml:"description"`
	Model        string         `yaml:"model"`
	MaxTokens    int            `yaml:"max-tokens"`
	MaxSteps     int            `yaml:"max-steps"`
	Temperature  float64        `yaml:"temperature"`
	Thinking     string         `yaml:"thinking"`
	Effort       string         `yaml:"effort"`
	Tools        stringList     `yaml:"tools"`
	Skills       stringList     `yaml:"skills"`
	Commands     stringList     `yaml:"commands"`
	SkillSources stringList     `yaml:"skill-sources"`
	Capabilities capabilityList `yaml:"capabilities"`
}

type stringList []string

func (l *stringList) UnmarshalYAML(unmarshal func(any) error) error {
	var values []string
	if err := unmarshal(&values); err == nil {
		*l = cleanStringList(values)
		return nil
	}
	var single string
	if err := unmarshal(&single); err != nil {
		return err
	}
	var out []string
	for _, part := range strings.Split(single, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	*l = out
	return nil
}

func cleanStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

// capabilityList supports both short and long forms in YAML frontmatter:
//
//	capabilities: [planner]                          # short: name only
//	capabilities:
//	  - planner                                      # short: name only
//	  - name: planner                                # long: explicit fields
//	    instance-id: my-planner
type capabilityList []capability.AttachSpec

func (l *capabilityList) UnmarshalYAML(unmarshal func(any) error) error {
	// Try structured list first.
	var structured []capabilityEntry
	if err := unmarshal(&structured); err == nil {
		specs := make([]capability.AttachSpec, 0, len(structured))
		for _, entry := range structured {
			spec := entry.toAttachSpec()
			if spec.CapabilityName != "" {
				specs = append(specs, spec)
			}
		}
		*l = specs
		return nil
	}
	// Fall back to plain string list.
	var names []string
	if err := unmarshal(&names); err == nil {
		specs := make([]capability.AttachSpec, 0, len(names))
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name != "" {
				specs = append(specs, capability.AttachSpec{
					CapabilityName: name,
					InstanceID:     "default",
				})
			}
		}
		*l = specs
		return nil
	}
	// Single string.
	var single string
	if err := unmarshal(&single); err != nil {
		return err
	}
	var specs []capability.AttachSpec
	for _, part := range strings.Split(single, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			specs = append(specs, capability.AttachSpec{
				CapabilityName: part,
				InstanceID:     "default",
			})
		}
	}
	*l = specs
	return nil
}

type capabilityEntry struct {
	Name       string `yaml:"name"`
	InstanceID string `yaml:"instance-id"`
}

func (e *capabilityEntry) UnmarshalYAML(unmarshal func(any) error) error {
	// Try structured map first.
	type plain capabilityEntry
	var p plain
	if err := unmarshal(&p); err == nil {
		*e = capabilityEntry(p)
		return nil
	}
	// Fall back to bare string.
	var name string
	if err := unmarshal(&name); err != nil {
		return err
	}
	e.Name = strings.TrimSpace(name)
	return nil
}

func (e capabilityEntry) toAttachSpec() capability.AttachSpec {
	instanceID := strings.TrimSpace(e.InstanceID)
	if instanceID == "" {
		instanceID = "default"
	}
	return capability.AttachSpec{
		CapabilityName: strings.TrimSpace(e.Name),
		InstanceID:     instanceID,
	}
}

// LoadDir loads resources from an OS directory.
func LoadDir(dir string) (resource.ContributionBundle, error) {
	return LoadFSWithSource(os.DirFS(dir), ".", sourceForPath(dir, resource.ScopeProject))
}

func LoadDirWithSource(dir string, source resource.SourceRef) (resource.ContributionBundle, error) {
	return LoadFSWithSource(os.DirFS(dir), ".", source)
}

// LoadFS loads resources from fsys rooted at root.
func LoadFS(fsys fs.FS, root string) (resource.ContributionBundle, error) {
	return LoadFSWithSource(fsys, root, sourceForPath(root, resource.ScopeEmbedded))
}

func LoadFSWithSource(fsys fs.FS, root string, source resource.SourceRef) (resource.ContributionBundle, error) {
	var out resource.ContributionBundle
	out.Source = source
	out.ID = source.ID
	out.Name = source.Label()
	root = clean(root)
	for _, dir := range []string{
		path.Join(root, ".agents", "commands"),
		path.Join(root, ".claude", "commands"),
		path.Join(root, "commands"),
	} {
		cmds, err := cmdmarkdown.LoadFS(fsys, dir)
		if err != nil {
			return resource.ContributionBundle{}, err
		}
		out.Commands = append(out.Commands, cmds...)
	}
	for _, dir := range []string{
		path.Join(root, ".agents", "agents"),
		path.Join(root, ".claude", "agents"),
		path.Join(root, "agents"),
	} {
		loaded, err := loadAgentSpecs(fsys, dir, source)
		if err != nil {
			return resource.ContributionBundle{}, err
		}
		out.AgentSpecs = append(out.AgentSpecs, loaded.Specs...)
		out.Skills = append(out.Skills, loaded.Skills...)
	}
	for order, dir := range []string{
		path.Join(root, ".agents", "skills"),
		path.Join(root, ".claude", "skills"),
		path.Join(root, "skills"),
	} {
		if exists, err := dirExists(fsys, dir); err != nil {
			return resource.ContributionBundle{}, err
		} else if exists {
			out.SkillSources = append(out.SkillSources, skill.FSSource(dir, dir, fsys, dir, sourceKind(dir), order))
			skills, err := loadSkillContributions(fsys, dir, source)
			if err != nil {
				return resource.ContributionBundle{}, err
			}
			out.Skills = append(out.Skills, skills...)
		}
	}
	return out, nil
}

func instructionPathsForAgentFile(file string) []string {
	file = clean(file)
	if file == "" {
		return nil
	}
	dir := path.Dir(file)
	if dir == "." || dir == "" {
		return []string{"AGENTS.md"}
	}
	seen := map[string]bool{}
	var out []string
	for {
		candidate := path.Join(dir, "AGENTS.md")
		if !seen[candidate] {
			seen[candidate] = true
			out = append(out, candidate)
		}
		if dir == "." || dir == "" || dir == "/" {
			break
		}
		next := path.Dir(dir)
		if next == dir {
			break
		}
		dir = next
	}
	if !seen["AGENTS.md"] {
		out = append(out, "AGENTS.md")
	}
	return out
}

type loadedAgentSpecs struct {
	Specs  []agent.Spec
	Skills []resource.SkillContribution
}

func loadAgentSpecs(fsys fs.FS, dir string, source resource.SourceRef) (loadedAgentSpecs, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return loadedAgentSpecs{}, nil
		}
		return loadedAgentSpecs{}, err
	}
	var out loadedAgentSpecs
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		file := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return loadedAgentSpecs{}, fmt.Errorf("read agent spec %q: %w", file, err)
		}
		spec, fm, err := parseAgentSpec(entry.Name(), data)
		if err != nil {
			return loadedAgentSpecs{}, fmt.Errorf("parse agent spec %q: %w", file, err)
		}
		spec.ResourceID = resource.QualifiedID(source, "agent", spec.Name, qualifiedResourcePath(source, file))
		spec.ResourceFrom = source.Label()
		sources, skills, err := skillSourcesFromFrontmatter(fsys, path.Dir(file), file, []string(fm.SkillSources), source)
		if err != nil {
			return loadedAgentSpecs{}, err
		}
		spec.SkillSources = append(spec.SkillSources, sources...)
		spec.InstructionPaths = append(spec.InstructionPaths, instructionPathsForAgentFile(file)...)
		out.Specs = append(out.Specs, spec)
		out.Skills = append(out.Skills, skills...)
	}
	for i := range out.Specs {
		out.Specs[i].InstructionPaths = uniqueStrings(out.Specs[i].InstructionPaths)
	}
	return out, nil
}

func ParseAgentSpec(name string, content []byte) (agent.Spec, error) {
	spec, _, err := parseAgentSpec(name, content)
	return spec, err
}

func parseAgentSpec(name string, content []byte) (agent.Spec, AgentFrontmatter, error) {
	meta, body, err := md.Parse(strings.NewReader(string(content)))
	if err != nil {
		return agent.Spec{}, AgentFrontmatter{}, err
	}
	fm, err := md.Bind[AgentFrontmatter](meta)
	if err != nil {
		return agent.Spec{}, AgentFrontmatter{}, err
	}
	if fm.Name == "" {
		fm.Name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}
	inference := agent.DefaultInferenceOptions()
	if fm.Model != "" {
		inference.Model = fm.Model
	}
	if fm.MaxTokens > 0 {
		inference.MaxTokens = fm.MaxTokens
	}
	if fm.Temperature != 0 {
		inference.Temperature = fm.Temperature
	}
	if fm.Thinking != "" {
		inference.Thinking = agent.ThinkingMode(fm.Thinking)
	}
	if fm.Effort != "" {
		inference.Effort = unified.ReasoningEffort(fm.Effort)
	}
	return agent.Spec{
		Name:         fm.Name,
		Description:  fm.Description,
		System:       body,
		Inference:    inference,
		MaxSteps:     fm.MaxSteps,
		Tools:        append([]string(nil), []string(fm.Tools)...),
		Skills:       append([]string(nil), []string(fm.Skills)...),
		Commands:     append([]string(nil), []string(fm.Commands)...),
		Capabilities: append([]capability.AttachSpec(nil), []capability.AttachSpec(fm.Capabilities)...),
	}, fm, nil
}

func skillSourcesFromFrontmatter(fsys fs.FS, agentDir string, agentFile string, roots []string, source resource.SourceRef) ([]skill.Source, []resource.SkillContribution, error) {
	var sources []skill.Source
	var skills []resource.SkillContribution
	for order, root := range roots {
		root = clean(root)
		if root == "." {
			continue
		}
		sourceRoot := root
		if !path.IsAbs(sourceRoot) {
			sourceRoot = path.Join(agentDir, sourceRoot)
		}
		sourceRoot = clean(sourceRoot)
		id := fmt.Sprintf("%s:skill-sources:%s", agentFile, root)
		sources = append(sources, skill.FSSource(id, sourceRoot, fsys, sourceRoot, sourceKind(sourceRoot), order))
		loaded, err := loadSkillContributions(fsys, sourceRoot, source)
		if err != nil {
			return nil, nil, err
		}
		skills = append(skills, loaded...)
	}
	return sources, skills, nil
}

func loadSkillContributions(fsys fs.FS, dir string, source resource.SourceRef) ([]resource.SkillContribution, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []resource.SkillContribution
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		file := path.Join(dir, entry.Name(), "SKILL.md")
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, fmt.Errorf("read skill %q: %w", file, err)
		}
		meta, _, err := md.Parse(strings.NewReader(string(data)))
		if err != nil {
			return nil, fmt.Errorf("parse skill %q: %w", file, err)
		}
		fm, err := md.Bind[skill.SkillMetadata](meta)
		if err != nil {
			return nil, fmt.Errorf("parse skill metadata %q: %w", file, err)
		}
		if fm.Name == "" {
			fm.Name = entry.Name()
		}
		out = append(out, resource.SkillContribution{
			ID:          resource.QualifiedID(source, "skill", fm.Name, qualifiedResourcePath(source, path.Dir(file))),
			Name:        fm.Name,
			Description: strings.TrimSpace(fm.Description),
			Source:      source,
			Path:        path.Dir(file),
			Metadata:    fm,
		})
	}
	return out, nil
}

func clean(root string) string {
	root = strings.TrimPrefix(filepath.ToSlash(root), "/")
	if root == "" || root == "." {
		return "."
	}
	return path.Clean(root)
}

func dirExists(fsys fs.FS, dir string) (bool, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return len(entries) >= 0, nil
}

func sourceKind(dir string) skill.SourceKind {
	switch {
	case strings.Contains(dir, ".claude"):
		return skill.SourceClaudeProject
	case strings.Contains(dir, ".agents"):
		return skill.SourceAgentsCompat
	default:
		return skill.SourcePluginRoot
	}
}

func sourceForPath(root string, scope resource.Scope) resource.SourceRef {
	ecosystem := "agents"
	if strings.Contains(root, ".claude") {
		ecosystem = "claude"
	}
	if scope == "" {
		scope = resource.ScopeProject
	}
	source := resource.SourceRef{
		Ecosystem: ecosystem,
		Scope:     scope,
		Root:      root,
		Path:      root,
		Trust:     resource.TrustDeclarative,
	}
	source.ID = resource.QualifiedID(source, "source", "", root)
	return source
}

func qualifiedResourcePath(source resource.SourceRef, file string) string {
	file = clean(file)
	sourcePath := strings.TrimSpace(filepath.ToSlash(source.Path))
	if sourcePath == "" || sourcePath == "." {
		return file
	}
	if strings.Contains(sourcePath, "://") || strings.HasPrefix(sourcePath, "git+") {
		return strings.TrimRight(sourcePath, "/") + "//" + file
	}
	sourcePath = clean(sourcePath)
	if sourcePath == "." || strings.HasPrefix(file, sourcePath+"/") || file == sourcePath {
		return file
	}
	return path.Join(sourcePath, file)
}
