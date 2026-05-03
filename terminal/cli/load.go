package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/harness"
	"github.com/codewandler/agentsdk/plugins/localcli"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/terminal/ui"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/unified"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Resources Resources

	AgentName string
	Task      string

	Workspace          string
	SessionsDir        string
	DefaultSessionsDir string
	Session            string
	ContinueLast       bool

	Inference        agent.InferenceOptions
	ApplyInference   bool
	SourceAPI        string
	ApplySourceAPI   bool
	ModelPolicy      agent.ModelPolicy
	ApplyModelPolicy bool
	MaxSteps         int
	ApplyMaxSteps    bool
	SystemOverride   string
	ToolTimeout      time.Duration
	TotalTimeout     time.Duration
	Verbose          bool
	DebugMessage     bool
	Prompt           string

	PluginNames      []string
	NoDefaultPlugins bool
	PluginFactory    app.PluginFactory

	AgentOptions    []agent.Option
	AppOptions      []app.Option
	DiscoveryPolicy resource.DiscoveryPolicy

	In  io.Reader
	Out io.Writer
	Err io.Writer
}

// Loaded captures everything the CLI sets up before sending a task or
// rendering a request, so callers (run, render) can share the load path.
type Loaded struct {
	App       *app.App
	Agent     *agent.Instance
	Harness   *harness.Service
	Session   *harness.Session
	AgentName string
	Workspace string
	In        io.Reader
	Out       io.Writer
	Err       io.Writer
}

type loadEnvironment struct {
	In          io.Reader
	Out         io.Writer
	Err         io.Writer
	Workspace   string
	SessionsDir string
	ResumePath  string
	Discovery   resource.DiscoveryPolicy
}

// Load resolves resources, instantiates the default agent, and returns the
// app + agent without executing any task or REPL. It is the shared prelude
// used by Run and the render command.
func Load(ctx context.Context, cfg Config) (*Loaded, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	env, err := loadEnvironmentFromConfig(cfg)
	if err != nil {
		return nil, err
	}
	resolved, err := resolveConfiguredResources(cfg, env)
	if err != nil {
		return nil, err
	}
	selection, err := harness.PrepareResolvedAgent(&resolved, cfg.AgentName, harness.AgentSpecOverrides{
		Inference:      cfg.Inference,
		ApplyInference: cfg.ApplyInference,
		MaxSteps:       cfg.MaxSteps,
		ApplyMaxSteps:  cfg.ApplyMaxSteps,
		System:         cfg.SystemOverride,
	})
	if err != nil {
		return nil, err
	}
	name := selection.Name
	modelPolicy, applyModelPolicy, err := selectModelPolicy(resolved, cfg)
	if err != nil {
		return nil, err
	}
	sourceAPI, applySourceAPI, err := sourceAPIOption(cfg)
	if err != nil {
		return nil, err
	}
	appOpts, err := appOptions(ctx, resolved, cfg, env)
	if err != nil {
		return nil, err
	}
	agentOpts, err := agentOptions(cfg, env)
	if err != nil {
		return nil, err
	}
	loaded, err := harness.LoadSession(harness.SessionLoadConfig{
		App: harness.AppLoadConfig{
			Output:                     env.Out,
			ResourceBundle:             resolved.Bundle,
			DefaultAgent:               name,
			Workspace:                  env.Workspace,
			IncludeGlobalUserResources: env.Discovery.IncludeGlobalUserResources,
			Verbose:                    cfg.Verbose,
			ToolTimeout:                cfg.ToolTimeout,
		},
		Agent: harness.AgentLoadConfig{
			ModelPolicy:      modelPolicy,
			ApplyModelPolicy: applyModelPolicy,
			SourceAPI:        sourceAPI,
			ApplySourceAPI:   applySourceAPI,
		},
		Session: harness.SessionOpenConfig{
			StoreDir: env.SessionsDir,
			Resume:   env.ResumePath,
		},
		AppOptions:   appOpts,
		AgentOptions: agentOpts,
	})
	if err != nil {
		return nil, err
	}
	return &Loaded{
		App:       loaded.App,
		Agent:     loaded.Agent,
		Harness:   loaded.Service,
		Session:   loaded.Session,
		AgentName: name,
		Workspace: env.Workspace,
		In:        env.In,
		Out:       env.Out,
		Err:       env.Err,
	}, nil
}

