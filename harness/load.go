package harness

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/llmadapter/adapt"
)

// SessionLoadConfig describes the app and agent configuration needed to create
// a harness service plus its default session. Hosts remain responsible for
// channel-specific policy and presentation adapters; harness owns the generic
// app/agent/session wiring.
type SessionLoadConfig struct {
	App     AppLoadConfig
	Agent   AgentLoadConfig
	Session SessionOpenConfig
	Plugins []app.Plugin

	AppOptions   []app.Option
	AgentOptions []agent.Option
}

type AppLoadConfig struct {
	Output                     io.Writer
	ResourceBundle             resource.ContributionBundle
	DefaultAgent               string
	Workspace                  string
	IncludeGlobalUserResources bool
	Verbose                    bool
	ToolTimeout                time.Duration
}

type AgentLoadOverrides struct {
	ModelPolicy      agent.ModelPolicy
	ApplyModelPolicy bool
	SourceAPI        adapt.ApiKind
	ApplySourceAPI   bool
}

type AgentLoadConfig struct {
	ModelPolicy      agent.ModelPolicy
	ApplyModelPolicy bool
	SourceAPI        adapt.ApiKind
	ApplySourceAPI   bool
}

type SessionOpenConfig struct {
	StoreDir string
	Resume   string
}

type FallbackAgent struct {
	Enabled bool
	Spec    agent.Spec
}

func EnsureFallbackAgent(resolved *agentdir.Resolution, requestedName string, fallback FallbackAgent) bool {
	if resolved == nil || !fallback.Enabled || strings.TrimSpace(requestedName) != "" || fallback.Spec.Name == "" || len(resolved.Bundle.AgentSpecs) > 0 {
		return false
	}
	resolved.Bundle.AgentSpecs = append(resolved.Bundle.AgentSpecs, fallback.Spec)
	resolved.DefaultAgent = fallback.Spec.Name
	return true
}

type PluginLoadConfig struct {
	Factory  app.PluginFactory
	Defaults []agentdir.PluginRef
	Manifest []agentdir.PluginRef
	Explicit []agentdir.PluginRef
}

func ResolvePlugins(ctx context.Context, cfg PluginLoadConfig) ([]app.Plugin, error) {
	refs := orderedPluginRefs(cfg)
	if len(refs) == 0 {
		return nil, nil
	}
	if cfg.Factory == nil {
		return nil, fmt.Errorf("harness: plugin factory is required when plugins are configured")
	}
	seen := map[string]bool{}
	plugins := make([]app.Plugin, 0, len(refs))
	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		plugin, err := cfg.Factory.PluginForName(ctx, name, ref.Config)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, plugin)
	}
	return plugins, nil
}

func orderedPluginRefs(cfg PluginLoadConfig) []agentdir.PluginRef {
	refs := make([]agentdir.PluginRef, 0, len(cfg.Defaults)+len(cfg.Manifest)+len(cfg.Explicit))
	refs = append(refs, cfg.Defaults...)
	refs = append(refs, cfg.Manifest...)
	refs = append(refs, cfg.Explicit...)
	return refs
}

func ResolveAgentLoadConfig(resolved agentdir.Resolution, overrides AgentLoadOverrides) AgentLoadConfig {
	modelPolicy := overrides.ModelPolicy
	applyModelPolicy := overrides.ApplyModelPolicy
	if resolved.HasModelPolicy {
		if applyModelPolicy {
			modelPolicy = overlayModelPolicy(resolved.ModelPolicy, overrides.ModelPolicy)
		} else {
			modelPolicy = resolved.ModelPolicy
		}
		applyModelPolicy = true
	}
	if overrides.ApplySourceAPI && applyModelPolicy {
		modelPolicy.SourceAPI = overrides.SourceAPI
	}
	return AgentLoadConfig{
		ModelPolicy:      modelPolicy,
		ApplyModelPolicy: applyModelPolicy,
		SourceAPI:        overrides.SourceAPI,
		ApplySourceAPI:   overrides.ApplySourceAPI,
	}
}

func overlayModelPolicy(base agent.ModelPolicy, override agent.ModelPolicy) agent.ModelPolicy {
	out := base
	if override.UseCase != "" {
		out.UseCase = override.UseCase
	}
	if override.SourceAPI != "" {
		out.SourceAPI = override.SourceAPI
	}
	if override.ApprovedOnly {
		out.ApprovedOnly = true
	}
	if override.AllowDegraded {
		out.AllowDegraded = true
	}
	if override.AllowUntested {
		out.AllowUntested = true
	}
	if override.EvidencePath != "" {
		out.EvidencePath = override.EvidencePath
	}
	return out
}

