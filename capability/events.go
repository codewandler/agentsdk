package capability

import (
	"encoding/json"

	"github.com/codewandler/agentsdk/thread"
)

const (
	EventAttached             thread.EventKind = "capability.attached"
	EventDetached             thread.EventKind = "capability.detached"
	EventStateEventDispatched thread.EventKind = "capability.state_event_dispatched"
)

type AttachSpec struct {
	ThreadID       thread.ID       `json:"thread_id,omitempty"`
	BranchID       thread.BranchID `json:"branch_id,omitempty"`
	CapabilityName string          `json:"capability_name"`
	InstanceID     string          `json:"instance_id"`
	Config         json.RawMessage `json:"config,omitempty"`
}

type Attached struct {
	CapabilityName string          `json:"capability_name"`
	InstanceID     string          `json:"instance_id"`
	Config         json.RawMessage `json:"config,omitempty"`
}

type Detached struct {
	CapabilityName string `json:"capability_name"`
	InstanceID     string `json:"instance_id"`
}

type StateEvent struct {
	Name string          `json:"name"`
	Body json.RawMessage `json:"body,omitempty"`
}

type StateEventDispatched struct {
	CapabilityName string          `json:"capability_name"`
	InstanceID     string          `json:"instance_id"`
	EventName      string          `json:"event_name"`
	Body           json.RawMessage `json:"body,omitempty"`
}

func AttachEvent(spec AttachSpec) (thread.Event, error) {
	payload, err := json.Marshal(Attached{
		CapabilityName: spec.CapabilityName,
		InstanceID:     spec.InstanceID,
		Config:         spec.Config,
	})
	if err != nil {
		return thread.Event{}, err
	}
	return thread.Event{
		ThreadID: spec.ThreadID,
		BranchID: spec.BranchID,
		Kind:     EventAttached,
		Payload:  payload,
		Source: thread.EventSource{
			Type: "capability",
			ID:   spec.InstanceID,
		},
	}, nil
}

func DetachEvent(spec AttachSpec) (thread.Event, error) {
	payload, err := json.Marshal(Detached{
		CapabilityName: spec.CapabilityName,
		InstanceID:     spec.InstanceID,
	})
	if err != nil {
		return thread.Event{}, err
	}
	return thread.Event{
		ThreadID: spec.ThreadID,
		BranchID: spec.BranchID,
		Kind:     EventDetached,
		Payload:  payload,
		Source: thread.EventSource{
			Type: "capability",
			ID:   spec.InstanceID,
		},
	}, nil
}

func DispatchEvent(spec AttachSpec, event StateEvent) (thread.Event, error) {
	payload, err := json.Marshal(StateEventDispatched{
		CapabilityName: spec.CapabilityName,
		InstanceID:     spec.InstanceID,
		EventName:      event.Name,
		Body:           event.Body,
	})
	if err != nil {
		return thread.Event{}, err
	}
	return thread.Event{
		ThreadID: spec.ThreadID,
		BranchID: spec.BranchID,
		Kind:     EventStateEventDispatched,
		Payload:  payload,
		Source: thread.EventSource{
			Type: "capability",
			ID:   spec.InstanceID,
		},
	}, nil
}
