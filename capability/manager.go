package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
)

type Manager struct {
	mu           sync.RWMutex
	registry     Registry
	runtime      Runtime
	capabilities map[string]Capability
}

func NewManager(registry Registry, runtime Runtime) *Manager {
	return &Manager{
		registry:     registry,
		runtime:      runtime,
		capabilities: make(map[string]Capability),
	}
}

func (m *Manager) Attach(ctx context.Context, spec AttachSpec) (Capability, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.registry == nil {
		return nil, fmt.Errorf("capability: registry is required")
	}
	if m.runtime == nil {
		return nil, fmt.Errorf("capability: runtime is required")
	}
	spec = m.normalizeSpec(spec)
	if spec.InstanceID == "" {
		return nil, fmt.Errorf("capability: instance id is required")
	}
	if _, ok := m.capabilities[spec.InstanceID]; ok {
		return nil, fmt.Errorf("capability: instance %q already attached", spec.InstanceID)
	}

	instance, err := m.registry.Create(ctx, spec, m.runtime)
	if err != nil {
		return nil, err
	}
	event, err := AttachEvent(spec)
	if err != nil {
		return nil, err
	}
	if err := m.runtime.AppendEvents(ctx, event); err != nil {
		return nil, err
	}
	m.capabilities[spec.InstanceID] = instance
	return instance, nil
}

func (m *Manager) Replay(ctx context.Context, events []thread.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.registry == nil {
		return fmt.Errorf("capability: registry is required")
	}
	if m.runtime == nil {
		return fmt.Errorf("capability: runtime is required")
	}
	for _, event := range events {
		switch event.Kind {
		case EventAttached:
			var payload Attached
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			spec := AttachSpec{
				ThreadID:       m.runtime.ThreadID(),
				BranchID:       m.runtime.BranchID(),
				CapabilityName: payload.CapabilityName,
				InstanceID:     payload.InstanceID,
				Config:         payload.Config,
			}
			if spec.InstanceID == "" {
				return fmt.Errorf("capability: attached event has no instance id")
			}
			instance, err := m.registry.Create(ctx, spec, m.runtime)
			if err != nil {
				return err
			}
			m.capabilities[spec.InstanceID] = instance
		case EventDetached:
			var payload Detached
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			delete(m.capabilities, payload.InstanceID)
		case EventStateEventDispatched:
			var payload StateEventDispatched
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			instance, ok := m.capabilities[payload.InstanceID]
			if !ok {
				return fmt.Errorf("capability: instance %q not attached", payload.InstanceID)
			}
			applier, ok := instance.(StateApplier)
			if !ok {
				return fmt.Errorf("capability: instance %q cannot apply state events", payload.InstanceID)
			}
			if err := applier.ApplyEvent(ctx, StateEvent{Name: payload.EventName, Body: payload.Body}); err != nil {
				return err
			}
		}
	}
	return nil
}

func (m *Manager) Capability(instanceID string) (Capability, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	instance, ok := m.capabilities[instanceID]
	return instance, ok
}

func (m *Manager) Tools() []tool.Tool {
	instances := m.sortedCapabilities()
	var tools []tool.Tool
	for _, instance := range instances {
		tools = append(tools, instance.Tools()...)
	}
	return tools
}

func (m *Manager) ContextProvider() agentcontext.Provider {
	return managerProvider{manager: m}
}

func (m *Manager) normalizeSpec(spec AttachSpec) AttachSpec {
	if spec.ThreadID == "" && m.runtime != nil {
		spec.ThreadID = m.runtime.ThreadID()
	}
	if spec.BranchID == "" && m.runtime != nil {
		spec.BranchID = m.runtime.BranchID()
	}
	return spec
}

func (m *Manager) sortedCapabilities() []Capability {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sortedCapabilitiesLocked()
}

func (m *Manager) sortedCapabilitiesLocked() []Capability {
	keys := make([]string, 0, len(m.capabilities))
	for key := range m.capabilities {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Capability, 0, len(keys))
	for _, key := range keys {
		out = append(out, m.capabilities[key])
	}
	return out
}

type managerProvider struct {
	manager *Manager
}

func (p managerProvider) Key() agentcontext.ProviderKey {
	return "capabilities"
}

func (p managerProvider) GetContext(ctx context.Context, req agentcontext.Request) (agentcontext.ProviderContext, error) {
	var fragments []agentcontext.ContextFragment
	seen := map[agentcontext.FragmentKey]struct{}{}
	for _, instance := range p.manager.sortedCapabilities() {
		provider := instance.ContextProvider()
		if provider == nil {
			continue
		}
		context, err := provider.GetContext(ctx, req)
		if err != nil {
			return agentcontext.ProviderContext{}, err
		}
		for _, fragment := range context.Fragments {
			fragment.Key = agentcontext.FragmentKey(instance.InstanceID() + "/" + string(fragment.Key))
			if _, ok := seen[fragment.Key]; ok {
				return agentcontext.ProviderContext{}, fmt.Errorf("capability: duplicate context fragment key %q", fragment.Key)
			}
			seen[fragment.Key] = struct{}{}
			fragments = append(fragments, fragment)
		}
	}
	return agentcontext.ProviderContext{Fragments: fragments}, nil
}
