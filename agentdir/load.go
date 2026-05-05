// Package agentdir loads filesystem-described agent resources.
package agentdir

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/command"
	cmdmarkdown "github.com/codewandler/agentsdk/command/markdown"
	md "github.com/codewandler/agentsdk/markdown"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/llmadapter/unified"
	"gopkg.in/yaml.v3"
)

type AgentFrontmatter struct {
	Name           string                     `yaml:"name"`
	Description    string                     `yaml:"description"`
	Model          string                     `yaml:"model"`
	MaxTokens      int                        `yaml:"max-tokens"`
	MaxSteps       int                        `yaml:"max-steps"`
	Temperature    float64                    `yaml:"temperature"`
	Thinking       string                     `yaml:"thinking"`
	Effort         string                     `yaml:"effort"`
	Tools          stringList                 `yaml:"tools"`
	Skills         stringList                 `yaml:"skills"`
	Commands       stringList                 `yaml:"commands"`
	SkillSources   stringList                 `yaml:"skill-sources"`
	Capabilities   capabilityList             `yaml:"capabilities"`
	AutoCompaction *AutoCompactionFrontmatter `yaml:"auto-compaction"`
}

// AutoCompactionFrontmatter configures automatic conversation compaction for
// filesystem-described agents. Omitted fields use agent defaults.
type AutoCompactionFrontmatter struct {
	Enabled            *bool   `yaml:"enabled"`
	TokenThreshold     int     `yaml:"token-threshold"` // deprecated: use context-window-ratio
	ContextWindowRatio float64 `yaml:"context-window-ratio"`
	KeepWindow         int     `yaml:"keep-window"`
}

func (fm *AutoCompactionFrontmatter) config() agentconfig.AutoCompactionConfig {
	if fm == nil {
		return agentconfig.AutoCompactionConfig{}
	}
	enabled := true
	if fm.Enabled != nil {
		enabled = *fm.Enabled
	}
	return agentconfig.AutoCompactionConfig{
		Enabled:            enabled,
		ContextWindowRatio: fm.ContextWindowRatio,
		KeepWindow:         fm.KeepWindow,
	}
}

