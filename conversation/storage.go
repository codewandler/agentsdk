package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/codewandler/llmadapter/unified"
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

type Event struct {
	Kind           StructuralEventKind `json:"kind"`
	ConversationID ConversationID      `json:"conversation_id,omitempty"`
	SessionID      SessionID           `json:"session_id,omitempty"`
	BranchID       BranchID            `json:"branch_id,omitempty"`
	NodeID         NodeID              `json:"node_id,omitempty"`
	ParentNodeID   NodeID              `json:"parent_node_id,omitempty"`
	FromBranchID   BranchID            `json:"from_branch_id,omitempty"`
	Payload        Payload             `json:"-"`
	At             time.Time           `json:"at"`
}

type EventStore interface {
	AppendEvents(ctx context.Context, events ...Event) error
	LoadEvents(ctx context.Context, conversationID ConversationID) ([]Event, error)
}

type MemoryStore struct {
	events []Event
}

func NewMemoryStore() *MemoryStore { return &MemoryStore{} }

func (s *MemoryStore) AppendEvents(_ context.Context, events ...Event) error {
	s.events = append(s.events, events...)
	return nil
}

func (s *MemoryStore) LoadEvents(_ context.Context, conversationID ConversationID) ([]Event, error) {
	out := make([]Event, 0, len(s.events))
	for _, event := range s.events {
		if conversationID == "" || event.ConversationID == conversationID {
			out = append(out, event)
		}
	}
	return out, nil
}

func MarshalPayload(payload Payload) (PayloadKind, json.RawMessage, error) {
	if payload == nil {
		return "", nil, fmt.Errorf("conversation: payload is required")
	}
	kind := payload.Kind()
	wire, err := payloadToWire(payload)
	if err != nil {
		return "", nil, err
	}
	b, err := json.Marshal(wire)
	if err != nil {
		return "", nil, err
	}
	return kind, b, nil
}

func UnmarshalPayload(kind PayloadKind, b []byte) (Payload, error) {
	switch kind {
	case PayloadMessage:
		var payload wireMessageEvent
		if err := json.Unmarshal(b, &payload); err != nil {
			return nil, err
		}
		return MessageEvent{Message: payload.Message.unified()}, nil
	case PayloadAssistantTurn:
		var payload wireAssistantTurnEvent
		if err := json.Unmarshal(b, &payload); err != nil {
			return nil, err
		}
		return AssistantTurnEvent{
			Message:       payload.Message.unified(),
			FinishReason:  payload.FinishReason,
			Usage:         payload.Usage,
			Continuations: payload.Continuations,
		}, nil
	case PayloadCompaction:
		var payload CompactionEvent
		if err := json.Unmarshal(b, &payload); err != nil {
			return nil, err
		}
		return payload, nil
	case PayloadAnnotation:
		var payload AnnotationEvent
		if err := json.Unmarshal(b, &payload); err != nil {
			return nil, err
		}
		return payload, nil
	default:
		return nil, fmt.Errorf("conversation: unsupported payload kind %q", kind)
	}
}

type wireMessageEvent struct {
	Message wireMessage `json:"message"`
}

type wireAssistantTurnEvent struct {
	Message       wireMessage            `json:"message"`
	FinishReason  unified.FinishReason   `json:"finish_reason,omitempty"`
	Usage         unified.Usage          `json:"usage,omitempty"`
	Continuations []ProviderContinuation `json:"continuations,omitempty"`
}

type wireMessage struct {
	Role        unified.Role       `json:"role"`
	ID          string             `json:"id,omitempty"`
	Name        string             `json:"name,omitempty"`
	Content     []wireContentPart  `json:"content,omitempty"`
	ToolCalls   []unified.ToolCall `json:"tool_calls,omitempty"`
	ToolResults []wireToolResult   `json:"tool_results,omitempty"`
	Meta        map[string]any     `json:"meta,omitempty"`
}

type wireToolResult struct {
	ToolCallID string            `json:"tool_call_id"`
	Name       string            `json:"name,omitempty"`
	Content    []wireContentPart `json:"content,omitempty"`
	IsError    bool              `json:"is_error,omitempty"`
}

type wireContentPart struct {
	Type         unified.ContentKind   `json:"type"`
	Text         string                `json:"text,omitempty"`
	CacheControl *unified.CacheControl `json:"cache_control,omitempty"`
	Source       *unified.BlobSource   `json:"source,omitempty"`
	Alt          string                `json:"alt,omitempty"`
	Filename     string                `json:"filename,omitempty"`
	MIMEType     string                `json:"mime_type,omitempty"`
}

func payloadToWire(payload Payload) (any, error) {
	switch payload := payload.(type) {
	case MessageEvent:
		msg, err := messageToWire(payload.Message)
		if err != nil {
			return nil, err
		}
		return wireMessageEvent{Message: msg}, nil
	case *MessageEvent:
		if payload == nil {
			return nil, fmt.Errorf("conversation: message payload is nil")
		}
		msg, err := messageToWire(payload.Message)
		if err != nil {
			return nil, err
		}
		return wireMessageEvent{Message: msg}, nil
	case AssistantTurnEvent:
		msg, err := messageToWire(payload.Message)
		if err != nil {
			return nil, err
		}
		return wireAssistantTurnEvent{
			Message:       msg,
			FinishReason:  payload.FinishReason,
			Usage:         payload.Usage,
			Continuations: payload.Continuations,
		}, nil
	case *AssistantTurnEvent:
		if payload == nil {
			return nil, fmt.Errorf("conversation: assistant turn payload is nil")
		}
		msg, err := messageToWire(payload.Message)
		if err != nil {
			return nil, err
		}
		return wireAssistantTurnEvent{
			Message:       msg,
			FinishReason:  payload.FinishReason,
			Usage:         payload.Usage,
			Continuations: payload.Continuations,
		}, nil
	default:
		return payload, nil
	}
}