type AgentSpecOverrides struct {
	Inference      agent.InferenceOptions
	ApplyInference bool
	MaxSteps       int
	ApplyMaxSteps  bool
	System         string
}

type AgentSelection struct {
	Name string
}

func PrepareResolvedAgent(resolved *agentdir.Resolution, requestedName string, overrides AgentSpecOverrides) (AgentSelection, error) {
	if resolved == nil {
		return AgentSelection{}, fmt.Errorf("harness: resolved resources are required")
	}
	name, err := resolved.ResolveDefaultAgent(requestedName)
	if err != nil {
		return AgentSelection{}, err
	}
	if err := resolved.UpdateAgentSpec(name, func(spec *agent.Spec) {
		if overrides.ApplyInference {
			spec.Inference = overrides.Inference
		}
		if overrides.ApplyMaxSteps {
			spec.MaxSteps = overrides.MaxSteps
		}
		if strings.TrimSpace(overrides.System) != "" {
			spec.System = overrides.System
		}
	}); err != nil {
		return AgentSelection{}, err
	}
	return AgentSelection{Name: name}, nil
}

// LoadedSession is the running harness stack created from SessionLoadConfig.
type LoadedSession struct {
	App     *app.App
	Agent   *agent.Instance
	Service *Service
	Session *Session
}

func LoadSession(cfg SessionLoadConfig) (*LoadedSession, error) {
	application, err := app.New(sessionAppOptions(cfg)...)
	if err != nil {
		return nil, err
	}
	inst, err := application.InstantiateDefaultAgent(sessionAgentOptions(cfg)...)
	if err != nil {
		return nil, err
	}
	service := NewService(application)
	session, err := service.DefaultSession()
	if err != nil {
		return nil, err
	}
	return &LoadedSession{App: application, Agent: inst, Service: service, Session: session}, nil
}

func sessionAppOptions(cfg SessionLoadConfig) []app.Option {
	var opts []app.Option
	appCfg := cfg.App
	if appCfg.Output != nil {
		opts = append(opts, app.WithOutput(appCfg.Output), app.WithAgentOutput(appCfg.Output))
	}
	if hasResourceBundle(appCfg.ResourceBundle) {
		opts = append(opts, app.WithResourceBundle(appCfg.ResourceBundle))
	}
	if appCfg.DefaultAgent != "" {
		opts = append(opts, app.WithDefaultAgent(appCfg.DefaultAgent))
	}
	if appCfg.Workspace != "" {
		opts = append(opts,
			app.WithDefaultSkillSourceDiscovery(app.SkillSourceDiscovery{WorkspaceDir: appCfg.Workspace, IncludeGlobalUserResources: appCfg.IncludeGlobalUserResources}),
			app.WithAgentWorkspace(appCfg.Workspace),
		)
	}
	if appCfg.Verbose {
		opts = append(opts, app.WithAgentOptions(agent.WithVerbose(true)))
	}
	if appCfg.ToolTimeout > 0 {
		opts = append(opts, app.WithAgentToolTimeout(appCfg.ToolTimeout))
	}
	if cfg.Session.StoreDir != "" {
		opts = append(opts, app.WithAgentSessionStoreDir(cfg.Session.StoreDir))
	}
	for _, plugin := range cfg.Plugins {
		if plugin != nil {
			opts = append(opts, app.WithPlugin(plugin))
		}
	}
	if len(cfg.AppOptions) > 0 {
		opts = append(opts, cfg.AppOptions...)
	}
	return opts
}

func hasResourceBundle(bundle resource.ContributionBundle) bool {
	return bundle.Source.ID != "" ||
		len(bundle.Commands) > 0 ||
		len(bundle.AgentSpecs) > 0 ||
		len(bundle.DataSources) > 0 ||
		len(bundle.Workflows) > 0 ||
		len(bundle.SkillSources) > 0 ||
		len(bundle.Diagnostics) > 0
}

func sessionAgentOptions(cfg SessionLoadConfig) []agent.Option {
	opts := append([]agent.Option(nil), cfg.AgentOptions...)
	if cfg.Agent.ApplySourceAPI {
		opts = append(opts, agent.WithSourceAPI(cfg.Agent.SourceAPI))
	}
	if cfg.Agent.ApplyModelPolicy {
		opts = append(opts, agent.WithModelPolicy(cfg.Agent.ModelPolicy))
	}
	if cfg.Session.Resume != "" {
		opts = append(opts, agent.WithResumeSession(cfg.Session.Resume))
	}
	return opts
}
