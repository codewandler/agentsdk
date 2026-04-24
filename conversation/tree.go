package conversation

import (
	"fmt"
	"sync"
	"time"
)

type Node struct {
	ID        NodeID
	Parent    NodeID
	Payload   Payload
	CreatedAt time.Time
}

type Tree struct {
	mu       sync.RWMutex
	nodes    map[NodeID]Node
	branches map[BranchID]NodeID
}

func NewTree() *Tree {
	return &Tree{
		nodes:    make(map[NodeID]Node),
		branches: map[BranchID]NodeID{MainBranch: ""},
	}
}

func (t *Tree) Append(branch BranchID, payload Payload) (NodeID, error) {
	ids, err := t.AppendMany(branch, payload)
	if err != nil {
		return "", err
	}
	return ids[0], nil
}

func (t *Tree) AppendMany(branch BranchID, payloads ...Payload) ([]NodeID, error) {
	if len(payloads) == 0 {
		return nil, fmt.Errorf("conversation: at least one payload is required")
	}
	for _, payload := range payloads {
		if payload == nil {
			return nil, fmt.Errorf("conversation: payload is required")
		}
	}
	if branch == "" {
		branch = MainBranch
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	parent, ok := t.branches[branch]
	if !ok {
		return nil, fmt.Errorf("conversation: branch %q not found", branch)
	}
	ids := make([]NodeID, 0, len(payloads))
	for _, payload := range payloads {
		id := NewNodeID()
		t.nodes[id] = Node{ID: id, Parent: parent, Payload: payload, CreatedAt: time.Now()}
		parent = id
		ids = append(ids, id)
	}
	t.branches[branch] = parent
	return ids, nil
}

func (t *Tree) InsertNode(branch BranchID, node Node) error {
	if branch == "" {
		branch = MainBranch
	}
	if node.ID == "" {
		return fmt.Errorf("conversation: node id is required")
	}
	if node.Payload == nil {
		return fmt.Errorf("conversation: payload is required")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, exists := t.nodes[node.ID]; exists {
		return fmt.Errorf("conversation: node %q already exists", node.ID)
	}
	if node.Parent != "" {
		if _, ok := t.nodes[node.Parent]; !ok {
			return fmt.Errorf("conversation: parent node %q not found", node.Parent)
		}
	}
	if node.CreatedAt.IsZero() {
		node.CreatedAt = time.Now()
	}
	t.nodes[node.ID] = node
	if _, ok := t.branches[branch]; !ok {
		t.branches[branch] = ""
	}
	t.branches[branch] = node.ID
	return nil
}

func (t *Tree) Node(id NodeID) (Node, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	node, ok := t.nodes[id]
	return node, ok
}

func (t *Tree) Fork(from BranchID, to BranchID) error {
	if from == "" {
		from = MainBranch
	}
	if to == "" {
		to = NewBranchID()
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	head, ok := t.branches[from]
	if !ok {
		return fmt.Errorf("conversation: branch %q not found", from)
	}
	if _, exists := t.branches[to]; exists {
		return fmt.Errorf("conversation: branch %q already exists", to)
	}
	t.branches[to] = head
	return nil
}

func (t *Tree) MoveHead(branch BranchID, node NodeID) error {
	if branch == "" {
		branch = MainBranch
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.branches[branch]; !ok {
		return fmt.Errorf("conversation: branch %q not found", branch)
	}
	if node != "" {
		if _, ok := t.nodes[node]; !ok {
			return fmt.Errorf("conversation: node %q not found", node)
		}
	}
	t.branches[branch] = node
	return nil
}

func (t *Tree) Head(branch BranchID) (NodeID, bool) {
	if branch == "" {
		branch = MainBranch
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	head, ok := t.branches[branch]
	return head, ok
}

func (t *Tree) Path(branch BranchID) ([]Node, error) {
	if branch == "" {
		branch = MainBranch
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	head, ok := t.branches[branch]
	if !ok {
		return nil, fmt.Errorf("conversation: branch %q not found", branch)
	}
	var reversed []Node
	for id := head; id != ""; {
		node, ok := t.nodes[id]
		if !ok {
			return nil, fmt.Errorf("conversation: node %q not found", id)
		}
		reversed = append(reversed, node)
		id = node.Parent
	}
	out := make([]Node, len(reversed))
	for i := range reversed {
		out[len(reversed)-1-i] = reversed[i]
	}
	return out, nil
}

func (t *Tree) Branches() map[BranchID]NodeID {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make(map[BranchID]NodeID, len(t.branches))
	for branch, head := range t.branches {
		out[branch] = head
	}
	return out
}