func (fm *AutoCompactionFrontmatter) validate() error {
	if fm == nil {
		return nil
	}
	if fm.TokenThreshold > 0 {
		return fmt.Errorf("auto-compaction.token-threshold is deprecated; use context-window-ratio")
	}
	if fm.TokenThreshold < 0 {
		return fmt.Errorf("auto-compaction.token-threshold must be >= 0")
	}
	if fm.ContextWindowRatio < 0 || fm.ContextWindowRatio > 1 {
		return fmt.Errorf("auto-compaction.context-window-ratio must be between 0 and 1")
	}
	if fm.KeepWindow < 0 {
		return fmt.Errorf("auto-compaction.keep-window must be >= 0")
	}
	return nil
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
		structured, err := loadCommandContributions(fsys, dir, source)
		if err != nil {
			return resource.ContributionBundle{}, err
		}
		out.CommandResources = append(out.CommandResources, structured.Commands...)
		out.Workflows = append(out.Workflows, structured.Workflows...)
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
	for _, dir := range []string{
		path.Join(root, ".agents", "datasources"),
		path.Join(root, "datasources"),
	} {
		datasources, err := loadDataSourceContributions(fsys, dir, source)
		if err != nil {
			return resource.ContributionBundle{}, err
		}
		out.DataSources = append(out.DataSources, datasources...)
	}
	for _, dir := range []string{
		path.Join(root, ".agents", "workflows"),
		path.Join(root, "workflows"),
	} {
		workflows, err := loadWorkflowContributions(fsys, dir, source)
		if err != nil {
			return resource.ContributionBundle{}, err
		}
		out.Workflows = append(out.Workflows, workflows.Workflows...)
		out.CommandResources = append(out.CommandResources, workflows.Commands...)
		out.Triggers = append(out.Triggers, workflows.Triggers...)
	}
	for _, dir := range []string{
		path.Join(root, ".agents", "actions"),
		path.Join(root, "actions"),
	} {
		actions, err := loadActionContributions(fsys, dir, source)
		if err != nil {
			return resource.ContributionBundle{}, err
		}
		out.Actions = append(out.Actions, actions...)
	}
	for _, dir := range []string{
		path.Join(root, ".agents", "triggers"),
		path.Join(root, "triggers"),
	} {
		triggers, err := loadTriggerContributions(fsys, dir, source)
		if err != nil {
			return resource.ContributionBundle{}, err
		}
		out.Triggers = append(out.Triggers, triggers.Triggers...)
		out.Workflows = append(out.Workflows, triggers.Workflows...)
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
	stampResourceIDs(&out, source)
	return out, nil
}

// stampResourceIDs populates the RID field on all contributions in the bundle
// by deriving a ResourceID from the bundle's SourceRef and each resource's name/kind.
func stampResourceIDs(bundle *resource.ContributionBundle, source resource.SourceRef) {
	for i := range bundle.CommandResources {
		c := &bundle.CommandResources[i]
		if c.RID.IsZero() {
			c.RID = resource.DeriveResourceID(source, "command", c.Name)
		}
	}
	for i := range bundle.Skills {
		c := &bundle.Skills[i]
		if c.RID.IsZero() {
			c.RID = resource.DeriveResourceID(source, "skill", c.Name)
		}
	}
	for i := range bundle.DataSources {
		c := &bundle.DataSources[i]
		if c.RID.IsZero() {
			c.RID = resource.DeriveResourceID(source, "datasource", c.Name)
		}
	}
	for i := range bundle.Workflows {
		c := &bundle.Workflows[i]
		if c.RID.IsZero() {
			c.RID = resource.DeriveResourceID(source, "workflow", c.Name)
		}
	}
	for i := range bundle.Actions {
		c := &bundle.Actions[i]
		if c.RID.IsZero() {
			c.RID = resource.DeriveResourceID(source, "action", c.Name)
		}
	}
	for i := range bundle.Triggers {
		c := &bundle.Triggers[i]
		if c.RID.IsZero() {
			c.RID = resource.DeriveResourceID(source, "trigger", c.Name)
		}
	}
	for i := range bundle.Tools {
		c := &bundle.Tools[i]
		if c.RID.IsZero() {
			c.RID = resource.DeriveResourceID(source, "tool", c.Name)
		}
	}
	for i := range bundle.Hooks {
		c := &bundle.Hooks[i]
		if c.RID.IsZero() {
			c.RID = resource.DeriveResourceID(source, "hook", c.Name)
		}
	}
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
	Specs  []agentconfig.Spec
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

func ParseAgentSpec(name string, content []byte) (agentconfig.Spec, error) {
	spec, _, err := parseAgentSpec(name, content)
	return spec, err
}

func parseAgentSpec(name string, content []byte) (agentconfig.Spec, AgentFrontmatter, error) {
	meta, body, err := md.Parse(strings.NewReader(string(content)))
	if err != nil {
		return agentconfig.Spec{}, AgentFrontmatter{}, err
	}
	fm, err := md.Bind[AgentFrontmatter](meta)
	if err != nil {
		return agentconfig.Spec{}, AgentFrontmatter{}, err
	}
	if err := fm.AutoCompaction.validate(); err != nil {
		return agentconfig.Spec{}, AgentFrontmatter{}, err
	}
	if fm.Name == "" {
		fm.Name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	}
	inference := agentconfig.DefaultInferenceOptions()
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
		inference.Thinking = agentconfig.ThinkingMode(fm.Thinking)
	}
	if fm.Effort != "" {
		inference.Effort = unified.ReasoningEffort(fm.Effort)
	}
	spec := agentconfig.Spec{
		Name:           fm.Name,
		Description:    fm.Description,
		System:         body,
		Inference:      inference,
		MaxSteps:       fm.MaxSteps,
		Tools:          append([]string(nil), []string(fm.Tools)...),
		Skills:         append([]string(nil), []string(fm.Skills)...),
		Commands:       append([]string(nil), []string(fm.Commands)...),
		Capabilities:   append([]capability.AttachSpec(nil), []capability.AttachSpec(fm.Capabilities)...),
		HasFrontmatter: meta != nil,
	}
	if fm.AutoCompaction != nil {
		spec.AutoCompaction = fm.AutoCompaction.config()
		spec.AutoCompactionSet = true
	}
	return spec, fm, nil
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

type dataSourceResourceFile struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Kind        string         `yaml:"kind"`
	Type        string         `yaml:"type"`
	Config      map[string]any `yaml:"config"`
	Metadata    map[string]any `yaml:"metadata"`
}

type workflowResourceFile struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Metadata    map[string]any `yaml:"metadata"`
}

