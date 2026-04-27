package conversation

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/codewandler/agentsdk/thread"
)

const (
	EventConversationUserMessage      thread.EventKind = "conversation.user_message"
	EventConversationAssistantMessage thread.EventKind = "conversation.assistant_message"
	EventConversationToolResult       thread.EventKind = "conversation.tool_result"
	EventConversationMessage          thread.EventKind = "conversation.message"
	EventConversationCompaction       thread.EventKind = "conversation.compaction"
	EventConversationAnnotation       thread.EventKind = "conversation.annotation"
)

type ThreadEventStore struct {
	store thread.Store
	live  thread.Live
}

func NewThreadEventStore(store thread.Store, live thread.Live) *ThreadEventStore {
	return &ThreadEventStore{store: store, live: live}
}

func (s *ThreadEventStore) AppendEvents(ctx context.Context, events ...Event) error {
	if s == nil || s.live == nil {
		return fmt.Errorf("conversation: live thread is required")
	}
	threadEvents := make([]thread.Event, 0, len(events))
	for _, event := range events {
		threadEvent, err := threadEventFromConversationEvent(event)
		if err != nil {
			return err
		}
		threadEvents = append(threadEvents, threadEvent)
	}
	return s.live.Append(ctx, threadEvents...)
}

func (s *ThreadEventStore) LoadEvents(ctx context.Context, conversationID ConversationID) ([]Event, error) {
	if s == nil || s.store == nil || s.live == nil {
		return nil, fmt.Errorf("conversation: thread store and live thread are required")
	}
	stored, err := s.store.Read(ctx, thread.ReadParams{ID: s.live.ID()})
	if err != nil {
		return nil, err
	}
	branchEvents, err := stored.EventsForBranch(s.live.BranchID())
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, threadEvent := range branchEvents {
		event, ok, err := conversationEventFromThreadEvent(threadEvent)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if conversationID != "" {
			event.ConversationID = conversationID
		}
		out = append(out, event)
	}
	return out, nil
}

func threadEventFromConversationEvent(event Event) (thread.Event, error) {
	switch event.Kind {
	case StructuralNodeAppended:
		kind, payload, err := marshalThreadPayload(event.Payload)
		if err != nil {
			return thread.Event{}, err
		}
		return thread.Event{
			Kind:         kind,
			BranchID:     thread.BranchID(event.BranchID),
			NodeID:       thread.NodeID(event.NodeID),
			ParentNodeID: thread.NodeID(event.ParentNodeID),
			Payload:      payload,
			At:           event.At,
			Source:       thread.EventSource{Type: "conversation", SessionID: string(event.SessionID)},
		}, nil
	case StructuralBranchCreated:
		payload, err := json.Marshal(struct {
			FromBranchID thread.BranchID `json:"from_branch_id"`
			ToBranchID   thread.BranchID `json:"to_branch_id"`
		}{
			FromBranchID: thread.BranchID(event.FromBranchID),
			ToBranchID:   thread.BranchID(event.BranchID),
		})
		if err != nil {
			return thread.Event{}, err
		}
		return thread.Event{
			Kind:     thread.EventBranchCreated,
			BranchID: thread.BranchID(event.BranchID),
			NodeID:   thread.NodeID(event.NodeID),
			Payload:  payload,
			At:       event.At,
			Source:   thread.EventSource{Type: "conversation", SessionID: string(event.SessionID)},
		}, nil
	default:
		return thread.Event{}, fmt.Errorf("conversation: unsupported thread-backed structural event %q", event.Kind)
	}
}

