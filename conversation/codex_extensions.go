package conversation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/codewandler/llmadapter/unified"
)

func AddCodexSessionHints(req *unified.Request, identity ProviderIdentity, sessionID string, tree *Tree, branch BranchID) error {
	head, _ := tree.Head(branch)
	hints := unified.CodexExtensions{
		InteractionMode: unified.InteractionSession,
		SessionID:       sessionID,
		BranchID:        string(branch),
		BranchHeadID:    string(head),
		InputBaseHash:   codexInputBaseHash(*req),
	}
	if continuation, ok, err := ContinuationAtBranchHead(tree, branch, identity); err == nil && ok {
		hints.ParentResponseID = continuation.ResponseID
	} else if err != nil {
		return err
	}
	return unified.SetCodexExtensions(&req.Extensions, hints)
}

func codexInputBaseHash(req unified.Request) string {
	req.Extensions = unified.Extensions{}
	raw, err := json.Marshal(struct {
		Model        string                `json:"model,omitempty"`
		Instructions []unified.Instruction `json:"instructions,omitempty"`
		Messages     []unified.Message     `json:"messages,omitempty"`
		Tools        []unified.Tool        `json:"tools,omitempty"`
		ToolChoice   *unified.ToolChoice   `json:"tool_choice,omitempty"`
	}{
		Model:        req.Model,
		Instructions: req.Instructions,
		Messages:     req.Messages,
		Tools:        req.Tools,
		ToolChoice:   req.ToolChoice,
	})
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func IsCodexResponsesIdentity(identity ProviderIdentity) bool {
	provider := strings.ToLower(strings.TrimSpace(identity.ProviderName))
	apiKind := strings.ToLower(strings.TrimSpace(identity.APIKind))
	apiFamily := strings.ToLower(strings.TrimSpace(identity.APIFamily))
	return strings.Contains(provider, "codex") ||
		strings.Contains(apiKind, "codex") ||
		strings.Contains(apiFamily, "codex")
}
