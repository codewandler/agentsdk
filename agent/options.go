package agent

import (
	"context"
	"time"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/toolactivation"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/unified"
)

type Option func(*Instance)

// ThinkingMode controls extended thinking behavior.
// The canonical definition is in [agentconfig.ThinkingMode].
type ThinkingMode = agentconfig.ThinkingMode

const (
	ThinkingModeAuto = agentconfig.ThinkingModeAuto
	ThinkingModeOn   = agentconfig.ThinkingModeOn
	ThinkingModeOff  = agentconfig.ThinkingModeOff
)

// InferenceOptions holds model request parameters.
// The canonical definition is in [agentconfig.InferenceOptions].
type InferenceOptions = agentconfig.InferenceOptions

// DefaultInferenceOptions returns conservative defaults for a terminal agent.
func DefaultInferenceOptions() InferenceOptions {
	return agentconfig.DefaultInferenceOptions()
}

func WithInferenceOptions(opts InferenceOptions) Option {
	return func(a *Instance) { a.inference = opts }
}

func WithSpec(spec Spec) Option {
	return func(a *Instance) {
		a.spec = Spec{
			Name:             spec.Name,
			Description:      spec.Description,
			Tools:            append([]string(nil), spec.Tools...),
			Skills:           append([]string(nil), spec.Skills...),
			SkillSources:     append([]skill.Source(nil), spec.SkillSources...),
			Commands:         append([]string(nil), spec.Commands...),
			InstructionPaths: append([]string(nil), spec.InstructionPaths...),
			ResourceID:       spec.ResourceID,
			ResourceFrom:     spec.ResourceFrom,
		}
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
		if spec.AutoCompactionSet {
			a.autoCompaction = spec.AutoCompaction
		}
	}
}

func WithMaxSteps(max int) Option {
	return func(a *Instance) { a.maxSteps = max }
}

// DiagnosticHandler receives structured diagnostic messages from the agent.
// Diagnostics replace the former fmt.Fprintf(a.Out(), ...) paths for usage
// persistence errors and similar operational notices.
type DiagnosticHandler func(Diagnostic)

// Diagnostic is a structured operational message emitted by the agent.
type Diagnostic struct {
	Component string // e.g. "usage_persistence", "session"
	Message   string
	Error     error
}

// WithDiagnosticHandler installs a handler for structured diagnostic messages.
// When set, the agent publishes operational notices (usage persistence errors,
// etc.) through this handler instead of writing to an io.Writer.
func WithDiagnosticHandler(handler DiagnosticHandler) Option {
	return func(a *Instance) { a.diagnosticHandler = handler }
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

func WithResumeSession(id string) Option {
	return func(a *Instance) { a.resumeSession = id }
}

// WithThreadStore provides a pre-opened thread store so the agent does not need
// to know about JSONL paths or store backends. When set, initSession uses this
// store instead of creating an in-memory store. The caller (typically harness)
// retains ownership of the store for inspection and workflow run lookups.
func WithThreadStore(store thread.Store) Option {
	return func(a *Instance) { a.threadStore = store }
}

func WithClient(client unified.Client) Option {
	return func(a *Instance) { a.route.client = client }
}

func WithAutoMux(autoMux func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error)) Option {
	return func(a *Instance) { a.route.autoMux = autoMux }
}

func WithSourceAPI(api adapt.ApiKind) Option {
	return func(a *Instance) {
		a.route.sourceAPI = api
		a.route.sourceAPIExplicit = true
	}
}

func WithModelPolicy(policy ModelPolicy) Option {
	return func(a *Instance) { a.route.modelPolicy = policy }
}

func WithTools(tools []tool.Tool) Option {
	return func(a *Instance) { a.toolActivation = toolactivation.New(tools...) }
}

func WithEventHandlerFactory(factory func(runner.EventHandlerContext) runner.EventHandler) Option {
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

// WithBaselineProviderFactory replaces the default baseline context provider
// builder. The factory is called each time the agent assembles its context
// provider list (every turn). When not set, [DefaultBaselineProviders] is used.
func WithBaselineProviderFactory(factory BaselineProviderFactory) Option {
	return func(a *Instance) { a.baselineProviderFactory = factory }
}

// WithAutoCompaction configures automatic compaction between turns.
// When enabled, the agent checks projected token count after each turn
// and compacts if it exceeds the threshold.
func WithAutoCompaction(config AutoCompactionConfig) Option {
	return func(a *Instance) { a.autoCompaction = config }
}
