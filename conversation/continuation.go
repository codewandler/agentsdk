package conversation

import "github.com/codewandler/llmadapter/unified"

type ProviderIdentity struct {
	ProviderName string `json:"provider_name,omitempty"`
	APIKind      string `json:"api_kind,omitempty"`
	APIFamily    string `json:"api_family,omitempty"`
	NativeModel  string `json:"native_model,omitempty"`
}

type ProviderContinuation struct {
	ProviderName string             `json:"provider_name,omitempty"`
	APIKind      string             `json:"api_kind,omitempty"`
	APIFamily    string             `json:"api_family,omitempty"`
	NativeModel  string             `json:"native_model,omitempty"`
	ResponseID   string             `json:"response_id,omitempty"`
	Extensions   unified.Extensions `json:"extensions,omitempty"`
}

func NewProviderContinuation(identity ProviderIdentity, responseID string, extensions unified.Extensions) ProviderContinuation {
	return ProviderContinuation{
		ProviderName: identity.ProviderName,
		APIKind:      identity.APIKind,
		APIFamily:    identity.APIFamily,
		NativeModel:  identity.NativeModel,
		ResponseID:   responseID,
		Extensions:   extensions,
	}
}

func (c ProviderContinuation) Matches(identity ProviderIdentity) bool {
	if identity.ProviderName != "" && c.ProviderName != "" && identity.ProviderName != c.ProviderName {
		return false
	}
	if identity.APIKind != "" && c.APIKind != "" && identity.APIKind != c.APIKind {
		return false
	}
	if identity.APIFamily != "" && c.APIFamily != "" && identity.APIFamily != c.APIFamily {
		return false
	}
	if identity.NativeModel != "" && c.NativeModel != "" && identity.NativeModel != c.NativeModel {
		return false
	}
	return true
}

func ContinuationAtHead(tree *Tree, branch BranchID, identity ProviderIdentity) (ProviderContinuation, bool, error) {
	path, err := tree.Path(branch)
	if err != nil {
		return ProviderContinuation{}, false, err
	}
	for i := len(path) - 1; i >= 0; i-- {
		var continuations []ProviderContinuation
		switch ev := path[i].Payload.(type) {
		case AssistantTurnEvent:
			continuations = ev.Continuations
		case *AssistantTurnEvent:
			continuations = ev.Continuations
		}
		for _, continuation := range continuations {
			if continuation.ResponseID != "" && continuation.Matches(identity) {
				return continuation, true, nil
			}
		}
	}
	return ProviderContinuation{}, false, nil
}
