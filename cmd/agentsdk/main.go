package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/codewandler/markdown"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/appconfig"
	builderapp "github.com/codewandler/agentsdk/apps/builder"
	engineerapp "github.com/codewandler/agentsdk/apps/engineer"
	"github.com/codewandler/agentsdk/apps/runapp"
	"github.com/codewandler/agentsdk/daemon"
	"github.com/codewandler/agentsdk/plugins/configplugin"
	"github.com/codewandler/agentsdk/resource"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	"github.com/codewandler/agentsdk/terminal/cli"
	"github.com/codewandler/agentsdk/trigger"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/compatibility"
	"github.com/spf13/cobra"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cmd := rootCmd()
	cmd.SetArgs(args)
	return cmd.Execute()
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "agentsdk",
		Short:         "Run agentsdk resource bundles",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cli.Mount(cmd,
		runapp.Spec(),
		engineerapp.Spec(),
		builderapp.Spec(),
	)
	cmd.AddCommand(serveCmd())
	cmd.AddCommand(discoverCmd())
	cmd.AddCommand(configCmd())
	cmd.AddCommand(validateCmd())
	cmd.AddCommand(modelsCmd())
	cmd.AddCommand(toolCmd())
	return cmd
}

func configCmd() *cobra.Command {
	var sources []string
	cmd := &cobra.Command{
		Use:           "config",
		Short:         "Inspect application configuration",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	printCmd := &cobra.Command{
		Use:           "print [path]",
		Short:         "Print the expanded configuration as YAML",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := loadConfig(args, sources)
			if err != nil {
				return err
			}
			return printConfigYAML(cmd.OutOrStdout(), result)
		},
	}

	validateSubCmd := &cobra.Command{
		Use:           "validate [path]",
		Short:         "Validate configuration for structural correctness",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := loadConfig(args, sources)
			if err != nil {
				return fmt.Errorf("config validation failed: %w", err)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Config: %s\n", result.EntryPath)
			fmt.Fprintf(out, "Name: %s\n", result.Config.Name)
			fmt.Fprintf(out, "Default agent: %s\n", result.Config.DefaultAgent)
			fmt.Fprintf(out, "Agents: %d\n", len(result.Agents))
			fmt.Fprintf(out, "Commands: %d\n", len(result.Commands))
			fmt.Fprintf(out, "Workflows: %d\n", len(result.Workflows))
			fmt.Fprintf(out, "Actions: %d\n", len(result.Actions))
			fmt.Fprintf(out, "Datasources: %d\n", len(result.Datasources))
			fmt.Fprintf(out, "Triggers: %d\n", len(result.Triggers))
			fmt.Fprintf(out, "Sources: %d\n", len(result.Config.Sources))
			fmt.Fprintf(out, "Plugins: %d\n", len(result.Config.Plugins))
			fmt.Fprintln(out, "\n\u2713 configuration is valid")
			return nil
		},
	}

	var discoverOutputFormat string
	discoverSubCmd := &cobra.Command{
		Use:           "discover [path]",
		Short:         "Discover configured resources and ResourceID diagnostics",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := loadConfig(args, sources)
			if err != nil {
				return err
			}
			resolved, err := configplugin.ResolutionFromAppConfig(result)
			if err != nil {
				return err
			}
			switch discoverOutputFormat {
			case "json":
				return configplugin.PrintDiscoveryJSON(cmd.OutOrStdout(), resolved)
			case "yaml":
				return configplugin.PrintDiscoveryYAML(cmd.OutOrStdout(), resolved)
			default:
				return configplugin.PrintDiscoveryTree(cmd.OutOrStdout(), resolved)
			}
		},
	}
	var outDir string
	schemaCmd := &cobra.Command{
		Use:           "schema",
		Short:         "Print the app config JSON Schema",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			data, err := appconfig.GenerateJSONSchemaBytes()
			if err != nil {
				return fmt.Errorf("generating schema: %w", err)
			}
			if outDir != "" {
				if err := os.MkdirAll(outDir, 0o755); err != nil {
					return fmt.Errorf("creating output directory: %w", err)
				}
				path := filepath.Join(outDir, "agentsdk.schema.json")
				if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
					return fmt.Errorf("writing schema: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Schema written to %s\n", path)
				return nil
			}
			_, err = cmd.OutOrStdout().Write(append(data, '\n'))
			return err
		},
	}
	schemaCmd.Flags().StringVar(&outDir, "out-dir", "", "Write schema to <out-dir>/agentsdk.schema.json instead of stdout")

	printCmd.Flags().StringSliceVar(&sources, "source", nil, "Additional source file(s) to load (repeatable)")
	validateSubCmd.Flags().StringSliceVar(&sources, "source", nil, "Additional source file(s) to load (repeatable)")

	discoverSubCmd.Flags().StringVarP(&discoverOutputFormat, "output", "o", "pretty", "Output format: pretty|json|yaml")
	discoverSubCmd.Flags().StringSliceVar(&sources, "source", nil, "Additional source file(s) to load (repeatable)")
	cmd.AddCommand(printCmd)
	cmd.AddCommand(validateSubCmd)
	cmd.AddCommand(schemaCmd)
	cmd.AddCommand(discoverSubCmd)
	return cmd
}

