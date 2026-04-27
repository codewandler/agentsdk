package conversation

import (
	"unicode/utf8"

	"github.com/codewandler/llmadapter/unified"
)

type ProjectionInput struct {
	Tree                    *Tree
	Branch                  BranchID
	ProviderIdentity        ProviderIdentity
	Items                   []Item
	PendingItems            []Item
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

type TokenEstimator func(unified.Message) int

func EstimateMessagesTokens(messages []unified.Message, estimator TokenEstimator) int {
	if estimator == nil {
		estimator = EstimateMessageTokens
	}
	total := 0
	for _, message := range messages {
		total += estimator(message)
	}
	return total
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
	items := append([]Item(nil), input.Items...)
	pendingItems := append([]Item(nil), input.PendingItems...)
	pendingMessages := MessagesFromItems(pendingItems)
	if input.AllowNativeContinuation && !extensions.Has(unified.ExtOpenAIPreviousResponseID) {
		continuation, ok, err := ContinuationAtBranchHead(input.Tree, input.Branch, input.ProviderIdentity)
		if err != nil {
			return ProjectionResult{}, err
		}
		if ok && continuation.SupportsPublicPreviousResponseID() {
			extensions = mergeExtensions(continuation.Extensions, input.Extensions)
			if !extensions.Has(unified.ExtOpenAIPreviousResponseID) {
				if err := extensions.Set(unified.ExtOpenAIPreviousResponseID, continuation.ResponseID); err != nil {
					return ProjectionResult{}, err
				}
			}
			return ProjectionResult{Messages: pendingMessages, Extensions: extensions}, nil
		}
	}
	messages := MessagesFromItems(items)
	messages = append(messages, pendingMessages...)
	return ProjectionResult{Messages: messages, Extensions: extensions}, nil
}

func ItemsFromMessages(messages []unified.Message) []Item {
	items := make([]Item, 0, len(messages))
	for _, message := range messages {
		items = append(items, Item{Kind: ItemMessage, Message: message})
	}
	return items
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
