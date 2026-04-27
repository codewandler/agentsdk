package agent

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/conversation/jsonlstore"
	"github.com/codewandler/agentsdk/runner"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/terminal/ui"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/standard"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/compatibility"
	"github.com/codewandler/llmadapter/unified"
)

var ErrMaxStepsReached = errors.New("maximum steps reached")

// Spec describes an agent identity/configuration independent of a running
// conversation session.
type Spec struct {
	Name         string
	Description  string
	System       string
	Inference    InferenceOptions
	MaxSteps     int
	Tools        []string
	Skills       []string
	SkillSources []skill.Source
	Commands     []string
	ResourceID   string
	ResourceFrom string
}

// Instance is a running session-backed agent built from a Spec and runtime
// options.
type Instance struct {
	client              unified.Client
	autoMux             func(adapterconfig.AutoOptions) (adapterconfig.AutoResult, error)
	autoResult          adapterconfig.AutoResult
	providerIdentity    conversation.ProviderIdentity
	resolvedProvider    string
	resolvedModel       string
	sourceAPI           adapt.ApiKind
	sourceAPIExplicit   bool
	modelPolicy         ModelPolicy
	modelCompatibility  modelCompatibilityState
	runtime             *agentruntime.Engine
	tracker             *usage.Tracker
	toolset             *standard.Toolset
	inference           InferenceOptions
	maxSteps            int
	out                 io.Writer
	terminalUI          bool
	workspace           string
	toolTimeout         time.Duration
	system              string
	systemBuilder       func(workspace, prompt string) string
	sessionID           string
	session             *conversation.Session
	sessionStoreDir     string
	resumeSession       string
	sessionStorePath    string
	cacheKeyPrefix      string
	verbose             bool
	initErrs            []error
	eventHandlerFactory func(*Instance, int) runner.EventHandler
	toolCtxFactory      func(context.Context) tool.Ctx
	specName            string
	specDescription     string
	specTools           []string
	specSkills          []string
	specSkillSources    []skill.Source
	specCommands        []string
	specResourceID      string
	specResourceFrom    string
	skillRepo           *skill.Repository
	materializedSystem  string
}

func New(opts ...Option) (*Instance, error) {
	sessionID, err := newSessionID()
	if err != nil {
		return nil, err
	}
	a := &Instance{
		inference:      DefaultInferenceOptions(),
		maxSteps:       30,
		out:            io.Discard,
		toolTimeout:    30 * time.Second,
		sessionID:      sessionID,
		sourceAPI:      adapt.ApiOpenAIResponses,
		cacheKeyPrefix: "agentsdk:",
		systemBuilder:  func(_ string, prompt string) string { return prompt },
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
	if a.toolset == nil {
		a.toolset = standard.DefaultToolset()
	}
	a.applySpecTools()
	if a.tracker == nil {
		a.tracker = usage.NewTracker()
	}
	if err := a.initSkills(); err != nil {
		return nil, err
	}
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
		Name:         a.specName,
		Description:  a.specDescription,
		System:       a.system,
		Inference:    a.inference,
		MaxSteps:     a.maxSteps,
		Tools:        append([]string(nil), a.specTools...),
		Skills:       append([]string(nil), a.specSkills...),
		SkillSources: append([]skill.Source(nil), a.specSkillSources...),
		Commands:     append([]string(nil), a.specCommands...),
		ResourceID:   a.specResourceID,
		ResourceFrom: a.specResourceFrom,
	}
}

func (a *Instance) SkillRepository() *skill.Repository {
	if a == nil {
		return nil
	}
	return a.skillRepo
}

