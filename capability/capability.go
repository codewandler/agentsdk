package capability

import (
	"context"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/tool"
)

type Capability interface {
	Name() string
	InstanceID() string
	Tools() []tool.Tool
	ContextProvider() agentcontext.Provider
}

type StatefulCapability[T any] interface {
	Capability
	State(context.Context) (T, error)
	ApplyEvent(context.Context, StateEvent) error
}

type StateApplier interface {
	ApplyEvent(context.Context, StateEvent) error
}

type Factory interface {
	Name() string
	New(context.Context, AttachSpec, Runtime) (Capability, error)
}

type Registry interface {
	Register(...Factory) error
	Create(context.Context, AttachSpec, Runtime) (Capability, error)
}
