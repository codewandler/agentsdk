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

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/standard"
	"github.com/codewandler/agentsdk/usage"
)

// App is the user-facing composition root. It owns command dispatch and one or
// more running agent instances.
type App struct {
	out          io.Writer
	commands     *command.Registry
	agents       map[string]*agent.Instance
	specs        map[string]agent.Spec
	specCommands map[string][]string
	protected    map[string]bool
	diagnostics  []resource.Diagnostic
	defaultAgent string
	plugins             map[string]Plugin
	contextProviders    []agentcontext.Provider
	agentContextPlugins []AgentContextPlugin
	skillSources       []skill.Source
	agentOptions       []agent.Option
	tools        *tool.Catalog
	defaultTools []tool.Tool
	turnID       int
}

type Option func(*config)

type config struct {
	out          io.Writer
	commands     []command.Command
	agents       map[string]*agent.Instance
	specs        []agent.Spec
	defaultAgent string
	plugins      []Plugin
	bundles      []resource.ContributionBundle
	skillSources []skill.Source
	discoveries  []SkillSourceDiscovery
	agentOptions []agent.Option
	tools        []tool.Tool
	noBuiltins   bool
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

func WithAgentCacheKeyPrefix(prefix string) Option {
	return WithAgentOptions(agent.WithCacheKeyPrefix(prefix))
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

func WithoutBuiltins() Option {
	return func(c *config) { c.noBuiltins = true }
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
	a.skillSources = append(a.skillSources, bundle.SkillSources...)
	a.diagnostics = append(a.diagnostics, bundle.Diagnostics...)
	return nil
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