func messageToWire(msg unified.Message) (wireMessage, error) {
	content, err := contentPartsToWire(msg.Content)
	if err != nil {
		return wireMessage{}, err
	}
	toolResults := make([]wireToolResult, 0, len(msg.ToolResults))
	for _, result := range msg.ToolResults {
		resultContent, err := contentPartsToWire(result.Content)
		if err != nil {
			return wireMessage{}, err
		}
		toolResults = append(toolResults, wireToolResult{
			ToolCallID: result.ToolCallID,
			Name:       result.Name,
			Content:    resultContent,
			IsError:    result.IsError,
		})
	}
	return wireMessage{
		Role:        msg.Role,
		ID:          msg.ID,
		Name:        msg.Name,
		Content:     content,
		ToolCalls:   append([]unified.ToolCall(nil), msg.ToolCalls...),
		ToolResults: toolResults,
		Meta:        msg.Meta,
	}, nil
}

func (msg wireMessage) unified() unified.Message {
	toolResults := make([]unified.ToolResult, 0, len(msg.ToolResults))
	for _, result := range msg.ToolResults {
		toolResults = append(toolResults, unified.ToolResult{
			ToolCallID: result.ToolCallID,
			Name:       result.Name,
			Content:    contentPartsFromWire(result.Content),
			IsError:    result.IsError,
		})
	}
	return unified.Message{
		Role:        msg.Role,
		ID:          msg.ID,
		Name:        msg.Name,
		Content:     contentPartsFromWire(msg.Content),
		ToolCalls:   append([]unified.ToolCall(nil), msg.ToolCalls...),
		ToolResults: toolResults,
		Meta:        msg.Meta,
	}
}

func contentPartsToWire(parts []unified.ContentPart) ([]wireContentPart, error) {
	out := make([]wireContentPart, 0, len(parts))
	for _, part := range parts {
		wire, err := contentPartToWire(part)
		if err != nil {
			return nil, err
		}
		out = append(out, wire)
	}
	return out, nil
}

func contentPartToWire(part unified.ContentPart) (wireContentPart, error) {
	switch part := part.(type) {
	case unified.TextPart:
		return wireContentPart{Type: unified.ContentKindText, Text: part.Text, CacheControl: part.CacheControl}, nil
	case *unified.TextPart:
		if part == nil {
			return wireContentPart{}, fmt.Errorf("conversation: text content part is nil")
		}
		return wireContentPart{Type: unified.ContentKindText, Text: part.Text, CacheControl: part.CacheControl}, nil
	case unified.ImagePart:
		return wireContentPart{Type: unified.ContentKindImage, Source: &part.Source, Alt: part.Alt}, nil
	case unified.AudioPart:
		return wireContentPart{Type: unified.ContentKindAudio, Source: &part.Source}, nil
	case unified.VideoPart:
		return wireContentPart{Type: unified.ContentKindVideo, Source: &part.Source}, nil
	case unified.FilePart:
		return wireContentPart{Type: unified.ContentKindFile, Source: &part.Source, Filename: part.Filename, MIMEType: part.MIMEType}, nil
	case unified.ReasoningPart:
		return wireContentPart{Type: unified.ContentKindReasoning, Text: part.Text}, nil
	case unified.RefusalPart:
		return wireContentPart{Type: unified.ContentKindRefusal, Text: part.Text}, nil
	default:
		return wireContentPart{}, fmt.Errorf("conversation: unsupported content part type %T", part)
	}
}

func contentPartsFromWire(parts []wireContentPart) []unified.ContentPart {
	out := make([]unified.ContentPart, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case unified.ContentKindText, "":
			out = append(out, unified.TextPart{Text: part.Text, CacheControl: part.CacheControl})
		case unified.ContentKindImage:
			out = append(out, unified.ImagePart{Source: derefBlobSource(part.Source), Alt: part.Alt})
		case unified.ContentKindAudio:
			out = append(out, unified.AudioPart{Source: derefBlobSource(part.Source)})
		case unified.ContentKindVideo:
			out = append(out, unified.VideoPart{Source: derefBlobSource(part.Source)})
		case unified.ContentKindFile:
			out = append(out, unified.FilePart{Source: derefBlobSource(part.Source), Filename: part.Filename, MIMEType: part.MIMEType})
		case unified.ContentKindReasoning:
			out = append(out, unified.ReasoningPart{Text: part.Text})
		case unified.ContentKindRefusal:
			out = append(out, unified.RefusalPart{Text: part.Text})
		}
	}
	return out
}

func derefBlobSource(source *unified.BlobSource) unified.BlobSource {
	if source == nil {
		return unified.BlobSource{}
	}
	return *source
}
