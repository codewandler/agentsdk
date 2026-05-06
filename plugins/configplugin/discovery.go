package configplugin

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/appconfig"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/skill"
)

const maxDiscoverDescriptionRunes = 180

type discoveryWriter interface {
	Write([]byte) (int, error)
}

// DiscoverResources loads resources through appconfig so discovery follows the
// same config/include expansion path as config inspection.
func DiscoverResources(dir string) (agentdir.Resolution, *appconfig.LoadResult, error) {
	cfgResult, err := appconfig.Load(dir)
	if err != nil {
		return agentdir.Resolution{}, nil, err
	}
	resolved, err := ResolutionFromAppConfig(cfgResult)
	if err != nil {
		return agentdir.Resolution{}, nil, err
	}
	return resolved, &cfgResult, nil
}

// ResolutionFromAppConfig converts a loaded appconfig result into an agentdir
// resolution suitable for app construction and resource diagnostics.
func ResolutionFromAppConfig(cfgResult appconfig.LoadResult) (agentdir.Resolution, error) {
	var resolved agentdir.Resolution
	for _, source := range cfgResult.Sources {
		resolved.Sources = append(resolved.Sources, source.Path)
	}
	if cfgResult.EntryPath != "" && len(resolved.Sources) == 0 {
		resolved.Sources = append(resolved.Sources, cfgResult.EntryPath)
	}
	// Set a source for the appconfig bundle so agent ResourceIDs derive
	// correctly. Ecosystem="config" with no scope makes DeriveOrigin return
	// "config". Root is the config name so DeriveNamespace returns it as the
	// namespace.
	if resolved.Bundle.Source.ID == "" {
		cfgNS := cfgResult.Config.Name
		if cfgNS == "" && cfgResult.EntryPath != "" {
			cfgNS = filepath.Base(filepath.Dir(cfgResult.EntryPath))
		}
		resolved.Bundle.Source = resource.SourceRef{
			ID:        "appconfig:" + cfgResult.EntryPath,
			Ecosystem: "config",
			Root:      cfgNS,
			Path:      cfgResult.EntryPath,
			Trust:     resource.TrustDeclarative,
		}
	}
	// Merge inline appconfig resources.
	resolved.Bundle.Append(cfgResult.ToContributionBundle())
	// Merge agentdir bundles loaded via include directives.
	for _, b := range cfgResult.Bundles {
		resolved.Bundle.Append(b)
		if resolved.Bundle.Source.ID == "" && b.Source.ID != "" {
			resolved.Bundle.Source = b.Source
		}
	}
	// Merge inline agent specs into the bundle.
	for _, spec := range cfgResult.ToAgentSpecs() {
		resolved.Bundle.AgentSpecs = append(resolved.Bundle.AgentSpecs, spec)
	}
	if cfgResult.Config.DefaultAgent != "" {
		resolved.DefaultAgent = cfgResult.Config.DefaultAgent
	}
	// Validate plugin refs.
	for _, p := range cfgResult.Config.Plugins {
		if strings.TrimSpace(p.Name) == "" {
			return agentdir.Resolution{}, fmt.Errorf("plugin name is required")
		}
	}
	// Populate manifest for backward compatibility with JSON output and plugin refs.
	manifest := &agentdir.AppManifest{DefaultAgent: cfgResult.Config.DefaultAgent}
	for _, p := range cfgResult.Config.Plugins {
		manifest.Plugins = append(manifest.Plugins, agentdir.PluginRef{
			Name:   p.Name,
			Config: p.Config,
		})
	}
	resolved.Manifest = manifest
	return resolved, nil
}

