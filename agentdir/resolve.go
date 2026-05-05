package agentdir

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/resource"
)

var manifestNames = []string{"app.manifest.json", "agentsdk.app.json"}

type AppManifest struct {
	DefaultAgent string              `json:"default_agent"`
	Discovery    ManifestDiscovery   `json:"discovery"`
	ModelPolicy  ManifestModelPolicy `json:"model_policy"`
	Sources      []string            `json:"sources"`
	Plugins      []PluginRef         `json:"plugins,omitempty"`
}

type PluginRef struct {
	Name   string         `json:"name"`
	Config map[string]any `json:"config,omitempty"`
}

func (r *PluginRef) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		r.Name = name
		return nil
	}
	var shaped struct {
		Name   string         `json:"name"`
		Config map[string]any `json:"config,omitempty"`
		Path   string         `json:"path,omitempty"`
	}
	if err := json.Unmarshal(data, &shaped); err != nil {
		return err
	}
	if shaped.Path != "" {
		return fmt.Errorf("plugin path references are no longer supported; use plugin name references")
	}
	r.Name = shaped.Name
	r.Config = shaped.Config
	return nil
}

func (r PluginRef) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("plugin name is required")
	}
	return nil
}

func (m AppManifest) Validate() error {
	for i, ref := range m.Plugins {
		if err := ref.Validate(); err != nil {
			return fmt.Errorf("plugins[%d]: %w", i, err)
		}
	}
	return nil
}

type ManifestDiscovery struct {
	IncludeGlobalUserResources *bool  `json:"include_global_user_resources"`
	IncludeExternalEcosystems  *bool  `json:"include_external_ecosystems"`
	AllowRemote                *bool  `json:"allow_remote"`
	TrustStoreDir              string `json:"trust_store_dir"`
}

type ResolveOptions struct {
	Policy       resource.DiscoveryPolicy
	HomeDir      string
	WorkspaceDir string
	LocalOnly    bool
}

type Resolution struct {
	Bundle         resource.ContributionBundle
	DefaultAgent   string
	Manifest       *AppManifest
	ModelPolicy    agentconfig.ModelPolicy
	HasModelPolicy bool
	Sources        []string
}

// ResolveDir resolves a path as an app manifest, embedded plugin roots, or a
// plugin root in the deterministic order used by agentsdk run.
func ResolveDir(dir string) (Resolution, error) {
	return ResolveDirWithOptions(dir, ResolveOptions{})
}

func ResolveDirWithOptions(dir string, opts ResolveOptions) (Resolution, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return Resolution{}, err
	}
	if opts.WorkspaceDir == "" {
		opts.WorkspaceDir = dir
	}
	if opts.Policy.TrustStoreDir == "" {
		opts.Policy.TrustStoreDir = filepath.Join(opts.WorkspaceDir, ".agentsdk")
	}
	if opts.LocalOnly {
		opts.Policy.IncludeGlobalUserResources = false
		opts.Policy.AllowRemote = false
	}
	manifestPath, manifest, ok, err := readManifest(dir)
	if err != nil {
		return Resolution{}, err
	}
	if ok {
		return resolveManifest(dir, manifestPath, manifest, opts)
	}
	var out Resolution
	for _, name := range []string{".agents", ".claude"} {
		candidate := filepath.Join(dir, name)
		if exists, err := osDirExists(candidate); err != nil {
			return Resolution{}, err
		} else if exists {
			bundle, err := LoadDirWithSource(candidate, sourceForCandidate(candidate, dir, resource.ScopeProject))
			if err != nil {
				return Resolution{}, fmt.Errorf("load %s: %w", candidate, err)
			}
			appendResolvedBundle(&out, candidate, bundle)
		}
	}
	if len(out.Sources) == 0 {
		bundle, err := LoadDirWithSource(dir, sourceForCandidate(dir, dir, resource.ScopeProject))
		if err != nil {
			return Resolution{}, err
		}
		out.Bundle = bundle
		out.Sources = append(out.Sources, dir)
	}
	if opts.Policy.IncludeGlobalUserResources {
		home := opts.HomeDir
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		for _, name := range []string{".agents", ".claude"} {
			if home == "" {
				continue
			}
			candidate := filepath.Join(home, name)
			if exists, err := osDirExists(candidate); err != nil {
				return Resolution{}, err
			} else if exists {
				bundle, err := LoadDirWithSource(candidate, sourceForCandidate(candidate, home, resource.ScopeUser))
				if err != nil {
					return Resolution{}, fmt.Errorf("load %s: %w", candidate, err)
				}
				appendResolvedBundle(&out, candidate, bundle)
			}
		}
	}
	appendExternalCandidates(&out.Bundle, dir, opts.Policy)
	return out, nil
}

