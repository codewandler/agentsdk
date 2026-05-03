// Package action provides surface-neutral Go-native executable primitives.
package action

import "context"

// Ctx is the execution context passed to actions.
//
// It intentionally starts as only context.Context. Runtime, workflow, tool, or
// command-specific values can be layered on later through context values or
// narrower adapter interfaces without making action depend on those packages.
type Ctx interface {
	context.Context
}

// Event is an action-emitted event payload.
//
// The core action package intentionally imposes no event shape. Runtimes,
// workflow executors, harnesses, or thread adapters decide which event payloads
// are observable, persistable, or dispatchable.
type Event = any

// Result is the execution outcome of an Action.
type Result struct {
	Status Status
	Data   any
	Error  error
	Events []Event
}

// Handler is the untyped execution form used by middleware and adapters.
type Handler func(Ctx, any) Result

// Action is the core executable primitive.
type Action interface {
	Spec() Spec
	Execute(Ctx, any) Result
}

// Spec describes an action without tying it to any particular invocation
// surface such as tools, commands, workflows, or triggers.
type Spec struct {
	Name        string
	Description string
	Input       Type
	Output      Type
}

type actionFunc struct {
	spec    Spec
	handler Handler
}

// New returns an Action backed by handler.
func New(spec Spec, handler Handler) Action {
	return &actionFunc{spec: spec, handler: handler}
}

func (a *actionFunc) Spec() Spec { return a.spec }

func (a *actionFunc) Execute(ctx Ctx, input any) Result {
	if a.handler == nil {
		return Result{}
	}
	return a.handler(ctx, input)
}

// NewTyped returns an Action from an idiomatic Go function shaped as
// func(Ctx, I) (O, error). The input and output Types are inferred from I and O
// when they are not explicitly supplied in spec.
func NewTyped[I, O any](spec Spec, fn func(Ctx, I) (O, error)) Action {
	if spec.Input.IsZero() {
		spec.Input = TypeOf[I]()
	}
	if spec.Output.IsZero() {
		spec.Output = TypeOf[O]()
	}
	return New(spec, func(ctx Ctx, input any) Result {
		var zero O
		if fn == nil {
			return OK(zero)
		}
		in, err := CastInput[I](input)
		if err != nil {
			return Failed(err)
		}
		out, err := fn(ctx, in)
		if err != nil {
			return Failed(err)
		}
		return OK(out)
	})
}
