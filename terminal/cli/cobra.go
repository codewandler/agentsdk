package cli

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/llmadapter/unified"
	"github.com/spf13/cobra"
)

type ModelCompleter func(toComplete string) []string

type CommandConfig struct {
	Name  string
	Use   string
	Short string
	Long  string

	Resources    Resources
	ResourceArg  bool // Deprecated: use DiscoverFlag instead.
	DiscoverFlag bool
	AgentFlag    bool

	// EmbeddedBase, when set alongside DiscoverFlag, uses the embedded FS as
	// the primary resource source and merges -d paths on top. This is used by
	// first-party app commands (dev, build) that ship their own resources.
	EmbeddedBase     fs.FS
	EmbeddedBaseRoot string

	DefaultAgent       string
	DefaultSessionsDir string
	Prompt             string
	DiscoveryPolicy    resource.DiscoveryPolicy

	DefaultInference      agentconfig.InferenceOptions
	DefaultMaxSteps       int
	DefaultToolTimeout    time.Duration
	ApplyDefaultInference bool
	ApplyDefaultMaxSteps  bool

	ModelCompleter ModelCompleter
	Profile        Profile

	NoDefaultPlugins bool
	AgentOptions    []agent.Option
	AppOptions      []app.Option
	PluginFactory   app.PluginFactory

	In  io.Reader
	Out io.Writer
	Err io.Writer
}