// ResolveFS resolves an embedded or virtual filesystem root as a plugin root.
func ResolveFS(fsys fs.FS, root string) (Resolution, error) {
	bundle, err := LoadFSWithSource(fsys, root, resource.SourceRef{
		ID:        resource.QualifiedID(resource.SourceRef{Ecosystem: "agents", Scope: resource.ScopeEmbedded}, "source", "", root),
		Ecosystem: "agents",
		Scope:     resource.ScopeEmbedded,
		Root:      root,
		Path:      root,
		Trust:     resource.TrustDeclarative,
	})
	if err != nil {
		return Resolution{}, err
	}
	return Resolution{Bundle: bundle, Sources: []string{root}}, nil
}

func ResolveDefaultAgent(specs []string, explicit string, manifestDefault string) (string, error) {
	names := uniqueStrings(specs)
	sort.Strings(names)
	has := func(name string) bool {
		for _, candidate := range names {
			if candidate == name {
				return true
			}
		}
		return false
	}
	if explicit != "" {
		if !has(explicit) {
			return "", fmt.Errorf("agentdir: agent %q not found; available agents: %v", explicit, names)
		}
		return explicit, nil
	}
	if manifestDefault != "" {
		if !has(manifestDefault) {
			return "", fmt.Errorf("agentdir: default agent %q not found; available agents: %v", manifestDefault, names)
		}
		return manifestDefault, nil
	}
	if len(names) == 1 {
		return names[0], nil
	}
	for _, conventional := range []string{"main", "default"} {
		if has(conventional) {
			return conventional, nil
		}
	}
	if len(names) == 0 {
		return "", fmt.Errorf("agentdir: no agents found")
	}
	return "", fmt.Errorf("agentdir: multiple agents found; choose one with --agent: %v", names)
}

func (r Resolution) AgentNames() []string {
	names := make([]string, 0, len(r.Bundle.AgentSpecs))
	for _, spec := range r.Bundle.AgentSpecs {
		names = append(names, spec.Name)
	}
	names = uniqueStrings(names)
	sort.Strings(names)
	return names
}

func (r Resolution) ResolveDefaultAgent(explicit string) (string, error) {
	return ResolveDefaultAgent(r.AgentNames(), explicit, r.DefaultAgent)
}

func (r Resolution) ManifestPluginRefs() []PluginRef {
	if r.Manifest == nil || len(r.Manifest.Plugins) == 0 {
		return nil
	}
	return append([]PluginRef(nil), r.Manifest.Plugins...)
}

func (r *Resolution) UpdateAgentSpec(name string, update func(*agentconfig.Spec)) error {
	if r == nil {
		return fmt.Errorf("agentdir: resolution is nil")
	}
	if name == "" {
		return fmt.Errorf("agentdir: agent name is required")
	}
	for i := range r.Bundle.AgentSpecs {
		if r.Bundle.AgentSpecs[i].Name == name {
			if update != nil {
				update(&r.Bundle.AgentSpecs[i])
			}
			return nil
		}
	}
	return fmt.Errorf("agentdir: agent spec %q not found; available agents: %v", name, r.AgentNames())
}

