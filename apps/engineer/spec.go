package engineerapp

import (
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/plugins/browserplugin"
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
			}, nil
		},
	}
}
