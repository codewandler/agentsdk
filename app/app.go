package app

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/datasource"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/workflow"
)

// App is the user-facing composition root. It owns registries and running agent
// instances; channel dispatch lives at harness/session boundaries.
type App struct {
	commands              *command.Registry
	agents                map[string]*agent.Instance
	specs                 map[string]agent.Spec
	specCommands          map[string][]string
	diagnostics           []resource.Diagnostic
	defaultAgent          string
	plugins               map[string]Plugin
	toolMiddlewarePlugins []ToolMiddlewarePlugin
	toolTargetedMwPlugins []ToolTargetedMiddlewarePlugin
	toolMwApplied         bool
	contextProviders      []agentcontext.Provider
	agentContextPlugins   []AgentContextPlugin
	capabilityFactories   []capability.Factory
	skillSources          []skill.Source
	agentOptions          []agent.Option
	actions               *action.Registry
	datasources           *datasource.Registry
	workflows             map[string]workflow.Definition
	workflowOrder         []string
	tools                 *tool.Catalog
	defaultTools          []tool.Tool
}

type Option func(*config)

type config struct {
	commands        []command.Command
	specs           []agent.Spec
	defaultAgent    string
	plugins         []Plugin
	bundles         []resource.ContributionBundle
	discoveries     []SkillSourceDiscovery
	agentOptions    []agent.Option
	actions         []action.Action
	datasources     []datasource.Definition
	workflows       []workflow.Definition
	defaultTools    []tool.Tool
	catalogTools    []tool.Tool
	toolMiddlewares []tool.Middleware
}

