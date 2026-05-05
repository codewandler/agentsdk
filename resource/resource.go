// Package resource defines normalized agent application resources discovered
// from local directories, embedded filesystems, git repositories, and external
// ecosystem adapters.
package resource

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/skill"
)

type TrustLevel string

const (
	TrustUntrusted   TrustLevel = "untrusted"
	TrustDeclarative TrustLevel = "declarative"
	TrustTrusted     TrustLevel = "trusted"
)

type Scope string

const (
	ScopeProject  Scope = "project"
	ScopeUser     Scope = "user"
	ScopeRemote   Scope = "remote"
	ScopeEmbedded Scope = "embedded"
	ScopeGit      Scope = "git"
)

type Severity string

const (
	SeverityInfo    Severity = "info"
	SeverityWarning Severity = "warning"
	SeverityError   Severity = "error"
)

type SourceRef struct {
	ID        string
	Ecosystem string
	Scope     Scope
	Root      string
	Path      string
	Ref       string
	Trust     TrustLevel
}

func (s SourceRef) Label() string {
	if s.ID != "" {
		return s.ID
	}
	parts := []string{s.Ecosystem, string(s.Scope)}
	if s.Root != "" {
		parts = append(parts, s.Root)
	}
	return strings.Join(nonEmpty(parts), ":")
}

type DiscoveryPolicy struct {
	IncludeGlobalUserResources bool   `json:"include_global_user_resources"`
	IncludeExternalEcosystems  bool   `json:"include_external_ecosystems"`
	AllowRemote                bool   `json:"allow_remote"`
	TrustStoreDir              string `json:"trust_store_dir"`
}

type Candidate struct {
	Source SourceRef
	Kind   string
}

type Diagnostic struct {
	Severity Severity
	Source   SourceRef
	Message  string
}

func Info(source SourceRef, msg string) Diagnostic {
	return Diagnostic{Severity: SeverityInfo, Source: source, Message: msg}
}

func Warning(source SourceRef, msg string) Diagnostic {
	return Diagnostic{Severity: SeverityWarning, Source: source, Message: msg}
}

func Error(source SourceRef, msg string) Diagnostic {
	return Diagnostic{Severity: SeverityError, Source: source, Message: msg}
}

type ToolContribution struct {
	ID          string
	Name        string
	Description string
	Source      SourceRef
	Enabled     bool
	Metadata    map[string]any
	RID         ResourceID
}

type SkillContribution struct {
	ID          string
	Name        string
	Description string
	Source      SourceRef
	Path        string
	Metadata    skill.SkillMetadata
	RID         ResourceID
}

type DataSourceContribution struct {
	ID          string
	Name        string
	Description string
	Kind        string
	Source      SourceRef
	Path        string
	Config      map[string]any
	Metadata    map[string]any
	RID         ResourceID
}

type WorkflowContribution struct {
	ID          string
	Name        string
	Description string
	Source      SourceRef
	Path        string
	Metadata    map[string]any
	Definition  map[string]any
	RID         ResourceID
}

type ActionContribution struct {
	ID          string
	Name        string
	Description string
	Kind        string
	Source      SourceRef
	Path        string
	Config      map[string]any
	Metadata    map[string]any
	RID         ResourceID
}

type TriggerContribution struct {
	ID          string
	Name        string
	Description string
	Source      SourceRef
	Path        string
	Definition  map[string]any
	Metadata    map[string]any
	RID         ResourceID
}

type CommandTargetKind string

const (
	CommandTargetWorkflow CommandTargetKind = "workflow"
	CommandTargetAction   CommandTargetKind = "action"
	CommandTargetPrompt   CommandTargetKind = "prompt"
)

type CommandTarget struct {
	Kind               CommandTargetKind
	Workflow           string
	Action             string
	Prompt             string
	Input              any
	IncludeEvent       bool
	WorkflowDefinition map[string]any
}

type CommandContribution struct {
	ID          string
	Name        string
	Description string
	Source      SourceRef
	Path        string
	CommandPath []string
	InputSchema command.JSONSchema
	Output      command.OutputDescriptor
	Policy      command.Policy
	Target      CommandTarget
	Metadata    map[string]any
	RID         ResourceID
}

type HookContribution struct {
	ID       string
	Name     string
	Source   SourceRef
	Enabled  bool
	Metadata map[string]any
	RID      ResourceID
}

type Permission struct {
	ID          string
	Description string
	Source      SourceRef
}

