package app

import "io/fs"

// Spec declares a first-party application's identity and construction factory.
// The Name and Description are used for logging, session metadata, and CLI
// presentation. The Options factory is called at runtime to produce the
// [Option] slice that configures [App] via [New].
//
// Pre-construction fields (EmbeddedFS, EmbeddedOnly, NoDefaultPlugins) are
// read by the host (e.g. cli.Mount) before calling [New] to control resource
// resolution and plugin loading.
type Spec struct {
	// Name is the application identity (e.g. "run", "dev", "build").
	Name string

	// Description is a short human-readable summary.
	Description string

	// EmbeddedFS, when set, provides an embedded filesystem used as the
	// primary resource source. EmbeddedRoot is the root path inside the FS.
	EmbeddedFS   fs.FS
	EmbeddedRoot string

	// EmbeddedOnly, when true alongside EmbeddedFS, prevents merging
	// directory resources from discovery paths into the app's resource set.
	// This is used by apps that operate ON a target project and must not
	// load the target's .agents directory as their own resources.
	EmbeddedOnly bool

	// NoDefaultPlugins disables the built-in default plugin (e.g. local_cli).
	NoDefaultPlugins bool

	// Options returns the app.Option slice that configures the App.
	// Called at runtime (e.g. in cobra RunE), not at registration time.
	// May be nil for apps that need no additional options.
	Options func() ([]Option, error)
}
