package thread

import (
	"encoding/json"
	"time"
)

type EventKind string

const CurrentEventSchemaVersion = 1

const (
	EventThreadCreated    EventKind = "thread.created"
	EventMetadataUpdated  EventKind = "thread.metadata_updated"
	EventThreadArchived   EventKind = "thread.archived"
	EventThreadUnarchived EventKind = "thread.unarchived"

	EventBranchCreated   EventKind = "branch.created"
	EventBranchHeadMoved EventKind = "branch.head_moved"
)

type EventSource struct {
	Type      string `json:"type,omitempty"`
	ID        string `json:"id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

type Event struct {
	ID            EventID         `json:"id"`
	ThreadID      ID              `json:"thread_id"`
	BranchID      BranchID        `json:"branch_id"`
	NodeID        NodeID          `json:"node_id,omitempty"`
	ParentNodeID  NodeID          `json:"parent_node_id,omitempty"`
	SchemaVersion int             `json:"schema_version,omitempty"`
	Seq           int64           `json:"seq"`
	Kind          EventKind       `json:"kind"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	At            time.Time       `json:"at"`
	Source        EventSource     `json:"source,omitempty"`
	CausationID   EventID         `json:"causation_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
}

func cloneEvent(event Event) Event {
	if event.Payload != nil {
		event.Payload = append(json.RawMessage(nil), event.Payload...)
	}
	return event
}

func cloneEvents(events []Event) []Event {
	out := make([]Event, len(events))
	for i, event := range events {
		out[i] = cloneEvent(event)
	}
	return out
}