func loadConfig(args []string, extraSources []string) (appconfig.LoadResult, error) {
	dir := "."
	var opts []appconfig.LoadOption
	if len(args) == 1 {
		arg := args[0]
		if info, err := os.Stat(arg); err == nil && !info.IsDir() {
			dir = filepath.Dir(arg)
			opts = append(opts, appconfig.WithConfigRoots(arg))
		} else {
			dir = arg
		}
	}
	opts = append(opts, appconfig.WithWorkDir(dir), appconfig.WithoutUserConfig())
	if len(extraSources) > 0 {
		opts = append(opts, appconfig.WithConfigRoots(extraSources...))
	}
	return appconfig.NewLoader().Load(opts...)
}

func printConfigYAML(out io.Writer, result appconfig.LoadResult) error {
	var buf strings.Builder
	fmt.Fprintf(&buf, "# Config: %s\n\n", result.EntryPath)
	fmt.Fprintln(&buf, "```yaml")
	docs, err := result.MaterializedDocuments()
	if err != nil {
		return err
	}
	for i, doc := range docs {
		if i > 0 {
			fmt.Fprintln(&buf, "---")
		}
		enc := yaml.NewEncoder(&buf)
		enc.SetIndent(2)
		if err := enc.Encode(doc); err != nil {
			return err
		}
		if err := enc.Close(); err != nil {
			return err
		}
	}
	fmt.Fprintln(&buf, "```")
	return markdown.RenderToWriter(out, buf.String())
}

func serveCmd() *cobra.Command {
	var (
		agentName        string
		workspace        string
		sessionsDir      string
		sessionName      string
		statusOnly       bool
		noDefaultPlugins bool
		pluginNames      []string
		triggerInterval  time.Duration
		triggerWorkflow  string
		triggerPrompt    string
		triggerInput     string
	)
	cmd := &cobra.Command{
		Use:           "serve [path]",
		Short:         "Run an agentsdk harness service host",
		Long:          "Run agentsdk as a long-running harness service host. The service host owns process lifecycle and storage conventions while harness.Service remains the runtime/session owner.",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			resourcePath := "."
			if len(args) == 1 {
				resourcePath = args[0]
			}
			if sessionsDir == "" {
				sessionsDir = defaultServeSessionsDir(resourcePath)
			}
			loaded, err := cli.Load(cmd.Context(), cli.Config{
				Resources:        cli.DirResources(resourcePath),
				AgentName:        agentName,
				Workspace:        workspace,
				SessionsDir:      sessionsDir,
				PluginNames:      pluginNames,
				NoDefaultPlugins: noDefaultPlugins,
				In:               cmd.InOrStdin(),
				Out:              cmd.OutOrStdout(),
				Err:              cmd.ErrOrStderr(),
			})
			if err != nil {
				return err
			}
			host, err := daemon.New(daemon.Config{Service: loaded.Harness, SessionsDir: sessionsDir, ConfigPath: filepath.Join(resourcePath, "agentsdk.app.json")})
			if err != nil {
				return err
			}
			if sessionName != "" && loaded.Session != nil {
				loaded.Session.Name = sessionName
			}
			if err := addResourceTriggers(cmd.Context(), host, loaded.Resources.Bundle.Triggers, loaded.AgentName); err != nil {
				return err
			}
			if triggerInterval > 0 {
				rule, err := serveTriggerRule(triggerInterval, triggerWorkflow, triggerPrompt, triggerInput, agentName)
				if err != nil {
					return err
				}
				events, cancelEvents := host.TriggerEvents(16)
				defer cancelEvents()
				if err := host.AddTrigger(cmd.Context(), rule); err != nil {
					return err
				}
				if statusOnly {
					for event := range events {
						if event.Type == trigger.JobEventCompleted || event.Type == trigger.JobEventFailed || event.Type == trigger.JobEventSkipped {
							break
						}
					}
				}
			}
			printServeStatus(cmd.OutOrStdout(), host.Status())
			if statusOnly {
				return host.Shutdown(cmd.Context())
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			<-ctx.Done()
			if err := host.Shutdown(context.Background()); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "agentsdk service stopped")
			return nil
		},
	}
	cmd.Flags().StringVar(&agentName, "agent", "", "Agent name to host")
	cmd.Flags().StringVarP(&workspace, "workspace", "w", "", "Working directory (default: $PWD)")
	cmd.Flags().StringVar(&sessionsDir, "sessions-dir", "", "Session storage directory (default: <path>/.agentsdk/sessions)")
	cmd.Flags().StringVar(&sessionName, "session-name", "", "Registry name to report for the opened service session")
	cmd.Flags().BoolVar(&statusOnly, "status", false, "Print service status and exit without waiting")
	cmd.Flags().StringSliceVar(&pluginNames, "plugin", nil, "Activate named app plugin (repeatable)")
	cmd.Flags().BoolVar(&noDefaultPlugins, "no-default-plugins", false, "Disable the built-in local_cli fallback plugin")
	cmd.Flags().DurationVar(&triggerInterval, "trigger-interval", 0, "Start an interval trigger for smoke/service use")
	cmd.Flags().StringVar(&triggerWorkflow, "trigger-workflow", "", "Workflow to start for --trigger-interval")
	cmd.Flags().StringVar(&triggerPrompt, "trigger-prompt", "", "Agent prompt to run for --trigger-interval")
	cmd.Flags().StringVar(&triggerInput, "trigger-input", "", "Input for --trigger-workflow")
	return cmd
}

