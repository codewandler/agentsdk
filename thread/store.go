package thread

import (
	"context"
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
	Metadata  map[string]string
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
	Events    []Event
}

type Store interface {
	Create(context.Context, CreateParams) (Live, error)
	Resume(context.Context, ResumeParams) (Live, error)
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
