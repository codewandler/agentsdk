package app

// Spec declares a first-party application's identity and construction factory.
// The Name and Description are used for logging, session metadata, and CLI
// presentation. The Options factory is called at runtime to produce the
// [Option] slice that configures [App] via [New].
//
// Pre-construction options like [WithEmbeddedResources], [WithEmbeddedOnly],
// and [WithoutDefaultPlugins] are included in the Options slice and extracted
// by hosts via [ResolveHints] before calling [New].
type Spec struct {
	// Name is the application identity (e.g. "run", "dev", "build").
	Name string

	// Description is a short human-readable summary.
	Description string

	// Options returns the app.Option slice that configures the App.
	// Called at runtime (e.g. in cobra RunE), not at registration time.
	// May be nil for apps that need no additional options.
	Options func() ([]Option, error)
}
