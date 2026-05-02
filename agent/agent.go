package agent

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/thread"
	threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/toolactivation"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/compatibility"
	"github.com/codewandler/llmadapter/unified"
)

var ErrMaxStepsReached = errors.New("maximum steps reached")

const EventUsageRecorded thread.EventKind = "harness.usage_recorded"

// Spec describes an agent identity/configuration independent of a running
// conversation session.
type Spec struct {
	Name             string
	Description      string
	System           string
	Inference        InferenceOptions
	MaxSteps         int
	Tools            []string
	Skills           []string
	SkillSources     []skill.Source
	Commands         []string
	InstructionPaths []string
	ResourceID       string
	ResourceFrom     string
	Capabilities     []capability.AttachSpec
}

// Instance is a running session-backed agent built from a Spec and runtime
// options.
type Instance struct {
	client                   unified.Client
	autoMux                  func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error)
	autoResult               adapterconfig.AutoResult
	providerIdentity         conversation.ProviderIdentity
	resolvedProvider         string
	resolvedModel            string
	sourceAPI                adapt.ApiKind
	sourceAPIExplicit        bool
	modelPolicy              ModelPolicy
	modelCompatibility       modelCompatibilityState
	runtime                  *agentruntime.Engine
	tracker                  *usage.Tracker
	toolActivation           *toolactivation.Manager
	inference                InferenceOptions
	maxSteps                 int
	out                      io.Writer
	workspace                string
	toolTimeout              time.Duration
	system                   string
	systemBuilder            func(workspace, prompt string) string
	sessionID                string
	history                  *agentruntime.History
	sessionStoreDir          string
	resumeSession            string
	sessionStorePath         string
	verbose                  bool
	initErrs                 []error
	eventHandlerFactory      func(*Instance, int) runner.EventHandler
	requestObserver          runner.RequestObserver
	toolCtxFactory           func(context.Context) tool.Ctx
	specName                 string
	specDescription          string
	specTools                []string
	specSkills               []string
	specSkillSources         []skill.Source
	specCommands             []string
	specInstructionPaths     []string
	specResourceID           string
	specResourceFrom         string
	skillRepo                *skill.Repository
	skillState               *skill.ActivationState
	materializedSystem       string
	capabilitySpecs          []capability.AttachSpec
	capabilityRegistry       capability.Registry
	threadRuntime            *agentruntime.ThreadRuntime
	extraContextProviders    []agentcontext.Provider
	contextProviderFactories []ContextProviderFactory
	contextWindow            int
	autoCompaction           AutoCompactionConfig
}

func New(opts ...Option) (*Instance, error) {
	sessionID, err := newSessionID()
	if err != nil {
		return nil, err
	}
	a := &Instance{
		inference:     DefaultInferenceOptions(),
		maxSteps:      30,
		out:           io.Discard,
		toolTimeout:   30 * time.Second,
		sessionID:     sessionID,
		sourceAPI:     adapt.ApiOpenAIResponses,
		systemBuilder: func(_ string, prompt string) string { return prompt },
	}
	for _, opt := range opts {
		if opt != nil {
			opt(a)
		}
	}
	if len(a.initErrs) > 0 {
		return nil, errors.Join(a.initErrs...)
	}
	if a.workspace == "" {
		a.workspace, _ = os.Getwd()
	}
	if abs, err := filepath.Abs(a.workspace); err == nil {
		a.workspace = abs
	}
	if a.toolActivation == nil {
		a.toolActivation = toolactivation.New()
	}
	a.applySpecTools()
	if a.tracker == nil {
		a.tracker = usage.NewTracker()
	}
	if err := a.initSkills(); err != nil {
		return nil, err
	}
	a.runContextProviderFactories()
	if err := a.initRuntime(); err != nil {
		return nil, err
	}
	return a, nil
}

func NewInstance(opts ...Option) (*Instance, error) {
	return New(opts...)
}

func Must(opts ...Option) *Instance {
	a, err := New(opts...)
	if err != nil {
		panic(err)
	}
	return a
}

func (a *Instance) SessionID() string {
	if a == nil {
		return ""
	}
	return a.sessionID
}

func (a *Instance) SessionStorePath() string {
	if a == nil {
		return ""
	}
	return a.sessionStorePath
}

func (a *Instance) LiveThread() thread.Live {
	if a == nil {
		return nil
	}
	if a.threadRuntime != nil && a.threadRuntime.Live() != nil {
		return a.threadRuntime.Live()
	}
	if a.history != nil {
		return a.history.LiveThread()
	}
	if a.runtime != nil && a.runtime.History() != nil {
		return a.runtime.History().LiveThread()
	}
	return nil
}

func (a *Instance) Tracker() *usage.Tracker {
	if a == nil {
		return nil
	}
	return a.tracker
}

