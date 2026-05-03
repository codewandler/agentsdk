package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"

	"github.com/codewandler/markdown"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	"github.com/codewandler/agentsdk/terminal/cli"
	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/compatibility"
	"github.com/spf13/cobra"
)

const maxDiscoverDescriptionRunes = 180

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
	cmd.AddCommand(cli.NewCommand(cli.CommandConfig{
		Name:        "agentsdk",
		Use:         "run [path] [task]",
		Short:       "Run an agent resource bundle",
		ResourceArg: true,
		AgentFlag:   true,
	}))
	cmd.AddCommand(discoverCmd())
	cmd.AddCommand(modelsCmd())
	cmd.AddCommand(toolCmd())
	return cmd
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
	)
	cmd := &cobra.Command{
		Use:           "models [model]",
		Short:         "Inspect model compatibility candidates",
		Args:          cobra.MaximumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			sourceAPI, err := agent.ParseSourceAPI(sourceAPIFlag)
			if err != nil {
				return err
			}
			useCase := agent.ModelUseCaseAgenticCoding
			if useCaseFlag != "" {
				useCase, err = agent.ParseModelUseCase(useCaseFlag)
				if err != nil {
					return err
				}
			}
			policy := agent.ModelPolicy{
				UseCase:       useCase,
				SourceAPI:     sourceAPI,
				ApprovedOnly:  approvedOnly,
				AllowDegraded: allowDegraded,
				AllowUntested: allowUntested,
				EvidencePath:  compatEvidence,
			}
			if len(args) == 0 {
				return runModelsCatalog(cmd.OutOrStdout(), policy)
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
	cli.AnnotateFlagGroup(cmd, cli.GroupInference, "source-api")
	cli.AnnotateFlagGroup(cmd, cli.GroupModelCompatibility, "model-use-case", "model-approved-only", "model-allow-degraded", "model-allow-untested", "model-compat-evidence")
	cli.InstallGroupedHelp(cmd)
	return cmd
}

func runModelsCatalog(out discoveryWriter, policy agent.ModelPolicy) error {
	evidence, evidenceSource, err := agent.LoadCompatibilityEvidence(policy)
	if err != nil {
		return err
	}
	models := evidenceModels(evidence)
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

func evidenceModels(evidence adapterconfig.CompatibilityEvidence) []string {
	seen := map[string]bool{}
	var out []string
	for _, row := range evidence.Rows {
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
			policy := resource.DiscoveryPolicy{
				IncludeGlobalUserResources: true,
				IncludeExternalEcosystems:  true,
				AllowRemote:                true,
			}
			if localOnly {
				policy.IncludeGlobalUserResources = false
				policy.AllowRemote = false
			}
			resolved, err := agentdir.ResolveDirWithOptions(dir, agentdir.ResolveOptions{Policy: policy, LocalOnly: localOnly})
			if err != nil {
				return err
			}
			return printDiscovery(cmd.OutOrStdout(), resolved)
		},
	}
	cmd.Flags().BoolVar(&localOnly, "local", false, "Only inspect the specified workspace/path")
	return cmd
}

type discoveryWriter interface {
	Write([]byte) (int, error)
}

func printDiscovery(out discoveryWriter, resolved agentdir.Resolution) error {
	imported, err := app.New(app.WithResourceBundle(resolved.Bundle))
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "Sources:")
	if len(resolved.Sources) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, source := range resolved.Sources {
		fmt.Fprintf(out, "  %s\n", source)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Agents:")
	agentSpecs := imported.AgentSpecs()
	if len(agentSpecs) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, spec := range agentSpecs {
		id := spec.ResourceID
		if id == "" {
			id = spec.Name
		}
		fmt.Fprintf(out, "  %s  %s  %s\n", spec.Name, displayDescription(spec.Description), id)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Commands:")
	commands := imported.Commands().All()
	if len(commands) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, cmd := range commands {
		spec := cmd.Spec()
		fmt.Fprintf(out, "  /%s  %s\n", spec.Name, displayDescription(spec.Description))
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Skills:")
	skills := firstSkillContributions(resolved.Bundle.Skills)
	if len(skills) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, skill := range skills {
		fmt.Fprintf(out, "  %s  %s  %s\n", skill.Name, displayDescription(skill.Description), skill.ID)
	}
	printDiscoveryDataSources(out, resolved.Bundle.DataSources)
	printDiscoveryWorkflows(out, resolved.Bundle.Workflows)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Skill sources:")
	skillSources := imported.SkillSources()
	if len(skillSources) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, source := range skillSources {
		fmt.Fprintf(out, "  %s  %s\n", source.ID, source.Label)
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Disabled suggestions:")
	hasDisabled := false
	for _, tool := range resolved.Bundle.Tools {
		if tool.Enabled {
			continue
		}
		hasDisabled = true
		fmt.Fprintf(out, "  tool %s  %s\n", tool.ID, tool.Description)
	}
	if !hasDisabled {
		fmt.Fprintln(out, "  none")
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Diagnostics:")
	diagnostics := imported.Diagnostics()
	if len(diagnostics) == 0 {
		fmt.Fprintln(out, "  none")
	}
	for _, diag := range diagnostics {
		fmt.Fprintf(out, "  %s  %s  %s\n", diag.Severity, diag.Source.Label(), diag.Message)
	}
	return nil
}

func printDiscoveryDataSources(out discoveryWriter, datasources []resource.DataSourceContribution) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Datasources:")
	if len(datasources) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, datasource := range datasources {
		kind := datasource.Kind
		if kind == "" {
			kind = "unknown"
		}
		fmt.Fprintf(out, "  %s  %s  kind=%s  %s\n", datasource.Name, displayDescription(datasource.Description), kind, datasource.ID)
	}
}

func printDiscoveryWorkflows(out discoveryWriter, workflows []resource.WorkflowContribution) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Workflows:")
	if len(workflows) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, workflow := range workflows {
		fmt.Fprintf(out, "  %s  %s  %s\n", workflow.Name, displayDescription(workflow.Description), workflow.ID)
	}
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
			agent.FormatSourceAPI(candidate.SourceAPI),
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
			agent.FormatSourceAPI(resolution.SourceAPI),
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

func firstSkillContributions(skills []resource.SkillContribution) []resource.SkillContribution {
	seen := map[string]bool{}
	out := make([]resource.SkillContribution, 0, len(skills))
	for _, skill := range skills {
		if skill.Name == "" || seen[skill.Name] {
			continue
		}
		seen[skill.Name] = true
		out = append(out, skill)
	}
	return out
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, `\n`, " ")
	return strings.Join(strings.Fields(s), " ")
}

func displayDescription(s string) string {
	s = oneLine(s)
	if utf8.RuneCountInString(s) <= maxDiscoverDescriptionRunes {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:maxDiscoverDescriptionRunes-1])) + "..."
}