type actionResourceFile struct {
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Kind        string         `yaml:"kind"`
	Type        string         `yaml:"type"`
	Config      map[string]any `yaml:"config"`
	Metadata    map[string]any `yaml:"metadata"`
}

type triggerResourceFile struct {
	ID          string         `yaml:"id"`
	Name        string         `yaml:"name"`
	Description string         `yaml:"description"`
	Metadata    map[string]any `yaml:"metadata"`
}

type commandResourceFile struct {
	Name        string                   `yaml:"name"`
	Description string                   `yaml:"description"`
	Path        []string                 `yaml:"path"`
	InputSchema command.JSONSchema       `yaml:"input_schema"`
	Output      command.OutputDescriptor `yaml:"output"`
	Policy      commandPolicyResource    `yaml:"policy"`
	Target      map[string]any           `yaml:"target"`
	Metadata    map[string]any           `yaml:"metadata"`
}

type commandPolicyResource struct {
	UserCallable  *bool `yaml:"user_callable"`
	AgentCallable *bool `yaml:"agent_callable"`
	Internal      *bool `yaml:"internal"`
}

func (p commandPolicyResource) commandPolicy() command.Policy {
	out := command.UserPolicy()
	if p.UserCallable != nil {
		out.UserCallable = *p.UserCallable
	}
	if p.AgentCallable != nil {
		out.AgentCallable = *p.AgentCallable
	}
	if p.Internal != nil {
		out.Internal = *p.Internal
	}
	return out
}

type loadedCommandResources struct {
	Commands  []resource.CommandContribution
	Workflows []resource.WorkflowContribution
}

func loadDataSourceContributions(fsys fs.FS, dir string, source resource.SourceRef) ([]resource.DataSourceContribution, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []resource.DataSourceContribution
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		file := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("read datasource %q: %w", file, err)
		}
		var spec dataSourceResourceFile
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("parse datasource %q: %w", file, err)
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		}
		kind := strings.TrimSpace(spec.Kind)
		if kind == "" {
			kind = strings.TrimSpace(spec.Type)
		}
		out = append(out, resource.DataSourceContribution{
			ID:          resource.QualifiedID(source, "datasource", name, qualifiedResourcePath(source, file)),
			Name:        name,
			Description: strings.TrimSpace(spec.Description),
			Kind:        kind,
			Source:      source,
			Path:        file,
			Config:      spec.Config,
			Metadata:    spec.Metadata,
		})
	}
	return out, nil
}

type loadedWorkflowResources struct {
	Workflows []resource.WorkflowContribution
	Commands  []resource.CommandContribution
	Triggers  []resource.TriggerContribution
}

