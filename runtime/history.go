package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
)

type History struct {
	sessionID         string
	sessionIDExplicit bool
	branch            conversation.BranchID
	tree              *conversation.Tree
	defaults          historyDefaults
	live              thread.Live
}

type historyDefaults struct {
	model           string
	maxOutputTokens *int
	temperature     *float64
	topP            *float64
	topK            *int
	stop            []string
	seed            *int64
	responseFormat  *unified.ResponseFormat
	reasoning       *unified.ReasoningConfig
	safety          *unified.SafetyConfig
	instructions    []unified.Instruction
	tools           []unified.Tool
	toolChoice      *unified.ToolChoice
	user            string
	cachePolicy     unified.CachePolicy
	cacheKey        string
	cacheTTL        string
	projection      conversation.ProjectionPolicy
}

type HistoryOption func(*History)

func NewHistory(opts ...HistoryOption) *History {
	h := &History{
		sessionID: newHistorySessionID(),
		branch:    conversation.MainBranch,
		tree:      conversation.NewTree(),
		defaults:  historyDefaults{cachePolicy: unified.CachePolicyOn},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(h)
		}
	}
	return h
}

func ResumeHistoryFromThread(ctx context.Context, store thread.Store, live thread.Live, opts ...HistoryOption) (*History, error) {
	if store == nil || live == nil {
		return nil, fmt.Errorf("runtime: thread store and live thread are required")
	}
	h := NewHistory(append(opts, WithHistoryLiveThread(live))...)
	h.branch = conversation.BranchID(live.BranchID())
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		return nil, err
	}
	events, err := stored.EventsForBranch(live.BranchID())
	if err != nil {
		return nil, err
	}
	var inferredSessionID string
	for _, event := range events {
		if inferredSessionID == "" && event.Source.SessionID != "" {
			inferredSessionID = event.Source.SessionID
		}
		payload, ok, err := payloadFromThreadEvent(event)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if err := h.tree.InsertNode(conversation.BranchID(event.BranchID), conversation.Node{
			ID:        conversation.NodeID(event.NodeID),
			Parent:    conversation.NodeID(event.ParentNodeID),
			Payload:   payload,
			CreatedAt: event.At,
		}); err != nil {
			return nil, err
		}
	}
	if !h.sessionIDExplicit && inferredSessionID != "" {
		h.sessionID = inferredSessionID
	}
	return h, nil
}

func WithHistorySessionID(id string) HistoryOption {
	return func(h *History) {
		if id != "" {
			h.sessionID = id
			h.sessionIDExplicit = true
		}
	}
}

func WithHistoryLiveThread(live thread.Live) HistoryOption {
	return func(h *History) { h.live = live }
}

func WithHistoryModel(model string) HistoryOption {
	return func(h *History) { h.defaults.model = model }
}

func WithHistoryMaxOutputTokens(max int) HistoryOption {
	return func(h *History) { h.defaults.maxOutputTokens = &max }
}

func WithHistoryTemperature(value float64) HistoryOption {
	return func(h *History) { h.defaults.temperature = &value }
}

func WithHistorySystem(text string) HistoryOption {
	return WithHistoryInstructions(unified.Instruction{
		Kind:    unified.InstructionSystem,
		Content: []unified.ContentPart{unified.TextPart{Text: text}},
	})
}

func WithHistoryInstructions(instructions ...unified.Instruction) HistoryOption {
	return func(h *History) { h.defaults.instructions = append([]unified.Instruction(nil), instructions...) }
}

func WithHistoryReasoning(reasoning unified.ReasoningConfig) HistoryOption {
	return func(h *History) { h.defaults.reasoning = &reasoning }
}

func WithHistoryTools(tools []unified.Tool) HistoryOption {
	return func(h *History) { h.defaults.tools = append([]unified.Tool(nil), tools...) }
}

func WithHistoryToolChoice(choice unified.ToolChoice) HistoryOption {
	return func(h *History) { h.defaults.toolChoice = &choice }
}

func WithHistoryCachePolicy(policy unified.CachePolicy) HistoryOption {
	return func(h *History) { h.defaults.cachePolicy = policy }
}

func WithHistoryCacheKey(key string) HistoryOption {
	return func(h *History) { h.defaults.cacheKey = key }
}

func WithHistoryCacheTTL(ttl string) HistoryOption {
	return func(h *History) { h.defaults.cacheTTL = ttl }
}

func WithHistoryProjectionPolicy(policy conversation.ProjectionPolicy) HistoryOption {
	return func(h *History) { h.defaults.projection = policy }
}