func NewCommand(cfg CommandConfig) *cobra.Command {
	cfg = applyProfileDefaults(cfg)
	inference := cfg.DefaultInference
	if inference == (agentconfig.InferenceOptions{}) {
		inference = agentconfig.DefaultInferenceOptions()
	}
	applyProfileInferenceDefaults(&inference, cfg.Profile.Defaults)
	maxSteps := cfg.DefaultMaxSteps
	if maxSteps <= 0 {
		maxSteps = 30
	}
	toolTimeout := cfg.DefaultToolTimeout
	if toolTimeout <= 0 {
		toolTimeout = 30 * time.Second
	}
	var (
		agentName        = cfg.DefaultAgent
		workspace        string
		systemPrompt     string
		totalTimeout     time.Duration
		thinkingFlag     = string(inference.Thinking)
		effortFlag       = string(inference.Effort)
		session          string
		continueLast     bool
		sessionsDir      string
		verbose          bool
		debugMessage     bool
		includeGlobal    bool
		discoverPaths    []string
		pluginNames      []string
		noDefaultPlugins = cfg.NoDefaultPlugins
		sourceAPIFlag    = cfg.Profile.Defaults.SourceAPI
		useCaseFlag      string
		approvedOnly     = cfg.Profile.Defaults.ModelPolicy.ApprovedOnly
		allowDegraded    = cfg.Profile.Defaults.ModelPolicy.AllowDegraded
		allowUntested    = cfg.Profile.Defaults.ModelPolicy.AllowUntested
		compatEvidence   = cfg.Profile.Defaults.ModelPolicy.EvidencePath
	)
	if cfg.Profile.Defaults.ModelPolicy.UseCase != "" {
		useCaseFlag = string(cfg.Profile.Defaults.ModelPolicy.UseCase)
	}
	if cfg.Profile.Defaults.ModelPolicy.SourceAPI != "" {
		sourceAPIFlag = agentconfig.FormatSourceAPI(cfg.Profile.Defaults.ModelPolicy.SourceAPI)
	}
	cmd := &cobra.Command{
		Use:           cfg.Use,
		Short:         cfg.Short,
		Long:          cfg.Long,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resources := cfg.Resources
			taskArgs := args
			if cfg.DiscoverFlag {
				// All positional args are the task; discovery roots come from -d flags.
				paths := discoverPaths
				if len(paths) == 0 {
					paths = []string{"."}
				}
				if cfg.EmbeddedBase != nil {
					resources = EmbeddedWithDirResources(cfg.EmbeddedBase, cfg.EmbeddedBaseRoot, paths)
				} else {
					resources = MultiDirResources(paths)
				}
			} else if cfg.ResourceArg {
				// Legacy: first positional arg is the resource path.
				resourcePath := "."
				if len(args) > 0 {
					resourcePath = args[0]
					taskArgs = args[1:]
				}
				resources = DirResources(resourcePath)
			}
			if resources == nil {
				return fmt.Errorf("cli: resources are required")
			}
			if thinkingFlag != "" {
				inference.Thinking = agentconfig.ThinkingMode(thinkingFlag)
			}
			if effortFlag != "" {
				inference.Effort = unified.ReasoningEffort(effortFlag)
			}
			flags := cmd.Flags()
			modelPolicy := cfg.Profile.Defaults.ModelPolicy
			applyModelPolicy := modelPolicy.Configured() ||
				flags.Changed("model-use-case") ||
				flags.Changed("model-approved-only") ||
				flags.Changed("model-allow-degraded") ||
				flags.Changed("model-allow-untested") ||
				flags.Changed("model-compat-evidence")
			if applyModelPolicy {
				if useCaseFlag != "" {
					useCase, err := agentconfig.ParseModelUseCase(useCaseFlag)
					if err != nil {
						return err
					}
					modelPolicy.UseCase = useCase
				}
				modelPolicy.ApprovedOnly = approvedOnly
				modelPolicy.AllowDegraded = allowDegraded
				modelPolicy.AllowUntested = allowUntested
				modelPolicy.EvidencePath = compatEvidence
				if modelPolicy.ApprovedOnly && modelPolicy.UseCase == "" {
					modelPolicy.UseCase = agentconfig.ModelUseCaseAgenticCoding
				}
			}
			if flags.Changed("source-api") && applyModelPolicy {
				sourceAPI, err := agentconfig.ParseSourceAPI(sourceAPIFlag)
				if err != nil {
					return err
				}
				modelPolicy.SourceAPI = sourceAPI
			}
			applyInference := cfg.ApplyDefaultInference ||
				flags.Changed("model") ||
				flags.Changed("max-tokens") ||
				flags.Changed("temperature") ||
				flags.Changed("thinking") ||
				flags.Changed("effort")
			runCfg := Config{
				Resources:          resources,
				AgentName:          agentName,
				Task:               strings.Join(taskArgs, " "),
				Workspace:          workspace,
				SessionsDir:        sessionsDir,
				DefaultSessionsDir: cfg.DefaultSessionsDir,
				Session:            session,
				ContinueLast:       continueLast,
				Inference:          inference,
				ApplyInference:     applyInference,
				SourceAPI:          sourceAPIFlag,
				ApplySourceAPI:     flags.Changed("source-api"),
				ModelPolicy:        modelPolicy,
				ApplyModelPolicy:   applyModelPolicy,
				MaxSteps:           maxSteps,
				ApplyMaxSteps:      cfg.ApplyDefaultMaxSteps || flags.Changed("max-steps"),
				SystemOverride:     systemPrompt,
				ToolTimeout:        toolTimeout,
				TotalTimeout:       totalTimeout,
				Verbose:            verbose,
				DebugMessage:       debugMessage,
				Prompt:             cfg.Prompt,
				AgentOptions:       append([]agent.Option(nil), cfg.AgentOptions...),
				AppOptions:         append([]app.Option(nil), cfg.AppOptions...),
				PluginNames:        append([]string(nil), pluginNames...),
				NoDefaultPlugins:   noDefaultPlugins,
				PluginFactory:      cfg.PluginFactory,
				DiscoveryPolicy:    cfg.DiscoveryPolicy,
				In:                 firstReader(cfg.In, os.Stdin),
				Out:                firstWriter(cfg.Out, cmd.OutOrStdout()),
				Err:                firstWriter(cfg.Err, cmd.ErrOrStderr()),
			}
			runCfg.DiscoveryPolicy.IncludeGlobalUserResources = includeGlobal || runCfg.DiscoveryPolicy.IncludeGlobalUserResources
			return Run(context.Background(), runCfg)
		},
	}
	addCoreFlags(cmd, cfg, &agentName, &workspace, &systemPrompt)
	addResourceFlags(cmd, cfg, &includeGlobal, &discoverPaths, &pluginNames, &noDefaultPlugins)
	addInferenceFlags(cmd, cfg, &inference, &thinkingFlag, &effortFlag, &sourceAPIFlag)
	addRuntimeFlags(cmd, cfg, &maxSteps, &totalTimeout, &toolTimeout)
	addSessionFlags(cmd, cfg, &session, &continueLast, &sessionsDir)
	addModelCompatibilityFlags(cmd, cfg, &useCaseFlag, &approvedOnly, &allowDegraded, &allowUntested, &compatEvidence)
	addDebugFlags(cmd, cfg, &verbose, &debugMessage)
	applyProfileFlagVisibility(cmd, cfg.Profile)
	_ = cmd.RegisterFlagCompletionFunc("model", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeModels(cfg.ModelCompleter, toComplete), cobra.ShellCompDirectiveNoFileComp
	})
	installGroupedHelp(cmd)
	cmd.AddCommand(CompletionCommand(cmd, cfg.Name))
	return cmd
}

