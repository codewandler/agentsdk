package builderapp

import (
	"os"
	"path/filepath"

	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/plugins/configplugin"
)

// Spec returns the app.Spec for the builder application.
func Spec() app.Spec {
	return app.Spec{
		Name:        "build",
		Description: "Start the first-party agentsdk builder app",
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
				app.WithEmbeddedResources(Resources(), ResourcesRoot),
				app.WithEmbeddedOnly(),
				app.WithDefaultAgent("builder"),
				app.WithoutDefaultPlugins(),
				app.WithPlugin(Plugin{cfg: cfg}),
				app.WithPlugin(configplugin.New(configplugin.WithWorkspace(workspace))),
			}, nil
		},
	}
}