func New(opts ...Option) (*App, error) {
	cfg := config{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	discoveredSources := []skill.Source{}
	for _, discovery := range cfg.discoveries {
		sources, err := DiscoverDefaultSkillSources(discovery)
		if err != nil {
			return nil, err
		}
		discoveredSources = append(discoveredSources, sources...)
	}

	a := &App{
		commands:     command.NewRegistry(),
		agents:       map[string]*agent.Instance{},
		specs:        map[string]agent.Spec{},
		specCommands: map[string][]string{},
		defaultAgent: cfg.defaultAgent,
		plugins:      map[string]Plugin{},
		skillSources: discoveredSources,
		agentOptions: append([]agent.Option(nil), cfg.agentOptions...),
		actions:      action.NewRegistry(),
		datasources:  datasource.NewRegistry(),
		workflows:    map[string]workflow.Definition{},
	}
	defaultTools := append([]tool.Tool(nil), cfg.defaultTools...)
	catalog, err := tool.NewCatalog(cfg.catalogTools...)
	if err != nil {
		return nil, err
	}
	a.tools = catalog
	if len(cfg.toolMiddlewares) > 0 {
		a.tools.ApplyAll(cfg.toolMiddlewares...)
		// Also wrap default tools so agents without explicit tool lists get middlewares.
		for i, t := range defaultTools {
			defaultTools[i] = tool.Apply(t, cfg.toolMiddlewares...)
		}
	}
	a.defaultTools = append([]tool.Tool(nil), defaultTools...)
	if len(cfg.commands) > 0 {
		if err := a.registerCommands(cfg.commands...); err != nil {
			return nil, err
		}
	}
	if err := a.RegisterActions(cfg.actions...); err != nil {
		return nil, err
	}
	if err := a.registerDataSources(cfg.datasources...); err != nil {
		return nil, err
	}
	if err := a.registerWorkflows(cfg.workflows...); err != nil {
		return nil, err
	}
	for _, spec := range cfg.specs {
		if err := a.registerAgentSpec(spec); err != nil {
			return nil, err
		}
	}
	for _, bundle := range cfg.bundles {
		if err := a.registerResourceBundle(bundle); err != nil {
			return nil, err
		}
	}
	for _, plugin := range cfg.plugins {
		if err := a.registerPlugin(plugin); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func WithCommand(commands ...command.Command) Option {
	return func(c *config) { c.commands = append(c.commands, commands...) }
}

func WithAgentSpec(spec agent.Spec) Option {
	return func(c *config) { c.specs = append(c.specs, spec) }
}

func WithResourceBundle(bundle resource.ContributionBundle) Option {
	return func(c *config) { c.bundles = append(c.bundles, bundle) }
}

func WithDefaultAgent(name string) Option {
	return func(c *config) { c.defaultAgent = name }
}

func WithPlugin(plugin Plugin) Option {
	return func(c *config) {
		if plugin != nil {
			c.plugins = append(c.plugins, plugin)
		}
	}
}

func WithDefaultSkillSourceDiscovery(discovery SkillSourceDiscovery) Option {
	return func(c *config) { c.discoveries = append(c.discoveries, discovery) }
}

func WithAgentOptions(opts ...agent.Option) Option {
	return func(c *config) { c.agentOptions = append(c.agentOptions, opts...) }
}

// WithDefaultTools adds tools used by agents that do not explicitly select tools.
func WithDefaultTools(tools ...tool.Tool) Option {
	return func(c *config) { c.defaultTools = append(c.defaultTools, tools...) }
}

// WithCatalogTools adds tools that can be selected by agent specs without making
// them active by default.
func WithCatalogTools(tools ...tool.Tool) Option {
	return func(c *config) { c.catalogTools = append(c.catalogTools, tools...) }
}

func WithActions(actions ...action.Action) Option {
	return func(c *config) { c.actions = append(c.actions, actions...) }
}

func WithDataSources(defs ...datasource.Definition) Option {
	return func(c *config) { c.datasources = append(c.datasources, defs...) }
}

func WithWorkflows(defs ...workflow.Definition) Option {
	return func(c *config) { c.workflows = append(c.workflows, defs...) }
}

// WithToolMiddlewares adds middlewares that will be applied to all tools
// in the catalog after construction. These are applied in order (first =
// innermost) before any plugin-contributed middlewares.
func WithToolMiddlewares(middlewares ...tool.Middleware) Option {
	return func(c *config) { c.toolMiddlewares = append(c.toolMiddlewares, middlewares...) }
}

func (a *App) Commands() *command.Registry {
	if a == nil {
		return nil
	}
	return a.commands
}

func (a *App) registerCommands(commands ...command.Command) error {
	if a.commands == nil {
		a.commands = command.NewRegistry()
	}
	return a.commands.Register(commands...)
}

func (a *App) RegisterActions(actions ...action.Action) error {
	if a.actions == nil {
		a.actions = action.NewRegistry()
	}
	return a.actions.Register(actions...)
}

func (a *App) registerDataSources(defs ...datasource.Definition) error {
	if a.datasources == nil {
		a.datasources = datasource.NewRegistry()
	}
	return a.datasources.Register(defs...)
}

func (a *App) registerWorkflows(defs ...workflow.Definition) error {
	if a.workflows == nil {
		a.workflows = map[string]workflow.Definition{}
	}
	for _, def := range defs {
		if def.Name == "" {
			return fmt.Errorf("app: workflow name is required")
		}
		if err := workflow.Validate(def); err != nil {
			return err
		}
		if _, exists := a.workflows[def.Name]; exists {
			return fmt.Errorf("app: workflow %q already registered", def.Name)
		}
		a.workflows[def.Name] = def
		a.workflowOrder = append(a.workflowOrder, def.Name)
	}
	return nil
}

func (a *App) Workflow(name string) (workflow.Definition, bool) {
	if a == nil {
		return workflow.Definition{}, false
	}
	def, ok := a.workflows[name]
	return def, ok
}

func (a *App) Workflows() []workflow.Definition {
	if a == nil {
		return nil
	}
	out := make([]workflow.Definition, 0, len(a.workflowOrder))
	for _, name := range a.workflowOrder {
		out = append(out, a.workflows[name])
	}
	return out
}

func (a *App) workflowExecutor(opts ...workflow.ExecuteOption) workflow.Executor {
	return workflow.NewExecutor(workflow.RegistryResolver{Registry: a.actions}, opts...)
}

func (a *App) ExecuteWorkflow(ctx action.Ctx, name string, input any, opts ...workflow.ExecuteOption) action.Result {
	if a == nil {
		return action.Result{Error: fmt.Errorf("app: nil app")}
	}
	def, ok := a.Workflow(name)
	if !ok {
		return action.Result{Error: fmt.Errorf("app: workflow %q not found", name)}
	}
	return a.workflowExecutor(opts...).Execute(ctx, def, input)
}

func (a *App) agent(name string) (*agent.Instance, bool) {
	if a == nil {
		return nil, false
	}
	inst, ok := a.agents[name]
	return inst, ok
}

func (a *App) registerAgentSpec(spec agent.Spec) error {
	if spec.Name == "" {
		return fmt.Errorf("app: agent spec name is required")
	}
	if a.specs == nil {
		a.specs = map[string]agent.Spec{}
	}
	if a.specCommands == nil {
		a.specCommands = map[string][]string{}
	}
	if _, exists := a.specs[spec.Name]; exists {
		a.diagnostics = append(a.diagnostics, resource.Warning(resource.SourceRef{ID: spec.ResourceFrom}, fmt.Sprintf("agent %q ignored because the short name is already registered", spec.Name)))
		return nil
	}
	a.specs[spec.Name] = spec
	a.specCommands[spec.Name] = append([]string(nil), spec.Commands...)
	if a.defaultAgent == "" {
		a.defaultAgent = spec.Name
	}
	return nil
}

func (a *App) AgentSpec(name string) (agent.Spec, bool) {
	if a == nil {
		return agent.Spec{}, false
	}
	spec, ok := a.specs[name]
	return spec, ok
}

func (a *App) InstantiateAgent(name string, opts ...agent.Option) (*agent.Instance, error) {
	// Ensure middleware plugins are applied before tools are consumed.
	// Idempotent — safe to call multiple times.
	a.applyToolMiddlewares()

	spec, ok := a.AgentSpec(name)
	if !ok {
		return nil, fmt.Errorf("app: agent spec %q not found", name)
	}
	var tools []tool.Tool
	if len(spec.Tools) == 0 {
		tools = append([]tool.Tool(nil), a.defaultTools...)
	} else {
		var err error
		tools, err = a.tools.Select(spec.Tools)
		if err != nil {
			return nil, err
		}
	}
	if view := a.agentCommandView(name); len(view.AgentCommands()) > 0 {
		tools = append(tools, command.Tool(view))
	}
	spec.SkillSources = a.agentSkillSources(spec)
	base := []agent.Option{
		agent.WithSpec(spec),
		agent.WithTools(tools),
	}
	if len(a.contextProviders) > 0 {
		base = append(base, agent.WithContextProviders(a.contextProviders...))
	}
	if len(a.capabilityFactories) > 0 {
		registry, err := capability.NewRegistry(a.capabilityFactories...)
		if err != nil {
			return nil, fmt.Errorf("app: create capability registry: %w", err)
		}
		base = append(base, agent.WithCapabilityRegistry(registry))
	}
	if len(a.agentContextPlugins) > 0 {
		factories := make([]agent.ContextProviderFactory, len(a.agentContextPlugins))
		for i, acp := range a.agentContextPlugins {
			factories[i] = func(info agent.ContextProviderFactoryInfo) []agentcontext.Provider {
				return acp.AgentContextProviders(AgentContextInfo{
					SkillRepository: info.SkillRepository,
					SkillState:      info.SkillState,
					ActiveTools:     info.ActiveTools,
					Workspace:       info.Workspace,
					Model:           info.Model,
					Effort:          info.Effort,
				})
			}
		}
		base = append(base, agent.WithContextProviderFactories(factories...))
	}
	base = append(base, a.agentOptions...)
	base = append(base, opts...)
	inst, err := agent.New(base...)
	if err != nil {
		return nil, err
	}
	if a.agents == nil {
		a.agents = map[string]*agent.Instance{}
	}
	a.agents[name] = inst
	if a.defaultAgent == "" {
		a.defaultAgent = name
	}
	return inst, nil
}

func (a *App) InstantiateDefaultAgent(opts ...agent.Option) (*agent.Instance, error) {
	if a == nil || a.defaultAgent == "" {
		return nil, fmt.Errorf("app: no default agent configured")
	}
	return a.InstantiateAgent(a.defaultAgent, opts...)
}

func (a *App) agentCommandView(name string) *command.Registry {
	view := command.NewRegistry()
	if a == nil || a.commands == nil {
		return view
	}
	allowed := map[string]bool{}
	for _, commandName := range a.specCommands[name] {
		allowed[commandName] = true
	}
	if len(allowed) == 0 {
		return view
	}
	for _, cmd := range a.commands.All() {
		desc := cmd.Descriptor()
		if !allowed[desc.Name] {
			continue
		}
		if desc.AgentCallable() {
			_ = view.Register(cmd)
		}
	}
	return view
}

func (a *App) AgentSpecs() []agent.Spec {
	if a == nil {
		return nil
	}
	names := make([]string, 0, len(a.specs))
	for name := range a.specs {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]agent.Spec, 0, len(names))
	for _, name := range names {
		out = append(out, a.specs[name])
	}
	return out
}

func (a *App) DefaultAgent() (*agent.Instance, bool) {
	if a == nil || a.defaultAgent == "" {
		return nil, false
	}
	return a.agent(a.defaultAgent)
}

func (a *App) registerPlugin(plugin Plugin) error {
	if plugin == nil {
		return nil
	}
	if plugin.Name() == "" {
		return fmt.Errorf("app: plugin name is required")
	}
	if a.plugins == nil {
		a.plugins = map[string]Plugin{}
	}
	if _, exists := a.plugins[plugin.Name()]; exists {
		return fmt.Errorf("app: plugin %q already registered", plugin.Name())
	}
	a.plugins[plugin.Name()] = plugin
	if ap, ok := plugin.(ActionsPlugin); ok {
		if err := a.RegisterActions(ap.Actions()...); err != nil {
			return fmt.Errorf("app: register plugin %q actions: %w", plugin.Name(), err)
		}
	}
	if dp, ok := plugin.(DataSourcesPlugin); ok {
		if err := a.registerDataSources(dp.DataSources()...); err != nil {
			return fmt.Errorf("app: register plugin %q datasources: %w", plugin.Name(), err)
		}
	}
	if wp, ok := plugin.(WorkflowsPlugin); ok {
		if err := a.registerWorkflows(wp.Workflows()...); err != nil {
			return fmt.Errorf("app: register plugin %q workflows: %w", plugin.Name(), err)
		}
	}
	if cp, ok := plugin.(CommandsPlugin); ok {
		for _, cmd := range cp.Commands() {
			if err := a.registerCommandFromSource(cmd, resource.SourceRef{ID: plugin.Name()}); err != nil {
				return fmt.Errorf("app: register plugin %q commands: %w", plugin.Name(), err)
			}
		}
	}
	if ap, ok := plugin.(AgentSpecsPlugin); ok {
		for _, spec := range ap.AgentSpecs() {
			if err := a.registerAgentSpec(spec); err != nil {
				return fmt.Errorf("app: register plugin %q agent specs: %w", plugin.Name(), err)
			}
		}
	}
	if sp, ok := plugin.(SkillsPlugin); ok {
		a.skillSources = append(a.skillSources, sp.SkillSources()...)
	}
	if dtp, ok := plugin.(DefaultToolsPlugin); ok {
		a.defaultTools = append(a.defaultTools, dtp.DefaultTools()...)
	}
	if ctp, ok := plugin.(CatalogToolsPlugin); ok {
		if err := a.tools.Register(ctp.CatalogTools()...); err != nil {
			return fmt.Errorf("app: register plugin %q catalog tools: %w", plugin.Name(), err)
		}
	}
	if tp, ok := plugin.(ToolsPlugin); ok {
		if err := a.tools.Register(tp.Tools()...); err != nil {
			return fmt.Errorf("app: register plugin %q tools: %w", plugin.Name(), err)
		}
	}
	if cp, ok := plugin.(ContextProvidersPlugin); ok {
		a.contextProviders = append(a.contextProviders, cp.ContextProviders()...)
	}
	if cp, ok := plugin.(CapabilityFactoriesPlugin); ok {
		a.capabilityFactories = append(a.capabilityFactories, cp.CapabilityFactories()...)
	}
	if acp, ok := plugin.(AgentContextPlugin); ok {
		a.agentContextPlugins = append(a.agentContextPlugins, acp)
	}
	// Middleware plugins are collected now, applied later during agent instantiation.
	if tmp, ok := plugin.(ToolMiddlewarePlugin); ok {
		a.toolMiddlewarePlugins = append(a.toolMiddlewarePlugins, tmp)
	}
	if ttmp, ok := plugin.(ToolTargetedMiddlewarePlugin); ok {
		a.toolTargetedMwPlugins = append(a.toolTargetedMwPlugins, ttmp)
	}
	return nil
}

func (a *App) registerResourceBundle(bundle resource.ContributionBundle) error {
	for _, cmd := range bundle.Commands {
		if err := a.registerCommandFromSource(cmd, bundle.Source); err != nil {
			return err
		}
	}
	for _, spec := range bundle.AgentSpecs {
		if err := a.registerAgentSpec(spec); err != nil {
			return err
		}
	}
	for _, contribution := range bundle.DataSources {
		if err := a.registerDataSources(datasourceFromContribution(contribution)); err != nil {
			return err
		}
	}
	for _, contribution := range bundle.Workflows {
		if err := a.registerWorkflows(workflowFromContribution(contribution)); err != nil {
			return err
		}
	}
	a.skillSources = append(a.skillSources, bundle.SkillSources...)
	a.diagnostics = append(a.diagnostics, bundle.Diagnostics...)
	return nil
}

func datasourceFromContribution(c resource.DataSourceContribution) datasource.Definition {
	return datasource.Definition{
		Name:        c.Name,
		Description: c.Description,
		Kind:        datasource.Kind(c.Kind),
		Metadata:    c.Metadata,
	}
}

func workflowFromContribution(c resource.WorkflowContribution) workflow.Definition {
	def := workflow.Definition{Name: c.Name, Description: c.Description, Version: stringFromAny(c.Definition["version"])}
	if rawSteps, ok := c.Definition["steps"]; ok {
		def.Steps = workflowStepsFromContribution(rawSteps)
	}
	return def
}

func workflowStepsFromContribution(raw any) []workflow.Step {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	steps := make([]workflow.Step, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		step := workflow.Step{
			ID:             stringFromAny(m["id"]),
			Action:         workflowActionRefFromAny(m["action"]),
			Input:          m["input"],
			InputMap:       stringMapFromAny(firstNonNil(m["input_map"], m["inputMap"])),
			InputTemplate:  firstNonNil(m["input_template"], m["inputTemplate"]),
			DependsOn:      stringSliceFromAny(firstNonNil(m["depends_on"], m["dependsOn"])),
			When:           workflowConditionFromAny(m["when"]),
			Retry:          workflowRetryFromAny(m["retry"]),
			Timeout:        durationFromAny(m["timeout"]),
			ErrorPolicy:    workflow.StepErrorPolicy(stringFromAny(firstNonNil(m["error_policy"], m["errorPolicy"]))),
			IdempotencyKey: stringFromAny(firstNonNil(m["idempotency_key"], m["idempotencyKey"])),
		}
		steps = append(steps, step)
	}
	return steps
}

func workflowActionRefFromAny(raw any) workflow.ActionRef {
	switch v := raw.(type) {
	case string:
		return workflow.ActionRef{Name: strings.TrimSpace(v)}
	case map[string]any:
		return workflow.ActionRef{Name: stringFromAny(v["name"])}
	default:
		return workflow.ActionRef{}
	}
}

func stringFromAny(raw any) string {
	if s, ok := raw.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func stringSliceFromAny(raw any) []string {
	switch v := raw.(type) {
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{strings.TrimSpace(v)}
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringFromAny(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func stringMapFromAny(raw any) map[string]string {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for key, value := range m {
		if s := stringFromAny(value); s != "" {
			out[key] = s
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func workflowConditionFromAny(raw any) workflow.Condition {
	m, ok := raw.(map[string]any)
	if !ok {
		return workflow.Condition{}
	}
	return workflow.Condition{
		StepID: stringFromAny(firstNonNil(m["step_id"], m["stepID"], m["step"])),
		Equals: m["equals"],
		Exists: boolFromAny(m["exists"]),
		Not:    boolFromAny(m["not"]),
	}
}

func workflowRetryFromAny(raw any) workflow.RetryPolicy {
	m, ok := raw.(map[string]any)
	if !ok {
		return workflow.RetryPolicy{}
	}
	return workflow.RetryPolicy{MaxAttempts: intFromAny(firstNonNil(m["max_attempts"], m["maxAttempts"])), Backoff: durationFromAny(m["backoff"])}
}

func durationFromAny(raw any) time.Duration {
	s := stringFromAny(raw)
	if s == "" {
		return 0
	}
	d, _ := time.ParseDuration(s)
	return d
}

func boolFromAny(raw any) bool {
	b, _ := raw.(bool)
	return b
}

func intFromAny(raw any) int {
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func (a *App) registerCommandFromSource(cmd command.Command, source resource.SourceRef) error {
	if err := a.registerCommands(cmd); err != nil {
		var dup command.ErrDuplicate
		if errors.As(err, &dup) {
			a.diagnostics = append(a.diagnostics, resource.Warning(source, fmt.Sprintf("command %q ignored because the short name is already registered", dup.Name)))
			return nil
		}
		return err
	}
	return nil
}

func (a *App) Diagnostics() []resource.Diagnostic {
	if a == nil {
		return nil
	}
	return append([]resource.Diagnostic(nil), a.diagnostics...)
}

func (a *App) SkillSources() []skill.Source {
	if a == nil {
		return nil
	}
	return append([]skill.Source(nil), a.skillSources...)
}

// applyToolMiddlewares applies collected middleware plugins to the tool catalog.
// Application order:
//  1. ToolTargetedMiddlewarePlugin middlewares are applied per-tool (innermost).
//  2. ToolMiddlewarePlugin middlewares are applied globally (outermost).
func (a *App) applyToolMiddlewares() {
	if a == nil || a.tools == nil || a.toolMwApplied {
		return
	}
	a.toolMwApplied = true

	// Pass 1: targeted middlewares (per-tool, innermost).
	for _, plugin := range a.toolTargetedMwPlugins {
		for _, name := range a.tools.Names() {
			if mws := plugin.ToolMiddlewaresFor(name); len(mws) > 0 {
				a.tools.ApplyTo(name, mws...)
			}
		}
	}

	// Pass 2: global middlewares (outermost).
	for _, plugin := range a.toolMiddlewarePlugins {
		a.tools.ApplyAll(plugin.ToolMiddlewares()...)
	}
}

func (a *App) ToolCatalog() *tool.Catalog {
	if a == nil {
		return nil
	}
	return a.tools
}

func (a *App) agentSkillSources(spec agent.Spec) []skill.Source {
	sources := append([]skill.Source(nil), a.skillSources...)
	sources = append(sources, spec.SkillSources...)
	return sources
}
