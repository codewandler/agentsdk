package capability

import (
	"context"
	"fmt"
)

type MemoryRegistry struct {
	factories map[string]Factory
}

func NewRegistry(factories ...Factory) (*MemoryRegistry, error) {
	r := &MemoryRegistry{factories: make(map[string]Factory)}
	if err := r.Register(factories...); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *MemoryRegistry) Register(factories ...Factory) error {
	if r.factories == nil {
		r.factories = make(map[string]Factory)
	}
	for _, factory := range factories {
		if factory == nil {
			return fmt.Errorf("capability: factory is nil")
		}
		name := factory.Name()
		if name == "" {
			return fmt.Errorf("capability: factory name is required")
		}
		if _, ok := r.factories[name]; ok {
			return fmt.Errorf("capability: factory %q already registered", name)
		}
		r.factories[name] = factory
	}
	return nil
}

func (r *MemoryRegistry) Create(ctx context.Context, spec AttachSpec, runtime Runtime) (Capability, error) {
	if spec.CapabilityName == "" {
		return nil, fmt.Errorf("capability: capability name is required")
	}
	factory, ok := r.factories[spec.CapabilityName]
	if !ok {
		return nil, fmt.Errorf("capability: factory %q not registered", spec.CapabilityName)
	}
	return factory.New(ctx, spec, runtime)
}
