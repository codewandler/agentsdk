// Package runapp provides the default "run" application spec.
// It discovers resources from the working directory and discovery paths
// without shipping any embedded resources of its own.
package runapp

import "github.com/codewandler/agentsdk/app"

// Spec returns the app.Spec for the default run application.
func Spec() app.Spec {
	return app.Spec{
		Name:        "run",
		Description: "Run an agent resource bundle",
		Options:     func() ([]app.Option, error) { return nil, nil },
	}
}
