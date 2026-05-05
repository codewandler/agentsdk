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
		Name:             "agentsdk",
		Use:              s.Name + " [task]",
		Short:            s.Description,
		DiscoverFlag:     true,
		AgentFlag:        true,
		EmbeddedBase:     s.EmbeddedFS,
		EmbeddedBaseRoot: s.EmbeddedRoot,
		EmbeddedOnly:     s.EmbeddedOnly,
		NoDefaultPlugins: s.NoDefaultPlugins,
		Prompt:           prompt,
		AppOptionsFactory: func() ([]app.Option, error) {
			if s.Options == nil {
				return nil, nil
			}
			return s.Options()
		},
	})
}
