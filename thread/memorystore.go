package thread

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

type MemoryStore struct {
	mu      sync.Mutex
	threads map[ID]*memoryThread
}

type memoryThread struct {
	id        ID
	branchID  BranchID
	branches  map[BranchID]Branch
	metadata  map[string]string
	archived  bool
	createdAt time.Time
	updatedAt time.Time
	nextSeq   int64
	events    []Event
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{threads: make(map[ID]*memoryThread)}
}

func (s *MemoryStore) Create(ctx context.Context, params CreateParams) (Live, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.threads == nil {
		s.threads = make(map[ID]*memoryThread)
	}

	id := params.ID
	if id == "" {
		id = NewID()
	}
	if _, ok := s.threads[id]; ok {
		return nil, fmt.Errorf("thread: thread %q already exists", id)
	}
	branchID := params.BranchID
	if branchID == "" {
		branchID = MainBranch
	}
	now := params.Now
	if now.IsZero() {
		now = time.Now()
	}
	stored := &memoryThread{
		id:        id,
		branchID:  branchID,
		branches:  map[BranchID]Branch{branchID: {ID: branchID, CreatedAt: now}},
		metadata:  cloneMetadata(params.Metadata),
		createdAt: now,
		updatedAt: now,
		nextSeq:   1,
	}
	createdPayload, err := json.Marshal(struct {
		Metadata map[string]string `json:"metadata,omitempty"`
	}{Metadata: params.Metadata})
	if err != nil {
		return nil, err
	}
	stored.events = append(stored.events, Event{
		ID:       NewEventID(),
		ThreadID: id,
		BranchID: branchID,
		Seq:      stored.nextSeq,
		Kind:     EventThreadCreated,
		Payload:  createdPayload,
		At:       now,
		Source:   params.Source,
	})
	stored.nextSeq++
	s.threads[id] = stored
	return &memoryLive{store: s, threadID: id, branchID: branchID, source: params.Source}, nil
}

func (s *MemoryStore) Resume(ctx context.Context, params ResumeParams) (Live, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.threads[params.ID]
	if !ok {
		return nil, fmt.Errorf("thread: thread %q not found", params.ID)
	}
	branchID := params.BranchID
	if branchID == "" {
		branchID = stored.branchID
	}
	if _, ok := stored.branches[branchID]; !ok {
		return nil, fmt.Errorf("thread: branch %q not found", branchID)
	}
	return &memoryLive{store: s, threadID: stored.id, branchID: branchID, source: params.Source}, nil
}

func (s *MemoryStore) Fork(ctx context.Context, params ForkParams) (Live, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.threads[params.ID]
	if !ok {
		return nil, fmt.Errorf("thread: thread %q not found", params.ID)
	}
	from := params.FromBranchID
	if from == "" {
		from = stored.branchID
	}
	if from == "" {
		from = MainBranch
	}
	if _, ok := stored.branches[from]; !ok {
		return nil, fmt.Errorf("thread: branch %q not found", from)
	}
	to := params.ToBranchID
	if to == "" {
		to = NewBranchID()
	}
	if _, ok := stored.branches[to]; ok {
		return nil, fmt.Errorf("thread: branch %q already exists", to)
	}
	now := time.Now()
	forkSeq := stored.nextSeq - 1
	stored.branches[to] = Branch{ID: to, Parent: from, ForkSeq: forkSeq, CreatedAt: now}
	payload, err := json.Marshal(struct {
		FromBranchID BranchID `json:"from_branch_id"`
		ToBranchID   BranchID `json:"to_branch_id"`
		ForkSeq      int64    `json:"fork_seq"`
	}{FromBranchID: from, ToBranchID: to, ForkSeq: forkSeq})
	if err != nil {
		return nil, err
	}
	source := params.Source
	stored.appendLocked(to, Event{Kind: EventBranchCreated, Payload: payload, Source: source, At: now})
	return &memoryLive{store: s, threadID: stored.id, branchID: to, source: source}, nil
}