func loadEnvironmentFromConfig(cfg Config) (loadEnvironment, error) {
	in := cfg.In
	if in == nil {
		in = os.Stdin
	}
	out := cfg.Out
	if out == nil {
		out = os.Stdout
	}
	errOut := cfg.Err
	if errOut == nil {
		errOut = os.Stderr
	}
	workspace := cfg.Workspace
	if workspace == "" {
		wd, err := os.Getwd()
		if err != nil {
			return loadEnvironment{}, err
		}
		workspace = wd
	}
	if abs, err := filepath.Abs(workspace); err == nil {
		workspace = abs
	}
	sessionsDir, err := resolveSessionsDir(cfg)
	if err != nil {
		return loadEnvironment{}, err
	}
	resumePath, err := ResolveSessionPath(sessionsDir, cfg.Session, cfg.ContinueLast)
	if err != nil {
		return loadEnvironment{}, err
	}
	policy := cfg.DiscoveryPolicy
	if policy.TrustStoreDir == "" {
		policy.TrustStoreDir = filepath.Join(workspace, ".agentsdk")
	}
	return loadEnvironment{In: in, Out: out, Err: errOut, Workspace: workspace, SessionsDir: sessionsDir, ResumePath: resumePath, Discovery: policy}, nil
}

func resolveConfiguredResources(cfg Config, env loadEnvironment) (agentdir.Resolution, error) {
	if cfg.Resources == nil {
		return agentdir.Resolution{}, fmt.Errorf("cli: resources are required")
	}
	resolved, err := cfg.Resources.Resolve(env.Discovery)
	if err != nil {
		return agentdir.Resolution{}, err
	}
	harness.EnsureFallbackAgent(&resolved, cfg.AgentName, harness.FallbackAgent{
		Enabled: !cfg.NoDefaultPlugins,
		Spec:    localcli.DefaultAgent(),
	})
	return resolved, nil
}

func selectModelPolicy(resolved agentdir.Resolution, cfg Config) (agent.ModelPolicy, bool, error) {
	modelPolicy := cfg.ModelPolicy
	applyModelPolicy := cfg.ApplyModelPolicy
	if resolved.HasModelPolicy {
		if applyModelPolicy {
			modelPolicy = overlayModelPolicy(resolved.ModelPolicy, cfg.ModelPolicy)
		} else {
			modelPolicy = resolved.ModelPolicy
		}
		applyModelPolicy = true
	}
	if cfg.ApplySourceAPI && applyModelPolicy {
		sourceAPI, err := agent.ParseSourceAPI(cfg.SourceAPI)
		if err != nil {
			return agent.ModelPolicy{}, false, err
		}
		modelPolicy.SourceAPI = sourceAPI
	}
	return modelPolicy, applyModelPolicy, nil
}

func sourceAPIOption(cfg Config) (adapt.ApiKind, bool, error) {
	if !cfg.ApplySourceAPI {
		return "", false, nil
	}
	sourceAPI, err := agent.ParseSourceAPI(cfg.SourceAPI)
	if err != nil {
		return "", false, err
	}
	return sourceAPI, true, nil
}

func appOptions(ctx context.Context, resolved agentdir.Resolution, cfg Config, env loadEnvironment) ([]app.Option, error) {
	appOpts := []app.Option{
		app.WithAgentOptions(agent.WithEventHandlerFactory(ui.AgentEventHandlerFactory(env.Out))),
	}
	pluginOpts, err := pluginOptions(ctx, resolved, cfg)
	if err != nil {
		return nil, err
	}
	appOpts = append(appOpts, pluginOpts...)
	// Risk gate: log-only mode — observes all tool calls, always approves.
	// Write to stderr so TUI doesn't overwrite the output.
	appOpts = append(appOpts, app.WithToolMiddlewares(
		tool.HooksMiddleware(&riskLogMiddleware{out: os.Stderr}),
	))
	appOpts = append(appOpts, cfg.AppOptions...)
	return appOpts, nil
}