func loadWorkflowContributions(fsys fs.FS, dir string, source resource.SourceRef) (loadedWorkflowResources, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return loadedWorkflowResources{}, nil
		}
		return loadedWorkflowResources{}, err
	}
	var out loadedWorkflowResources
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		file := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return loadedWorkflowResources{}, fmt.Errorf("read workflow %q: %w", file, err)
		}
		var spec workflowResourceFile
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return loadedWorkflowResources{}, fmt.Errorf("parse workflow %q: %w", file, err)
		}
		var definition map[string]any
		if err := yaml.Unmarshal(data, &definition); err != nil {
			return loadedWorkflowResources{}, fmt.Errorf("parse workflow definition %q: %w", file, err)
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		}
		workflowContribution := resource.WorkflowContribution{
			ID:          resource.QualifiedID(source, "workflow", name, qualifiedResourcePath(source, file)),
			Name:        name,
			Description: strings.TrimSpace(spec.Description),
			Source:      source,
			Path:        file,
			Metadata:    spec.Metadata,
			Definition:  definition,
		}
		out.Workflows = append(out.Workflows, workflowContribution)
		commands, triggers := workflowExposures(workflowContribution)
		out.Commands = append(out.Commands, commands...)
		out.Triggers = append(out.Triggers, triggers...)
	}
	return out, nil
}

func isYAMLFile(name string) bool {
	ext := strings.ToLower(path.Ext(name))
	return ext == ".yaml" || ext == ".yml"
}

func loadActionContributions(fsys fs.FS, dir string, source resource.SourceRef) ([]resource.ActionContribution, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []resource.ActionContribution
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		file := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return nil, fmt.Errorf("read action %q: %w", file, err)
		}
		var spec actionResourceFile
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return nil, fmt.Errorf("parse action %q: %w", file, err)
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		}
		kind := strings.TrimSpace(spec.Kind)
		if kind == "" {
			kind = strings.TrimSpace(spec.Type)
		}
		out = append(out, resource.ActionContribution{
			ID:          resource.QualifiedID(source, "action", name, qualifiedResourcePath(source, file)),
			Name:        name,
			Description: strings.TrimSpace(spec.Description),
			Kind:        kind,
			Source:      source,
			Path:        file,
			Config:      spec.Config,
			Metadata:    spec.Metadata,
		})
	}
	return out, nil
}

type loadedTriggerResources struct {
	Triggers  []resource.TriggerContribution
	Workflows []resource.WorkflowContribution
}

func loadTriggerContributions(fsys fs.FS, dir string, source resource.SourceRef) (loadedTriggerResources, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return loadedTriggerResources{}, nil
		}
		return loadedTriggerResources{}, err
	}
	var out loadedTriggerResources
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		file := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return loadedTriggerResources{}, fmt.Errorf("read trigger %q: %w", file, err)
		}
		var spec triggerResourceFile
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return loadedTriggerResources{}, fmt.Errorf("parse trigger %q: %w", file, err)
		}
		var definition map[string]any
		if err := yaml.Unmarshal(data, &definition); err != nil {
			return loadedTriggerResources{}, fmt.Errorf("parse trigger definition %q: %w", file, err)
		}
		name := strings.TrimSpace(spec.ID)
		if name == "" {
			name = strings.TrimSpace(spec.Name)
		}
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		}
		if inline := normalizeInlineWorkflowTarget(definition, name, file, source); inline != nil {
			out.Workflows = append(out.Workflows, *inline)
		}
		out.Triggers = append(out.Triggers, resource.TriggerContribution{
			ID:          resource.QualifiedID(source, "trigger", name, qualifiedResourcePath(source, file)),
			Name:        name,
			Description: strings.TrimSpace(spec.Description),
			Source:      source,
			Path:        file,
			Definition:  definition,
			Metadata:    spec.Metadata,
		})
	}
	return out, nil
}

