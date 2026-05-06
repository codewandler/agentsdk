package engineerapp

import (
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/plugins/browserplugin"
	"github.com/codewandler/agentsdk/plugins/configplugin"
)

// Spec returns the app.Spec for the engineer (dev) application.
func Spec() app.Spec {
	return app.Spec{
		Name:        "dev",
		Description: "Run the first-party engineer agent with project discovery",
		Options: func() ([]app.Option, error) {
			return []app.Option{
				app.WithEmbeddedResources(Resources(), ResourcesRoot),
				app.WithDefaultAgent("main"),
				app.WithPlugin(browserplugin.New()),
				app.WithPlugin(configplugin.New()),
			}, nil
		},
	}
}