type ContributionBundle struct {
	ID               string
	Name             string
	Source           SourceRef
	AgentSpecs       []agentconfig.Spec
	Commands         []command.Command
	Skills           []SkillContribution
	SkillSources     []skill.Source
	DataSources      []DataSourceContribution
	Workflows        []WorkflowContribution
	Actions          []ActionContribution
	Triggers         []TriggerContribution
	CommandResources []CommandContribution
	Tools            []ToolContribution
	Hooks            []HookContribution
	Permissions      []Permission
	Diagnostics      []Diagnostic
}

func (b *ContributionBundle) Append(other ContributionBundle) {
	if b == nil {
		return
	}
	b.AgentSpecs = append(b.AgentSpecs, other.AgentSpecs...)
	b.Commands = append(b.Commands, other.Commands...)
	b.Skills = append(b.Skills, other.Skills...)
	b.SkillSources = append(b.SkillSources, other.SkillSources...)
	b.DataSources = append(b.DataSources, other.DataSources...)
	b.Workflows = append(b.Workflows, other.Workflows...)
	b.Actions = append(b.Actions, other.Actions...)
	b.Triggers = append(b.Triggers, other.Triggers...)
	b.CommandResources = append(b.CommandResources, other.CommandResources...)
	b.Tools = append(b.Tools, other.Tools...)
	b.Hooks = append(b.Hooks, other.Hooks...)
	b.Permissions = append(b.Permissions, other.Permissions...)
	b.Diagnostics = append(b.Diagnostics, other.Diagnostics...)
}

func QualifiedID(source SourceRef, kind string, name string, rel string) string {
	ecosystem := source.Ecosystem
	if ecosystem == "" {
		ecosystem = "resource"
	}
	scope := string(source.Scope)
	if scope == "" {
		scope = "project"
	}
	rel = cleanStablePath(rel)
	if rel == "" {
		rel = cleanStablePath(source.Path)
	}
	if rel == "" {
		rel = cleanStablePath(source.Root)
	}
	base := fmt.Sprintf("%s:%s:%s", ecosystem, scope, name)
	if kind != "" && name == "" {
		base = fmt.Sprintf("%s:%s:%s", ecosystem, scope, kind)
	}
	if rel == "" {
		return base
	}
	return base + "#" + escapeIDPart(rel)
}

func cleanStablePath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	if strings.Contains(p, "://") || strings.HasPrefix(p, "git+") {
		return p
	}
	p = strings.TrimPrefix(p, "file://")
	p = strings.TrimPrefix(p, "/")
	if p == "" || p == "." {
		return ""
	}
	return path.Clean(p)
}

func escapeIDPart(s string) string {
	replacer := strings.NewReplacer(
		"#", "%23",
		"\n", "%0A",
		"\r", "%0D",
		"\t", "%09",
	)
	return replacer.Replace(s)
}

type Registry struct {
	agents      map[string]agentconfig.Spec
	agentIDs    map[string]agentconfig.Spec
	diagnostics []Diagnostic
}

func NewRegistry() *Registry {
	return &Registry{agents: map[string]agentconfig.Spec{}, agentIDs: map[string]agentconfig.Spec{}}
}

func (r *Registry) RegisterAgent(source SourceRef, id string, spec agentconfig.Spec) {
	if r.agents == nil {
		r.agents = map[string]agentconfig.Spec{}
	}
	if r.agentIDs == nil {
		r.agentIDs = map[string]agentconfig.Spec{}
	}
	if id != "" {
		r.agentIDs[id] = spec
	}
	if _, exists := r.agents[spec.Name]; exists {
		r.diagnostics = append(r.diagnostics, Warning(source, fmt.Sprintf("agent %q ignored because the short name is already registered", spec.Name)))
		return
	}
	r.agents[spec.Name] = spec
}

func (r *Registry) Agent(name string) (agentconfig.Spec, bool) {
	if r == nil {
		return agentconfig.Spec{}, false
	}
	spec, ok := r.agents[strings.TrimSpace(name)]
	return spec, ok
}

func (r *Registry) AgentByID(id string) (agentconfig.Spec, bool) {
	if r == nil {
		return agentconfig.Spec{}, false
	}
	spec, ok := r.agentIDs[strings.TrimSpace(id)]
	return spec, ok
}

func (r *Registry) Diagnostics() []Diagnostic {
	if r == nil {
		return nil
	}
	return append([]Diagnostic(nil), r.diagnostics...)
}

func (r *Registry) AgentNames() []string {
	if r == nil {
		return nil
	}
	names := make([]string, 0, len(r.agents))
	for name := range r.agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func nonEmpty(in []string) []string {
	out := in[:0]
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
