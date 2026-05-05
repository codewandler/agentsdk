package builderapp

import (
	"os"
	"path/filepath"

	"github.com/codewandler/agentsdk/app"
)

// Spec returns the app.Spec for the builder application.
func Spec() app.Spec {
	return app.Spec{
		Name:             "build",
		Description:      "Start the first-party agentsdk builder app",
		EmbeddedFS:       Resources(),
		EmbeddedRoot:     ResourcesRoot,
		EmbeddedOnly:     true,
		NoDefaultPlugins: true,
		Options: func() ([]app.Option, error) {
			workspace, err := os.Getwd()
			if err != nil {
				return nil, err
			}
			workspace, err = filepath.Abs(workspace)
			if err != nil {
				return nil, err
			}
			cfg, err := NormalizeConfig(Config{ProjectDir: workspace})
			if err != nil {
				return nil, err
			}
			return []app.Option{
				app.WithDefaultAgent("builder"),
				app.WithPlugin(Plugin{cfg: cfg}),
			}, nil
		},
	}
}
