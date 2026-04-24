package conversation

import (
	"fmt"

	"github.com/codewandler/llmadapter/unified"
)

type Session struct {
	conversationID ConversationID
	sessionID      SessionID
	branch         BranchID
	tree           *Tree
	defaults       defaults
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
}

type Option func(*Session)

func New(opts ...Option) *Session {
	s := &Session{
		conversationID: NewConversationID(),
		sessionID:      NewSessionID(),
		branch:         MainBranch,
		tree:           NewTree(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s
}

func WithConversationID(id ConversationID) Option {
	return func(s *Session) {
		if id != "" {
			s.conversationID = id
		}
	}
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
	if err := s.tree.Fork(s.branch, to); err != nil {
		return err
	}
	s.branch = to
	return nil
}

func (s *Session) Append(payload Payload) (NodeID, error) {
	return s.tree.Append(s.branch, payload)
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

func (s *Session) Messages() ([]unified.Message, error) {
	return ProjectMessages(s.tree, s.branch)
}

func (s *Session) BuildRequest(req Request) (unified.Request, error) {
	messages, err := s.Messages()
	if err != nil {
		return unified.Request{}, err
	}
	messages = append(messages, req.Messages...)
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
		Messages:        messages,
		Stream:          req.Stream,
		User:            firstNonEmpty(req.User, s.defaults.user),
		Extensions:      req.Extensions,
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
	return out, nil
}

func (s *Session) CommitFragment(fragment *TurnFragment) ([]NodeID, error) {
	if fragment == nil {
		return nil, fmt.Errorf("conversation: turn fragment is nil")
	}
	payloads, err := fragment.Payloads()
	if err != nil {
		return nil, err
	}
	return s.tree.AppendMany(s.branch, payloads...)
}

func firstNonEmpty(a, b string) string {
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