func (h *History) SessionID() string             { return h.sessionID }
func (h *History) Branch() conversation.BranchID { return h.branch }
func (h *History) Tree() *conversation.Tree      { return h.tree }

func (h *History) Messages() ([]unified.Message, error) {
	return conversation.ProjectMessages(h.tree, h.branch)
}

func (h *History) AddUser(text string) (conversation.NodeID, error) {
	return h.AppendMessage(unified.Message{
		Role:    unified.RoleUser,
		Content: []unified.ContentPart{unified.TextPart{Text: text}},
	})
}

func (h *History) AppendMessage(msg unified.Message) (conversation.NodeID, error) {
	return h.AppendContext(context.Background(), conversation.MessageEvent{Message: msg})
}

func (h *History) AppendContext(ctx context.Context, payload conversation.Payload) (conversation.NodeID, error) {
	ids, err := h.appendPayloads(ctx, nil, payload)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

func (h *History) CompactContext(ctx context.Context, summary string, replaces ...conversation.NodeID) (conversation.NodeID, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", fmt.Errorf("runtime: compaction summary is required")
	}
	for _, id := range replaces {
		if id == "" {
			return "", fmt.Errorf("runtime: compaction replacement node id is required")
		}
		if _, ok := h.tree.Node(id); !ok {
			return "", fmt.Errorf("runtime: compaction replacement node %q not found", id)
		}
	}
	return h.AppendContext(ctx, conversation.CompactionEvent{
		Summary:  summary,
		Replaces: append([]conversation.NodeID(nil), replaces...),
	})
}

func (h *History) BuildRequestForProvider(req conversation.Request, identity conversation.ProviderIdentity) (unified.Request, error) {
	return h.buildRequest(req, identity, true)
}

func (h *History) buildRequest(req conversation.Request, identity conversation.ProviderIdentity, useNativeContinuation bool) (unified.Request, error) {
	items, err := conversation.ProjectItems(h.tree, h.branch)
	if err != nil {
		return unified.Request{}, err
	}
	pendingMessages := append([]unified.Message(nil), req.Messages...)
	pendingItems := append([]conversation.Item(nil), req.Items...)
	pendingItems = append(pendingItems, conversation.ItemsFromMessages(pendingMessages)...)
	projection, err := h.projection().Project(conversation.ProjectionInput{
		Tree:                    h.tree,
		Branch:                  h.branch,
		ProviderIdentity:        identity,
		Items:                   items,
		PendingItems:            pendingItems,
		PendingMessages:         pendingMessages,
		Extensions:              req.Extensions,
		AllowNativeContinuation: useNativeContinuation,
	})
	if err != nil {
		return unified.Request{}, err
	}
	out := unified.Request{
		Model:           firstNonEmpty(req.Model, h.defaults.model),
		MaxOutputTokens: firstIntPtr(req.MaxOutputTokens, h.defaults.maxOutputTokens),
		Temperature:     firstFloatPtr(req.Temperature, h.defaults.temperature),
		TopP:            firstFloatPtr(req.TopP, h.defaults.topP),
		TopK:            firstIntPtr(req.TopK, h.defaults.topK),
		Stop:            append([]string(nil), h.defaults.stop...),
		Seed:            firstInt64Ptr(req.Seed, h.defaults.seed),
		ResponseFormat:  firstResponseFormatPtr(req.ResponseFormat, h.defaults.responseFormat),
		Reasoning:       firstReasoningPtr(req.Reasoning, h.defaults.reasoning),
		Safety:          firstSafetyPtr(req.Safety, h.defaults.safety),
		Instructions:    append(append([]unified.Instruction(nil), h.defaults.instructions...), req.Instructions...),
		Tools:           append([]unified.Tool(nil), h.defaults.tools...),
		ToolChoice:      h.defaults.toolChoice,
		Messages:        projection.Messages,
		Stream:          req.Stream,
		User:            firstNonEmpty(req.User, h.defaults.user),
		CachePolicy:     firstCachePolicy(req.CachePolicy, h.defaults.cachePolicy),
		CacheKey:        firstNonEmpty(req.CacheKey, h.defaults.cacheKey),
		CacheTTL:        firstNonEmpty(req.CacheTTL, h.defaults.cacheTTL),
		Extensions:      projection.Extensions,
	}
	if len(req.Stop) > 0 {
		out.Stop = append([]string(nil), req.Stop...)
	}
	if len(req.Tools) > 0 {
		out.Tools = append([]unified.Tool(nil), req.Tools...)
	}
	if req.ToolChoice != nil {
		out.ToolChoice = req.ToolChoice
	}
	if conversation.IsCodexResponsesIdentity(identity) {
		conversation.AddCodexSessionHints(&out, identity, h.codexSessionID(), h.tree, h.branch)
	}
	return out, nil
}