// PrintDiscovery renders a detailed human-readable discovery report.
func PrintDiscovery(out discoveryWriter, resolved agentdir.Resolution) error {
	imported, err := app.New(app.WithResourceBundle(resolved.Bundle))
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Sources:")
	if len(resolved.Sources) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, source := range resolved.Sources {
		fmt.Fprintf(out, "  %s\n", source)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Agents:")
	agentSpecs := imported.AgentSpecs()
	if len(agentSpecs) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, spec := range agentSpecs {
		id := spec.ResourceID
		if id == "" {
			id = spec.Name
		}
		fmt.Fprintf(out, "  %s  %s  %s\n", spec.Name, displayDescription(spec.Description), id)
	}
	printDiscoveryCapabilities(out, agentSpecs)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	commands := imported.Commands().All()
	if len(commands) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, cmd := range commands {
		desc := cmd.Descriptor()
		policy := discoveryCommandPolicyLabel(desc.Policy)
		if policy != "" {
			policy = "  policy=" + policy
		}
		fmt.Fprintf(out, "  /%s  %s%s\n", desc.Name, displayDescription(desc.Description), policy)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Skills:")
	skills := firstSkillContributions(resolved.Bundle.Skills)
	if len(skills) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, skill := range skills {
		fmt.Fprintf(out, "  %s  %s  %s\n", skill.Name, displayDescription(skill.Description), skill.ID)
	}
	printDiscoverySkillReferences(out, imported.SkillSources())
	printDiscoveryDataSources(out, resolved.Bundle.DataSources)
	printDiscoveryWorkflows(out, resolved.Bundle.Workflows)
	printDiscoveryActions(out, resolved.Bundle.Actions)
	printDiscoveryTriggers(out, resolved.Bundle.Triggers)
	printDiscoveryStructuredCommands(out, resolved.Bundle.CommandResources)
	printDiscoveryPlugins(out, resolved.ManifestPluginRefs())
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Skill sources:")
	skillSources := imported.SkillSources()
	if len(skillSources) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, source := range skillSources {
		fmt.Fprintf(out, "  %s  %s\n", source.ID, source.Label)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Disabled suggestions:")
	hasDisabled := false
	for _, tool := range resolved.Bundle.Tools {
		if tool.Enabled {
			continue
		}
		hasDisabled = true
		fmt.Fprintf(out, "  tool %s  %s\n", tool.ID, tool.Description)
	}
	if !hasDisabled {
		fmt.Fprintln(out, "  none")
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Diagnostics:")
	diagnostics := imported.Diagnostics()
	if len(diagnostics) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, diag := range diagnostics {
		fmt.Fprintf(out, "  %s  %s  %s\n", diag.Severity, diag.Source.Label(), diag.Message)
	}
	return nil
}

// PrintDiscoveryTree renders discovery as a ResourceID-oriented tree with
// resolution diagnostics.
func PrintDiscoveryTree(out io.Writer, resolved agentdir.Resolution) error {
	imported, err := app.New(app.WithResourceBundle(resolved.Bundle))
	if err != nil {
		return err
	}

	// Sources.
	fmt.Fprintln(out, "Sources:")
	if len(resolved.Sources) == 0 {
		fmt.Fprintln(out, "  (none)")
	}
	for _, source := range resolved.Sources {
		fmt.Fprintf(out, "  %s\n", source)
	}

	idx := imported.ResourceIndex()
	if idx == nil || idx.Len() == 0 {
		fmt.Fprintln(out, "\n(no resources)")
		return nil
	}
	all := idx.All()

	// Group by origin:namespace → kind → name.
	type originKey struct {
		Origin    string
		Namespace string
	}
	type kindEntry struct {
		names []string
	}
	origins := map[originKey]map[string]*kindEntry{}
	var originOrder []originKey
	for _, rid := range all {
		key := originKey{Origin: rid.Origin, Namespace: rid.Namespace.String()}
		if _, ok := origins[key]; !ok {
			origins[key] = map[string]*kindEntry{}
			originOrder = append(originOrder, key)
		}
		kinds := origins[key]
		if kinds[rid.Kind] == nil {
			kinds[rid.Kind] = &kindEntry{}
		}
		kinds[rid.Kind].names = append(kinds[rid.Kind].names, rid.Name)
	}

	// Sort origins and render.
	sort.Slice(originOrder, func(i, j int) bool {
		if originOrder[i].Origin != originOrder[j].Origin {
			return originOrder[i].Origin < originOrder[j].Origin
		}
		return originOrder[i].Namespace < originOrder[j].Namespace
	})

	kindOrder := []string{"agent", "command", "workflow", "action", "skill", "datasource", "trigger", "tool", "hook"}

	// Build a resolver to show resolution results.
	resolver := resource.NewResolver(resource.ResolverConfig{Index: idx})

	for i, key := range originOrder {
		if i > 0 {
			fmt.Fprintln(out)
		}
		label := key.Origin
		if key.Namespace != "" {
			label += ":" + key.Namespace
		}
		fmt.Fprintf(out, "%s\n", label)
		kinds := origins[key]
		for _, kind := range kindOrder {
			entry, ok := kinds[kind]
			if !ok {
				continue
			}
			sort.Strings(entry.names)
			fmt.Fprintf(out, "├── %ss\n", kind)
			for j, name := range entry.names {
				connector := "│   ├── "
				if j == len(entry.names)-1 {
					connector = "│   └── "
				}
				// Check for shadows.
				candidates := idx.Lookup(kind, name)
				shadow := ""
				if len(candidates) > 1 {
					resolved, resolveErr := resolver.Resolve(kind, name)
					if resolveErr == nil && resolved.Origin != key.Origin {
						shadow = fmt.Sprintf(" ⚠ shadowed by %s", resolved.Address())
					} else if resolveErr == nil && resolved.Origin == key.Origin {
						// This origin wins.
						for _, c := range candidates {
							if c.Origin != key.Origin {
								shadow = fmt.Sprintf(" ⚠ shadows %s", c.Address())
								break
							}
						}
					}
				}
				fmt.Fprintf(out, "%s%s%s\n", connector, name, shadow)
			}
		}
	}

	// Resolution summary.
	fmt.Fprintf(out, "\nResolution:\n")
	seen := map[string]bool{}
	for _, rid := range all {
		if seen[rid.Kind+":"+rid.Name] {
			continue
		}
		seen[rid.Kind+":"+rid.Name] = true
		resolved, resolveErr := resolver.Resolve(rid.Kind, rid.Name)
		if resolveErr != nil {
			fmt.Fprintf(out, "  %-20s ⚠ %s\n", rid.Name, resolveErr)
		} else {
			fmt.Fprintf(out, "  %-20s → %s\n", rid.Name, resolved.Address())
		}
	}

	// Diagnostics.
	diagnostics := imported.Diagnostics()
	if len(diagnostics) > 0 {
		fmt.Fprintf(out, "\nDiagnostics:\n")
		for _, diag := range diagnostics {
			fmt.Fprintf(out, "  %s  %s\n", diag.Severity, diag.Message)
		}
	}
	return nil
}

type DiscoveryOutput struct {
	Sources             []string                          `json:"sources"`
	Agents              []DiscoveryAgent                  `json:"agents"`
	Commands            []command.Descriptor              `json:"commands"`
	Skills              []resource.SkillContribution      `json:"skills"`
	SkillReferences     []DiscoverySkillReference         `json:"skillReferences"`
	DataSources         []resource.DataSourceContribution `json:"datasources"`
	WorkflowDescriptors []resource.WorkflowContribution   `json:"workflows"`
	ActionDescriptors   []resource.ActionContribution     `json:"actions"`
	Triggers            []resource.TriggerContribution    `json:"triggers"`
	StructuredCommands  []resource.CommandContribution    `json:"structuredCommands"`
	Plugins             []agentdir.PluginRef              `json:"plugins"`
	Capabilities        []DiscoveryCapability             `json:"capabilities"`
	Diagnostics         []resource.Diagnostic             `json:"diagnostics"`
}

type DiscoveryAgent struct {
	Name         string                `json:"name"`
	Description  string                `json:"description,omitempty"`
	ResourceID   string                `json:"resourceId,omitempty"`
	ResourceFrom string                `json:"resourceFrom,omitempty"`
	Capabilities []DiscoveryCapability `json:"capabilities,omitempty"`
}

type DiscoveryCapability struct {
	Name       string          `json:"name"`
	InstanceID string          `json:"instanceId,omitempty"`
	Config     json.RawMessage `json:"config,omitempty"`
	Agent      string          `json:"agent,omitempty"`
}

type DiscoverySkillReference struct {
	Skill    string   `json:"skill"`
	Path     string   `json:"path"`
	Triggers []string `json:"triggers,omitempty"`
}

// PrintDiscoveryJSON renders machine-readable discovery JSON.
func PrintDiscoveryJSON(out discoveryWriter, resolved agentdir.Resolution) error {
	imported, err := app.New(app.WithResourceBundle(resolved.Bundle))
	if err != nil {
		return err
	}
	payload := BuildDiscoveryOutput(resolved, imported)
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(payload)
}

// PrintDiscoveryYAML renders machine-readable discovery YAML.
func PrintDiscoveryYAML(out discoveryWriter, resolved agentdir.Resolution) error {
	imported, err := app.New(app.WithResourceBundle(resolved.Bundle))
	if err != nil {
		return err
	}
	payload := BuildDiscoveryOutput(resolved, imported)
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var shaped any
	if err := yaml.Unmarshal(raw, &shaped); err != nil {
		return err
	}
	enc := yaml.NewEncoder(out)
	enc.SetIndent(2)
	return enc.Encode(shaped)
}

// BuildDiscoveryOutput constructs the shared discovery payload used by JSON/YAML renderers.
func BuildDiscoveryOutput(resolved agentdir.Resolution, imported *app.App) DiscoveryOutput {
	out := DiscoveryOutput{
		Sources:             append([]string(nil), resolved.Sources...),
		Skills:              firstSkillContributions(resolved.Bundle.Skills),
		DataSources:         append([]resource.DataSourceContribution(nil), resolved.Bundle.DataSources...),
		WorkflowDescriptors: append([]resource.WorkflowContribution(nil), resolved.Bundle.Workflows...),
		ActionDescriptors:   append([]resource.ActionContribution(nil), resolved.Bundle.Actions...),
		Triggers:            append([]resource.TriggerContribution(nil), resolved.Bundle.Triggers...),
		StructuredCommands:  append([]resource.CommandContribution(nil), resolved.Bundle.CommandResources...),
		Plugins:             append([]agentdir.PluginRef(nil), resolved.ManifestPluginRefs()...),
	}
	if imported != nil {
		for _, spec := range imported.AgentSpecs() {
			agentOut := DiscoveryAgent{Name: spec.Name, Description: spec.Description, ResourceID: spec.ResourceID, ResourceFrom: spec.ResourceFrom}
			for _, capSpec := range spec.Capabilities {
				capOut := DiscoveryCapability{Name: capSpec.CapabilityName, InstanceID: capSpec.InstanceID, Config: capSpec.Config, Agent: spec.Name}
				agentOut.Capabilities = append(agentOut.Capabilities, capOut)
				out.Capabilities = append(out.Capabilities, capOut)
			}
			out.Agents = append(out.Agents, agentOut)
		}
		out.Commands = imported.Commands().Descriptors()
		out.Diagnostics = append([]resource.Diagnostic(nil), imported.Diagnostics()...)
		out.SkillReferences = discoverySkillReferences(imported.SkillSources())
	}
	return out
}

func discoverySkillReferences(sources []skill.Source) []DiscoverySkillReference {
	repo, err := skill.NewRepository(sources, nil)
	if err != nil {
		return nil
	}
	var out []DiscoverySkillReference
	for _, item := range repo.List() {
		for _, ref := range repo.ListReferences(item.Name) {
			out = append(out, DiscoverySkillReference{Skill: item.Name, Path: ref.Path, Triggers: ref.Metadata.AllTriggers()})
		}
	}
	return out
}

func discoveryCommandPolicyLabel(policy command.Policy) string {
	parts := []string{}
	if policy.UserCallable {
		parts = append(parts, "user")
	}
	if policy.AgentCallable {
		parts = append(parts, "agent")
	}
	if policy.Internal {
		parts = append(parts, "internal")
	}
	if policy.SafetyClass != "" {
		parts = append(parts, "safety:"+policy.SafetyClass)
	}
	if policy.RequiresApproval {
		parts = append(parts, "approval")
	}
	return strings.Join(parts, ",")
}

func printDiscoveryCapabilities(out discoveryWriter, specs []agentconfig.Spec) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Capabilities:")
	hasCapabilities := false
	for _, spec := range specs {
		for _, capSpec := range spec.Capabilities {
			hasCapabilities = true
			instanceID := capSpec.InstanceID
			if instanceID == "" {
				instanceID = capSpec.CapabilityName
			}
			fmt.Fprintf(out, "  %s  agent=%s  instance=%s\n", capSpec.CapabilityName, spec.Name, instanceID)
		}
	}
	if !hasCapabilities {
		fmt.Fprintln(out, "  none")
	}
}

func printDiscoverySkillReferences(out discoveryWriter, sources []skill.Source) {
	if len(sources) == 0 {
		return
	}
	repo, err := skill.NewRepository(sources, nil)
	if err != nil {
		fmt.Fprintf(out, "  references: unavailable (%v)\n", err)
		return
	}
	wroteHeader := false
	for _, item := range repo.List() {
		refs := repo.ListReferences(item.Name)
		if len(refs) == 0 {
			continue
		}
		if !wroteHeader {
			fmt.Fprintln(out, "  References:")
			wroteHeader = true
		}
		for _, ref := range refs {
			triggers := strings.Join(ref.Metadata.AllTriggers(), ",")
			if triggers != "" {
				triggers = "  triggers=" + triggers
			}
			fmt.Fprintf(out, "    %s/%s%s\n", item.Name, ref.Path, triggers)
		}
	}
}

func printDiscoveryDataSources(out discoveryWriter, datasources []resource.DataSourceContribution) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Datasources:")
	if len(datasources) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, datasource := range datasources {
		kind := datasource.Kind
		if kind == "" {
			kind = "unknown"
		}
		fmt.Fprintf(out, "  %s  %s  kind=%s  %s\n", datasource.Name, displayDescription(datasource.Description), kind, datasource.ID)
	}
}

func printDiscoveryWorkflows(out discoveryWriter, workflows []resource.WorkflowContribution) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Workflows:")
	if len(workflows) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, workflow := range workflows {
		fmt.Fprintf(out, "  %s  %s  %s\n", workflow.Name, displayDescription(workflow.Description), workflow.ID)
	}
}

func printDiscoveryActions(out discoveryWriter, actions []resource.ActionContribution) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Actions:")
	if len(actions) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, action := range actions {
		kind := action.Kind
		if kind == "" {
			kind = "declarative"
		}
		fmt.Fprintf(out, "  %s  %s  kind=%s  %s\n", action.Name, displayDescription(action.Description), kind, action.ID)
	}
}