func (s *MemoryStore) Read(ctx context.Context, params ReadParams) (Stored, error) {
	if err := ctx.Err(); err != nil {
		return Stored{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.threads[params.ID]
	if !ok {
		return Stored{}, fmt.Errorf("thread: thread %q not found", params.ID)
	}
	return stored.snapshot(), nil
}

func (s *MemoryStore) List(ctx context.Context, params ListParams) (Page, error) {
	if err := ctx.Err(); err != nil {
		return Page{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	threads := make([]Stored, 0, len(s.threads))
	for _, stored := range s.threads {
		if stored.archived && !params.IncludeArchived {
			continue
		}
		threads = append(threads, stored.snapshot())
	}
	sort.Slice(threads, func(i, j int) bool {
		return threads[i].UpdatedAt.After(threads[j].UpdatedAt)
	})
	if params.Limit > 0 && len(threads) > params.Limit {
		threads = threads[:params.Limit]
	}
	return Page{Threads: threads}, nil
}

func (s *MemoryStore) Archive(ctx context.Context, id ID) error {
	return s.setArchived(ctx, id, true, EventThreadArchived)
}

func (s *MemoryStore) Unarchive(ctx context.Context, id ID) error {
	return s.setArchived(ctx, id, false, EventThreadUnarchived)
}

func (s *MemoryStore) setArchived(ctx context.Context, id ID, archived bool, kind EventKind) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	stored, ok := s.threads[id]
	if !ok {
		return fmt.Errorf("thread: thread %q not found", id)
	}
	stored.archived = archived
	stored.appendLocked(stored.branchID, Event{Kind: kind})
	return nil
}

func (t *memoryThread) snapshot() Stored {
	return Stored{
		ID:        t.id,
		BranchID:  t.branchID,
		Branches:  cloneBranches(t.branches),
		Metadata:  cloneMetadata(t.metadata),
		Archived:  t.archived,
		CreatedAt: t.createdAt,
		UpdatedAt: t.updatedAt,
		Events:    cloneEvents(t.events),
	}
}

func (t *memoryThread) appendLocked(branchID BranchID, events ...Event) {
	now := time.Now()
	for _, event := range events {
		if event.ID == "" {
			event.ID = NewEventID()
		}
		event.ThreadID = t.id
		if event.BranchID == "" {
			event.BranchID = branchID
		}
		if event.BranchID == "" {
			event.BranchID = t.branchID
		}
		event.Seq = t.nextSeq
		t.nextSeq++
		if event.At.IsZero() {
			event.At = now
		}
		t.events = append(t.events, cloneEvent(event))
		t.updatedAt = event.At
	}
}

type memoryLive struct {
	store    *MemoryStore
	threadID ID
	branchID BranchID
	source   EventSource
	closed   bool
}

func (l *memoryLive) ID() ID             { return l.threadID }
func (l *memoryLive) BranchID() BranchID { return l.branchID }

func (l *memoryLive) Append(ctx context.Context, events ...Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(events) == 0 {
		return nil
	}
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	if l.closed {
		return fmt.Errorf("thread: live thread %q is closed", l.threadID)
	}
	stored, ok := l.store.threads[l.threadID]
	if !ok {
		return fmt.Errorf("thread: thread %q not found", l.threadID)
	}
	batch := make([]Event, len(events))
	for i, event := range events {
		if event.Kind == "" {
			return fmt.Errorf("thread: event kind is required")
		}
		event.ThreadID = l.threadID
		if event.BranchID == "" {
			event.BranchID = l.branchID
		}
		if event.Source.Type == "" && event.Source.ID == "" && event.Source.SessionID == "" {
			event.Source = l.source
		}
		batch[i] = event
	}
	stored.appendLocked(l.branchID, batch...)
	return nil
}

func (l *memoryLive) Flush(context.Context) error { return nil }

func (l *memoryLive) Shutdown(context.Context) error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	l.closed = true
	return nil
}

func (l *memoryLive) Discard(context.Context) error {
	l.store.mu.Lock()
	defer l.store.mu.Unlock()
	delete(l.store.threads, l.threadID)
	l.closed = true
	return nil
}