func marshalThreadPayload(payload Payload) (thread.EventKind, json.RawMessage, error) {
	switch payload := payload.(type) {
	case MessageEvent:
		return marshalMessagePayload(payload)
	case *MessageEvent:
		if payload == nil {
			return "", nil, fmt.Errorf("conversation: message payload is nil")
		}
		return marshalMessagePayload(*payload)
	case AssistantTurnEvent:
		wire, err := payloadToWire(payload)
		if err != nil {
			return "", nil, err
		}
		raw, err := json.Marshal(wire)
		return EventConversationAssistantMessage, raw, err
	case *AssistantTurnEvent:
		if payload == nil {
			return "", nil, fmt.Errorf("conversation: assistant turn payload is nil")
		}
		wire, err := payloadToWire(payload)
		if err != nil {
			return "", nil, err
		}
		raw, err := json.Marshal(wire)
		return EventConversationAssistantMessage, raw, err
	case CompactionEvent:
		raw, err := json.Marshal(payload)
		return EventConversationCompaction, raw, err
	case *CompactionEvent:
		if payload == nil {
			return "", nil, fmt.Errorf("conversation: compaction payload is nil")
		}
		raw, err := json.Marshal(payload)
		return EventConversationCompaction, raw, err
	case AnnotationEvent:
		raw, err := json.Marshal(payload)
		return EventConversationAnnotation, raw, err
	case *AnnotationEvent:
		if payload == nil {
			return "", nil, fmt.Errorf("conversation: annotation payload is nil")
		}
		raw, err := json.Marshal(payload)
		return EventConversationAnnotation, raw, err
	default:
		return "", nil, fmt.Errorf("conversation: unsupported thread payload %T", payload)
	}
}

func marshalMessagePayload(payload MessageEvent) (thread.EventKind, json.RawMessage, error) {
	wire, err := payloadToWire(payload)
	if err != nil {
		return "", nil, err
	}
	raw, err := json.Marshal(wire)
	if err != nil {
		return "", nil, err
	}
	switch payload.Message.Role {
	case "user":
		return EventConversationUserMessage, raw, nil
	case "tool":
		return EventConversationToolResult, raw, nil
	case "assistant":
		return EventConversationAssistantMessage, raw, nil
	default:
		return EventConversationMessage, raw, nil
	}
}

func conversationEventFromThreadEvent(event thread.Event) (Event, bool, error) {
	switch event.Kind {
	case EventConversationUserMessage, EventConversationToolResult, EventConversationMessage:
		payload, err := UnmarshalPayload(PayloadMessage, event.Payload)
		if err != nil {
			return Event{}, false, err
		}
		return eventWithPayload(event, payload), true, nil
	case EventConversationAssistantMessage:
		payload, err := UnmarshalPayload(PayloadAssistantTurn, event.Payload)
		if err != nil {
			if messagePayload, messageErr := UnmarshalPayload(PayloadMessage, event.Payload); messageErr == nil {
				return eventWithPayload(event, messagePayload), true, nil
			}
			return Event{}, false, err
		}
		return eventWithPayload(event, payload), true, nil
	case EventConversationCompaction:
		payload, err := UnmarshalPayload(PayloadCompaction, event.Payload)
		if err != nil {
			return Event{}, false, err
		}
		return eventWithPayload(event, payload), true, nil
	case EventConversationAnnotation:
		payload, err := UnmarshalPayload(PayloadAnnotation, event.Payload)
		if err != nil {
			return Event{}, false, err
		}
		return eventWithPayload(event, payload), true, nil
	case thread.EventBranchCreated:
		var payload struct {
			FromBranchID thread.BranchID `json:"from_branch_id"`
			ToBranchID   thread.BranchID `json:"to_branch_id"`
		}
		if len(event.Payload) > 0 {
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return Event{}, false, err
			}
		}
		branchID := payload.ToBranchID
		if branchID == "" {
			branchID = event.BranchID
		}
		return Event{
			Kind:           StructuralBranchCreated,
			ConversationID: ConversationID(event.ThreadID),
			SessionID:      SessionID(event.Source.SessionID),
			BranchID:       BranchID(branchID),
			FromBranchID:   BranchID(payload.FromBranchID),
			NodeID:         NodeID(event.NodeID),
			At:             event.At,
		}, true, nil
	default:
		return Event{}, false, nil
	}
}

func eventWithPayload(event thread.Event, payload Payload) Event {
	return Event{
		Kind:           StructuralNodeAppended,
		ConversationID: ConversationID(event.ThreadID),
		SessionID:      SessionID(event.Source.SessionID),
		BranchID:       BranchID(event.BranchID),
		NodeID:         NodeID(event.NodeID),
		ParentNodeID:   NodeID(event.ParentNodeID),
		Payload:        payload,
		At:             event.At,
	}
}