func (h *History) codexSessionID() string {
	if h != nil && h.live != nil && h.live.ID() != "" {
		return string(h.live.ID())
	}
	if h == nil {
		return ""
	}
	return h.sessionID
}

func (h *History) projection() conversation.ProjectionPolicy {
	if h.defaults.projection != nil {
		return h.defaults.projection
	}
	return conversation.DefaultProjectionPolicy()
}

func (h *History) CommitFragment(fragment *conversation.TurnFragment) ([]conversation.NodeID, error) {
	return h.CommitFragmentWithThreadEvents(context.Background(), fragment)
}

func (h *History) CommitFragmentWithThreadEvents(ctx context.Context, fragment *conversation.TurnFragment, events ...thread.Event) ([]conversation.NodeID, error) {
	if fragment == nil {
		return nil, fmt.Errorf("runtime: turn fragment is nil")
	}
	payloads, err := fragment.Payloads()
	if err != nil {
		return nil, err
	}
	return h.appendPayloads(ctx, events, payloads...)
}

func (h *History) AppendThreadEvents(ctx context.Context, events ...thread.Event) error {
	if len(events) == 0 {
		return nil
	}
	if h.live == nil {
		return fmt.Errorf("runtime: thread events require live thread")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return h.live.Append(ctx, events...)
}

func (h *History) appendPayloads(ctx context.Context, prefixEvents []thread.Event, payloads ...conversation.Payload) ([]conversation.NodeID, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	parent, _ := h.tree.Head(h.branch)
	nodes := make([]conversation.Node, 0, len(payloads))
	ids := make([]conversation.NodeID, 0, len(payloads))
	now := time.Now()
	currentParent := parent
	for _, payload := range payloads {
		if payload == nil {
			return nil, fmt.Errorf("runtime: payload is required")
		}
		id := conversation.NewNodeID()
		if _, exists := h.tree.Node(id); exists {
			return nil, fmt.Errorf("runtime: generated duplicate node %q", id)
		}
		node := conversation.Node{
			ID:        id,
			Parent:    currentParent,
			Payload:   payload,
			CreatedAt: now,
		}
		nodes = append(nodes, node)
		ids = append(ids, id)
		currentParent = id
	}
	if h.live == nil {
		if len(prefixEvents) > 0 {
			return nil, fmt.Errorf("runtime: thread events require live thread")
		}
		for _, node := range nodes {
			if err := h.tree.InsertNode(h.branch, node); err != nil {
				return nil, err
			}
		}
		return ids, nil
	}
	events := make([]thread.Event, 0, len(prefixEvents)+len(nodes))
	events = append(events, prefixEvents...)
	for i, node := range nodes {
		event, err := threadEventFromPayload(payloads[i], thread.Event{
			BranchID:     thread.BranchID(h.branch),
			NodeID:       thread.NodeID(node.ID),
			ParentNodeID: thread.NodeID(parent),
			At:           node.CreatedAt,
			Source:       thread.EventSource{Type: "conversation", SessionID: h.sessionID},
		})
		if err != nil {
			return nil, err
		}
		events = append(events, event)
		parent = node.ID
	}
	if err := h.live.Append(ctx, events...); err != nil {
		return nil, err
	}
	for _, node := range nodes {
		if err := h.tree.InsertNode(h.branch, node); err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func firstCachePolicy(a, b unified.CachePolicy) unified.CachePolicy {
	if a != "" {
		return a
	}
	return b
}

func firstIntPtr(a, b *int) *int {
	if a != nil {
		return a
	}
	return b
}

func firstFloatPtr(a, b *float64) *float64 {
	if a != nil {
		return a
	}
	return b
}

func firstInt64Ptr(a, b *int64) *int64 {
	if a != nil {
		return a
	}
	return b
}

func firstResponseFormatPtr(a, b *unified.ResponseFormat) *unified.ResponseFormat {
	if a != nil {
		return a
	}
	return b
}

func firstReasoningPtr(a, b *unified.ReasoningConfig) *unified.ReasoningConfig {
	if a != nil {
		return a
	}
	return b
}

func firstSafetyPtr(a, b *unified.SafetyConfig) *unified.SafetyConfig {
	if a != nil {
		return a
	}
	return b
}

func newHistorySessionID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return "sess_" + hex.EncodeToString(b[:])
}
