package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/datasource"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/standard"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/agentsdk/workflow"
)

// App is the user-facing composition root. It owns command dispatch and one or
// more running agent instances.
type App struct {
	out                   io.Writer
	commands              *command.Registry
	agents                map[string]*agent.Instance
	specs                 map[string]agent.Spec
	specCommands          map[string][]string
	protected             map[string]bool
	diagnostics           []resource.Diagnostic
	defaultAgent          string
	plugins               map[string]Plugin
	toolMiddlewarePlugins []ToolMiddlewarePlugin
	toolTargetedMwPlugins []ToolTargetedMiddlewarePlugin
	toolMwApplied         bool
	contextProviders      []agentcontext.Provider
	agentContextPlugins   []AgentContextPlugin
	skillSources          []skill.Source
	agentOptions          []agent.Option
	actions               *action.Registry
	datasources           *datasource.Registry
	workflows             map[string]workflow.Definition
	workflowOrder         []string
	tools                 *tool.Catalog
	defaultTools          []tool.Tool
	turnID                int
}

type Option func(*config)

type config struct {
	out             io.Writer
	commands        []command.Command
	agents          map[string]*agent.Instance
	specs           []agent.Spec
	defaultAgent    string
	plugins         []Plugin
	bundles         []resource.ContributionBundle
	skillSources    []skill.Source
	discoveries     []SkillSourceDiscovery
	agentOptions    []agent.Option
	actions         []action.Action
	datasources     []datasource.Definition
	workflows       []workflow.Definition
	tools           []tool.Tool
	noBuiltins      bool
	toolMiddlewares []tool.Middleware
}

