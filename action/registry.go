package action

import "fmt"

// Registry stores actions by name.
type Registry struct {
	index   map[string]Action
	ordered []Action
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{index: map[string]Action{}}
}

// Register adds actions to the registry. Names must be non-empty and unique.
func (r *Registry) Register(actions ...Action) error {
	if r.index == nil {
		r.index = map[string]Action{}
	}
	for _, a := range actions {
		if a == nil {
			continue
		}
		name := a.Spec().Name
		if name == "" {
			return fmt.Errorf("action: action name is required")
		}
		if _, exists := r.index[name]; exists {
			return ErrDuplicate{Name: name}
		}
		r.index[name] = a
		r.ordered = append(r.ordered, a)
	}
	return nil
}

// Get resolves an action by name.
func (r *Registry) Get(name string) (Action, bool) {
	if r == nil {
		return nil, false
	}
	a, ok := r.index[name]
	return a, ok
}

// All returns actions in registration order.
func (r *Registry) All() []Action {
	if r == nil {
		return nil
	}
	out := make([]Action, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// ErrDuplicate is returned when an action name is already registered.
type ErrDuplicate struct {
	Name string
}

func (e ErrDuplicate) Error() string {
	return fmt.Sprintf("action: %q is already registered", e.Name)
}