func loadCommandContributions(fsys fs.FS, dir string, source resource.SourceRef) (loadedCommandResources, error) {
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		if os.IsNotExist(err) {
			return loadedCommandResources{}, nil
		}
		return loadedCommandResources{}, err
	}
	var out loadedCommandResources
	for _, entry := range entries {
		if entry.IsDir() || !isYAMLFile(entry.Name()) {
			continue
		}
		file := path.Join(dir, entry.Name())
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return loadedCommandResources{}, fmt.Errorf("read command %q: %w", file, err)
		}
		var spec commandResourceFile
		if err := yaml.Unmarshal(data, &spec); err != nil {
			return loadedCommandResources{}, fmt.Errorf("parse command %q: %w", file, err)
		}
		name := strings.TrimSpace(spec.Name)
		if name == "" {
			name = strings.TrimSuffix(entry.Name(), path.Ext(entry.Name()))
		}
		commandPath := cleanStringList(spec.Path)
		if len(commandPath) == 0 {
			commandPath = []string{name}
		}
		target, inlineWorkflow, err := commandTargetFromMap(spec.Target, name, file, source)
		if err != nil {
			return loadedCommandResources{}, fmt.Errorf("parse command target %q: %w", file, err)
		}
		out.Commands = append(out.Commands, resource.CommandContribution{
			ID:          resource.QualifiedID(source, "command", name, qualifiedResourcePath(source, file)),
			Name:        name,
			Description: strings.TrimSpace(spec.Description),
			Source:      source,
			Path:        file,
			CommandPath: commandPath,
			InputSchema: spec.InputSchema,
			Output:      spec.Output,
			Policy:      spec.Policy.commandPolicy(),
			Target:      target,
			Metadata:    spec.Metadata,
		})
		if inlineWorkflow != nil {
			out.Workflows = append(out.Workflows, *inlineWorkflow)
		}
	}
	return out, nil
}

func commandTargetFromMap(targetDef map[string]any, ownerName string, file string, source resource.SourceRef) (resource.CommandTarget, *resource.WorkflowContribution, error) {
	if len(targetDef) == 0 {
		return resource.CommandTarget{}, nil, fmt.Errorf("target is required")
	}
	if raw, ok := targetDef["workflow"]; ok {
		target := resource.CommandTarget{Kind: resource.CommandTargetWorkflow, Input: targetDef["input"], IncludeEvent: boolFromAny(targetDef["include_event"])}
		switch v := raw.(type) {
		case string:
			target.Workflow = strings.TrimSpace(v)
		case map[string]any:
			workflowName := stringFromAny(v["name"])
			if workflowName == "" {
				workflowName = ownerName + "_workflow"
			}
			v["name"] = workflowName
			target.Workflow = workflowName
			target.WorkflowDefinition = cloneMap(v)
			inline := workflowContributionFromDefinition(workflowName, file, source, v)
			return target, &inline, nil
		default:
			return resource.CommandTarget{}, nil, fmt.Errorf("target.workflow must be a string or object")
		}
		if target.Workflow == "" {
			return resource.CommandTarget{}, nil, fmt.Errorf("target.workflow is required")
		}
		return target, nil, nil
	}
	if actionName := stringFromAny(targetDef["action"]); actionName != "" {
		return resource.CommandTarget{Kind: resource.CommandTargetAction, Action: actionName, Input: targetDef["input"]}, nil, nil
	}
	if prompt := stringFromAny(targetDef["prompt"]); prompt != "" {
		return resource.CommandTarget{Kind: resource.CommandTargetPrompt, Prompt: prompt, Input: targetDef["input"]}, nil, nil
	}
	return resource.CommandTarget{}, nil, fmt.Errorf("target.workflow, target.action, or target.prompt is required")
}

