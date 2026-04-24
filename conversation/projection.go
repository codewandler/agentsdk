package conversation

import (
	"fmt"

	"github.com/codewandler/llmadapter/unified"
)

func ProjectMessages(tree *Tree, branch BranchID) ([]unified.Message, error) {
	if tree == nil {
		return nil, fmt.Errorf("conversation: tree is nil")
	}
	path, err := tree.Path(branch)
	if err != nil {
		return nil, err
	}
	var out []unified.Message
	for _, node := range path {
		switch ev := node.Payload.(type) {
		case MessageEvent:
			out = append(out, ev.Message)
		case *MessageEvent:
			out = append(out, ev.Message)
		case AssistantTurnEvent:
			out = append(out, ev.Message)
		case *AssistantTurnEvent:
			out = append(out, ev.Message)
		}
	}
	return out, nil
}
