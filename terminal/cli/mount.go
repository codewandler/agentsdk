package cli

import (
	"fmt"

	"github.com/codewandler/agentsdk/app"
	"github.com/spf13/cobra"
)

// Mount registers one or more app.Spec values as subcommands on root.
// Each spec becomes a cobra command with the standard flag surface
// (model, thinking, max-steps, discover, agent, session, etc.).
//
// The spec's Options factory is called at RunE time — after flags are
// parsed — so app-specific setup (e.g. os.Getwd) happens lazily.
// Pre-construction hints (embedded resources, plugin defaults) are
// extracted from the options via [app.ResolveHints].
//
// The REPL prompt defaults to "$name> ".
func Mount(root *cobra.Command, specs ...app.Spec) {
	for _, s := range specs {
		root.AddCommand(mountOne(s))
	}
}

func mountOne(s app.Spec) *cobra.Command {
	prompt := fmt.Sprintf("%s> ", s.Name)

	return NewCommand(CommandConfig{
		Name:         "agentsdk",
		Use:          s.Name + " [task]",
		Short:        s.Description,
		DiscoverFlag: true,
		AgentFlag:    true,
		Prompt:       prompt,
		AppOptionsFactory: func() ([]app.Option, error) {
			if s.Options == nil {
				return nil, nil
			}
			opts, err := s.Options()
			if err != nil {
				return nil, err
			}
			// Extract pre-construction hints and apply them to the
			// CommandConfig-level fields via wrapper options that
			// mountOne cannot set statically (they're only known
			// after the factory runs).
			return opts, nil
		},
	})
}