func workflowContributionFromDefinition(name string, file string, source resource.SourceRef, definition map[string]any) resource.WorkflowContribution {
	return resource.WorkflowContribution{
		ID:          resource.QualifiedID(source, "workflow", name, qualifiedResourcePath(source, file)+"@inline-workflow"),
		Name:        name,
		Description: stringFromAny(definition["description"]),
		Source:      source,
		Path:        file,
		Metadata:    mapFromAny(definition["metadata"]),
		Definition:  cloneMap(definition),
	}
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func normalizeInlineWorkflowTarget(definition map[string]any, ownerName string, file string, source resource.SourceRef) *resource.WorkflowContribution {
	targetDef := mapFromAny(definition["target"])
	raw, ok := targetDef["workflow"]
	if !ok {
		return nil
	}
	workflowDef, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	workflowName := stringFromAny(workflowDef["name"])
	if workflowName == "" {
		workflowName = ownerName + "_workflow"
	}
	workflowDef["name"] = workflowName
	targetDef["workflow"] = workflowName
	definition["target"] = targetDef
	inline := workflowContributionFromDefinition(workflowName, file, source, workflowDef)
	return &inline
}

func workflowExposures(workflow resource.WorkflowContribution) ([]resource.CommandContribution, []resource.TriggerContribution) {
	expose := mapFromAny(workflow.Definition["expose"])
	if len(expose) == 0 {
		return nil, nil
	}
	var commands []resource.CommandContribution
	for _, raw := range sliceFromAny(expose["commands"]) {
		m := mapFromAny(raw)
		if len(m) == 0 {
			continue
		}
		name := stringFromAny(m["name"])
		if name == "" {
			name = workflow.Name
		}
		commandPath := stringSliceFromAny(m["path"])
		if len(commandPath) == 0 {
			commandPath = []string{name}
		}
		commands = append(commands, resource.CommandContribution{
			ID:          resource.QualifiedID(workflow.Source, "command", name, qualifiedResourcePath(workflow.Source, workflow.Path)+"@expose.commands."+name),
			Name:        name,
			Description: firstNonEmpty(stringFromAny(m["description"]), workflow.Description),
			Source:      workflow.Source,
			Path:        workflow.Path,
			CommandPath: commandPath,
			Policy:      commandPolicyFromMap(mapFromAny(m["policy"])),
			Target: resource.CommandTarget{
				Kind:     resource.CommandTargetWorkflow,
				Workflow: workflow.Name,
				Input:    m["input"],
			},
			Metadata: mapFromAny(m["metadata"]),
		})
	}
	var triggers []resource.TriggerContribution
	for _, raw := range sliceFromAny(expose["triggers"]) {
		m := cloneMap(mapFromAny(raw))
		if len(m) == 0 {
			continue
		}
		id := stringFromAny(m["id"])
		if id == "" {
			id = workflow.Name + "-trigger"
		}
		target := mapFromAny(m["target"])
		if len(target) == 0 {
			target = map[string]any{}
		}
		target["workflow"] = workflow.Name
		if _, ok := target["input"]; !ok {
			target["input"] = m["input"]
		}
		m["target"] = target
		triggers = append(triggers, resource.TriggerContribution{
			ID:          resource.QualifiedID(workflow.Source, "trigger", id, qualifiedResourcePath(workflow.Source, workflow.Path)+"@expose.triggers."+id),
			Name:        id,
			Description: stringFromAny(m["description"]),
			Source:      workflow.Source,
			Path:        workflow.Path,
			Definition:  m,
			Metadata:    mapFromAny(m["metadata"]),
		})
	}
	return commands, triggers
}

func commandPolicyFromMap(m map[string]any) command.Policy {
	return commandPolicyResource{
		UserCallable:  boolPtrFromMap(m, "user_callable"),
		AgentCallable: boolPtrFromMap(m, "agent_callable"),
		Internal:      boolPtrFromMap(m, "internal"),
	}.commandPolicy()
}

func boolPtrFromMap(m map[string]any, key string) *bool {
	value, ok := m[key].(bool)
	if !ok {
		return nil
	}
	return &value
}

func mapFromAny(raw any) map[string]any {
	m, _ := raw.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func sliceFromAny(raw any) []any {
	items, _ := raw.([]any)
	return items
}

func stringFromAny(raw any) string {
	s, _ := raw.(string)
	return strings.TrimSpace(s)
}

func boolFromAny(raw any) bool {
	b, _ := raw.(bool)
	return b
}

func stringSliceFromAny(raw any) []string {
	switch v := raw.(type) {
	case []string:
		return cleanStringList(v)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringFromAny(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	default:
		return nil
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