func (a *Instance) LoadedSkills() []skill.Skill {
	if a == nil || a.skillRepo == nil {
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

func (a *Instance) applySpecTools() {
	if a == nil || a.toolset == nil || len(a.specTools) == 0 {
		return
	}
	activation := a.toolset.Activation()
	if activation == nil {
		return
	}
	activation.Deactivate("*")
	activation.Activate(a.specTools...)
}

func (a *Instance) initSkills() error {
	if a.skillRepo == nil {
		repo, err := skill.NewRepository(a.specSkillSources, a.specSkills)
		if err != nil {
			return err
		}
		a.skillRepo = repo
	} else {
		for _, name := range a.specSkills {
			if err := a.skillRepo.Load(name); err != nil {
				return err
			}
		}
	}
	base := a.systemBuilder(a.workspace, a.system)
	skills := a.skillRepo.Materialize()
	if strings.TrimSpace(skills) == "" {
		a.materializedSystem = base
		return nil
	}
	if strings.TrimSpace(base) == "" {
		a.materializedSystem = skills
		return nil
	}
	a.materializedSystem = strings.TrimRight(base, "\n") + "\n\n" + skills
	return nil
}

func (a *Instance) Reset() {
	if a == nil {
		return
	}
	a.tracker.Reset()
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
	}
	if a.runtime != nil {
		a.runtime.ResetSession(conversation.WithSessionID(conversation.SessionID(a.sessionID)))
	}
}

func (a *Instance) RunTurn(ctx context.Context, turnID int, task string) error {
	if a == nil || a.runtime == nil {
		return fmt.Errorf("agent: runtime is not initialized")
	}
	if a.verbose {
		ui.PrintResolvedModel(a.Out(), strings.TrimSpace(fmt.Sprintf("input=%s  instance=%s  resolved=%s%s", a.inference.Model, a.resolvedProvider, a.resolvedModel, a.modelCompatibilitySummary())))
	}
	handler := a.newEventHandler(turnID)
	_, err := a.runtime.RunTurn(
		ctx,
		task,
		agentruntime.WithTurnMaxSteps(a.maxSteps),
		agentruntime.WithTurnTools(a.toolset.ActiveTools()),
		agentruntime.WithTurnProviderIdentity(a.providerIdentity),
		agentruntime.WithTurnEventHandler(handler),
	)
	if errors.Is(err, runner.ErrMaxStepsReached) {
		return ErrMaxStepsReached
	}
	if err != nil {
		return fmt.Errorf("provider=%s model=%s: %w", a.resolvedProvider, a.resolvedModel, err)
	}
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
	if a.session != nil {
		opts = append(opts, agentruntime.WithSession(a.session))
	}
	return opts
}

func (a *Instance) baseRuntimeOptions(includeSessionID bool) []agentruntime.Option {
	opts := []agentruntime.Option{
		agentruntime.WithModel(a.inference.Model),
		agentruntime.WithMaxOutputTokens(a.inference.MaxTokens),
		agentruntime.WithTemperature(a.inference.Temperature),
		agentruntime.WithSystem(a.MaterializedSystem()),
		agentruntime.WithTools(a.toolset.ActiveTools()),
		agentruntime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
		agentruntime.WithCachePolicy(unified.CachePolicyOn),
		agentruntime.WithCacheKey(a.cacheKey()),
		agentruntime.WithMaxSteps(a.maxSteps),
		agentruntime.WithToolTimeout(a.toolTimeout),
		agentruntime.WithProviderIdentity(a.providerIdentity),
		agentruntime.WithContextProviders(
			contextproviders.Environment(contextproviders.WithWorkDir(a.workspace)),
			contextproviders.Time(time.Minute),
		),
		agentruntime.WithToolContextFactory(func(ctx context.Context) tool.Ctx {
			if a.toolCtxFactory != nil {
				return a.toolCtxFactory(ctx)
			}
			return agentruntime.NewToolContext(ctx,
				agentruntime.WithToolWorkDir(a.workspace),
				agentruntime.WithToolSessionID(a.sessionID),
				agentruntime.WithToolActivation(a.toolset.Activation()),
			)
		}),
	}
	if includeSessionID {
		opts = append([]agentruntime.Option{agentruntime.WithSessionOptions(conversation.WithSessionID(conversation.SessionID(a.sessionID)))}, opts...)
	}
	if reasoning, ok := a.reasoningConfig(); ok {
		opts = append(opts, agentruntime.WithReasoning(reasoning))
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
	a.providerIdentity = identity
}

func (a *Instance) initSession(ctx context.Context) error {
	if a.resumeSession == "" && a.sessionStoreDir == "" {
		return nil
	}
	opts := a.conversationOptions(false)
	if a.resumeSession != "" {
		store := jsonlstore.Open(a.resumeSession)
		session, err := conversation.Resume(ctx, store, "", opts...)
		if err != nil {
			return fmt.Errorf("resume session %s: %w", a.resumeSession, err)
		}
		a.session = session
		a.sessionID = string(session.SessionID())
		a.sessionStorePath = a.resumeSession
		return nil
	}
	return a.startPersistentSession(time.Now())
}

func (a *Instance) startPersistentSession(now time.Time) error {
	if a.sessionStoreDir == "" {
		a.session = nil
		a.sessionStorePath = ""
		return nil
	}
	path := filepath.Join(a.sessionStoreDir, fmt.Sprintf("%s-%s.jsonl", now.UTC().Format("20060102T150405Z"), a.sessionID))
	store := jsonlstore.Open(path)
	opts := append(a.conversationOptions(true),
		conversation.WithStore(store),
		conversation.WithConversationID(conversation.ConversationID("conv_"+a.sessionID)),
	)
	a.session = conversation.New(opts...)
	a.sessionStorePath = path
	return nil
}

func (a *Instance) conversationOptions(includeSessionID bool) []conversation.Option {
	return agentruntime.SessionOptions(a.baseRuntimeOptions(includeSessionID)...)
}

func (a *Instance) reasoningConfig() (unified.ReasoningConfig, bool) {
	switch a.inference.Thinking {
	case ThinkingModeOff, ThinkingModeAuto, "":
		return unified.ReasoningConfig{}, false
	default:
		return unified.ReasoningConfig{Effort: a.inference.Effort, Expose: true}, true
	}
}

func (a *Instance) cacheKey() string {
	if a.sessionID == "" {
		return ""
	}
	return a.cacheKeyPrefix + a.sessionID
}

func (a *Instance) newEventHandler(turnID int) runner.EventHandler {
	var display *ui.EventDisplay
	if a.terminalUI && a.out != nil && a.out != io.Discard {
		display = ui.NewEventDisplay(a.out,
			ui.WithTracker(a.tracker),
			ui.WithTurnID(strconv.Itoa(turnID)),
			ui.WithSessionID(a.sessionID),
			ui.WithFallbackModel(a.inference.Model),
			ui.WithRouteState(usage.RouteState{Provider: a.resolvedProvider, Model: a.resolvedModel}),
		)
	}
	extra := runner.EventHandler(nil)
	if a.eventHandlerFactory != nil {
		extra = a.eventHandlerFactory(a, turnID)
	}
	return func(event runner.Event) {
		if display != nil {
			display.Handle(event)
		} else {
			a.recordEvent(turnID, event)
		}
		if ev, ok := event.(runner.RouteEvent); ok {
			a.providerIdentity = ev.ProviderIdentity
			a.resolvedProvider = ev.ProviderIdentity.ProviderName
			a.resolvedModel = ev.ProviderIdentity.NativeModel
		}
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
		a.tracker.Record(usage.FromRunnerEvent(ev, usage.RunnerEventOptions{
			TurnID:        strconv.Itoa(turnID),
			SessionID:     a.sessionID,
			FallbackModel: a.inference.Model,
			RouteState: usage.RouteState{
				Provider: a.resolvedProvider,
				Model:    a.resolvedModel,
			},
		}))
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
