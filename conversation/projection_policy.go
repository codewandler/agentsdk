package conversation

import (
	"unicode/utf8"

	"github.com/codewandler/llmadapter/unified"
)

type ProjectionInput struct {
	Tree                    *Tree
	Branch                  BranchID
	ProviderIdentity        ProviderIdentity
	Messages                []unified.Message
	PendingMessages         []unified.Message
	Extensions              unified.Extensions
	AllowNativeContinuation bool
}

type ProjectionResult struct {
	Messages   []unified.Message
	Extensions unified.Extensions
}

type ProjectionPolicy interface {
	Project(input ProjectionInput) (ProjectionResult, error)
}

type ProjectionPolicyFunc func(ProjectionInput) (ProjectionResult, error)

func (f ProjectionPolicyFunc) Project(input ProjectionInput) (ProjectionResult, error) {
	return f(input)
}

func DefaultProjectionPolicy() ProjectionPolicy {
	return ProjectionPolicyFunc(defaultProject)
}

func NewMessageBudgetProjectionPolicy(maxMessages int) ProjectionPolicy {
	return NewBudgetProjectionPolicy(DefaultProjectionPolicy(), BudgetOptions{MaxMessages: maxMessages})
}

func NewTokenBudgetProjectionPolicy(maxTokens int) ProjectionPolicy {
	return NewBudgetProjectionPolicy(DefaultProjectionPolicy(), BudgetOptions{MaxTokens: maxTokens})
}

type BudgetOptions struct {
	MaxMessages             int
	MaxTokens               int
	ProtectedRecentMessages int
	CompactionSummary       string
	Estimator               TokenEstimator
}

type TokenEstimator func(unified.Message) int

func NewBudgetProjectionPolicy(base ProjectionPolicy, opts BudgetOptions) ProjectionPolicy {
	if base == nil {
		base = DefaultProjectionPolicy()
	}
	return ProjectionPolicyFunc(func(input ProjectionInput) (ProjectionResult, error) {
		result, err := base.Project(input)
		if err != nil {
			return ProjectionResult{}, err
		}
		result.Messages = applyBudget(result.Messages, opts)
		return result, nil
	})
}

func applyBudget(messages []unified.Message, opts BudgetOptions) []unified.Message {
	out := append([]unified.Message(nil), messages...)
	if opts.MaxMessages > 0 && len(out) > opts.MaxMessages {
		start := repairToolBoundary(out, len(out)-opts.MaxMessages)
		out = append([]unified.Message(nil), out[start:]...)
	}
	if opts.MaxTokens > 0 && estimateMessages(out, opts.estimator()) > opts.MaxTokens {
		out = trimToTokenBudget(out, opts)
	}
	return out
}

func trimToTokenBudget(messages []unified.Message, opts BudgetOptions) []unified.Message {
	if len(messages) == 0 {
		return nil
	}
	estimator := opts.estimator()
	protected := opts.ProtectedRecentMessages
	if protected < 0 {
		protected = 0
	}
	if protected > len(messages) {
		protected = len(messages)
	}
	start := len(messages)
	total := 0
	for i := len(messages) - 1; i >= 0; i-- {
		cost := estimator(messages[i])
		if i >= len(messages)-protected || total+cost <= opts.MaxTokens {
			total += cost
			start = i
			continue
		}
		break
	}
	if start == len(messages) {
		start = len(messages) - 1
	}
	start = repairToolBoundary(messages, start)
	kept := append([]unified.Message(nil), messages[start:]...)
	if start == 0 || opts.CompactionSummary == "" {
		return kept
	}
	summary := unified.Message{
		Role: unified.RoleSystem,
		Content: []unified.ContentPart{unified.TextPart{
			Text: opts.CompactionSummary,
		}},
	}
	return append([]unified.Message{summary}, kept...)
}

func repairToolBoundary(messages []unified.Message, start int) int {
	if start <= 0 || start >= len(messages) {
		return start
	}
	for start > 0 && messages[start].Role == unified.RoleTool {
		start--
	}
	return start
}

func estimateMessages(messages []unified.Message, estimator TokenEstimator) int {
	total := 0
	for _, message := range messages {
		total += estimator(message)
	}
	return total
}

func (o BudgetOptions) estimator() TokenEstimator {
	if o.Estimator != nil {
		return o.Estimator
	}
	return EstimateMessageTokens
}

func EstimateMessageTokens(message unified.Message) int {
	chars := len(message.Role) + len(message.Name) + len(message.ID) + 8
	for _, part := range message.Content {
		chars += estimateContentChars(part)
	}
	for _, call := range message.ToolCalls {
		chars += len(call.ID) + len(call.Name) + len(call.Arguments) + 16
	}
	for _, result := range message.ToolResults {
		chars += len(result.ToolCallID) + len(result.Name) + 16
		for _, part := range result.Content {
			chars += estimateContentChars(part)
		}
	}
	return max(1, chars/4)
}

func estimateContentChars(part unified.ContentPart) int {
	switch part := part.(type) {
	case unified.TextPart:
		return utf8.RuneCountInString(part.Text)
	case *unified.TextPart:
		if part == nil {
			return 0
		}
		return utf8.RuneCountInString(part.Text)
	case unified.ReasoningPart:
		return utf8.RuneCountInString(part.Text) + len(part.Signature)
	case *unified.ReasoningPart:
		if part == nil {
			return 0
		}
		return utf8.RuneCountInString(part.Text) + len(part.Signature)
	case unified.RefusalPart:
		return utf8.RuneCountInString(part.Text)
	case unified.FilePart:
		return len(part.Filename) + len(part.MIMEType) + len(part.Source.URL) + len(part.Source.Base64)
	case unified.ImagePart:
		return len(part.Alt) + len(part.Source.URL) + len(part.Source.Base64)
	case unified.AudioPart:
		return len(part.Source.URL) + len(part.Source.Base64)
	case unified.VideoPart:
		return len(part.Source.URL) + len(part.Source.Base64)
	default:
		return 0
	}
}

func defaultProject(input ProjectionInput) (ProjectionResult, error) {
	extensions := cloneExtensions(input.Extensions)
	pendingMessages := append([]unified.Message(nil), input.PendingMessages...)
	if input.AllowNativeContinuation && SupportsPreviousResponseID(input.ProviderIdentity) && !extensions.Has(unified.ExtOpenAIPreviousResponseID) {
		continuation, ok, err := ContinuationAtBranchHead(input.Tree, input.Branch, input.ProviderIdentity)
		if err != nil {
			return ProjectionResult{}, err
		}
		if ok {
			extensions = mergeExtensions(continuation.Extensions, input.Extensions)
			if !extensions.Has(unified.ExtOpenAIPreviousResponseID) {
				if err := extensions.Set(unified.ExtOpenAIPreviousResponseID, continuation.ResponseID); err != nil {
					return ProjectionResult{}, err
				}
			}
			return ProjectionResult{Messages: pendingMessages, Extensions: extensions}, nil
		}
	}
	messages := append([]unified.Message(nil), input.Messages...)
	messages = append(messages, pendingMessages...)
	return ProjectionResult{Messages: messages, Extensions: extensions}, nil
}

func cloneExtensions(in unified.Extensions) unified.Extensions {
	var out unified.Extensions
	for _, key := range in.Keys() {
		_ = out.SetRaw(key, in.Raw(key))
	}
	return out
}

func mergeExtensions(base unified.Extensions, overlay unified.Extensions) unified.Extensions {
	out := cloneExtensions(base)
	for _, key := range overlay.Keys() {
		_ = out.SetRaw(key, overlay.Raw(key))
	}
	return out
}