func defaultServeSessionsDir(resourcePath string) string {
	if strings.TrimSpace(resourcePath) == "" {
		resourcePath = "."
	}
	return filepath.Join(resourcePath, ".agentsdk", "sessions")
}

func printServeStatus(out io.Writer, status daemon.Status) {
	fmt.Fprintln(out, "agentsdk service")
	fmt.Fprintf(out, "mode: %s\n", status.Mode)
	fmt.Fprintf(out, "health: %s\n", status.Health)
	if status.Storage.SessionsDir != "" {
		fmt.Fprintf(out, "sessions: %s\n", status.Storage.SessionsDir)
	}
	fmt.Fprintf(out, "active_sessions: %d\n", status.ActiveSessions)
	for _, session := range status.Sessions {
		fmt.Fprintf(out, "- %s id=%s agent=%s thread_backed=%t\n", session.Name, session.SessionID, session.AgentName, session.ThreadBacked)
	}
	if len(status.Jobs) > 0 {
		fmt.Fprintf(out, "jobs: %d\n", len(status.Jobs))
		for _, job := range status.Jobs {
			fmt.Fprintf(out, "- job %s status=%s target=%s:%s matched=%d skipped=%d\n", job.RuleID, job.Status, job.TargetKind, job.TargetName, job.Matched, job.Skipped)
		}
	}
}

func serveTriggerRule(every time.Duration, workflowName, prompt, input, agentName string) (trigger.Rule, error) {
	if every <= 0 {
		return trigger.Rule{}, fmt.Errorf("--trigger-interval must be positive")
	}
	if workflowName == "" && prompt == "" {
		return trigger.Rule{}, fmt.Errorf("--trigger-interval requires --trigger-workflow or --trigger-prompt")
	}
	if workflowName != "" && prompt != "" {
		return trigger.Rule{}, fmt.Errorf("--trigger-workflow and --trigger-prompt cannot be used together")
	}
	target := trigger.Target{Kind: trigger.TargetAgentPrompt, AgentName: agentName, Prompt: prompt, Input: input}
	if workflowName != "" {
		target = trigger.Target{Kind: trigger.TargetWorkflow, WorkflowName: workflowName, AgentName: agentName, Input: input}
	}
	return trigger.Rule{
		ID:      "cli-interval",
		Source:  trigger.IntervalSource{SourceID: "cli-interval", Every: every, Immediate: true},
		Matcher: trigger.All{trigger.EventType(trigger.EventTypeInterval), trigger.SourceIs("cli-interval")},
		Target:  target,
		Session: trigger.SessionPolicy{Mode: trigger.SessionTriggerOwned, AgentName: agentName},
		Policy:  trigger.JobPolicy{Overlap: trigger.OverlapSkipIfRunning},
	}, nil
}

func addResourceTriggers(ctx context.Context, host *daemon.Host, contributions []resource.TriggerContribution, agentName string) error {
	for _, contribution := range contributions {
		rule, err := triggerRuleFromContribution(contribution, agentName)
		if err != nil {
			return err
		}
		if err := host.AddTrigger(ctx, rule); err != nil {
			return err
		}
	}
	return nil
}

