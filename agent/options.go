package agent

import (
	"context"
	"io"
	"time"

	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/standard"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/unified"
)

type Option func(*Instance)

type InferenceOption func(*InferenceOptions)

type ThinkingMode string

const (
	ThinkingModeAuto ThinkingMode = "auto"
	ThinkingModeOn   ThinkingMode = "on"
	ThinkingModeOff  ThinkingMode = "off"
)

// InferenceOptions holds model request parameters.
type InferenceOptions struct {
	Model       string
	MaxTokens   int
	Thinking    ThinkingMode
	Effort      unified.ReasoningEffort
	Temperature float64
}

// DefaultInferenceOptions returns conservative defaults for a terminal agent.
func DefaultInferenceOptions() InferenceOptions {
	return InferenceOptions{
		Model:       "codex/gpt-5.5",
		MaxTokens:   16_000,
		Thinking:    ThinkingModeAuto,
		Effort:      unified.ReasoningEffortMedium,
		Temperature: 0.1,
	}
}

func NewInferenceOptions(opts ...InferenceOption) InferenceOptions {
	cfg := DefaultInferenceOptions()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

func WithModel(model string) InferenceOption {
	return func(o *InferenceOptions) { o.Model = model }
}

func WithMaxTokens(max int) InferenceOption {
	return func(o *InferenceOptions) { o.MaxTokens = max }
}

func WithThinking(mode ThinkingMode) InferenceOption {
	return func(o *InferenceOptions) { o.Thinking = mode }
}

func WithEffort(effort unified.ReasoningEffort) InferenceOption {
	return func(o *InferenceOptions) { o.Effort = effort }
}

func WithTemperature(value float64) InferenceOption {
	return func(o *InferenceOptions) { o.Temperature = value }
}

func WithInferenceOptions(opts InferenceOptions) Option {
	return func(a *Instance) { a.inference = opts }
}

func WithSpec(spec Spec) Option {
	return func(a *Instance) {
		a.specName = spec.Name
		a.specDescription = spec.Description
		a.specTools = append([]string(nil), spec.Tools...)
		a.specSkills = append([]string(nil), spec.Skills...)
		a.specSkillSources = append([]skill.Source(nil), spec.SkillSources...)
		a.specCommands = append([]string(nil), spec.Commands...)
		a.specInstructionPaths = append([]string(nil), spec.InstructionPaths...)
		a.specResourceID = spec.ResourceID
		a.specResourceFrom = spec.ResourceFrom
		if len(spec.Capabilities) > 0 {
			a.capabilitySpecs = append([]capability.AttachSpec(nil), spec.Capabilities...)
		}
		if spec.System != "" {
			a.system = spec.System
		}
		if spec.Inference != (InferenceOptions{}) {
			a.inference = spec.Inference
		}
		if spec.MaxSteps > 0 {
			a.maxSteps = spec.MaxSteps
		}
	}
}

func WithSkillSources(sources ...skill.Source) Option {
	return func(a *Instance) { a.specSkillSources = append(a.specSkillSources, sources...) }
}

func WithSkillRepository(repo *skill.Repository) Option {
	return func(a *Instance) { a.skillRepo = repo }
}

func WithMaxSteps(max int) Option {
	return func(a *Instance) { a.maxSteps = max }
}

func WithOutput(w io.Writer) Option {
	return func(a *Instance) { a.out = w }
}

func WithTerminalUI(enabled bool) Option {
	return func(a *Instance) { a.terminalUI = enabled }
}

func WithWorkspace(dir string) Option {
	return func(a *Instance) { a.workspace = dir }
}

func WithToolTimeout(timeout time.Duration) Option {
	return func(a *Instance) { a.toolTimeout = timeout }
}

func WithSystem(prompt string) Option {
	return func(a *Instance) { a.system = prompt }
}

func WithSystemBuilder(builder func(workspace, prompt string) string) Option {
	return func(a *Instance) { a.systemBuilder = builder }
}

func WithSessionStoreDir(dir string) Option {
	return func(a *Instance) { a.sessionStoreDir = dir }
}

func WithResumeSession(path string) Option {
	return func(a *Instance) { a.resumeSession = path }
}

func WithCacheKeyPrefix(prefix string) Option {
	return func(a *Instance) { a.cacheKeyPrefix = prefix }
}

func WithVerbose(verbose bool) Option {
	return func(a *Instance) { a.verbose = verbose }
}

func WithClient(client unified.Client) Option {
	return func(a *Instance) { a.client = client }
}

func WithAutoMux(autoMux func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error)) Option {
	return func(a *Instance) { a.autoMux = autoMux }
}

func WithSourceAPI(api adapt.ApiKind) Option {
	return func(a *Instance) {
		a.sourceAPI = api
		a.sourceAPIExplicit = true
	}
}

func WithModelPolicy(policy ModelPolicy) Option {
	return func(a *Instance) { a.modelPolicy = policy }
}

func WithToolset(toolset *standard.Toolset) Option {
	return func(a *Instance) { a.toolset = toolset }
}

func WithTools(tools []tool.Tool) Option {
	return func(a *Instance) { a.toolset = standard.NewToolsetFromTools(tools...) }
}

func WithEventHandlerFactory(factory func(*Instance, int) runner.EventHandler) Option {
	return func(a *Instance) { a.eventHandlerFactory = factory }
}

func WithToolContextFactory(factory func(context.Context) tool.Ctx) Option {
	return func(a *Instance) { a.toolCtxFactory = factory }
}

// WithCapabilities configures capability instances that are attached to the
// agent's thread runtime on each turn. Each spec must have a CapabilityName
// and InstanceID. The default capability registry includes the built-in
// planner factory; use WithCapabilityRegistry to override.
func WithCapabilities(specs ...capability.AttachSpec) Option {
	return func(a *Instance) {
		a.capabilitySpecs = append(a.capabilitySpecs, specs...)
	}
}

// WithCapabilityRegistry overrides the default capability registry used to
// create capability instances. When nil, a registry containing the built-in
// planner factory is created automatically.
func WithCapabilityRegistry(registry capability.Registry) Option {
	return func(a *Instance) { a.capabilityRegistry = registry }
}