func resolveManifest(dir string, manifestPath string, manifest AppManifest, opts ResolveOptions) (Resolution, error) {
	out := Resolution{Manifest: &manifest, DefaultAgent: manifest.DefaultAgent}
	if policy, ok, err := manifest.ModelPolicy.AgentPolicy(dir); err != nil {
		return Resolution{}, err
	} else if ok {
		out.ModelPolicy = policy
		out.HasModelPolicy = true
	}
	policy := opts.Policy
	if manifest.Discovery.IncludeExternalEcosystems != nil {
		policy.IncludeExternalEcosystems = *manifest.Discovery.IncludeExternalEcosystems
	}
	if manifest.Discovery.TrustStoreDir != "" {
		policy.TrustStoreDir = manifest.Discovery.TrustStoreDir
		if !filepath.IsAbs(policy.TrustStoreDir) {
			policy.TrustStoreDir = filepath.Join(dir, policy.TrustStoreDir)
		}
	}
	if opts.LocalOnly {
		policy.IncludeGlobalUserResources = false
		policy.AllowRemote = false
	} else if manifest.Discovery.IncludeGlobalUserResources != nil {
		policy.IncludeGlobalUserResources = *manifest.Discovery.IncludeGlobalUserResources
	}
	if !opts.LocalOnly && manifest.Discovery.AllowRemote != nil {
		policy.AllowRemote = *manifest.Discovery.AllowRemote
	}
	if len(manifest.Sources) == 0 {
		manifest.Sources = []string{"."}
	}
	for _, sourceURI := range manifest.Sources {
		if opts.LocalOnly && isRemoteSource(sourceURI) {
			out.Bundle.Diagnostics = append(out.Bundle.Diagnostics, resource.Info(resource.SourceRef{
				Ecosystem: "agents",
				Scope:     resource.ScopeRemote,
				Path:      sourceURI,
				Trust:     resource.TrustUntrusted,
			}, fmt.Sprintf("remote source %q skipped in local-only discovery", sourceURI)))
			continue
		}
		sourcePath, source, err := materializeSource(dir, sourceURI, policy)
		if err != nil {
			return Resolution{}, err
		}
		bundle, err := LoadDirWithSource(sourcePath, source)
		if err != nil {
			return Resolution{}, fmt.Errorf("load manifest source %s: %w", sourceURI, err)
		}
		appendResolvedBundle(&out, sourceURI, bundle)
	}
	if policy.IncludeGlobalUserResources {
		home := opts.HomeDir
		if home == "" {
			home, _ = os.UserHomeDir()
		}
		for _, name := range []string{".agents", ".claude"} {
			if home == "" {
				continue
			}
			candidate := filepath.Join(home, name)
			if exists, err := osDirExists(candidate); err != nil {
				return Resolution{}, err
			} else if exists {
				bundle, err := LoadDirWithSource(candidate, sourceForCandidate(candidate, home, resource.ScopeUser))
				if err != nil {
					return Resolution{}, fmt.Errorf("load %s: %w", candidate, err)
				}
				appendResolvedBundle(&out, candidate, bundle)
			}
		}
	}
	appendExternalCandidates(&out.Bundle, dir, policy)
	return out, nil
}

func readManifest(dir string) (string, AppManifest, bool, error) {
	for _, name := range manifestNames {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", AppManifest{}, false, err
		}
		var manifest AppManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return "", AppManifest{}, false, fmt.Errorf("parse %s: %w", path, err)
		}
		if err := manifest.Validate(); err != nil {
			return "", AppManifest{}, false, fmt.Errorf("validate %s: %w", path, err)
		}
		return path, manifest, true, nil
	}
	return "", AppManifest{}, false, nil
}

func osDirExists(dir string) (bool, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func SourceExists(fsys fs.FS, dir string) bool {
	ok, _ := dirExists(fsys, dir)
	return ok
}

func appendResolvedBundle(out *Resolution, source string, bundle resource.ContributionBundle) {
	// Preserve the first non-empty Source so the merged bundle retains
	// provenance for resources that don't carry their own SourceRef
	// (e.g. agentconfig.Spec).
	if out.Bundle.Source.ID == "" && bundle.Source.ID != "" {
		out.Bundle.Source = bundle.Source
	}
	out.Bundle.Append(bundle)
	if bundleHasResources(bundle) {
		// Use the bundle's resolved root path as the source, not the
		// raw URI. This ensures sources are actual agentdir roots.
		root := bundle.Source.Root
		if root == "" {
			root = source
		}
		out.Sources = append(out.Sources, root)
	}
}

func bundleHasResources(bundle resource.ContributionBundle) bool {
	return len(bundle.AgentSpecs) > 0 ||
		len(bundle.Commands) > 0 ||
		len(bundle.Skills) > 0 ||
		len(bundle.SkillSources) > 0 ||
		len(bundle.DataSources) > 0 ||
		len(bundle.Workflows) > 0 ||
		len(bundle.Actions) > 0 ||
		len(bundle.Triggers) > 0 ||
		len(bundle.CommandResources) > 0 ||
		len(bundle.Tools) > 0 ||
		len(bundle.Hooks) > 0 ||
		len(bundle.Permissions) > 0 ||
		len(bundle.Diagnostics) > 0
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