func triggerRuleFromContribution(contribution resource.TriggerContribution, agentName string) (trigger.Rule, error) {
	def := contribution.Definition
	sourceDef := mapFromAny(def["source"])
	interval := stringFromMap(sourceDef, "interval")
	if interval == "" {
		return trigger.Rule{}, fmt.Errorf("trigger %q: source.interval is required", contribution.Name)
	}
	every, err := time.ParseDuration(interval)
	if err != nil {
		return trigger.Rule{}, fmt.Errorf("trigger %q: parse source.interval: %w", contribution.Name, err)
	}
	targetDef := mapFromAny(def["target"])
	target := trigger.Target{Input: targetDef["input"], IncludeEvent: boolFromMap(targetDef, "include_event")}
	if workflowName := stringFromMap(targetDef, "workflow"); workflowName != "" {
		target.Kind = trigger.TargetWorkflow
		target.WorkflowName = workflowName
	} else if prompt := stringFromMap(targetDef, "prompt"); prompt != "" {
		target.Kind = trigger.TargetAgentPrompt
		target.Prompt = prompt
	} else if actionName := stringFromMap(targetDef, "action"); actionName != "" {
		target.Kind = trigger.TargetAction
		target.ActionName = actionName
	} else {
		return trigger.Rule{}, fmt.Errorf("trigger %q: target.workflow or target.prompt is required", contribution.Name)
	}
	target.AgentName = firstNonEmpty(stringFromMap(targetDef, "agent"), agentName)
	sessionDef := mapFromAny(def["session"])
	mode := trigger.SessionMode(stringFromMap(sessionDef, "mode"))
	if mode == "" {
		mode = trigger.SessionTriggerOwned
	}
	policyDef := mapFromAny(def["policy"])
	overlap := trigger.OverlapPolicy(stringFromMap(policyDef, "overlap"))
	if overlap == "" {
		overlap = trigger.OverlapSkipIfRunning
	}
	ruleID := contribution.Name
	return trigger.Rule{
		ID:      trigger.RuleID(ruleID),
		Source:  trigger.IntervalSource{SourceID: trigger.SourceID(ruleID), Every: every, Immediate: boolFromMap(sourceDef, "immediate")},
		Matcher: trigger.All{trigger.EventType(trigger.EventTypeInterval), trigger.SourceIs(trigger.SourceID(ruleID))},
		Target:  target,
		Session: trigger.SessionPolicy{Mode: mode, Name: stringFromMap(sessionDef, "name"), AgentName: firstNonEmpty(stringFromMap(sessionDef, "agent"), target.AgentName)},
		Policy:  trigger.JobPolicy{Overlap: overlap, Timeout: durationFromMap(policyDef, "timeout")},
	}, nil
}

