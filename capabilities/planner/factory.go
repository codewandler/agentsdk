package planner

import (
	"context"
	"fmt"

	"github.com/codewandler/agentsdk/capability"
)

type Factory struct{}

func (Factory) Name() string { return CapabilityName }

func (Factory) New(_ context.Context, spec capability.AttachSpec, runtime capability.Runtime) (capability.Capability, error) {
	if spec.CapabilityName != "" && spec.CapabilityName != CapabilityName {
		return nil, fmt.Errorf("planner: unsupported capability name %q", spec.CapabilityName)
	}
	if spec.InstanceID == "" {
		return nil, fmt.Errorf("planner: instance id is required")
	}
	return New(spec, runtime), nil
}