func applyProfileDefaults(cfg CommandConfig) CommandConfig {
	defaults := cfg.Profile.Defaults
	if defaults.MaxSteps > 0 {
		cfg.DefaultMaxSteps = defaults.MaxSteps
	}
	if defaults.ToolTimeout > 0 {
		cfg.DefaultToolTimeout = defaults.ToolTimeout
	}
	if defaults.Prompt != "" && cfg.Prompt == "" {
		cfg.Prompt = defaults.Prompt
	}
	return cfg
}

func applyProfileInferenceDefaults(inference *agentconfig.InferenceOptions, defaults Defaults) {
	if inference == nil {
		return
	}
	if defaults.Model != "" {
		inference.Model = defaults.Model
	}
	if defaults.MaxTokens > 0 {
		inference.MaxTokens = defaults.MaxTokens
	}
	if defaults.Thinking != "" {
		inference.Thinking = defaults.Thinking
	}
	if defaults.Effort != "" {
		inference.Effort = unified.ReasoningEffort(defaults.Effort)
	}
	if defaults.Temperature != nil {
		inference.Temperature = *defaults.Temperature
	}
}

func addCoreFlags(cmd *cobra.Command, cfg CommandConfig, agentName *string, workspace *string, systemPrompt *string) {
	if !cfg.Profile.groupEnabled(GroupCore) {
		return
	}
	f := cmd.Flags()
	var names []string
	if (cfg.AgentFlag || cfg.ResourceArg || cfg.DiscoverFlag) && !cfg.Profile.flagDisabled("agent") {
		f.StringVar(agentName, "agent", *agentName, "Agent name to run")
		names = append(names, "agent")
	}
	if !cfg.Profile.flagDisabled("workspace") {
		f.StringVarP(workspace, "workspace", "w", "", "Working directory (default: $PWD)")
		names = append(names, "workspace")
	}
	if !cfg.Profile.flagDisabled("system") {
		f.StringVarP(systemPrompt, "system", "s", "", "Override the system prompt body")
		names = append(names, "system")
	}
	annotateFlags(cmd, GroupCore, names...)
}

func addResourceFlags(cmd *cobra.Command, cfg CommandConfig, includeGlobal *bool, discoverPaths *[]string, pluginNames *[]string, noDefaultPlugins *bool) {
	if !cfg.Profile.groupEnabled(GroupResources) {
		return
	}
	f := cmd.Flags()
	var names []string
	if cfg.DiscoverFlag && !cfg.Profile.flagDisabled("discover") {
		f.StringSliceVarP(discoverPaths, "discover", "d", nil, "Discovery root directory (repeatable; default: current directory)")
		names = append(names, "discover")
	}
	if !cfg.Profile.flagDisabled("include-global") {
		f.BoolVar(includeGlobal, "include-global", false, "Load ~/.agents and ~/.claude resources")
		names = append(names, "include-global")
	}
	if !cfg.Profile.flagDisabled("plugin") {
		f.StringSliceVar(pluginNames, "plugin", nil, "Activate named app plugin (repeatable)")
		names = append(names, "plugin")
	}
	if !cfg.Profile.flagDisabled("no-default-plugins") {
		f.BoolVar(noDefaultPlugins, "no-default-plugins", *noDefaultPlugins, "Disable the built-in local_cli fallback plugin")
		names = append(names, "no-default-plugins")
	}
	annotateFlags(cmd, GroupResources, names...)
}