func mapFromAny(raw any) map[string]any {
	m, _ := raw.(map[string]any)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func stringFromMap(m map[string]any, key string) string {
	value, _ := m[key].(string)
	return strings.TrimSpace(value)
}

func boolFromMap(m map[string]any, key string) bool {
	value, _ := m[key].(bool)
	return value
}

func durationFromMap(m map[string]any, key string) time.Duration {
	value := stringFromMap(m, key)
	if value == "" {
		return 0
	}
	d, _ := time.ParseDuration(value)
	return d
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func toolCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "tool",
		Short:         "Inspect tools registered in a resource bundle",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(toolSchemaCmd())
	return cmd
}

func toolSchemaCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "schema [path] [name]",
		Short:         "Print tool JSON schemas as YAML",
		Long:          "Print the JSON schema of every registered tool as YAML.\nOptionally filter to a single tool by name.\npath defaults to the current directory.",
		Args:          cobra.MaximumNArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			toolName := ""
			switch len(args) {
			case 1:
				if looksLikePath(args[0]) {
					dir = args[0]
				} else {
					toolName = args[0]
				}
			case 2:
				dir = args[0]
				toolName = args[1]
			}
			policy := resource.DiscoveryPolicy{
				IncludeGlobalUserResources: true,
				IncludeExternalEcosystems:  true,
				AllowRemote:                true,
			}
			resolved, err := agentdir.ResolveDirWithOptions(dir, agentdir.ResolveOptions{Policy: policy})
			if err != nil {
				return err
			}
			imported, err := app.New(app.WithResourceBundle(resolved.Bundle))
			if err != nil {
				return err
			}
			catalog := imported.ToolCatalog()
			var names []string
			if toolName != "" {
				names = []string{toolName}
			} else {
				names = catalog.Names()
			}
			out := cmd.OutOrStdout()
			for i, name := range names {
				selected, err := catalog.Select([]string{name})
				if err != nil {
					return fmt.Errorf("tool %q: %w", name, err)
				}
				if len(selected) == 0 {
					return fmt.Errorf("tool %q not found", name)
				}
				t := selected[0]
				schema := t.Schema()
				if schema == nil {
					continue
				}
				raw, err := json.Marshal(schema)
				if err != nil {
					return fmt.Errorf("tool %q: marshal schema: %w", name, err)
				}
				var shaped any
				if err := yaml.Unmarshal(raw, &shaped); err != nil {
					return fmt.Errorf("tool %q: yaml unmarshal: %w", name, err)
				}
				var yamlBuf strings.Builder
				enc := yaml.NewEncoder(&yamlBuf)
				enc.SetIndent(2)
				if err := enc.Encode(shaped); err != nil {
					return fmt.Errorf("tool %q: yaml marshal: %w", name, err)
				}
				yamlBytes := []byte(yamlBuf.String()) // encoder appends trailing newline; keep it for blank line before closing fence
				if i > 0 {
					fmt.Fprintln(out)
				}
				// Build Markdown source
				var md strings.Builder
				fmt.Fprintf(&md, "## %s\n\n", name)
				fmt.Fprintf(&md, "%s\n\n", t.Description())
				if guidance := t.Guidance(); guidance != "" {
					fmt.Fprintln(&md, "**Guidance:**")
					for _, line := range strings.Split(strings.TrimSpace(guidance), "\n") {
						fmt.Fprintf(&md, "- %s\n", line)
					}
					fmt.Fprintln(&md)
				}
				fmt.Fprintln(&md, "**Schema:**")
				fmt.Fprintln(&md, "```yaml")
				md.Write(yamlBytes)
				fmt.Fprintf(&md, "\n```\n")
				// Render through terminal Markdown renderer
				_ = markdown.RenderToWriter(out, md.String())
			}
			return nil
		},
	}
}

// looksLikePath returns true if s looks like a filesystem path rather than a
// tool name: starts with . or /, contains a path separator, or ends with /.
func looksLikePath(s string) bool {
	return strings.HasPrefix(s, ".") ||
		strings.HasPrefix(s, "/") ||
		strings.ContainsRune(s, os.PathSeparator)
}

func modelsCmd() *cobra.Command {
	var (
		sourceAPIFlag  string
		useCaseFlag    string
		approvedOnly   bool
		allowDegraded  bool
		allowUntested  bool
		compatEvidence string
		thinkingOnly   bool
	)
	cmd := &cobra.Command{
		Use:           "models [model]",
		Short:         "Inspect model compatibility candidates",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceAPI, err := agentconfig.ParseSourceAPI(sourceAPIFlag)
			if err != nil {
				return err
			}
			useCase := agentconfig.ModelUseCaseAgenticCoding
			if useCaseFlag != "" {
				useCase, err = agentconfig.ParseModelUseCase(useCaseFlag)
				if err != nil {
					return err
				}
			}
			policy := agentconfig.ModelPolicy{
				UseCase:       useCase,
				SourceAPI:     sourceAPI,
				ApprovedOnly:  approvedOnly,
				AllowDegraded: allowDegraded,
				AllowUntested: allowUntested,
				EvidencePath:  compatEvidence,
			}
			if len(args) == 0 {
				return runModelsCatalog(cmd.OutOrStdout(), policy, thinkingOnly)
			}
			model := args[0]
			result, err := agentruntime.AutoMuxClient(model, sourceAPI, nil)
			if err != nil {
				return err
			}
			evidence, source, evidenceErr := agent.LoadCompatibilityEvidence(policy)
			if evidenceErr == nil {
				selections, err := selectModelForInspection(result, model, sourceAPI, adapterconfig.UseCaseSelectionOptions{
					UseCase:       compatibility.UseCase(useCase),
					Evidence:      evidence,
					AllowDegraded: !approvedOnly || allowDegraded,
					AllowUntested: !approvedOnly || allowUntested,
				})
				if err == nil {
					if thinkingOnly {
						selections = thinkingModelSelections(selections)
					}
					return printApprovedModelSelections(cmd.OutOrStdout(), model, source, selections)
				}
				if approvedOnly {
					return err
				}
			} else if approvedOnly {
				return evidenceErr
			}
			evaluations, err := adapterconfig.EvaluateCompatibilityCandidates(result.Config, model, sourceAPI, compatibility.UseCase(useCase))
			if err != nil {
				return err
			}
			return printModelEvaluations(cmd.OutOrStdout(), model, evidenceErr, evaluations)
		},
	}
	cmd.Flags().StringVar(&sourceAPIFlag, "source-api", "auto", "Source API: auto|openai.responses|openai.chat_completions|anthropic.messages")
	cmd.Flags().StringVar(&useCaseFlag, "model-use-case", "agentic_coding", "Model compatibility use case: agentic_coding|summarization")
	cmd.Flags().BoolVar(&approvedOnly, "model-approved-only", false, "Only show candidates accepted by model compatibility evidence")
	cmd.Flags().BoolVar(&allowDegraded, "model-allow-degraded", false, "Allow degraded model compatibility evidence for approved-only output")
	cmd.Flags().BoolVar(&allowUntested, "model-allow-untested", false, "Allow untested model compatibility evidence for approved-only output")
	cmd.Flags().StringVar(&compatEvidence, "model-compat-evidence", "", "Model compatibility evidence JSON path")
	cmd.Flags().BoolVar(&thinkingOnly, "thinking", false, "Only show models with live reasoning evidence")
	cli.AnnotateFlagGroup(cmd, cli.GroupInference, "source-api")
	cli.AnnotateFlagGroup(cmd, cli.GroupModelCompatibility, "model-use-case", "model-approved-only", "model-allow-degraded", "model-allow-untested", "model-compat-evidence", "thinking")
	cli.InstallGroupedHelp(cmd)
	return cmd
}