func New(opts ...Option) (*App, error) {
	cfg := config{out: os.Stdout, agents: map[string]*agent.Instance{}}
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
	discoveredSources = append(discoveredSources, cfg.skillSources...)

	a := &App{
		out:          cfg.out,
		commands:     command.NewRegistry(),
		agents:       map[string]*agent.Instance{},
		specs:        map[string]agent.Spec{},
		specCommands: map[string][]string{},
		protected:    map[string]bool{},
		defaultAgent: cfg.defaultAgent,
		plugins:      map[string]Plugin{},
		skillSources: discoveredSources,
		agentOptions: append([]agent.Option(nil), cfg.agentOptions...),
		actions:      action.NewRegistry(),
		datasources:  datasource.NewRegistry(),
		workflows:    map[string]workflow.Definition{},
	}
	defaultTools := standard.DefaultTools()
	defaultTools = append(defaultTools, cfg.tools...)
	catalogTools := standard.CatalogTools()
	catalogTools = append(catalogTools, cfg.tools...)
	catalog, err := tool.NewCatalog(catalogTools...)
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
	for name, inst := range cfg.agents {
		if inst != nil {
			a.agents[name] = inst
		}
	}
	if len(a.agents) == 1 && a.defaultAgent == "" {
		for name := range a.agents {
			a.defaultAgent = name
		}
	}
	if !cfg.noBuiltins {
		// Built-ins are protected app-control commands. Resource commands that
		// collide with them fail registration instead of overriding them.
		builtins := a.builtins()
		a.protectCommands(builtins...)
		if err := a.RegisterCommands(builtins...); err != nil {
			return nil, err
		}
	}
	if len(cfg.commands) > 0 {
		if err := a.RegisterCommands(cfg.commands...); err != nil {
			return nil, err
		}
	}
	if err := a.RegisterActions(cfg.actions...); err != nil {
		return nil, err
	}
	if err := a.RegisterDataSources(cfg.datasources...); err != nil {
		return nil, err
	}
	if err := a.RegisterWorkflows(cfg.workflows...); err != nil {
		return nil, err
	}
	for _, spec := range cfg.specs {
		if err := a.RegisterAgentSpec(spec); err != nil {
			return nil, err
		}
	}
	for _, bundle := range cfg.bundles {
		if err := a.RegisterResourceBundle(bundle); err != nil {
			return nil, err
		}
	}
	for _, plugin := range cfg.plugins {
		if err := a.RegisterPlugin(plugin); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func WithOutput(out io.Writer) Option {
	return func(c *config) { c.out = out }
}

func WithCommand(commands ...command.Command) Option {
	return func(c *config) { c.commands = append(c.commands, commands...) }
}

func WithAgent(name string, inst *agent.Instance) Option {
	return func(c *config) {
		if c.agents == nil {
			c.agents = map[string]*agent.Instance{}
		}
		c.agents[name] = inst
	}
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

func WithSkillSources(sources ...skill.Source) Option {
	return func(c *config) { c.skillSources = append(c.skillSources, sources...) }
}

func WithDefaultSkillSourceDiscovery(discovery SkillSourceDiscovery) Option {
	return func(c *config) { c.discoveries = append(c.discoveries, discovery) }
}

func WithAgentOptions(opts ...agent.Option) Option {
	return func(c *config) { c.agentOptions = append(c.agentOptions, opts...) }
}

func WithAgentWorkspace(dir string) Option {
	return WithAgentOptions(agent.WithWorkspace(dir))
}

func WithAgentToolTimeout(timeout time.Duration) Option {
	return WithAgentOptions(agent.WithToolTimeout(timeout))
}

func WithAgentSessionStoreDir(dir string) Option {
	return WithAgentOptions(agent.WithSessionStoreDir(dir))
}

func WithAgentVerbose(verbose bool) Option {
	return WithAgentOptions(agent.WithVerbose(verbose))
}

func WithAgentOutput(out io.Writer) Option {
	return WithAgentOptions(agent.WithOutput(out))
}

func WithAgentTerminalUI(enabled bool) Option {
	return WithAgentOptions(agent.WithTerminalUI(enabled))
}

func WithTools(tools ...tool.Tool) Option {
	return func(c *config) { c.tools = append(c.tools, tools...) }
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

func WithoutBuiltins() Option {
	return func(c *config) { c.noBuiltins = true }
}

// WithToolMiddlewares adds middlewares that will be applied to all tools
// in the catalog after construction. These are applied in order (first =
// innermost) before any plugin-contributed middlewares.
func WithToolMiddlewares(middlewares ...tool.Middleware) Option {
	return func(c *config) { c.toolMiddlewares = append(c.toolMiddlewares, middlewares...) }
}

func (a *App) Out() io.Writer {
	if a == nil || a.out == nil {
		return io.Discard
	}
	return a.out
}

func (a *App) Commands() *command.Registry {
	if a == nil {
		return nil
	}
	return a.commands
}

func (a *App) RegisterCommands(commands ...command.Command) error {
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

func (a *App) ActionRegistry() *action.Registry {
	if a == nil {
		return nil
	}
	return a.actions
}

func (a *App) Actions() []action.Action {
	if a == nil || a.actions == nil {
		return nil
	}
	return a.actions.All()
}

func (a *App) RegisterDataSources(defs ...datasource.Definition) error {
	if a.datasources == nil {
		a.datasources = datasource.NewRegistry()
	}
	return a.datasources.Register(defs...)
}

func (a *App) DataSource(name string) (datasource.Definition, bool) {
	if a == nil || a.datasources == nil {
		return datasource.Definition{}, false
	}
	return a.datasources.Get(name)
}

func (a *App) DataSources() []datasource.Definition {
	if a == nil || a.datasources == nil {
		return nil
	}
	return a.datasources.All()
}

func (a *App) RegisterWorkflows(defs ...workflow.Definition) error {
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

type WorkflowExecutionOption func(*workflowExecutionConfig)

type workflowExecutionConfig struct {
	OnEvent  workflow.EventHandler
	RunID    workflow.RunID
	NewRunID func() workflow.RunID
}

func WithWorkflowEventHandler(handler workflow.EventHandler) WorkflowExecutionOption {
	return func(c *workflowExecutionConfig) { c.OnEvent = handler }
}

func WithWorkflowRunID(runID workflow.RunID) WorkflowExecutionOption {
	return func(c *workflowExecutionConfig) { c.RunID = runID }
}

func WithWorkflowRunIDGenerator(fn func() workflow.RunID) WorkflowExecutionOption {
	return func(c *workflowExecutionConfig) { c.NewRunID = fn }
}

func applyWorkflowExecutionOptions(opts []WorkflowExecutionOption) workflowExecutionConfig {
	var cfg workflowExecutionConfig
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cfg
}

func (a *App) workflowExecutionOptions(opts []WorkflowExecutionOption) ([]WorkflowExecutionOption, *workflow.ThreadRecorder) {
	if a == nil {
		return opts, nil
	}
	inst, ok := a.DefaultAgent()
	if !ok || inst == nil || inst.LiveThread() == nil {
		return opts, nil
	}
	cfg := applyWorkflowExecutionOptions(opts)
	recorder := &workflow.ThreadRecorder{Live: inst.LiveThread()}
	combined := func(ctx action.Ctx, event action.Event) {
		if cfg.OnEvent != nil {
			cfg.OnEvent(ctx, event)
		}
		recorder.OnEvent(ctx, event)
	}

	out := append([]WorkflowExecutionOption(nil), opts...)
	out = append(out, WithWorkflowEventHandler(combined))
	return out, recorder
}

func (a *App) AgentTurnAction(agentName string, spec action.Spec) (action.Action, error) {
	if a == nil {
		return nil, fmt.Errorf("app: nil app")
	}
	agentName = strings.TrimSpace(agentName)
	if agentName == "" {
		agentName = a.defaultAgent
	}
	if agentName == "" {
		return nil, fmt.Errorf("app: no default agent configured")
	}
	inst, ok := a.Agent(agentName)
	if !ok || inst == nil {
		return nil, fmt.Errorf("app: agent %q not found", agentName)
	}
	return agent.TurnAction(inst, spec), nil
}

func (a *App) DefaultAgentTurnAction(spec action.Spec) (action.Action, error) {
	return a.AgentTurnAction("", spec)
}

func (a *App) WorkflowExecutor(opts ...WorkflowExecutionOption) workflow.Executor {
	cfg := applyWorkflowExecutionOptions(opts)
	return workflow.Executor{Resolver: workflow.RegistryResolver{Registry: a.ActionRegistry()}, OnEvent: cfg.OnEvent, RunID: cfg.RunID, NewRunID: cfg.NewRunID}
}

func (a *App) ExecuteWorkflow(ctx action.Ctx, name string, input any, opts ...WorkflowExecutionOption) action.Result {
	if a == nil {
		return action.Result{Error: fmt.Errorf("app: nil app")}
	}
	def, ok := a.Workflow(name)
	if !ok {
		return action.Result{Error: fmt.Errorf("app: workflow %q not found", name)}
	}
	execOpts, recorder := a.workflowExecutionOptions(opts)
	result := a.WorkflowExecutor(execOpts...).Execute(ctx, def, input)
	if recorder != nil {
		result.Error = errors.Join(result.Error, recorder.Err())
	}
	return result
}

func (a *App) WorkflowAction(name string, opts ...WorkflowExecutionOption) (action.Action, bool) {
	def, ok := a.Workflow(name)
	if !ok {
		return nil, false
	}
	return workflow.WorkflowAction{Definition: def, Executor: a.WorkflowExecutor(opts...)}, true
}

func (a *App) RegisterWorkflowActions(names ...string) error {
	if a == nil {
		return fmt.Errorf("app: nil app")
	}
	if len(names) == 0 {
		for _, def := range a.Workflows() {
			names = append(names, def.Name)
		}
	}
	for _, name := range names {
		actionDef, ok := a.WorkflowAction(name)
		if !ok {
			return fmt.Errorf("app: workflow %q not found", name)
		}
		if err := a.RegisterActions(actionDef); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) RegisterWorkflowCommand(commandName, workflowName, description string, opts ...WorkflowExecutionOption) error {
	cmd, err := a.WorkflowCommand(commandName, workflowName, description, opts...)
	if err != nil {
		return err
	}
	return a.RegisterCommands(cmd)
}

func (a *App) WorkflowCommand(commandName, workflowName, description string, opts ...WorkflowExecutionOption) (command.Command, error) {
	if a == nil {
		return nil, fmt.Errorf("app: nil app")
	}
	commandName = strings.TrimPrefix(strings.TrimSpace(commandName), "/")
	workflowName = strings.TrimSpace(workflowName)
	if commandName == "" {
		return nil, fmt.Errorf("app: workflow command name is required")
	}
	if workflowName == "" {
		return nil, fmt.Errorf("app: workflow name is required")
	}
	if _, ok := a.Workflow(workflowName); !ok {
		return nil, fmt.Errorf("app: workflow %q not found", workflowName)
	}
	if description == "" {
		description = fmt.Sprintf("Run workflow %s", workflowName)
	}
	return command.New(command.Spec{Name: commandName, Description: description, ArgumentHint: "[input]"}, func(ctx context.Context, params command.Params) (command.Result, error) {
		result := a.ExecuteWorkflow(ctx, workflowName, params.Raw, opts...)
		if result.Error != nil {
			return command.Result{}, result.Error
		}
		return command.Text(renderWorkflowCommandResult(result.Data)), nil
	}), nil
}

func renderWorkflowCommandResult(data any) string {
	if wfResult, ok := data.(workflow.Result); ok {
		data = wfResult.Data
	}
	switch v := data.(type) {
	case nil:
		return ""
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	default:
		return fmt.Sprint(v)
	}
}
func (a *App) RegisterAgent(name string, inst *agent.Instance) error {
	if name == "" {
		return fmt.Errorf("app: agent name is required")
	}
	if inst == nil {
		return fmt.Errorf("app: agent %q is nil", name)
	}
	if a.agents == nil {
		a.agents = map[string]*agent.Instance{}
	}
	if _, exists := a.agents[name]; exists {
		return fmt.Errorf("app: agent %q already registered", name)
	}
	a.agents[name] = inst
	if a.defaultAgent == "" {
		a.defaultAgent = name
	}
	return nil
}

func (a *App) Agent(name string) (*agent.Instance, bool) {
	if a == nil {
		return nil, false
	}
	inst, ok := a.agents[name]
	return inst, ok
}

func (a *App) RegisterAgentSpec(spec agent.Spec) error {
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

func (a *App) AgentCommandNames(name string) []string {
	if a == nil {
		return nil
	}
	return append([]string(nil), a.specCommands[name]...)
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
	a.ApplyToolMiddlewares()

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
	if view := a.AgentCommandView(name); len(view.AgentCommands()) > 0 {
		tools = append(tools, command.Tool(view))
	}
	repo, err := skill.NewRepository(a.agentSkillSources(spec), spec.Skills)
	if err != nil {
		return nil, err
	}
	base := []agent.Option{
		agent.WithSpec(spec),
		agent.WithTools(tools),
		agent.WithSkillRepository(repo),
	}
	if len(a.contextProviders) > 0 {
		base = append(base, agent.WithContextProviders(a.contextProviders...))
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

func (a *App) AgentCommandView(name string) *command.Registry {
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
		spec := cmd.Spec()
		if !allowed[spec.Name] {
			continue
		}
		if spec.AgentCallable() {
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
	return a.Agent(a.defaultAgent)
}

func (a *App) AgentNames() []string {
	if a == nil {
		return nil
	}
	names := make([]string, 0, len(a.agents))
	for name := range a.agents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (a *App) RegisterPlugin(plugin Plugin) error {
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
		if err := a.RegisterDataSources(dp.DataSources()...); err != nil {
			return fmt.Errorf("app: register plugin %q datasources: %w", plugin.Name(), err)
		}
	}
	if wp, ok := plugin.(WorkflowsPlugin); ok {
		if err := a.RegisterWorkflows(wp.Workflows()...); err != nil {
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
			if err := a.RegisterAgentSpec(spec); err != nil {
				return fmt.Errorf("app: register plugin %q agent specs: %w", plugin.Name(), err)
			}
		}
	}
	if sp, ok := plugin.(SkillsPlugin); ok {
		a.skillSources = append(a.skillSources, sp.SkillSources()...)
	}
	if tp, ok := plugin.(ToolsPlugin); ok {
		if err := a.tools.Register(tp.Tools()...); err != nil {
			return fmt.Errorf("app: register plugin %q tools: %w", plugin.Name(), err)
		}
	}
	if cp, ok := plugin.(ContextProvidersPlugin); ok {
		a.contextProviders = append(a.contextProviders, cp.ContextProviders()...)
	}
	if acp, ok := plugin.(AgentContextPlugin); ok {
		a.agentContextPlugins = append(a.agentContextPlugins, acp)
	}
	// Middleware plugins are collected now, applied later via ApplyToolMiddlewares.
	if tmp, ok := plugin.(ToolMiddlewarePlugin); ok {
		a.toolMiddlewarePlugins = append(a.toolMiddlewarePlugins, tmp)
	}
	if ttmp, ok := plugin.(ToolTargetedMiddlewarePlugin); ok {
		a.toolTargetedMwPlugins = append(a.toolTargetedMwPlugins, ttmp)
	}
	return nil
}

func (a *App) RegisterResourceBundle(bundle resource.ContributionBundle) error {
	for _, cmd := range bundle.Commands {
		if err := a.registerCommandFromSource(cmd, bundle.Source); err != nil {
			return err
		}
	}
	for _, spec := range bundle.AgentSpecs {
		if err := a.RegisterAgentSpec(spec); err != nil {
			return err
		}
	}
	for _, contribution := range bundle.DataSources {
		if err := a.RegisterDataSources(datasourceFromContribution(contribution)); err != nil {
			return err
		}
	}
	for _, contribution := range bundle.Workflows {
		if err := a.RegisterWorkflows(workflowFromContribution(contribution)); err != nil {
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
	return workflow.Definition{
		Name:        c.Name,
		Description: c.Description,
	}
}

func (a *App) registerCommandFromSource(cmd command.Command, source resource.SourceRef) error {
	if err := a.RegisterCommands(cmd); err != nil {
		var dup command.ErrDuplicate
		if errors.As(err, &dup) {
			if a.isProtectedCommand(dup.Name) {
				return err
			}
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

func (a *App) ContextProviders() []agentcontext.Provider {
	if a == nil {
		return nil
	}
	return append([]agentcontext.Provider(nil), a.contextProviders...)
}

func (a *App) SkillSources() []skill.Source {
	if a == nil {
		return nil
	}
	return append([]skill.Source(nil), a.skillSources...)
}

func (a *App) protectCommands(commands ...command.Command) {
	if a.protected == nil {
		a.protected = map[string]bool{}
	}
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}
		spec := cmd.Spec()
		if spec.Name != "" {
			a.protected[spec.Name] = true
		}
		for _, alias := range spec.Aliases {
			if alias != "" {
				a.protected[alias] = true
			}
		}
	}
}

func (a *App) isProtectedCommand(name string) bool {
	if a == nil || a.protected == nil {
		return false
	}
	return a.protected[strings.TrimPrefix(strings.TrimSpace(name), "/")]
}

// ApplyToolMiddlewares applies collected middleware plugins to the tool catalog.
// Call this after all plugins have been registered and before agent instantiation.
//
// Application order:
//  1. ToolTargetedMiddlewarePlugin middlewares are applied per-tool (innermost).
//  2. ToolMiddlewarePlugin middlewares are applied globally (outermost).
func (a *App) ApplyToolMiddlewares() {
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

// Send routes user input through command dispatch or the default agent.
func (a *App) Send(ctx context.Context, input string) (command.Result, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return command.Handled(), nil
	}
	if strings.HasPrefix(input, "/") {
		result, err := a.commands.ExecuteUser(ctx, input)
		if err != nil {
			return command.Result{}, err
		}
		return a.Apply(ctx, result, 0)
	}
	inst, ok := a.DefaultAgent()
	if !ok {
		return command.Result{}, fmt.Errorf("app: no default agent configured")
	}
	return command.Handled(), inst.RunTurn(ctx, a.nextTurnID(), input)
}

// Apply performs an already-executed command result. turnID is used when a
// result asks to run the default agent.
func (a *App) Apply(ctx context.Context, result command.Result, turnID int) (command.Result, error) {
	switch result.Kind {
	case command.ResultAgentTurn:
		inst, ok := a.DefaultAgent()
		if !ok {
			return command.Result{}, fmt.Errorf("app: no default agent configured")
		}
		if strings.TrimSpace(result.Input) == "" {
			return command.Handled(), nil
		}
		if turnID <= 0 {
			turnID = a.nextTurnID()
		}
		return command.Handled(), inst.RunTurn(ctx, turnID, result.Input)
	case command.ResultReset:
		if inst, ok := a.DefaultAgent(); ok {
			inst.Reset()
		}
		a.turnID = 0
		return command.Handled(), nil
	default:
		return result, nil
	}
}

func (a *App) agentSkillSources(spec agent.Spec) []skill.Source {
	sources := append([]skill.Source(nil), a.skillSources...)
	sources = append(sources, spec.SkillSources...)
	return sources
}

func (a *App) nextTurnID() int {
	a.turnID++
	return a.turnID
}

// Methods below let terminal/repl use App directly.

func (a *App) RunTurn(ctx context.Context, turnID int, task string) error {
	inst, ok := a.DefaultAgent()
	if !ok {
		return fmt.Errorf("app: no default agent configured")
	}
	return inst.RunTurn(ctx, turnID, task)
}

func (a *App) Reset() {
	if inst, ok := a.DefaultAgent(); ok {
		inst.Reset()
	}
	a.turnID = 0
}

func (a *App) ParamsSummary() string {
	if inst, ok := a.DefaultAgent(); ok {
		return inst.ParamsSummary()
	}
	return ""
}

func (a *App) SessionID() string {
	if inst, ok := a.DefaultAgent(); ok {
		return inst.SessionID()
	}
	return ""
}

func (a *App) Tracker() *usage.Tracker {
	if inst, ok := a.DefaultAgent(); ok {
		return inst.Tracker()
	}
	return nil
}

func (a *App) builtins() []command.Command {
	return []command.Command{
		command.New(command.Spec{Name: "help", Aliases: []string{"?"}, Description: "Show available commands"}, func(context.Context, command.Params) (command.Result, error) {
			return command.Text(a.commands.HelpText()), nil
		}),
		command.New(command.Spec{Name: "agents", Description: "Show available agents"}, func(context.Context, command.Params) (command.Result, error) {
			return command.Text(a.agentsHelpText()), nil
		}),
		command.New(command.Spec{Name: "new", Aliases: []string{"reset"}, Description: "Start a new session"}, func(context.Context, command.Params) (command.Result, error) {
			return command.Reset(), nil
		}),
		command.New(command.Spec{Name: "quit", Aliases: []string{"q", "exit"}, Description: "Exit the app"}, func(context.Context, command.Params) (command.Result, error) {
			return command.Exit(), nil
		}),
		command.New(command.Spec{Name: "turn", Description: "Run a prompt as an agent turn", ArgumentHint: "[text]"}, func(_ context.Context, params command.Params) (command.Result, error) {
			if params.Raw == "" {
				return command.Text("usage: /turn <text>"), nil
			}
			return command.AgentTurn(params.Raw), nil
		}),
		command.New(command.Spec{Name: "session", Description: "Show session id and usage"}, func(context.Context, command.Params) (command.Result, error) {
			if inst, ok := a.DefaultAgent(); ok {
				record := inst.Tracker().Aggregate()
				return command.Text(fmt.Sprintf("session: %s\ninput=%d output=%d", inst.SessionID(), record.Usage.InputTokens(), record.Usage.OutputTokens())), nil
			}
			return command.Text("session: none"), nil
		}),
		command.New(command.Spec{Name: "context", Description: "Show last context render state"}, func(context.Context, command.Params) (command.Result, error) {
			if inst, ok := a.DefaultAgent(); ok {
				state := inst.ContextState()
				if state != "context: no render state" {
					return command.Text(state), nil
				}
			}
			if a.defaultAgent == "" {
				return command.Text("context: no default agent"), nil
			}
			return command.Text(fmt.Sprintf("context: no render state yet for agent %q\nrun a turn first to capture provider context", a.defaultAgent)), nil
		}),
		command.New(command.Spec{Name: "skills", Description: "List discovered skills and activation status"}, func(context.Context, command.Params) (command.Result, error) {
			if inst, ok := a.DefaultAgent(); ok {
				return command.Text(renderSkillsForAgent(inst)), nil
			}
			if a.defaultAgent == "" {
				return command.Text("skills: no default agent"), nil
			}
			spec, ok := a.AgentSpec(a.defaultAgent)
			if !ok {
				return command.Text("skills: no default agent"), nil
			}
			repo, err := skill.NewRepository(a.agentSkillSources(spec), spec.Skills)
			if err != nil {
				return command.Text(fmt.Sprintf("skills: %v", err)), nil
			}
			state, err := skill.NewActivationState(repo, repo.LoadedNames())
			if err != nil {
				return command.Text(fmt.Sprintf("skills: %v", err)), nil
			}
			return command.Text(renderSkillState(state)), nil
		}),
		command.New(command.Spec{Name: "skill", Description: "Activate a skill on the current agent", ArgumentHint: "<name>"}, func(_ context.Context, params command.Params) (command.Result, error) {
			name := strings.TrimSpace(params.Raw)
			if name == "" {
				return command.Text("usage: /skill <name>"), nil
			}
			inst, ok := a.DefaultAgent()
			if !ok {
				return command.Text("skill: no current agent"), nil
			}
			before := skill.StatusInactive
			if state := inst.SkillActivationState(); state != nil {
				before = state.Status(name)
			}
			status, err := inst.ActivateSkill(name)
			if err != nil {
				return command.Text("skill: " + err.Error()), nil
			}
			if before == skill.StatusBase || status == skill.StatusBase {
				return command.Text(fmt.Sprintf("skill: %q already active (base)", name)), nil
			}
			if before == skill.StatusDynamic {
				return command.Text(fmt.Sprintf("skill: %q already active (dynamic)", name)), nil
			}
			return command.Text(fmt.Sprintf("skill: activated %q", name)), nil
		}),
		command.New(command.Spec{Name: "compact", Description: "Summarize and compact conversation history"}, func(ctx context.Context, _ command.Params) (command.Result, error) {
			inst, ok := a.DefaultAgent()
			if !ok {
				return command.Text("compact: no current agent"), nil
			}
			result, err := inst.Compact(ctx)
			if err != nil {
				if errors.Is(err, agent.ErrNothingToCompact) {
					return command.Text("compact: conversation too short to compact"), nil
				}
				return command.Text(fmt.Sprintf("compact: %v", err)), nil
			}
			saved := result.TokensBefore - result.TokensAfter
			return command.Text(fmt.Sprintf(
				"Compacted: replaced %d messages with summary\nEstimated tokens: before=%d after=%d (saved ~%d)",
				result.ReplacedCount, result.TokensBefore, result.TokensAfter, saved,
			)), nil
		}),
	}
}

func (a *App) agentsHelpText() string {
	specs := a.AgentSpecs()
	if len(specs) == 0 {
		return "No agents registered."
	}
	var b strings.Builder
	b.WriteString("Agents:\n")
	for _, spec := range specs {
		marker := " "
		if spec.Name == a.defaultAgent {
			marker = "*"
		}
		fmt.Fprintf(&b, "%s %s", marker, spec.Name)
		if spec.Description != "" {
			fmt.Fprintf(&b, " - %s", spec.Description)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func renderSkillsForAgent(inst *agent.Instance) string {
	if inst == nil {
		return "skills: no current agent"
	}
	state := inst.SkillActivationState()
	if state == nil {
		return "skills: unavailable"
	}
	return renderSkillState(state)
}

func renderSkillState(state *skill.ActivationState) string {
	if state == nil || state.Repository() == nil {
		return "skills: unavailable"
	}
	var b strings.Builder
	b.WriteString("skills:\n")
	for _, item := range state.Repository().List() {
		marker := "[available]"
		switch state.Status(item.Name) {
		case skill.StatusBase:
			marker = "[active:base]"
		case skill.StatusDynamic:
			marker = "[active:dynamic]"
		}
		fmt.Fprintf(&b, "- %s %s", item.Name, marker)
		if item.Description != "" {
			fmt.Fprintf(&b, " — %s", item.Description)
		}
		b.WriteByte('\n')
		for _, ref := range item.References {
			refMarker := "[available]"
			for _, active := range state.ActiveReferences(item.Name) {
				if active.Path == ref.Path {
					refMarker = "[active]"
					break
				}
			}
			fmt.Fprintf(&b, "  - %s %s\n", ref.Path, refMarker)
		}
	}
	if diagnostics := state.Diagnostics(); len(diagnostics) > 0 {
		b.WriteString("warnings:\n")
		for _, diagnostic := range diagnostics {
			fmt.Fprintf(&b, "- %s\n", diagnostic)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