func agentOptions(cfg Config, env loadEnvironment) ([]agent.Option, error) {
	instOpts := append([]agent.Option(nil), cfg.AgentOptions...)
	if cfg.DebugMessage {
		instOpts = append(instOpts, agent.WithRequestObserver(debugMessageObserver(env.Out)))
	}
	return instOpts, nil
}

func pluginOptions(ctx context.Context, resolved agentdir.Resolution, cfg Config) ([]app.Option, error) {
	refs := append([]agentdir.PluginRef(nil), resolved.ManifestPluginRefs()...)
	for _, name := range cfg.PluginNames {
		refs = append(refs, agentdir.PluginRef{Name: name})
	}
	if !cfg.NoDefaultPlugins {
		refs = append([]agentdir.PluginRef{{Name: localcli.PluginName}}, refs...)
	}
	seen := map[string]bool{}
	var opts []app.Option
	for _, ref := range refs {
		name := strings.TrimSpace(ref.Name)
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		factory := cfg.PluginFactory
		if factory == nil {
			factory = localcli.NewFactory()
		}
		plugin, err := factory.PluginForName(ctx, name, ref.Config)
		if err != nil {
			return nil, err
		}
		opts = append(opts, app.WithPlugin(plugin))
	}
	return opts, nil
}

// debugMessageObserver returns a RequestObserver that prints each outgoing
// request's messages slice as YAML, separated by document markers so
// successive turns can be distinguished.
func debugMessageObserver(out io.Writer) runner.RequestObserver {
	return func(_ context.Context, req unified.Request) {
		payload, err := marshalMessagesYAML(req.Messages)
		if err != nil {
			fmt.Fprintf(out, "# debug-message: marshal error: %v\n", err)
			return
		}
		fmt.Fprintln(out, "---")
		_, _ = out.Write(payload)
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

// marshalMessagesYAML serializes the messages slice as YAML, routing through
// JSON so the snake_case names and omitempty semantics from unified's `json`
// tags are honored.
func marshalMessagesYAML(messages []unified.Message) ([]byte, error) {
	raw, err := json.Marshal(messages)
	if err != nil {
		return nil, err
	}
	var shaped any
	if err := yaml.Unmarshal(raw, &shaped); err != nil {
		return nil, err
	}
	return yaml.Marshal(shaped)
}

func resolveSessionsDir(cfg Config) (string, error) {
	dir := cfg.SessionsDir
	if dir == "" {
		dir = cfg.DefaultSessionsDir
	}
	if dir == "" {
		return "", nil
	}
	return filepath.Abs(dir)
}

// riskLogMiddleware logs intent + risk assessment for every tool call.
// It always allows execution — observation only.
type riskLogMiddleware struct {
	tool.HooksBase
	out io.Writer
}

func (m *riskLogMiddleware) OnInput(ctx tool.Ctx, inner tool.Tool, input json.RawMessage, state tool.CallState) (json.RawMessage, tool.Result, error) {
	intent := tool.ExtractIntent(inner, ctx, input)
	state["intent"] = intent

	var parts []string
	parts = append(parts, fmt.Sprintf("tool=%s class=%s confidence=%s", intent.Tool, intent.ToolClass, intent.Confidence))
	if intent.Opaque {
		parts = append(parts, "opaque=true")
	}
	for _, op := range intent.Operations {
		parts = append(parts, fmt.Sprintf("  %s %s:%s (%s)", op.Operation, op.Resource.Category, op.Resource.Value, op.Resource.Locality))
	}
	for _, b := range intent.Behaviors {
		parts = append(parts, fmt.Sprintf("  behavior=%s", b))
	}
	fmt.Fprintf(m.out, "\n\033[2m[risk] %s\033[0m\n", strings.Join(parts, " | "))
	return input, nil, nil
}