func runModelsCatalog(out discoveryWriter, policy agentconfig.ModelPolicy, thinkingOnly bool) error {
	evidence, evidenceSource, err := agent.LoadCompatibilityEvidence(policy)
	if err != nil {
		return err
	}
	models := evidenceModels(evidence, thinkingOnly)
	opts := agentruntime.DefaultAutoOptions("", policy.SourceAPI)
	opts.Intents = make([]adapterconfig.AutoIntent, 0, len(models))
	for _, model := range models {
		opts.Intents = append(opts.Intents, adapterconfig.AutoIntent{Name: model, SourceAPI: policy.SourceAPI})
	}
	result, err := adapterconfig.AutoMuxClient(opts)
	if err != nil {
		return err
	}
	var selections []adapterconfig.UseCaseModelSelection
	for _, model := range models {
		got, err := result.SelectModelsForUseCase(model, policy.SourceAPI, adapterconfig.UseCaseSelectionOptions{
			UseCase:       compatibility.UseCase(policy.UseCase),
			Evidence:      evidence,
			AllowDegraded: !policy.ApprovedOnly || policy.AllowDegraded,
			AllowUntested: !policy.ApprovedOnly || policy.AllowUntested,
		})
		if err != nil {
			continue
		}
		if thinkingOnly {
			got = thinkingModelSelections(got)
		}
		selections = append(selections, got...)
	}
	selections = uniqueModelSelections(selections)
	return printApprovedModelSelections(out, "", evidenceSource, selections)
}

func selectModelForInspection(result adapterconfig.AutoResult, model string, sourceAPI adapt.ApiKind, opts adapterconfig.UseCaseSelectionOptions) ([]adapterconfig.UseCaseModelSelection, error) {
	var lastErr error
	for _, candidate := range inspectionModelNames(model) {
		selections, err := result.SelectModelsForUseCase(candidate, sourceAPI, opts)
		if err == nil {
			return selections, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func inspectionModelNames(model string) []string {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil
	}
	names := []string{model}
	if slash := strings.LastIndex(model, "/"); slash >= 0 && slash < len(model)-1 {
		names = append(names, model[slash+1:])
	}
	return names
}

func evidenceModels(evidence adapterconfig.CompatibilityEvidence, thinkingOnly bool) []string {
	seen := map[string]bool{}
	var out []string
	for _, row := range evidence.Rows {
		if thinkingOnly && row.Reasoning != string(compatibility.EvidenceLive) {
			continue
		}
		model := row.PublicModel
		if model == "" {
			model = row.NativeModel
		}
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, model)
	}
	sort.Strings(out)
	return out
}

func thinkingModelSelections(selections []adapterconfig.UseCaseModelSelection) []adapterconfig.UseCaseModelSelection {
	out := selections[:0]
	for _, selection := range selections {
		if selection.Evidence.Reasoning == string(compatibility.EvidenceLive) {
			out = append(out, selection)
		}
	}
	return out
}

func uniqueModelSelections(selections []adapterconfig.UseCaseModelSelection) []adapterconfig.UseCaseModelSelection {
	seen := map[string]bool{}
	out := make([]adapterconfig.UseCaseModelSelection, 0, len(selections))
	for _, selection := range selections {
		resolution := selection.Resolution
		key := strings.Join([]string{
			string(resolution.SourceAPI),
			resolution.PublicModel,
			resolution.Provider,
			string(resolution.ProviderAPI),
			resolution.NativeModel,
		}, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, selection)
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i].Resolution
		right := out[j].Resolution
		if left.PublicModel != right.PublicModel {
			return left.PublicModel < right.PublicModel
		}
		if left.Provider != right.Provider {
			return left.Provider < right.Provider
		}
		if left.SourceAPI != right.SourceAPI {
			return left.SourceAPI < right.SourceAPI
		}
		return left.NativeModel < right.NativeModel
	})
	return out
}

