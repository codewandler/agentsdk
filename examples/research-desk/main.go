package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/profiles/localcli"
	"github.com/codewandler/agentsdk/terminal/repl"
	"github.com/codewandler/agentsdk/terminal/ui"
	"github.com/spf13/cobra"
)

//go:embed resources/.agents/agents/*.md resources/.agents/skills/*/SKILL.md
var resources embed.FS

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func rootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "research-desk",
		Short:         "Run a source-synthesis research assistant",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(askCmd())
	cmd.AddCommand(digestCmd())
	cmd.AddCommand(replCmd())
	return cmd
}

func askCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "ask <question>",
		Short:         "Ask a research question",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOneShot(cmd.Context(), strings.Join(args, " "))
		},
	}
}

func digestCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "digest <text>",
		Short:         "Summarize pasted notes or source text",
		Args:          cobra.MinimumNArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			task := "Digest these notes into claims, evidence, uncertainty, and follow-up questions:\n\n" + strings.Join(args, " ")
			return runOneShot(cmd.Context(), task)
		},
	}
}

func replCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "repl",
		Short:         "Start an interactive research session",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			application, err := newResearchApp()
			if err != nil {
				return err
			}
			if _, err := application.InstantiateDefaultAgent(); err != nil {
				return err
			}
			return repl.Run(cmd.Context(), application, os.Stdin, repl.WithPrompt("research> "))
		},
	}
}

func runOneShot(ctx context.Context, task string) error {
	application, err := newResearchApp()
	if err != nil {
		return err
	}
	if _, err := application.InstantiateDefaultAgent(); err != nil {
		return err
	}
	_, err = application.Send(ctx, task)
	if errors.Is(err, agent.ErrMaxStepsReached) {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		return nil
	}
	return err
}

func newResearchApp() (*app.App, error) {
	resolved, err := agentdir.ResolveFS(resources, "resources")
	if err != nil {
		return nil, err
	}
	name, err := resolved.ResolveDefaultAgent("")
	if err != nil {
		return nil, err
	}
	return app.New(
		app.WithResourceBundle(resolved.Bundle),
		app.WithDefaultAgent(name),
		app.WithDefaultSkillSourceDiscovery(app.SkillSourceDiscovery{WorkspaceDir: "."}),
		app.WithPlugin(localcli.New()),
		app.WithAgentWorkspace("."),
		app.WithAgentOutput(os.Stdout),
		app.WithAgentOptions(
			agent.WithEventHandlerFactory(ui.AgentEventHandlerFactory(os.Stdout)),
			agent.WithModelPolicy(agent.ModelPolicy{
				UseCase: agent.ModelUseCaseAgenticCoding,
			}),
		),
	)
}
