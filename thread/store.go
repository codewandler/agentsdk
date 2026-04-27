package thread

import (
	"context"
	"fmt"
	"time"
)

type CreateParams struct {
	ID       ID
	BranchID BranchID
	Metadata map[string]string
	Source   EventSource
	Now      time.Time
}

type ResumeParams struct {
	ID       ID
	BranchID BranchID
	Source   EventSource
}

type ForkParams struct {
	ID           ID
	FromBranchID BranchID
	ToBranchID   BranchID
	Source       EventSource
}

type ReadParams struct {
	ID ID
}

type ListParams struct {
	IncludeArchived bool
	Limit           int
}

type Page struct {
	Threads []Stored
}

type Stored struct {
	ID        ID
	BranchID  BranchID
	Branches  map[BranchID]Branch
	Metadata  map[string]string
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
	Events    []Event
}

type Branch struct {
	ID        BranchID
	Parent    BranchID
	ForkSeq   int64
	CreatedAt time.Time
}

type Store interface {
	Create(context.Context, CreateParams) (Live, error)
	Resume(context.Context, ResumeParams) (Live, error)
	Fork(context.Context, ForkParams) (Live, error)
	Read(context.Context, ReadParams) (Stored, error)
	List(context.Context, ListParams) (Page, error)
	Archive(context.Context, ID) error
	Unarchive(context.Context, ID) error
}

type Live interface {
	ID() ID
	BranchID() BranchID
	Append(context.Context, ...Event) error
	Flush(context.Context) error
	Shutdown(context.Context) error
	Discard(context.Context) error
}

func cloneMetadata(metadata map[string]string) map[string]string {
	if metadata == nil {
		return nil
	}
	out := make(map[string]string, len(metadata))
	for key, value := range metadata {
		out[key] = value
	}
	return out
}

func (s Stored) EventsForBranch(branchID BranchID) ([]Event, error) {
	if branchID == "" {
		branchID = s.BranchID
	}
	if branchID == "" {
		branchID = MainBranch
	}
	windows, err := s.branchWindows(branchID)
	if err != nil {
		return nil, err
	}
	var out []Event
	for _, event := range s.Events {
		window, ok := windows[event.BranchID]
		if !ok {
			continue
		}
		if event.Seq <= window.after {
			continue
		}
		if window.until > 0 && event.Seq > window.until {
			continue
		}
		out = append(out, cloneEvent(event))
	}
	return out, nil
}

type branchWindow struct {
	after int64
	until int64
}

func (s Stored) branchWindows(branchID BranchID) (map[BranchID]branchWindow, error) {
	if len(s.Branches) == 0 {
		if branchID == MainBranch || branchID == "" {
			return map[BranchID]branchWindow{MainBranch: {}}, nil
		}
		return nil, fmt.Errorf("thread: branch %q not found", branchID)
	}
	var reversed []Branch
	for current := branchID; current != ""; {
		branch, ok := s.Branches[current]
		if !ok {
			return nil, fmt.Errorf("thread: branch %q not found", current)
		}
		reversed = append(reversed, branch)
		current = branch.Parent
	}
	path := make([]Branch, len(reversed))
	for i := range reversed {
		path[len(reversed)-1-i] = reversed[i]
	}
	windows := make(map[BranchID]branchWindow, len(path))
	for i, branch := range path {
		window := branchWindow{after: branch.ForkSeq}
		if i+1 < len(path) {
			window.until = path[i+1].ForkSeq
		}
		windows[branch.ID] = window
	}
	return windows, nil
}

func cloneBranches(branches map[BranchID]Branch) map[BranchID]Branch {
	if branches == nil {
		return nil
	}
	out := make(map[BranchID]Branch, len(branches))
	for id, branch := range branches {
		out[id] = branch
	}
	return out
}