func printDiscoveryTriggers(out discoveryWriter, triggers []resource.TriggerContribution) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Triggers:")
	if len(triggers) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, item := range triggers {
		fmt.Fprintf(out, "  %s  %s  %s\n", item.Name, displayDescription(item.Description), item.ID)
	}
}

func printDiscoveryStructuredCommands(out discoveryWriter, commands []resource.CommandContribution) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Structured commands:")
	if len(commands) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, item := range commands {
		target := string(item.Target.Kind)
		targetName := item.Target.Workflow
		if targetName == "" {
			targetName = item.Target.Action
		}
		if targetName == "" {
			targetName = "prompt"
		}
		fmt.Fprintf(out, "  /%s  %s  target=%s:%s  %s\n", strings.Join(item.CommandPath, " "), displayDescription(item.Description), target, targetName, item.ID)
	}
}

func printDiscoveryPlugins(out discoveryWriter, plugins []agentdir.PluginRef) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Plugins:")
	if len(plugins) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, plugin := range plugins {
		config := ""
		if len(plugin.Config) > 0 {
			config = "  config=true"
		}
		fmt.Fprintf(out, "  %s%s\n", plugin.Name, config)
	}
}

func firstSkillContributions(skills []resource.SkillContribution) []resource.SkillContribution {
	seen := map[string]bool{}
	out := make([]resource.SkillContribution, 0, len(skills))
	for _, skill := range skills {
		if skill.Name == "" || seen[skill.Name] {
			continue
		}
		seen[skill.Name] = true
		out = append(out, skill)
	}
	return out
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, `\n`, " ")
	return strings.Join(strings.Fields(s), " ")
}

func displayDescription(s string) string {
	s = oneLine(s)
	if utf8.RuneCountInString(s) <= maxDiscoverDescriptionRunes {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:maxDiscoverDescriptionRunes-1])) + "..."
}
