package action

// OutputEvent is emitted when an action writes streaming output during
// execution. It carries a named stream (e.g. "stdout", "stderr") and the
// incremental chunk. Presentation layers can render these in real time.
type OutputEvent struct {
	Stream string // stream name: "stdout", "stderr", or a custom label
	Chunk  []byte // incremental output data
}

// StatusEvent is emitted by long-running actions to report progress.
// Presentation layers can render this as a spinner, progress bar, or
// status line update.
type StatusEvent struct {
	Progress float64 // 0.0–1.0 for determinate progress, -1 for indeterminate
	Message  string  // human-readable status message
}
