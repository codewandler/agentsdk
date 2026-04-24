package conversation

import (
	"context"
	"time"
)

type StructuralEventKind string

const (
	StructuralConversationCreated StructuralEventKind = "conversation_created"
	StructuralBranchCreated       StructuralEventKind = "branch_created"
	StructuralNodeAppended        StructuralEventKind = "node_appended"
	StructuralHeadMoved           StructuralEventKind = "head_moved"
)

type StructuralEvent struct {
	Kind           StructuralEventKind `json:"kind"`
	ConversationID ConversationID      `json:"conversation_id,omitempty"`
	SessionID      SessionID           `json:"session_id,omitempty"`
	BranchID       BranchID            `json:"branch_id,omitempty"`
	NodeID         NodeID              `json:"node_id,omitempty"`
	ParentNodeID   NodeID              `json:"parent_node_id,omitempty"`
	FromBranchID   BranchID            `json:"from_branch_id,omitempty"`
	At             time.Time           `json:"at"`
}

type EventStore interface {
	AppendStructural(ctx context.Context, events ...StructuralEvent) error
	LoadStructural(ctx context.Context, conversationID ConversationID) ([]StructuralEvent, error)
}

type MemoryStore struct {
	events []StructuralEvent
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (s *MemoryStore) AppendStructural(_ context.Context, events ...StructuralEvent) error {
	s.events = append(s.events, events...)
	return nil
}

func (s *MemoryStore) LoadStructural(_ context.Context, conversationID ConversationID) ([]StructuralEvent, error) {
	out := make([]StructuralEvent, 0, len(s.events))
	for _, event := range s.events {
		if conversationID == "" || event.ConversationID == conversationID {
			out = append(out, event)
		}
	}
	return out, nil
}
