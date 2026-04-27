package conversation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codewandler/llmadapter/unified"
)

type Session struct {
	conversationID ConversationID
	sessionID      SessionID
	branch         BranchID
	tree           *Tree
	defaults       defaults
	store          EventStore
}

type defaults struct {
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
	projection      ProjectionPolicy
}

type Option func(*Session)

func New(opts ...Option) *Session {
	s := &Session{
		conversationID: NewConversationID(),
		sessionID:      NewSessionID(),
		branch:         MainBranch,
		tree:           NewTree(),
		defaults:       defaultSettings(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func Resume(ctx context.Context, store EventStore, conversationID ConversationID, opts ...Option) (*Session, error) {
	if store == nil {
		return nil, fmt.Errorf("conversation: store is required")
	}
	events, err := store.LoadEvents(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, fmt.Errorf("conversation: no events found")
	}
	s := &Session{
		branch:   MainBranch,
		tree:     NewTree(),
		store:    store,
		defaults: defaultSettings(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	for _, event := range events {
		if s.conversationID == "" {
			s.conversationID = event.ConversationID
		}
		if s.sessionID == "" {
			s.sessionID = event.SessionID
		}
		switch event.Kind {
		case StructuralConversationCreated:
			if event.BranchID != "" {
				s.branch = event.BranchID
			}
		case StructuralBranchCreated:
			if event.BranchID != "" {
				if err := s.tree.MoveHead(event.BranchID, event.NodeID); err != nil {
					_ = s.tree.Fork(MainBranch, event.BranchID)
					_ = s.tree.MoveHead(event.BranchID, event.NodeID)
				}
			}
		case StructuralNodeAppended:
			if event.Payload == nil {
				return nil, fmt.Errorf("conversation: node event %q has no payload", event.NodeID)
			}
			if err := s.tree.InsertNode(event.BranchID, Node{
				ID:        event.NodeID,
				Parent:    event.ParentNodeID,
				Payload:   event.Payload,
				CreatedAt: event.At,
			}); err != nil {
				return nil, err
			}
		case StructuralHeadMoved:
			if err := s.tree.MoveHead(event.BranchID, event.NodeID); err != nil {
				return nil, err
			}
			if event.BranchID != "" {
				s.branch = event.BranchID
			}
		}
	}
	if s.conversationID == "" {
		s.conversationID = conversationID
	}
	if s.sessionID == "" {
		s.sessionID = NewSessionID()
	}
	return s, nil
}

func WithConversationID(id ConversationID) Option {
	return func(s *Session) {
		if id != "" {
			s.conversationID = id
		}
	}
}

func WithStore(store EventStore) Option {
	return func(s *Session) { s.store = store }
}

func WithSessionID(id SessionID) Option {
	return func(s *Session) {
		if id != "" {
			s.sessionID = id
		}
	}
}

func WithModel(model string) Option {
	return func(s *Session) { s.defaults.model = model }
}

func WithMaxOutputTokens(max int) Option {
	return func(s *Session) { s.defaults.maxOutputTokens = &max }
}

func WithTemperature(v float64) Option {
	return func(s *Session) { s.defaults.temperature = &v }
}

func WithTopP(v float64) Option {
	return func(s *Session) { s.defaults.topP = &v }
}

func WithTopK(v int) Option {
	return func(s *Session) { s.defaults.topK = &v }
}

func WithStop(stop ...string) Option {
	return func(s *Session) { s.defaults.stop = append([]string(nil), stop...) }
}

func WithSeed(seed int64) Option {
	return func(s *Session) { s.defaults.seed = &seed }
}

func WithResponseFormat(format unified.ResponseFormat) Option {
	return func(s *Session) { s.defaults.responseFormat = &format }
}

func WithReasoning(reasoning unified.ReasoningConfig) Option {
	return func(s *Session) { s.defaults.reasoning = &reasoning }
}

func WithSafety(safety unified.SafetyConfig) Option {
	return func(s *Session) { s.defaults.safety = &safety }
}

func WithInstructions(instructions ...unified.Instruction) Option {
	return func(s *Session) { s.defaults.instructions = append([]unified.Instruction(nil), instructions...) }
}

func WithSystem(text string) Option {
	return WithInstructions(unified.Instruction{
		Kind:    unified.InstructionSystem,
		Content: []unified.ContentPart{unified.TextPart{Text: text}},
	})
}

func WithTools(tools []unified.Tool) Option {
	return func(s *Session) { s.defaults.tools = append([]unified.Tool(nil), tools...) }
}

func WithToolChoice(choice unified.ToolChoice) Option {
	return func(s *Session) { s.defaults.toolChoice = &choice }
}

func WithUser(user string) Option {
	return func(s *Session) { s.defaults.user = user }
}

func WithCachePolicy(policy unified.CachePolicy) Option {
	return func(s *Session) { s.defaults.cachePolicy = policy }
}

func WithCacheKey(key string) Option {
	return func(s *Session) { s.defaults.cacheKey = key }
}

func WithCacheTTL(ttl string) Option {
	return func(s *Session) { s.defaults.cacheTTL = ttl }
}

func WithProjectionPolicy(policy ProjectionPolicy) Option {
	return func(s *Session) { s.defaults.projection = policy }
}

func (s *Session) ConversationID() ConversationID { return s.conversationID }
func (s *Session) SessionID() SessionID           { return s.sessionID }
func (s *Session) Branch() BranchID               { return s.branch }
func (s *Session) Tree() *Tree                    { return s.tree }

func (s *Session) Checkout(branch BranchID) error {
	if _, ok := s.tree.Head(branch); !ok {
		return fmt.Errorf("conversation: branch %q not found", branch)
	}
	s.branch = branch
	return nil
}

func (s *Session) Fork(to BranchID) error {
	from := s.branch
	if err := s.tree.Fork(s.branch, to); err != nil {
		return err
	}
	s.branch = to
	if s.store != nil {
		head, _ := s.tree.Head(s.branch)
		if err := s.store.AppendEvents(context.Background(), Event{
			Kind:           StructuralBranchCreated,
			ConversationID: s.conversationID,
			SessionID:      s.sessionID,
			BranchID:       s.branch,
			FromBranchID:   from,
			NodeID:         head,
			At:             time.Now(),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Session) Append(payload Payload) (NodeID, error) {
	return s.AppendContext(context.Background(), payload)
}

func (s *Session) AppendContext(ctx context.Context, payload Payload) (NodeID, error) {
	ids, err := s.appendPayloads(ctx, payload)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

func (s *Session) AppendMessage(msg unified.Message) (NodeID, error) {
	return s.Append(MessageEvent{Message: msg})
}

func (s *Session) AddUser(text string) (NodeID, error) {
	return s.AppendMessage(unified.Message{
		Role:    unified.RoleUser,
		Content: []unified.ContentPart{unified.TextPart{Text: text}},
	})
}

func (s *Session) Compact(summary string, replaces ...NodeID) (NodeID, error) {
	return s.CompactContext(context.Background(), summary, replaces...)
}

func (s *Session) CompactContext(ctx context.Context, summary string, replaces ...NodeID) (NodeID, error) {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return "", fmt.Errorf("conversation: compaction summary is required")
	}
	for _, id := range replaces {
		if id == "" {
			return "", fmt.Errorf("conversation: compaction replacement node id is required")
		}
		if _, ok := s.tree.Node(id); !ok {
			return "", fmt.Errorf("conversation: compaction replacement node %q not found", id)
		}
	}
	return s.AppendContext(ctx, CompactionEvent{
		Summary:  summary,
		Replaces: append([]NodeID(nil), replaces...),
	})
}

func (s *Session) Messages() ([]unified.Message, error) {
	return ProjectMessages(s.tree, s.branch)
}

func (s *Session) BuildRequest(req Request) (unified.Request, error) {
	return s.buildRequest(req, ProviderIdentity{}, false)
}

func (s *Session) BuildRequestForProvider(req Request, identity ProviderIdentity) (unified.Request, error) {
	return s.buildRequest(req, identity, true)
}

func (s *Session) buildRequest(req Request, identity ProviderIdentity, useNativeContinuation bool) (unified.Request, error) {
	items, err := ProjectItems(s.tree, s.branch)
	if err != nil {
		return unified.Request{}, err
	}
	pendingMessages := append([]unified.Message(nil), req.Messages...)
	pendingItems := append([]Item(nil), req.Items...)
	pendingItems = append(pendingItems, itemsFromMessages(pendingMessages)...)
	projection, err := s.projection().Project(ProjectionInput{
		Tree:                    s.tree,
		Branch:                  s.branch,
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
		Model:           firstNonEmpty(req.Model, s.defaults.model),
		MaxOutputTokens: firstIntPtr(req.MaxOutputTokens, s.defaults.maxOutputTokens),
		Temperature:     firstFloatPtr(req.Temperature, s.defaults.temperature),
		TopP:            firstFloatPtr(req.TopP, s.defaults.topP),
		TopK:            firstIntPtr(req.TopK, s.defaults.topK),
		Stop:            append([]string(nil), s.defaults.stop...),
		Seed:            firstInt64Ptr(req.Seed, s.defaults.seed),
		ResponseFormat:  firstResponseFormatPtr(req.ResponseFormat, s.defaults.responseFormat),
		Reasoning:       firstReasoningPtr(req.Reasoning, s.defaults.reasoning),
		Safety:          firstSafetyPtr(req.Safety, s.defaults.safety),
		Instructions:    append(append([]unified.Instruction(nil), s.defaults.instructions...), req.Instructions...),
		Tools:           append([]unified.Tool(nil), s.defaults.tools...),
		ToolChoice:      s.defaults.toolChoice,
		Messages:        projection.Messages,
		Stream:          req.Stream,
		User:            firstNonEmpty(req.User, s.defaults.user),
		CachePolicy:     firstCachePolicy(req.CachePolicy, s.defaults.cachePolicy),
		CacheKey:        firstNonEmpty(req.CacheKey, s.defaults.cacheKey),
		CacheTTL:        firstNonEmpty(req.CacheTTL, s.defaults.cacheTTL),
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
	if isCodexResponsesIdentity(identity) {
		if err := s.addCodexSessionHints(&out, identity); err != nil {
			return unified.Request{}, err
		}
	}
	return out, nil
}

func (s *Session) projection() ProjectionPolicy {
	if s.defaults.projection != nil {
		return s.defaults.projection
	}
	return DefaultProjectionPolicy()
}

func (s *Session) CommitFragment(fragment *TurnFragment) ([]NodeID, error) {
	if fragment == nil {
		return nil, fmt.Errorf("conversation: turn fragment is nil")
	}
	payloads, err := fragment.Payloads()
	if err != nil {
		return nil, err
	}
	return s.appendPayloads(context.Background(), payloads...)
}

func (s *Session) appendPayloads(ctx context.Context, payloads ...Payload) ([]NodeID, error) {
	parent, _ := s.tree.Head(s.branch)
	ids, err := s.tree.AppendMany(s.branch, payloads...)
	if err != nil {
		return nil, err
	}
	if s.store == nil {
		return ids, nil
	}
	events := make([]Event, 0, len(ids))
	for i, id := range ids {
		node, ok := s.tree.Node(id)
		if !ok {
			return nil, fmt.Errorf("conversation: node %q not found after append", id)
		}
		events = append(events, Event{
			Kind:           StructuralNodeAppended,
			ConversationID: s.conversationID,
			SessionID:      s.sessionID,
			BranchID:       s.branch,
			NodeID:         id,
			ParentNodeID:   parent,
			Payload:        payloads[i],
			At:             node.CreatedAt,
		})
		parent = id
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := s.store.AppendEvents(ctx, events...); err != nil {
		return nil, err
	}
	return ids, nil
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func defaultSettings() defaults {
	return defaults{cachePolicy: unified.CachePolicyOn}
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