func (a *Instance) Out() io.Writer {
	if a == nil || a.out == nil {
		return io.Discard
	}
	return a.out
}

func (a *Instance) ParamsSummary() string {
	if a == nil {
		return ""
	}
	compatibility := a.modelCompatibilitySummary()
	if a.resolvedProvider != "" || a.resolvedModel != "" {
		return strings.TrimSpace(fmt.Sprintf("model: %s  resolved_instance: %s  resolved_model: %s  thinking: %s  effort: %s%s", a.inference.Model, a.resolvedProvider, a.resolvedModel, a.inference.Thinking, a.inference.Effort, compatibility))
	}
	return strings.TrimSpace(fmt.Sprintf("model: %s  thinking: %s  effort: %s%s", a.inference.Model, a.inference.Thinking, a.inference.Effort, compatibility))
}

func (a *Instance) Spec() Spec {
	if a == nil {
		return Spec{}
	}
	return Spec{
		Name:             a.specName,
		Description:      a.specDescription,
		System:           a.system,
		Inference:        a.inference,
		MaxSteps:         a.maxSteps,
		Tools:            append([]string(nil), a.specTools...),
		Skills:           append([]string(nil), a.specSkills...),
		SkillSources:     append([]skill.Source(nil), a.specSkillSources...),
		Commands:         append([]string(nil), a.specCommands...),
		InstructionPaths: append([]string(nil), a.specInstructionPaths...),
		ResourceID:       a.specResourceID,
		ResourceFrom:     a.specResourceFrom,
		Capabilities:     append([]capability.AttachSpec(nil), a.capabilitySpecs...),
	}
}

func (a *Instance) SkillRepository() *skill.Repository {
	if a == nil {
		return nil
	}
	return a.skillRepo
}

func (a *Instance) LoadedSkills() []skill.Skill {
	if a == nil {
		return nil
	}
	if a.skillState != nil {
		return a.skillState.ActiveSkills()
	}
	if a.skillRepo == nil {
		return nil
	}
	return a.skillRepo.Loaded()
}

func (a *Instance) MaterializedSystem() string {
	if a == nil {
		return ""
	}
	if a.materializedSystem != "" {
		return a.materializedSystem
	}
	return a.systemBuilder(a.workspace, a.system)
}

func (a *Instance) activeTools() []tool.Tool {
	if a == nil || a.toolActivation == nil {
		return nil
	}
	return a.toolActivation.ActiveTools()
}

// RegisterTools adds tools to the running agent for future turns.
// Existing tool names are left unchanged so repeated registration is idempotent.
func (a *Instance) RegisterTools(tools ...tool.Tool) error {
	if a == nil {
		return fmt.Errorf("agent: instance is nil")
	}
	if len(tools) == 0 {
		return nil
	}
	if a.toolActivation == nil {
		a.toolActivation = toolactivation.New()
	}
	if err := a.toolActivation.Register(tools...); err != nil {
		return err
	}
	return nil
}