func addInferenceFlags(cmd *cobra.Command, cfg CommandConfig, inference *agentconfig.InferenceOptions, thinkingFlag *string, effortFlag *string, sourceAPIFlag *string) {
	if !cfg.Profile.groupEnabled(GroupInference) {
		return
	}
	f := cmd.Flags()
	var names []string
	if !cfg.Profile.flagDisabled("model") {
		f.StringVarP(&inference.Model, "model", "m", inference.Model, "Model alias or full path")
		names = append(names, "model")
	}
	if !cfg.Profile.flagDisabled("max-tokens") {
		f.IntVar(&inference.MaxTokens, "max-tokens", inference.MaxTokens, "Maximum output tokens per LLM call")
		names = append(names, "max-tokens")
	}
	if !cfg.Profile.flagDisabled("temperature") {
		f.Float64Var(&inference.Temperature, "temperature", inference.Temperature, "Sampling temperature 0.0-2.0")
		names = append(names, "temperature")
	}
	if !cfg.Profile.flagDisabled("thinking") {
		f.StringVar(thinkingFlag, "thinking", *thinkingFlag, "Thinking mode: auto|on|off")
		names = append(names, "thinking")
	}
	if !cfg.Profile.flagDisabled("effort") {
		f.StringVar(effortFlag, "effort", *effortFlag, "Effort level: low|medium|high")
		names = append(names, "effort")
	}
	if !cfg.Profile.flagDisabled("source-api") {
		f.StringVar(sourceAPIFlag, "source-api", *sourceAPIFlag, "Source API: auto|openai.responses|openai.chat_completions|anthropic.messages")
		names = append(names, "source-api")
	}
	annotateFlags(cmd, GroupInference, names...)
}

func addRuntimeFlags(cmd *cobra.Command, cfg CommandConfig, maxSteps *int, totalTimeout *time.Duration, toolTimeout *time.Duration) {
	if !cfg.Profile.groupEnabled(GroupRuntime) {
		return
	}
	f := cmd.Flags()
	var names []string
	if !cfg.Profile.flagDisabled("max-steps") {
		f.IntVar(maxSteps, "max-steps", *maxSteps, "Maximum agent loop iterations per turn")
		names = append(names, "max-steps")
	}
	if !cfg.Profile.flagDisabled("timeout") {
		f.DurationVar(totalTimeout, "timeout", 0, "Total runtime timeout for one-shot mode (0 = no limit)")
		names = append(names, "timeout")
	}
	if !cfg.Profile.flagDisabled("tool-timeout") {
		f.DurationVar(toolTimeout, "tool-timeout", *toolTimeout, "Per-tool call timeout")
		names = append(names, "tool-timeout")
	}
	annotateFlags(cmd, GroupRuntime, names...)
}

func addSessionFlags(cmd *cobra.Command, cfg CommandConfig, session *string, continueLast *bool, sessionsDir *string) {
	if !cfg.Profile.groupEnabled(GroupSession) {
		return
	}
	f := cmd.Flags()
	var names []string
	if !cfg.Profile.flagDisabled("session") {
		f.StringVar(session, "session", "", "Resume a session by id or JSONL path")
		names = append(names, "session")
	}
	if !cfg.Profile.flagDisabled("continue") {
		f.BoolVar(continueLast, "continue", false, "Resume the most recently active session")
		names = append(names, "continue")
	}
	if !cfg.Profile.flagDisabled("sessions-dir") {
		f.StringVar(sessionsDir, "sessions-dir", "", "Session storage directory")
		names = append(names, "sessions-dir")
	}
	annotateFlags(cmd, GroupSession, names...)
}

func addModelCompatibilityFlags(cmd *cobra.Command, cfg CommandConfig, useCaseFlag *string, approvedOnly *bool, allowDegraded *bool, allowUntested *bool, compatEvidence *string) {
	if !cfg.Profile.groupEnabled(GroupModelCompatibility) {
		return
	}
	f := cmd.Flags()
	var names []string
	if !cfg.Profile.flagDisabled("model-use-case") {
		f.StringVar(useCaseFlag, "model-use-case", *useCaseFlag, "Model compatibility use case: agentic_coding|summarization")
		names = append(names, "model-use-case")
	}
	if !cfg.Profile.flagDisabled("model-approved-only") {
		f.BoolVar(approvedOnly, "model-approved-only", *approvedOnly, "Require an approved model/provider route for the selected use case")
		names = append(names, "model-approved-only")
	}
	if !cfg.Profile.flagDisabled("model-allow-degraded") {
		f.BoolVar(allowDegraded, "model-allow-degraded", *allowDegraded, "Allow degraded model compatibility evidence for approved-only routing")
		names = append(names, "model-allow-degraded")
	}
	if !cfg.Profile.flagDisabled("model-allow-untested") {
		f.BoolVar(allowUntested, "model-allow-untested", *allowUntested, "Allow untested model compatibility evidence for approved-only routing")
		names = append(names, "model-allow-untested")
	}
	if !cfg.Profile.flagDisabled("model-compat-evidence") {
		f.StringVar(compatEvidence, "model-compat-evidence", *compatEvidence, "Model compatibility evidence JSON path")
		names = append(names, "model-compat-evidence")
	}
	annotateFlags(cmd, GroupModelCompatibility, names...)
}

