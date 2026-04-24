package jsonlstore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/codewandler/agentsdk/conversation"
)

type Store struct {
	path string
	mu   sync.Mutex
}

type record struct {
	Version        int                              `json:"version"`
	Kind           conversation.StructuralEventKind `json:"kind"`
	ConversationID conversation.ConversationID      `json:"conversation_id,omitempty"`
	SessionID      conversation.SessionID           `json:"session_id,omitempty"`
	BranchID       conversation.BranchID            `json:"branch_id,omitempty"`
	NodeID         conversation.NodeID              `json:"node_id,omitempty"`
	ParentNodeID   conversation.NodeID              `json:"parent_node_id,omitempty"`
	FromBranchID   conversation.BranchID            `json:"from_branch_id,omitempty"`
	PayloadKind    conversation.PayloadKind         `json:"payload_kind,omitempty"`
	Payload        json.RawMessage                  `json:"payload,omitempty"`
	At             string                           `json:"at"`
}

func Open(path string) *Store {
	return &Store{path: path}
}

func (s *Store) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *Store) AppendEvents(_ context.Context, events ...conversation.Event) error {
	if s == nil || s.path == "" {
		return fmt.Errorf("jsonlstore: path is required")
	}
	if len(events) == 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, event := range events {
		rec, err := encode(event)
		if err != nil {
			return err
		}
		if err := enc.Encode(rec); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoadEvents(_ context.Context, conversationID conversation.ConversationID) ([]conversation.Event, error) {
	if s == nil || s.path == "" {
		return nil, fmt.Errorf("jsonlstore: path is required")
	}
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out []conversation.Event
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, err
		}
		event, err := decode(rec)
		if err != nil {
			return nil, err
		}
		if conversationID == "" || event.ConversationID == conversationID {
			out = append(out, event)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func encode(event conversation.Event) (record, error) {
	rec := record{
		Version:        1,
		Kind:           event.Kind,
		ConversationID: event.ConversationID,
		SessionID:      event.SessionID,
		BranchID:       event.BranchID,
		NodeID:         event.NodeID,
		ParentNodeID:   event.ParentNodeID,
		FromBranchID:   event.FromBranchID,
		At:             event.At.Format(time.RFC3339Nano),
	}
	if event.Payload != nil {
		kind, payload, err := conversation.MarshalPayload(event.Payload)
		if err != nil {
			return record{}, err
		}
		rec.PayloadKind = kind
		rec.Payload = payload
	}
	return rec, nil
}

func decode(rec record) (conversation.Event, error) {
	var payload conversation.Payload
	if rec.PayloadKind != "" {
		decoded, err := conversation.UnmarshalPayload(rec.PayloadKind, rec.Payload)
		if err != nil {
			return conversation.Event{}, err
		}
		payload = decoded
	}
	at, err := parseTime(rec.At)
	if err != nil {
		return conversation.Event{}, err
	}
	return conversation.Event{
		Kind:           rec.Kind,
		ConversationID: rec.ConversationID,
		SessionID:      rec.SessionID,
		BranchID:       rec.BranchID,
		NodeID:         rec.NodeID,
		ParentNodeID:   rec.ParentNodeID,
		FromBranchID:   rec.FromBranchID,
		Payload:        payload,
		At:             at,
	}, nil
}

func parseTime(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339Nano, value)
}