func discoverCmd() *cobra.Command {
	var localOnly bool
	var outputFormat string
	cmd := &cobra.Command{
		Use:           "discover [path]",
		Short:         "Discover agent resources without running them",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			resolved, appCfg, err := configplugin.DiscoverResources(dir)
			if err != nil {
				return err
			}
			_ = appCfg // available for future use (resolution config, etc.)
			switch outputFormat {
			case "json":
				return configplugin.PrintDiscoveryJSON(cmd.OutOrStdout(), resolved)
			case "yaml":
				return configplugin.PrintDiscoveryYAML(cmd.OutOrStdout(), resolved)
			default:
				return configplugin.PrintDiscoveryTree(cmd.OutOrStdout(), resolved)
			}
		},
	}
	cmd.Flags().BoolVar(&localOnly, "local", false, "Only inspect the specified workspace/path")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "pretty", "Output format: pretty|json|yaml")
	return cmd
}

func validateCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:           "validate [path]",
		Short:         "Validate an agentsdk app directory for structural correctness",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) == 1 {
				dir = args[0]
			}
			result, err := agentdir.Validate(dir, agentdir.ValidateOptions{})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if jsonOutput {
				if err := printValidateJSON(out, result); err != nil {
					return err
				}
			} else {
				printValidateText(out, result)
			}
			if !result.OK() {
				return fmt.Errorf("validation failed")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Print machine-readable validation output")
	return cmd
}

func printValidateJSON(out io.Writer, result agentdir.ValidationResult) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

func printValidateText(out io.Writer, result agentdir.ValidationResult) {
	fmt.Fprintf(out, "Validating: %s\n\n", result.Dir)

	// Manifest.
	if result.Manifest.Found {
		fmt.Fprintf(out, "Manifest: %s\n", result.Manifest.Path)
		if result.Manifest.DefaultAgent != "" {
			fmt.Fprintf(out, "  default_agent: %s\n", result.Manifest.DefaultAgent)
		}
		if len(result.Manifest.Sources) > 0 {
			fmt.Fprintf(out, "  sources: %v\n", result.Manifest.Sources)
		}
		if result.Manifest.GlobalUserResources != nil {
			fmt.Fprintf(out, "  include_global_user_resources: %v\n", *result.Manifest.GlobalUserResources)
		}
	} else {
		fmt.Fprintln(out, "Manifest: not found")
	}

	// Agents.
	fmt.Fprintf(out, "\nAgents: %d\n", len(result.Agents))
	for _, a := range result.Agents {
		frontmatter := "✗"
		if a.HasFrontmatter {
			frontmatter = "✓"
		}
		fmt.Fprintf(out, "  %s  frontmatter=%s  tools=%d  skills=%d  capabilities=%d\n",
			a.Name, frontmatter, len(a.Tools), len(a.Skills), len(a.Capabilities))
	}

	// Skills.
	if len(result.Skills.Local) > 0 {
		fmt.Fprintf(out, "\nLocal skills: %v\n", result.Skills.Local)
	}
	if len(result.Skills.GlobalAvailable) > 0 {
		included := "not included"
		if result.Skills.GlobalIncluded {
			included = "included"
		}
		fmt.Fprintf(out, "Global skills: %v (%s)\n", result.Skills.GlobalAvailable, included)
	}
	if len(result.Skills.Unresolvable) > 0 {
		fmt.Fprintf(out, "Unresolvable skills: %v\n", result.Skills.Unresolvable)
	}

	// Resources.
	if len(result.Workflows) > 0 {
		fmt.Fprintf(out, "\nWorkflows: %v\n", result.Workflows)
	}
	if len(result.Actions) > 0 {
		fmt.Fprintf(out, "Actions: %v\n", result.Actions)
	}
	if len(result.StructuredCommands) > 0 {
		fmt.Fprintf(out, "Structured commands: %v\n", result.StructuredCommands)
	}
	if len(result.Commands) > 0 {
		fmt.Fprintf(out, "Prompt commands: %v\n", result.Commands)
	}
	if len(result.Triggers) > 0 {
		fmt.Fprintf(out, "Triggers: %v\n", result.Triggers)
	}

	// Checks.
	fmt.Fprintln(out, "\nChecks:")
	for _, c := range result.Checks {
		icon := "✓"
		switch c.Status {
		case agentdir.StatusWarning:
			icon = "⚠"
		case agentdir.StatusError:
			icon = "✗"
		}
		subject := ""
		if c.Subject != "" {
			subject = " [" + c.Subject + "]"
		}
		fmt.Fprintf(out, "  %s %s%s: %s\n", icon, c.Category, subject, c.Message)
	}

	// Summary.
	passed, warnings, errors := 0, 0, 0
	for _, c := range result.Checks {
		switch c.Status {
		case agentdir.StatusPassed:
			passed++
		case agentdir.StatusWarning:
			warnings++
		case agentdir.StatusError:
			errors++
		}
	}
	fmt.Fprintf(out, "\nResult: %d passed, %d warnings, %d errors\n", passed, warnings, errors)
}

