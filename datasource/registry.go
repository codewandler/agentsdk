package datasource

import "fmt"

// Registry stores datasource definitions by name.
type Registry struct {
	index   map[string]Definition
	ordered []Definition
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{index: map[string]Definition{}}
}

// Register adds datasource definitions. Names must be non-empty and unique.
func (r *Registry) Register(defs ...Definition) error {
	if r.index == nil {
		r.index = map[string]Definition{}
	}
	for _, def := range defs {
		if err := Validate(def); err != nil {
			return err
		}
		if _, exists := r.index[def.Name]; exists {
			return ErrDuplicate{Name: def.Name}
		}
		r.index[def.Name] = def
		r.ordered = append(r.ordered, def)
	}
	return nil
}

// Get resolves a datasource definition by name.
func (r *Registry) Get(name string) (Definition, bool) {
	if r == nil {
		return Definition{}, false
	}
	def, ok := r.index[name]
	return def, ok
}

// All returns datasource definitions in registration order.
func (r *Registry) All() []Definition {
	if r == nil {
		return nil
	}
	out := make([]Definition, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// ErrDuplicate is returned when a datasource name is already registered.
type ErrDuplicate struct {
	Name string
}

func (e ErrDuplicate) Error() string {
	return fmt.Sprintf("datasource: %q is already registered", e.Name)
}
