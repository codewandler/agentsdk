package thread

import (
	"crypto/rand"
	"encoding/hex"
)

type ID string
type BranchID string
type NodeID string
type EventID string

const MainBranch BranchID = "main"

func newID(prefix string) string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return prefix + hex.EncodeToString(b[:])
}

func NewID() ID             { return ID(newID("thread_")) }
func NewBranchID() BranchID { return BranchID(newID("branch_")) }
func NewNodeID() NodeID     { return NodeID(newID("node_")) }
func NewEventID() EventID   { return EventID(newID("event_")) }
