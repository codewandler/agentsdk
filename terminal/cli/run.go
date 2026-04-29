package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/terminal/repl"
	"github.com/codewandler/agentsdk/terminal/ui"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/toolmw"
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
	AgentName string
	Workspace string
	In        io.Reader
	Out       io.Writer
	Err       io.Writer
}

// Load resolves resources, instantiates the default agent, and returns the
// app + agent without executing any task or REPL. It is the shared prelude
// used by Run and the render command.
func Load(ctx context.Context, cfg Config) (*Loaded, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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
			return nil, err
		}
		workspace = wd
	}
	if abs, err := filepath.Abs(workspace); err == nil {
		workspace = abs
	}
	sessionsDir, err := resolveSessionsDir(cfg)
	if err != nil {
		return nil, err
	}
	resumePath, err := ResolveSessionPath(sessionsDir, cfg.Session, cfg.ContinueLast)
	if err != nil {
		return nil, err
	}
	if cfg.Resources == nil {
		return nil, fmt.Errorf("cli: resources are required")
	}
	policy := cfg.DiscoveryPolicy
	if policy.TrustStoreDir == "" {
		policy.TrustStoreDir = filepath.Join(workspace, ".agentsdk")
	}
	resolved, err := cfg.Resources.Resolve(policy)
	if err != nil {
		return nil, err
	}
	if len(resolved.Bundle.AgentSpecs) == 0 && strings.TrimSpace(cfg.AgentName) == "" {
		spec := agent.DefaultSpec()
		resolved.Bundle.AgentSpecs = append(resolved.Bundle.AgentSpecs, spec)
		resolved.DefaultAgent = spec.Name
	}
	name, err := resolved.ResolveDefaultAgent(cfg.AgentName)
	if err != nil {
		return nil, err
	}
	if err := resolved.UpdateAgentSpec(name, func(spec *agent.Spec) {
		if cfg.ApplyInference {
			spec.Inference = cfg.Inference
		}
		if cfg.ApplyMaxSteps {
			spec.MaxSteps = cfg.MaxSteps
		}
		if strings.TrimSpace(cfg.SystemOverride) != "" {
			spec.System = cfg.SystemOverride
		}
	}); err != nil {
		return nil, err
	}
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
			return nil, err
		}
		modelPolicy.SourceAPI = sourceAPI
	}

	appOpts := []app.Option{
		app.WithOutput(out),
		app.WithResourceBundle(resolved.Bundle),
		app.WithDefaultAgent(name),
		app.WithDefaultSkillSourceDiscovery(app.SkillSourceDiscovery{WorkspaceDir: workspace, IncludeGlobalUserResources: policy.IncludeGlobalUserResources}),
		app.WithAgentWorkspace(workspace),
		app.WithAgentOutput(out),
		app.WithAgentTerminalUI(true),
		app.WithAgentVerbose(cfg.Verbose),
	}
	if cfg.ToolTimeout > 0 {
		appOpts = append(appOpts, app.WithAgentToolTimeout(cfg.ToolTimeout))
	}
	if sessionsDir != "" {
		appOpts = append(appOpts, app.WithAgentSessionStoreDir(sessionsDir))
	}
	// Risk gate: log-only mode — observes all tool calls, always approves.
	riskGate := &toolmw.RiskGate{
		Assessor: toolmw.NewPolicyAssessor(),
		Approver: func(_ tool.Ctx, intent tool.Intent, detail any) (bool, error) {
			a, _ := detail.(toolmw.Assessment)
			slog.Info("risk gate",
				"tool", intent.Tool,
				"class", intent.ToolClass,
				"action", a.Decision.Action,
				"rationale", a.Decision.Rationale,
				"confidence", intent.Confidence,
			)
			return true, nil // observe only, always approve
		},
	}
	appOpts = append(appOpts, app.WithToolMiddlewares(tool.HooksMiddleware(riskGate)))
	appOpts = append(appOpts, cfg.AppOptions...)
	application, err := app.New(appOpts...)
	if err != nil {
		return nil, err
	}

	instOpts := append([]agent.Option(nil), cfg.AgentOptions...)
	if cfg.ApplySourceAPI {
		sourceAPI, err := agent.ParseSourceAPI(cfg.SourceAPI)
		if err != nil {
			return nil, err
		}
		instOpts = append(instOpts, agent.WithSourceAPI(sourceAPI))
	}
	if applyModelPolicy {
		instOpts = append(instOpts, agent.WithModelPolicy(modelPolicy))
	}
	if resumePath != "" {
		instOpts = append(instOpts, agent.WithResumeSession(resumePath))
	}
	if cfg.DebugMessage {
		instOpts = append(instOpts, agent.WithRequestObserver(debugMessageObserver(out)))
	}
	inst, err := application.InstantiateDefaultAgent(instOpts...)
	if err != nil {
		return nil, err
	}
	return &Loaded{
		App:       application,
		Agent:     inst,
		AgentName: name,
		Workspace: workspace,
		In:        in,
		Out:       out,
		Err:       errOut,
	}, nil
}

func Run(ctx context.Context, cfg Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	loaded, err := Load(ctx, cfg)
	if err != nil {
		return err
	}
	in := loaded.In
	out := loaded.Out
	errOut := loaded.Err
	application := loaded.App

	if strings.TrimSpace(cfg.Task) != "" {
		runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt)
		defer stopSignals()
		cancel := func() {}
		if cfg.TotalTimeout > 0 {
			runCtx, cancel = context.WithTimeout(runCtx, cfg.TotalTimeout)
		}
		defer cancel()
		_, err := application.Send(runCtx, cfg.Task)
		fmt.Fprintln(out)
		ui.PrintSessionUsage(out, application.SessionID(), application.Tracker().Aggregate())
		if errors.Is(err, agent.ErrMaxStepsReached) {
			fmt.Fprintf(errOut, "Warning: %v\n", err)
			return nil
		}
		return err
	}

	prompt := cfg.Prompt
	if prompt == "" || prompt == "agentsdk> " {
		prompt = fmt.Sprintf("agent(%s)> ", loaded.AgentName)
	}
	return repl.Run(ctx, application, in, repl.WithPrompt(prompt))
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
