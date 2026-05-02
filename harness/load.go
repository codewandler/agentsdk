package harness

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
)

// SessionLoadConfig describes the app and agent configuration needed to create
// a harness service plus its default session. Hosts remain responsible for
// channel-specific policy and presentation adapters; harness owns the generic
// app/agent/session wiring.
type SessionLoadConfig struct {
	Output                     io.Writer
	ResourceBundle             resource.ContributionBundle
	DefaultAgent               string
	Workspace                  string
	IncludeGlobalUserResources bool
	Verbose                    bool
	ToolTimeout                time.Duration
	SessionStoreDir            string

	AppOptions   []app.Option
	AgentOptions []agent.Option
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
	inst, err := application.InstantiateDefaultAgent(cfg.AgentOptions...)
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
	if cfg.Output != nil {
		opts = append(opts, app.WithOutput(cfg.Output), app.WithAgentOutput(cfg.Output))
	}
	if hasResourceBundle(cfg.ResourceBundle) {
		opts = append(opts, app.WithResourceBundle(cfg.ResourceBundle))
	}
	if cfg.DefaultAgent != "" {
		opts = append(opts, app.WithDefaultAgent(cfg.DefaultAgent))
	}
	if cfg.Workspace != "" {
		opts = append(opts,
			app.WithDefaultSkillSourceDiscovery(app.SkillSourceDiscovery{WorkspaceDir: cfg.Workspace, IncludeGlobalUserResources: cfg.IncludeGlobalUserResources}),
			app.WithAgentWorkspace(cfg.Workspace),
		)
	}
	if cfg.Verbose {
		opts = append(opts, app.WithAgentVerbose(true))
	}
	if cfg.ToolTimeout > 0 {
		opts = append(opts, app.WithAgentToolTimeout(cfg.ToolTimeout))
	}
	if cfg.SessionStoreDir != "" {
		opts = append(opts, app.WithAgentSessionStoreDir(cfg.SessionStoreDir))
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