type discoveryWriter interface {
	Write([]byte) (int, error)
}

func printModelEvaluations(out discoveryWriter, model string, evidenceErr error, evaluations []compatibility.Evaluation) error {
	fmt.Fprintf(out, "Model: %s\n", model)
	if evidenceErr != nil {
		fmt.Fprintf(out, "Evidence: unavailable (%v)\n", evidenceErr)
	}
	fmt.Fprintln(out, "Candidates:")
	if len(evaluations) == 0 {
		fmt.Fprintln(out, "  none")
		return nil
	}
	for _, evaluation := range evaluations {
		candidate := evaluation.Candidate
		fmt.Fprintf(out, "  %s  source_api=%s  provider=%s  provider_api=%s  native_model=%s",
			evaluation.Status,
			agentconfig.FormatSourceAPI(candidate.SourceAPI),
			candidate.Provider,
			candidate.ProviderAPI,
			candidate.NativeModel,
		)
		if candidate.CapabilitySource != "" {
			fmt.Fprintf(out, "  capability_source=%s", candidate.CapabilitySource)
		}
		if missing := compatibilityFeatureNames(evaluation.MissingRequired); missing != "" {
			fmt.Fprintf(out, "  missing_required=%s", missing)
		}
		if untested := compatibilityFeatureNames(evaluation.UntestedRequired); untested != "" {
			fmt.Fprintf(out, "  untested_required=%s", untested)
		}
		if degraded := compatibilityFeatureNames(evaluation.DegradedPreferred); degraded != "" {
			fmt.Fprintf(out, "  degraded_preferred=%s", degraded)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func printApprovedModelSelections(out discoveryWriter, model string, evidenceSource string, selections []adapterconfig.UseCaseModelSelection) error {
	if model == "" {
		fmt.Fprintln(out, "Models: discovered from compatibility evidence")
	} else {
		fmt.Fprintf(out, "Model: %s\n", model)
	}
	fmt.Fprintf(out, "Evidence: %s\n", evidenceSource)
	fmt.Fprintln(out, "Candidates:")
	if len(selections) == 0 {
		fmt.Fprintln(out, "  none")
		return nil
	}
	for _, selection := range selections {
		resolution := selection.Resolution
		evaluation := selection.Evaluation
		modelName := resolution.PublicModel
		if modelName == "" {
			modelName = resolution.Input
		}
		fmt.Fprintf(out, "  %s  model=%s  source_api=%s  provider=%s  provider_api=%s  native_model=%s",
			evaluation.Status,
			modelName,
			agentconfig.FormatSourceAPI(resolution.SourceAPI),
			resolution.Provider,
			resolution.ProviderAPI,
			resolution.NativeModel,
		)
		if resolution.CapabilitySource != "" {
			fmt.Fprintf(out, "  capability_source=%s", resolution.CapabilitySource)
		}
		if runtimeID := selection.RuntimeID; runtimeID != "" {
			fmt.Fprintf(out, "  runtime_id=%s", runtimeID)
		}
		fmt.Fprintln(out)
	}
	return nil
}

func compatibilityFeatureNames(features []compatibility.Feature) string {
	if len(features) == 0 {
		return ""
	}
	names := make([]string, 0, len(features))
	for _, feature := range features {
		names = append(names, string(feature))
	}
	return strings.Join(names, ",")
}
