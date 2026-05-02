package agent

import (
	"context"
	"io"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/toolactivation"
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

func WithTools(tools []tool.Tool) Option {
	return func(a *Instance) { a.toolActivation = toolactivation.New(tools...) }
}

func WithEventHandlerFactory(factory func(*Instance, int) runner.EventHandler) Option {
	return func(a *Instance) { a.eventHandlerFactory = factory }
}

// WithRequestObserver installs a hook called with each wire-level
// unified.Request right before it is dispatched to the model client.
// Useful for debug logging of every turn's outgoing payload.
func WithRequestObserver(observer runner.RequestObserver) Option {
	return func(a *Instance) { a.requestObserver = observer }
}

func WithToolContextFactory(factory func(context.Context) tool.Ctx) Option {
	return func(a *Instance) { a.toolCtxFactory = factory }
}

// WithCapabilities configures capability instances that are attached to the
// agent's thread runtime on each turn. Each spec must have a CapabilityName
// and InstanceID. Hosts must also provide a capability registry with
// WithCapabilityRegistry.
func WithCapabilities(specs ...capability.AttachSpec) Option {
	return func(a *Instance) {
		a.capabilitySpecs = append(a.capabilitySpecs, specs...)
	}
}

// WithCapabilityRegistry configures the registry used to create capability
// instances. It is required when capabilities are configured.
func WithCapabilityRegistry(registry capability.Registry) Option {
	return func(a *Instance) { a.capabilityRegistry = registry }
}

// WithContextProviders adds extra context providers that are registered on the
// agent's context manager alongside the baseline providers. Plugin-contributed
// providers flow through this option. Provider keys must not collide with the
// agent's built-in provider keys; if a plugin provider has the same key as a
// built-in, the built-in is skipped in favor of the plugin provider.
func WithContextProviders(providers ...agentcontext.Provider) Option {
	return func(a *Instance) {
		a.extraContextProviders = append(a.extraContextProviders, providers...)
	}
}

// ContextProviderFactoryInfo carries per-agent state available when a
// [ContextProviderFactory] is called during agent instantiation.
//
// This struct mirrors [app.AgentContextInfo]. When adding fields here,
// update AgentContextInfo and the bridge in [app.App.InstantiateAgent].
type ContextProviderFactoryInfo struct {
	SkillRepository *skill.Repository
	SkillState      *skill.ActivationState
	ActiveTools     func() []tool.Tool
	Workspace       string
	Model           string
	Effort          string
}

// ContextProviderFactory creates context providers that depend on per-agent
// runtime state. Factories are called once during [New], after skill and tool
// initialization, so the info struct is fully populated.
type ContextProviderFactory func(ContextProviderFactoryInfo) []agentcontext.Provider

// WithContextProviderFactories adds factories that produce context providers
// from per-agent runtime state. The factories are called during [New] after
// skill initialization. The resulting providers are appended to
// extraContextProviders and participate in the same key-set dedup as
// [WithContextProviders].
func WithContextProviderFactories(factories ...ContextProviderFactory) Option {
	return func(a *Instance) {
		a.contextProviderFactories = append(a.contextProviderFactories, factories...)
	}
}

// WithAutoCompaction configures automatic compaction between turns.
// When enabled, the agent checks projected token count after each turn
// and compacts if it exceeds the threshold.
func WithAutoCompaction(config AutoCompactionConfig) Option {
	return func(a *Instance) { a.autoCompaction = config }
}
