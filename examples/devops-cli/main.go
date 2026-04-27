package main

import (
	"embed"
	"fmt"
	"os"
	"time"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/terminal/cli"
	"github.com/codewandler/llmadapter/unified"
)

//go:embed agent/.agents/agents/*.md agent/.agents/commands/*.md agent/.agents/skills/*/SKILL.md
var resources embed.FS

func main() {
	inference := agent.DefaultInferenceOptions()
	inference.Model = "codex/gpt-5.5"
	inference.Effort = unified.ReasoningEffortMedium

	cmd := cli.NewCommand(cli.CommandConfig{
		Name:      "devops-cli",
		Use:       "devops-cli [task]",
		Short:     "Run a runbook-oriented operations agent",
		Resources: cli.EmbeddedResources(resources, "agent"),
		Prompt:    "ops> ",
		Profile: cli.Profile{
			Groups: cli.Groups(
				cli.GroupCore,
				cli.GroupInference,
				cli.GroupRuntime,
				cli.GroupSession,
				cli.GroupModelCompatibility,
				cli.GroupDebug,
			),
			Defaults: cli.Defaults{
				Model:       inference.Model,
				MaxSteps:    20,
				ToolTimeout: 45 * time.Second,
				ModelPolicy: agent.ModelPolicy{
					UseCase: agent.ModelUseCaseAgenticCoding,
				},
			},
		},
	})
	if err := cmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
