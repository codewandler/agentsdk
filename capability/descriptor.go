package capability

import (
	"sort"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agentcontext"
)

// Descriptor is side-effect-free metadata about an attached capability instance.
// It is intended for harness/channel/debug inspection; execution remains owned by
// the capability instance and thread runtime.
type Descriptor struct {
	Name       string                          `json:"name"`
	InstanceID string                          `json:"instance_id"`
	Tools      []string                        `json:"tools,omitempty"`
	Actions    []string                        `json:"actions,omitempty"`
	Context    agentcontext.ProviderDescriptor `json:"context,omitempty"`
	Stateful   bool                            `json:"stateful,omitempty"`
	Replayable bool                            `json:"replayable,omitempty"`
}

// ActionsProvider is an optional capability facet for Go-native action
// projection. Tool projection remains LLM-facing; actions are for workflows,
// triggers, harnesses, or other typed execution surfaces that explicitly opt in.
type ActionsProvider interface {
	Actions() []action.Action
}

func Describe(instance Capability) Descriptor {
	if instance == nil {
		return Descriptor{}
	}
	desc := Descriptor{
		Name:       instance.Name(),
		InstanceID: instance.InstanceID(),
	}
	for _, t := range instance.Tools() {
		if t != nil && t.Name() != "" {
			desc.Tools = append(desc.Tools, t.Name())
		}
	}
	if provider := instance.ContextProvider(); provider != nil {
		desc.Context = providerDescriptor(provider)
	}
	if actions, ok := instance.(ActionsProvider); ok {
		for _, a := range actions.Actions() {
			if a != nil && a.Spec().Name != "" {
				desc.Actions = append(desc.Actions, a.Spec().Name)
			}
		}
	}
	_, desc.Stateful = instance.(StateApplier)
	desc.Replayable = desc.Stateful
	sort.Strings(desc.Tools)
	sort.Strings(desc.Actions)
	return desc
}

func providerDescriptor(provider agentcontext.Provider) agentcontext.ProviderDescriptor {
	if provider == nil {
		return agentcontext.ProviderDescriptor{}
	}
	if described, ok := provider.(agentcontext.DescribedProvider); ok {
		desc := described.Descriptor()
		if desc.Key == "" {
			desc.Key = provider.Key()
		}
		return desc
	}
	return agentcontext.ProviderDescriptor{Key: provider.Key()}
}
