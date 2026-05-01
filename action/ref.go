package action

// Ref identifies an action by name for declarative references from workflows,
// datasources, commands, triggers, or app resources. Resolution is deliberately
// owned by the caller's registry/executor so refs stay serializable and
// surface-neutral.
type Ref struct {
	Name string `json:"name" yaml:"name"`
}

// IsZero reports whether ref does not identify an action.
func (r Ref) IsZero() bool { return r.Name == "" }