// RegisterContextProviders adds context providers to the running agent for
// future turns. Provider keys already present on the agent are skipped so
// repeated registration is idempotent.
func (a *Instance) RegisterContextProviders(providers ...agentcontext.Provider) error {
	if a == nil {
		return fmt.Errorf("agent: instance is nil")
	}
	if len(providers) == 0 {
		return nil
	}
	seen := make(map[agentcontext.ProviderKey]bool)
	for _, provider := range a.contextProviders() {
		if provider != nil {
			seen[provider.Key()] = true
		}
	}
	newProviders := make([]agentcontext.Provider, 0, len(providers))
	for _, provider := range providers {
		if provider == nil {
			return fmt.Errorf("agent: context provider is nil")
		}
		key := provider.Key()
		if key == "" {
			return fmt.Errorf("agent: context provider key is required")
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		newProviders = append(newProviders, provider)
	}
	if len(newProviders) == 0 {
		return nil
	}
	a.extraContextProviders = append(a.extraContextProviders, newProviders...)
	if a.runtime != nil {
		return a.runtime.RegisterContextProviders(newProviders...)
	}
	return nil
}

func (a *Instance) applySpecTools() {
	if a == nil || a.toolActivation == nil || len(a.specTools) == 0 {
		return
	}
	a.toolActivation.Deactivate("*")
	a.toolActivation.Activate(a.specTools...)
}

func (a *Instance) initSkills() error {
	if a.skillRepo == nil {
		repo, err := skill.NewRepository(a.specSkillSources, a.specSkills)
		if err != nil {
			if len(a.specSkillSources) > 0 || len(a.specSkills) == 0 {
				return err
			}
			repo, err = skill.NewRepository(a.specSkillSources, nil)
			if err != nil {
				return err
			}
		}
		a.skillRepo = repo
	} else {
		for _, name := range a.specSkills {
			if err := a.skillRepo.Load(name); err != nil && len(a.specSkillSources) > 0 {
				return err
			}
		}
	}
	state, err := skill.NewActivationState(a.skillRepo, a.skillRepo.LoadedNames())
	if err != nil {
		return err
	}
	a.skillState = state
	a.refreshMaterializedSystem()
	return nil
}

func (a *Instance) refreshMaterializedSystem() {
	if a == nil {
		return
	}
	base := a.systemBuilder(a.workspace, a.system)
	skills := ""
	if a.skillState != nil {
		skills = a.skillState.Materialize()
	} else if a.skillRepo != nil {
		skills = a.skillRepo.Materialize()
	}
	if strings.TrimSpace(skills) == "" {
		a.materializedSystem = base
		return
	}
	if strings.TrimSpace(base) == "" {
		a.materializedSystem = skills
		return
	}
	a.materializedSystem = strings.TrimRight(base, "\n") + "\n\n" + skills
}

func (a *Instance) ActivateSkill(name string) (skill.Status, error) {
	if a == nil || a.skillState == nil {
		return skill.StatusInactive, fmt.Errorf("agent: skill activation is not initialized")
	}
	before := a.skillState.Status(name)
	status, err := a.skillState.ActivateSkill(name)
	if err != nil {
		return skill.StatusInactive, err
	}
	if before == skill.StatusInactive {
		if err := a.appendSkillEvent(skill.EventSkillActivated, skill.SkillActivatedEvent{Skill: strings.TrimSpace(name)}); err != nil {
			return skill.StatusInactive, err
		}
	}
	a.refreshMaterializedSystem()
	return status, nil
}

func (a *Instance) ActivateSkillReferences(name string, refs []string) ([]string, error) {
	if a == nil || a.skillState == nil {
		return nil, fmt.Errorf("agent: skill activation is not initialized")
	}
	activated, err := a.skillState.ActivateReferences(name, refs)
	if err != nil {
		return nil, err
	}
	for _, ref := range activated {
		if err := a.appendSkillEvent(skill.EventSkillReferenceActivated, skill.SkillReferenceActivatedEvent{Skill: strings.TrimSpace(name), Path: ref}); err != nil {
			return nil, err
		}
	}
	a.refreshMaterializedSystem()
	return activated, nil
}

func (a *Instance) SkillActivationState() *skill.ActivationState {
	if a == nil {
		return nil
	}
	return a.skillState
}

func (a *Instance) appendSkillEvent(kind thread.EventKind, payload any) error {
	if a == nil {
		return nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	event := thread.Event{Kind: kind, Payload: raw}
	if a.threadRuntime != nil && a.threadRuntime.Live() != nil {
		return a.threadRuntime.Live().Append(context.Background(), event)
	}
	if a.history != nil {
		return a.history.AppendThreadEvents(context.Background(), event)
	}
	return nil
}

func (a *Instance) Reset() {
	if a == nil {
		return
	}
	a.tracker.Reset()
	a.threadRuntime = nil
	sessionID, err := newSessionID()
	if err == nil {
		a.sessionID = sessionID
	}
	if a.sessionStoreDir != "" {
		if err := a.startPersistentSession(time.Now()); err == nil {
			if runtimeAgent, err := agentruntime.New(a.client, a.runtimeOptions()...); err == nil {
				a.runtime = runtimeAgent
				return
			}
		}
	} else if len(a.capabilitySpecs) > 0 {
		if err := a.startEphemeralCapabilitySession(context.Background()); err == nil {
			if runtimeAgent, err := agentruntime.New(a.client, a.runtimeOptions()...); err == nil {
				a.runtime = runtimeAgent
				return
			}
		}
	}
	if a.runtime != nil {
		a.runtime.ResetHistory(agentruntime.WithHistorySessionID(a.sessionID))
	}
}

func (a *Instance) RunTurn(ctx context.Context, turnID int, task string) error {
	if a == nil || a.runtime == nil {
		return fmt.Errorf("agent: runtime is not initialized")
	}
	handler := a.newEventHandler(turnID)
	_, err := a.runtime.RunTurn(
		ctx,
		task,
		agentruntime.WithTurnMaxSteps(a.maxSteps),
		agentruntime.WithTurnTools(a.activeTools()),
		agentruntime.WithTurnProviderIdentity(a.providerIdentity),
		agentruntime.WithTurnEventHandler(handler),
	)
	if errors.Is(err, runner.ErrMaxStepsReached) {
		return ErrMaxStepsReached
	}
	if err != nil {
		return fmt.Errorf("provider=%s model=%s: %w", a.resolvedProvider, a.resolvedModel, err)
	}
	a.maybeAutoCompact(ctx)
	return nil
}

func (a *Instance) ContextState() string {
	if a == nil || a.runtime == nil {
		return "context: unavailable"
	}
	return a.runtime.ContextState()
}

func (a *Instance) initRuntime() error {
	if a.client == nil {
		result, err := agentruntime.AutoMuxClient(a.inference.Model, a.autoMuxSourceAPI(), a.autoMux)
		if err != nil {
			return err
		}
		a.client = result.Client
		a.autoResult = result
		if err := a.applyModelPolicy(); err != nil {
			return err
		}
	} else if a.modelPolicy.Configured() {
		if err := a.applyModelPolicyWithoutAutoResult(); err != nil {
			return err
		}
	}
	a.resolveRouteIdentity()
	if err := a.initSession(context.Background()); err != nil {
		return err
	}
	runtimeAgent, err := agentruntime.New(a.client, a.runtimeOptions()...)
	if err != nil {
		return err
	}
	a.runtime = runtimeAgent
	return nil
}

func (a *Instance) autoMuxSourceAPI() adapt.ApiKind {
	if a == nil {
		return adapt.ApiOpenAIResponses
	}
	if a.modelPolicy.Configured() {
		if a.modelPolicy.SourceAPI != "" {
			return a.modelPolicy.SourceAPI
		}
		if !a.sourceAPIExplicit {
			return ""
		}
	}
	return a.sourceAPI
}

func (a *Instance) policySourceAPI() adapt.ApiKind {
	if a == nil {
		return ""
	}
	if a.modelPolicy.SourceAPI != "" {
		return a.modelPolicy.SourceAPI
	}
	if a.sourceAPIExplicit {
		return a.sourceAPI
	}
	if a.modelPolicy.Configured() {
		return ""
	}
	return a.sourceAPI
}

func (a *Instance) applyModelPolicyWithoutAutoResult() error {
	useCase, err := a.modelPolicy.llmUseCase()
	if err != nil {
		return err
	}
	if useCase == "" {
		return nil
	}
	if a.modelPolicy.ApprovedOnly {
		return fmt.Errorf("agent: approved-only model policy requires auto mux routing")
	}
	a.modelCompatibility = modelCompatibilityState{
		UseCase:    useCase,
		Status:     compatibility.StatusUnavailable,
		Diagnostic: "custom client has no llmadapter route config",
	}
	return nil
}

func (a *Instance) applyModelPolicy() error {
	if a == nil || !a.modelPolicy.Configured() {
		return nil
	}
	useCase, err := a.modelPolicy.llmUseCase()
	if err != nil {
		return err
	}
	if useCase == "" {
		return nil
	}
	sourceAPI := a.policySourceAPI()
	if a.modelPolicy.ApprovedOnly {
		return a.applyApprovedOnlyModelPolicy(useCase, sourceAPI)
	}
	return a.applyEvaluationModelPolicy(useCase, sourceAPI)
}

func (a *Instance) applyApprovedOnlyModelPolicy(useCase compatibility.UseCase, sourceAPI adapt.ApiKind) error {
	evidence, evidenceSource, err := LoadCompatibilityEvidence(a.modelPolicy)
	if err != nil {
		return err
	}
	selection, err := selectModelForPolicy(a.autoResult, a.inference.Model, sourceAPI, adapterconfig.UseCaseSelectionOptions{
		UseCase:       useCase,
		Evidence:      evidence,
		AllowDegraded: a.modelPolicy.AllowDegraded,
		AllowUntested: a.modelPolicy.AllowUntested,
	})
	if err != nil {
		return err
	}
	pinnedConfig, err := pinnedConfigForSelection(a.autoResult.Config, selection, a.inference.Model)
	if err != nil {
		return err
	}
	client, err := adapterconfig.NewMuxClient(pinnedConfig, adapterconfig.WithSourceAPI(selection.Resolution.SourceAPI), adapterconfig.WithFallback(false))
	if err != nil {
		return err
	}
	a.client = client
	a.autoResult.Config = pinnedConfig
	a.autoResult.Client = client
	a.sourceAPI = selection.Resolution.SourceAPI
	a.sourceAPIExplicit = true
	a.modelCompatibility = modelCompatibilityFromEvaluation(selection.Evaluation, evidenceSource, true)
	a.modelCompatibility.SourceAPI = selection.Resolution.SourceAPI
	a.modelCompatibility.ProviderAPI = selection.Resolution.ProviderAPI
	return nil
}

func (a *Instance) applyEvaluationModelPolicy(useCase compatibility.UseCase, sourceAPI adapt.ApiKind) error {
	evidenceDiagnostic := ""
	if evidence, evidenceSource, err := LoadCompatibilityEvidence(a.modelPolicy); err == nil {
		selection, err := selectModelForPolicy(a.autoResult, a.inference.Model, sourceAPI, adapterconfig.UseCaseSelectionOptions{
			UseCase:       useCase,
			Evidence:      evidence,
			AllowDegraded: true,
			AllowUntested: true,
		})
		if err == nil {
			a.modelCompatibility = modelCompatibilityFromEvaluation(selection.Evaluation, evidenceSource, false)
			a.modelCompatibility.SourceAPI = selection.Resolution.SourceAPI
			a.modelCompatibility.ProviderAPI = selection.Resolution.ProviderAPI
			return nil
		}
	} else {
		evidenceDiagnostic = err.Error()
	}
	evaluations, err := adapterconfig.EvaluateCompatibilityCandidates(a.autoResult.Config, a.inference.Model, sourceAPI, useCase)
	if err != nil {
		a.modelCompatibility = modelCompatibilityState{
			UseCase:    useCase,
			Status:     compatibility.StatusUnavailable,
			Diagnostic: err.Error(),
		}
		return nil
	}
	if len(evaluations) == 0 {
		a.modelCompatibility = modelCompatibilityState{
			UseCase:    useCase,
			Status:     compatibility.StatusUnavailable,
			Diagnostic: "no compatibility candidates",
		}
		return nil
	}
	a.modelCompatibility = modelCompatibilityFromEvaluation(evaluations[0], "", false)
	a.modelCompatibility.Diagnostic = evidenceDiagnostic
	return nil
}

func selectModelForPolicy(result adapterconfig.AutoResult, model string, sourceAPI adapt.ApiKind, opts adapterconfig.UseCaseSelectionOptions) (adapterconfig.UseCaseModelSelection, error) {
	var lastErr error
	for _, candidate := range modelPolicyLookupNames(model) {
		selection, err := result.SelectModelForUseCase(candidate, sourceAPI, opts)
		if err == nil {
			return selection, nil
		}
		lastErr = err
	}
	return adapterconfig.UseCaseModelSelection{}, lastErr
}

func (a *Instance) modelCompatibilitySummary() string {
	if a == nil || !a.modelCompatibility.configured() {
		return ""
	}
	state := a.modelCompatibility
	parts := []string{}
	if state.SourceAPI != "" {
		parts = append(parts, "source_api: "+string(state.SourceAPI))
	}
	if state.ProviderAPI != "" {
		parts = append(parts, "provider_api: "+string(state.ProviderAPI))
	}
	if state.UseCase != "" {
		parts = append(parts, "use_case: "+string(state.UseCase))
	}
	if state.Status != "" {
		parts = append(parts, "compatibility: "+string(state.Status))
	}
	if missing := featureNames(state.MissingRequired); missing != "" {
		parts = append(parts, "missing_required: "+missing)
	}
	if untested := featureNames(state.UntestedRequired); untested != "" {
		parts = append(parts, "untested_required: "+untested)
	}
	if degraded := featureNames(state.DegradedPreferred); degraded != "" {
		parts = append(parts, "degraded_preferred: "+degraded)
	}
	if state.Diagnostic != "" {
		parts = append(parts, "reason: "+state.Diagnostic)
	}
	if len(parts) == 0 {
		return ""
	}
	return "  " + strings.Join(parts, "  ")
}

func (a *Instance) runtimeOptions() []agentruntime.Option {
	opts := a.baseRuntimeOptions(true)
	if a.history != nil {
		opts = append(opts, agentruntime.WithHistory(a.history))
	}
	if a.threadRuntime != nil {
		opts = append(opts, agentruntime.WithThreadRuntime(a.threadRuntime))
	}
	if len(a.capabilitySpecs) > 0 {
		opts = append(opts, agentruntime.WithCapabilities(a.capabilitySpecs...))
	}
	return opts
}

func (a *Instance) baseRuntimeOptions(includeSessionID bool) []agentruntime.Option {
	opts := []agentruntime.Option{
		agentruntime.WithModel(a.inference.Model),
		agentruntime.WithMaxOutputTokens(a.inference.MaxTokens),
		agentruntime.WithTemperature(a.inference.Temperature),
		agentruntime.WithSystem(a.MaterializedSystem()),
		agentruntime.WithTools(a.activeTools()),
		agentruntime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
		agentruntime.WithCachePolicy(unified.CachePolicyOn),
		agentruntime.WithMaxSteps(a.maxSteps),
		agentruntime.WithToolTimeout(a.toolTimeout),
		agentruntime.WithProviderIdentity(a.providerIdentity),
		agentruntime.WithToolContextFactory(func(ctx context.Context) tool.Ctx {
			if a.toolCtxFactory != nil {
				return a.toolCtxFactory(ctx)
			}
			return agentruntime.NewToolContext(ctx,
				agentruntime.WithToolWorkDir(a.workspace),
				agentruntime.WithToolSessionID(a.sessionID),
				agentruntime.WithToolActivation(a.toolActivation),
				agentruntime.WithToolSkillActivation(a.skillState),
			)
		}),
	}
	// When a ThreadRuntime is present, context providers are registered on
	// its context manager directly (see initThreadRuntime). Passing them
	// again via engine options would cause a duplicate-provider error.
	if a.threadRuntime == nil {
		opts = append(opts, agentruntime.WithContextProviders(a.contextProviders()...))
	}
	if includeSessionID {
		opts = append([]agentruntime.Option{agentruntime.WithHistoryOptions(agentruntime.WithHistorySessionID(a.sessionID))}, opts...)
	}
	if reasoning, ok := a.reasoningConfig(); ok {
		opts = append(opts, agentruntime.WithReasoning(reasoning))
	}
	if a.requestObserver != nil {
		opts = append(opts, agentruntime.WithRequestObserver(a.requestObserver))
	}
	return opts
}

func (a *Instance) resolveRouteIdentity() {
	a.providerIdentity = conversation.ProviderIdentity{}
	a.resolvedProvider = ""
	a.resolvedModel = ""
	identity, summary, ok := agentruntime.RouteIdentity(a.autoResult, a.sourceAPI, a.inference.Model)
	if !ok {
		return
	}
	a.resolvedProvider = summary.Provider
	a.resolvedModel = summary.NativeModel
	a.contextWindow = summary.ContextWindow
	a.providerIdentity = identity
}

func (a *Instance) initSession(ctx context.Context) error {
	hasCapabilities := len(a.capabilitySpecs) > 0
	if a.resumeSession == "" && a.sessionStoreDir == "" && !hasCapabilities {
		return nil
	}
	if a.resumeSession != "" {
		dir, id := splitThreadSessionRef(a.resumeSession)
		if dir == "." && a.sessionStoreDir != "" && filepath.Ext(strings.TrimSpace(a.resumeSession)) == "" {
			dir = a.sessionStoreDir
		}
		store := threadjsonlstore.Open(dir)
		source := thread.EventSource{Type: "session", SessionID: id}
		if hasCapabilities {
			registry, err := a.ensureCapabilityRegistry()
			if err != nil {
				return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
			}
			tr, stored, err := agentruntime.ResumeThreadRuntime(ctx, store, thread.ResumeParams{
				ID:     thread.ID(id),
				Source: source,
			}, registry, agentruntime.WithThreadRuntimeSource(source))
			if err != nil {
				return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
			}
			if err := tr.ContextManager().Register(a.contextProviders()...); err != nil {
				return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
			}
			a.threadRuntime = tr
			events, err := stored.EventsForBranch(tr.Live().BranchID())
			if err != nil {
				return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
			}
			_ = events // capability replay already handled by ResumeThreadRuntime
			if err := a.replaySkillEvents(events); err != nil {
				return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
			}
			a.replayUsageEvents(events)
			history, err := agentruntime.ResumeHistoryFromThread(ctx, store, tr.Live(), append(a.historyOptions(false), agentruntime.WithHistorySessionID(id))...)
			if err != nil {
				return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
			}
			a.history = history
		} else {
			live, err := store.Resume(ctx, thread.ResumeParams{
				ID:     thread.ID(id),
				Source: source,
			})
			if err != nil {
				return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
			}
			history, err := agentruntime.ResumeHistoryFromThread(ctx, store, live, append(a.historyOptions(false), agentruntime.WithHistorySessionID(id))...)
			if err != nil {
				return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
			}
			a.history = history
			stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
			if err == nil {
				if branchEvents, err := stored.EventsForBranch(live.BranchID()); err == nil {
					_ = a.replaySkillEvents(branchEvents)
					a.replayUsageEvents(branchEvents)
				}
			}
		}
		a.sessionID = id
		a.sessionStoreDir = dir
		a.sessionStorePath = filepath.Join(dir, id+".jsonl")
		return nil
	}
	if a.sessionStoreDir != "" {
		return a.startPersistentSession(time.Now())
	}
	// Capabilities without a persistent session: create an in-memory thread
	// so the ThreadRuntime has a live thread to append capability events to.
	if hasCapabilities {
		return a.startEphemeralCapabilitySession(ctx)
	}
	return nil
}

func (a *Instance) startPersistentSession(now time.Time) error {
	if a.sessionStoreDir == "" {
		a.history = nil
		a.sessionStorePath = ""
		a.threadRuntime = nil
		return nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	store := threadjsonlstore.Open(a.sessionStoreDir)
	live, err := store.Create(context.Background(), thread.CreateParams{
		ID:     thread.ID(a.sessionID),
		Now:    now,
		Source: thread.EventSource{Type: "session", SessionID: a.sessionID},
	})
	if err != nil {
		return err
	}
	if len(a.capabilitySpecs) > 0 {
		registry, err := a.ensureCapabilityRegistry()
		if err != nil {
			return err
		}
		if err := a.initThreadRuntime(live, registry); err != nil {
			return err
		}
	}
	a.history = agentruntime.NewHistory(append(a.historyOptions(true), agentruntime.WithHistoryLiveThread(live))...)
	a.sessionStorePath = filepath.Join(a.sessionStoreDir, a.sessionID+".jsonl")
	return nil
}

// startEphemeralCapabilitySession creates an in-memory thread and ThreadRuntime
// for agents that use capabilities without a persistent session store.
func (a *Instance) startEphemeralCapabilitySession(ctx context.Context) error {
	store := thread.NewMemoryStore()
	live, err := store.Create(ctx, thread.CreateParams{
		ID:     thread.ID(a.sessionID),
		Source: thread.EventSource{Type: "session", SessionID: a.sessionID},
	})
	if err != nil {
		return err
	}
	registry, err := a.ensureCapabilityRegistry()
	if err != nil {
		return err
	}
	return a.initThreadRuntime(live, registry)
}

func (a *Instance) historyOptions(includeSessionID bool) []agentruntime.HistoryOption {
	return agentruntime.HistoryOptions(a.baseRuntimeOptions(includeSessionID)...)
}

// ensureCapabilityRegistry returns the configured registry. Capability factories
// are host/plugin-owned; agent construction does not install hidden defaults.
func (a *Instance) ensureCapabilityRegistry() (capability.Registry, error) {
	if a.capabilityRegistry != nil {
		return a.capabilityRegistry, nil
	}
	return nil, fmt.Errorf("agent: capability registry is required when capabilities are configured")
}

func (a *Instance) replaySkillEvents(events []thread.Event) error {
	if a == nil || a.skillState == nil {
		return nil
	}
	for _, event := range events {
		switch event.Kind {
		case skill.EventSkillActivated:
			var payload skill.SkillActivatedEvent
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			if _, err := a.skillState.ActivateSkill(payload.Skill); err != nil {
				a.skillState.AddDiagnostic("replay skipped skill %q: %v", payload.Skill, err)
				continue
			}
		case skill.EventSkillReferenceActivated:
			var payload skill.SkillReferenceActivatedEvent
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			if _, err := a.skillState.ActivateReferences(payload.Skill, []string{payload.Path}); err != nil {
				a.skillState.AddDiagnostic("replay skipped reference %q for skill %q: %v", payload.Path, payload.Skill, err)
				continue
			}
		}
	}
	a.refreshMaterializedSystem()
	return nil
}

// runContextProviderFactories calls each registered factory with the current
// agent state and appends the resulting providers to extraContextProviders.
// This runs once during New, after initSkills, so skill repo/state and tool
// activation are available.
func (a *Instance) runContextProviderFactories() {
	if len(a.contextProviderFactories) == 0 {
		return
	}
	info := ContextProviderFactoryInfo{
		SkillRepository: a.skillRepo,
		SkillState:      a.skillState,
		Workspace:       a.workspace,
		Model:           a.inference.Model,
		Effort:          string(a.inference.Effort),
	}
	if a.toolActivation != nil {
		info.ActiveTools = a.toolActivation.ActiveTools
	}
	for _, factory := range a.contextProviderFactories {
		if factory != nil {
			a.extraContextProviders = append(a.extraContextProviders, factory(info)...)
		}
	}
}

// contextProviders returns the context providers for this agent instance.
// These are registered on the context manager (thread-backed or standalone)
// so the model sees current environment, time, model identity, active tools,
// loaded skills, and any plugin-contributed context as diffable fragments.
//
// Plugin-contributed providers (extraContextProviders) are appended after the
// baseline set. If a plugin provider has the same key as a built-in, the
// built-in is skipped so the plugin can replace it.
func (a *Instance) contextProviders() []agentcontext.Provider {
	// Build a key set from extra (plugin) providers so built-ins with
	// colliding keys are skipped in favor of the plugin contribution.
	extraKeys := make(map[agentcontext.ProviderKey]bool, len(a.extraContextProviders))
	for _, p := range a.extraContextProviders {
		if p != nil {
			extraKeys[p.Key()] = true
		}
	}

	// addIfNotOverridden appends a provider only when no plugin already
	// contributes the same key.
	var providers []agentcontext.Provider
	addIfNotOverridden := func(p agentcontext.Provider) {
		if !extraKeys[p.Key()] {
			providers = append(providers, p)
		}
	}

	addIfNotOverridden(contextproviders.Environment(contextproviders.WithWorkDir(a.workspace)))
	addIfNotOverridden(contextproviders.Time(time.Minute))
	addIfNotOverridden(contextproviders.Model(contextproviders.ModelInfo{
		Name:          a.resolvedModel,
		Provider:      a.resolvedProvider,
		ContextWindow: a.contextWindow,
		Effort:        string(a.inference.Effort),
	}))
	if a.toolActivation != nil {
		addIfNotOverridden(contextproviders.Tools(a.toolActivation.ActiveTools()...))
	}
	if a.skillRepo != nil || a.skillState != nil {
		addIfNotOverridden(contextproviders.SkillInventoryProvider(contextproviders.SkillInventory{
			Catalog: a.skillRepo,
			State:   a.skillState,
		}))
	}
	if len(a.specInstructionPaths) > 0 {
		addIfNotOverridden(contextproviders.AgentsMarkdown(a.specInstructionPaths, contextproviders.AgentsMarkdownOption(contextproviders.WithFileWorkDir(a.workspace))))
	}

	// Append plugin-contributed providers after the baseline set.
	for _, p := range a.extraContextProviders {
		if p != nil {
			providers = append(providers, p)
		}
	}
	return providers
}

// initThreadRuntime creates a ThreadRuntime backed by the given live thread and
// store. It registers the agent's context providers on the thread runtime's
// context manager.
func (a *Instance) initThreadRuntime(live thread.Live, registry capability.Registry) error {
	source := thread.EventSource{Type: "session", SessionID: a.sessionID}
	tr, err := agentruntime.NewThreadRuntime(live, registry,
		agentruntime.WithThreadRuntimeSource(source),
	)
	if err != nil {
		return err
	}
	// Register context providers on the thread runtime's context manager so
	// they survive replay and are available for manager-owned diffs.
	if err := tr.ContextManager().Register(a.contextProviders()...); err != nil {
		return err
	}
	a.threadRuntime = tr
	return nil
}

func (a *Instance) reasoningConfig() (unified.ReasoningConfig, bool) {
	switch a.inference.Thinking {
	case ThinkingModeOff, ThinkingModeAuto, "":
		return unified.ReasoningConfig{}, false
	default:
		return unified.ReasoningConfig{Effort: a.inference.Effort, Expose: true}, true
	}
}

func (a *Instance) newEventHandler(turnID int) runner.EventHandler {
	extra := runner.EventHandler(nil)
	if a.eventHandlerFactory != nil {
		extra = a.eventHandlerFactory(a, turnID)
	}
	return func(event runner.Event) {
		a.recordEvent(turnID, event)
		if extra != nil {
			extra(event)
		}
	}
}

func (a *Instance) recordEvent(turnID int, event runner.Event) {
	switch ev := event.(type) {
	case runner.RouteEvent:
		a.providerIdentity = ev.ProviderIdentity
		a.resolvedProvider = ev.ProviderIdentity.ProviderName
		a.resolvedModel = ev.ProviderIdentity.NativeModel
	case runner.UsageEvent:
		record := usage.FromRunnerEvent(ev, usage.RunnerEventOptions{
			TurnID:        strconv.Itoa(turnID),
			SessionID:     a.sessionID,
			FallbackModel: a.inference.Model,
			RouteState: usage.RouteState{
				Provider: a.resolvedProvider,
				Model:    a.resolvedModel,
			},
		})
		a.tracker.Record(record)
		a.persistUsageEvent(record)
	}
}

// persistUsageEvent appends a usage record to the thread event log so it
// survives session resume.
func (a *Instance) persistUsageEvent(record usage.Record) {
	if a.threadRuntime == nil || a.threadRuntime.Live() == nil {
		return
	}
	raw, err := json.Marshal(record)
	if err != nil {
		if a.verbose {
			fmt.Fprintf(a.Out(), "[usage persist: marshal error: %v]\n", err)
		}
		return
	}
	if err := a.threadRuntime.Live().Append(context.Background(), thread.Event{
		Kind:    EventUsageRecorded,
		Payload: raw,
		Source:  thread.EventSource{Type: "session", SessionID: a.sessionID},
	}); err != nil && a.verbose {
		fmt.Fprintf(a.Out(), "[usage persist: append error: %v]\n", err)
	}
}

// replayUsageEvents rebuilds the usage tracker from persisted thread events.
// It deduplicates by request ID to avoid double-counting on repeated resumes.
func (a *Instance) replayUsageEvents(events []thread.Event) {
	if a.tracker == nil {
		return
	}
	seen := make(map[string]struct{})
	for _, event := range events {
		if event.Kind != EventUsageRecorded {
			continue
		}
		var record usage.Record
		if err := json.Unmarshal(event.Payload, &record); err != nil {
			continue
		}
		if id := record.Dims.RequestID; id != "" {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
		}
		a.tracker.Record(record)
	}
}

func newSessionID() (string, error) {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	out := make([]byte, len(b))
	for i, v := range b {
		out[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(out), nil
}

func splitThreadSessionRef(ref string) (dir string, id string) {
	cleaned := strings.TrimSpace(ref)
	if cleaned == "" {
		return ".", ""
	}
	if filepath.Ext(cleaned) == ".jsonl" || strings.Contains(cleaned, string(os.PathSeparator)) {
		dir = filepath.Dir(cleaned)
		id = strings.TrimSuffix(filepath.Base(cleaned), filepath.Ext(cleaned))
		return dir, id
	}
	return ".", cleaned
}