func addDebugFlags(cmd *cobra.Command, cfg CommandConfig, verbose *bool, debugMessage *bool) {
	if !cfg.Profile.groupEnabled(GroupDebug) {
		return
	}
	f := cmd.Flags()
	var names []string
	if !cfg.Profile.flagDisabled("verbose") {
		f.BoolVarP(verbose, "verbose", "v", false, "Show resolved provider/model diagnostics")
		names = append(names, "verbose")
	}
	if !cfg.Profile.flagDisabled("debug-message") {
		f.BoolVar(debugMessage, "debug-message", false, "Render the messages that would be sent and exit without calling the model")
		names = append(names, "debug-message")
	}
	annotateFlags(cmd, GroupDebug, names...)
}

func CompletionCommand(root *cobra.Command, name string) *cobra.Command {
	if name == "" {
		name = root.Name()
	}
	cmd := &cobra.Command{Use: "completion", Short: "Generate or install shell completion", SilenceUsage: true, SilenceErrors: true}
	cmd.AddCommand(&cobra.Command{Use: "bash", Short: "Generate bash completion script", RunE: func(cmd *cobra.Command, _ []string) error {
		return root.GenBashCompletionV2(cmd.OutOrStdout(), true)
	}})
	cmd.AddCommand(&cobra.Command{Use: "zsh", Short: "Generate zsh completion script", RunE: func(cmd *cobra.Command, _ []string) error {
		return root.GenZshCompletion(cmd.OutOrStdout())
	}})
	cmd.AddCommand(&cobra.Command{Use: "fish", Short: "Generate fish completion script", RunE: func(cmd *cobra.Command, _ []string) error {
		return root.GenFishCompletion(cmd.OutOrStdout(), true)
	}})
	cmd.AddCommand(completionInstallCmd(root, name))
	return cmd
}

func completionInstallCmd(root *cobra.Command, name string) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:           "install [bash|zsh|fish]",
		Short:         "Install shell completion to a standard user location",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := ""
			if len(args) == 1 {
				shell = strings.ToLower(args[0])
			} else {
				shell = detectShell()
			}
			if shell == "" {
				return fmt.Errorf("unable to detect shell; pass bash, zsh, or fish")
			}
			target, err := completionInstallPath(shell, name, file)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create completion directory: %w", err)
			}
			f, err := os.Create(target)
			if err != nil {
				return fmt.Errorf("create completion file: %w", err)
			}
			defer f.Close()
			switch shell {
			case "bash":
				err = root.GenBashCompletionV2(f, true)
			case "zsh":
				err = root.GenZshCompletion(f)
			case "fish":
				err = root.GenFishCompletion(f, true)
			default:
				return fmt.Errorf("unsupported shell %q", shell)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Installed %s completion to %s\n", shell, target)
			fmt.Fprintln(cmd.OutOrStdout(), "Restart your shell or source the file to enable completions.")
			return nil
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Override installation target file")
	return cmd
}

func DefaultModelCompleter(toComplete string) []string {
	return completeModels(nil, toComplete)
}

func completeModels(completer ModelCompleter, toComplete string) []string {
	if completer != nil {
		return completer(toComplete)
	}
	models := []string{"default", "fast", "powerful", "codex/gpt-5.5"}
	var matches []string
	for _, model := range models {
		if strings.Contains(strings.ToLower(model), strings.ToLower(toComplete)) {
			matches = append(matches, model)
		}
	}
	return matches
}

func detectShell() string {
	shell := strings.ToLower(filepath.Base(os.Getenv("SHELL")))
	switch shell {
	case "bash", "zsh", "fish":
		return shell
	default:
		return ""
	}
}

func completionInstallPath(shell, name, override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get home directory: %w", err)
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".local/share/bash-completion/completions", name), nil
	case "zsh":
		return filepath.Join(home, ".zsh/completions", "_"+name), nil
	case "fish":
		return filepath.Join(home, ".config/fish/completions", name+".fish"), nil
	default:
		return "", fmt.Errorf("unsupported shell %q", shell)
	}
}

func firstReader(primary io.Reader, fallback io.Reader) io.Reader {
	if primary != nil {
		return primary
	}
	return fallback
}

func firstWriter(primary io.Writer, fallback io.Writer) io.Writer {
	if primary != nil {
		return primary
	}
	return fallback
}
