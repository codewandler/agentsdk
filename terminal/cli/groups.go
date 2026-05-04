package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const flagGroupAnnotation = "agentsdk/flag-group"

type Group string

const (
	GroupCore               Group = "core"
	GroupInference          Group = "inference"
	GroupRuntime            Group = "runtime"
	GroupSession            Group = "session"
	GroupResources          Group = "resources"
	GroupModelCompatibility Group = "model_compatibility"
	GroupDebug              Group = "debug"
)

type GroupSet map[Group]bool

type Profile struct {
	Groups        GroupSet
	Defaults      Defaults
	HiddenFlags   []string
	DisabledFlags []string
}

type Defaults struct {
	Model       string
	MaxSteps    int
	MaxTokens   int
	ToolTimeout time.Duration
	Prompt      string
	SourceAPI   string
	ModelPolicy agentconfig.ModelPolicy
	Thinking    agentconfig.ThinkingMode
	Effort      string
	Temperature *float64
}

func Groups(groups ...Group) GroupSet {
	set := GroupSet{}
	for _, group := range groups {
		set[group] = true
	}
	return set
}

func AllGroups() GroupSet {
	return Groups(
		GroupCore,
		GroupInference,
		GroupRuntime,
		GroupSession,
		GroupResources,
		GroupModelCompatibility,
		GroupDebug,
	)
}

func (p Profile) groupEnabled(group Group) bool {
	if len(p.Groups) == 0 {
		return true
	}
	return p.Groups[group]
}

func (p Profile) flagDisabled(name string) bool {
	for _, disabled := range p.DisabledFlags {
		if disabled == name {
			return true
		}
	}
	return false
}

func (p Profile) flagHidden(name string) bool {
	for _, hidden := range p.HiddenFlags {
		if hidden == name {
			return true
		}
	}
	return false
}

func annotateFlag(cmd *cobra.Command, group Group, name string) {
	flag := cmd.Flags().Lookup(name)
	if flag == nil {
		return
	}
	if flag.Annotations == nil {
		flag.Annotations = map[string][]string{}
	}
	flag.Annotations[flagGroupAnnotation] = []string{string(group)}
}

func annotateFlags(cmd *cobra.Command, group Group, names ...string) {
	for _, name := range names {
		annotateFlag(cmd, group, name)
	}
}

func AnnotateFlagGroup(cmd *cobra.Command, group Group, names ...string) {
	annotateFlags(cmd, group, names...)
}

func applyProfileFlagVisibility(cmd *cobra.Command, profile Profile) {
	cmd.Flags().VisitAll(func(flag *pflag.Flag) {
		if profile.flagHidden(flag.Name) {
			flag.Hidden = true
		}
	})
}

func installGroupedHelp(cmd *cobra.Command) {
	InstallGroupedHelp(cmd)
}

func InstallGroupedHelp(cmd *cobra.Command) {
	cmd.SetHelpFunc(func(c *cobra.Command, _ []string) {
		_ = writeGroupedHelp(c.OutOrStdout(), c)
	})
	cmd.SetUsageFunc(func(c *cobra.Command) error {
		return writeGroupedHelp(c.ErrOrStderr(), c)
	})
}

func writeGroupedHelp(out io.Writer, cmd *cobra.Command) error {
	if cmd.UseLine() != "" {
		fmt.Fprintf(out, "Usage:\n  %s\n", cmd.UseLine())
	}
	if cmd.Long != "" {
		fmt.Fprintf(out, "\n%s\n", strings.TrimSpace(cmd.Long))
	} else if cmd.Short != "" {
		fmt.Fprintf(out, "\n%s\n", strings.TrimSpace(cmd.Short))
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintln(out, "\nCommands:")
		for _, sub := range cmd.Commands() {
			if !sub.IsAvailableCommand() {
				continue
			}
			fmt.Fprintf(out, "  %-16s %s\n", sub.Name(), sub.Short)
		}
	}
	writeGroupedFlags(out, cmd)
	return nil
}

func writeGroupedFlags(out io.Writer, cmd *cobra.Command) {
	ordered := []Group{
		GroupCore,
		GroupResources,
		GroupInference,
		GroupRuntime,
		GroupSession,
		GroupModelCompatibility,
		GroupDebug,
	}
	for _, group := range ordered {
		flags := flagsForGroup(cmd.Flags(), group)
		if len(flags) == 0 {
			continue
		}
		fmt.Fprintf(out, "\n%s:\n", groupTitle(group))
		for _, flag := range flags {
			fmt.Fprintf(out, "  %s\n", flagUsage(flag))
		}
	}
	ungrouped := ungroupedFlags(cmd.Flags())
	if len(ungrouped) == 0 {
		return
	}
	fmt.Fprintln(out, "\nFlags:")
	for _, flag := range ungrouped {
		fmt.Fprintf(out, "  %s\n", flagUsage(flag))
	}
}

func flagsForGroup(flags *pflag.FlagSet, group Group) []*pflag.Flag {
	var out []*pflag.Flag
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		if len(flag.Annotations[flagGroupAnnotation]) > 0 && flag.Annotations[flagGroupAnnotation][0] == string(group) {
			out = append(out, flag)
		}
	})
	return out
}

func ungroupedFlags(flags *pflag.FlagSet) []*pflag.Flag {
	var out []*pflag.Flag
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Hidden {
			return
		}
		if len(flag.Annotations[flagGroupAnnotation]) == 0 {
			out = append(out, flag)
		}
	})
	return out
}

func groupTitle(group Group) string {
	switch group {
	case GroupCore:
		return "Core"
	case GroupResources:
		return "Resources"
	case GroupInference:
		return "Inference"
	case GroupRuntime:
		return "Runtime"
	case GroupSession:
		return "Session"
	case GroupModelCompatibility:
		return "Model Compatibility"
	case GroupDebug:
		return "Debug"
	default:
		return string(group)
	}
}

func flagUsage(flag *pflag.Flag) string {
	name := "--" + flag.Name
	if flag.Shorthand != "" {
		name = "-" + flag.Shorthand + ", " + name
	}
	if flag.NoOptDefVal == "" {
		name += " " + flag.Value.Type()
	}
	if flag.DefValue != "" && flag.DefValue != "false" {
		return fmt.Sprintf("%-28s %s (default %q)", name, flag.Usage, flag.DefValue)
	}
	return fmt.Sprintf("%-28s %s", name, flag.Usage)
}
