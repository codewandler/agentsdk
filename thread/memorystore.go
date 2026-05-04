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
	mu       sync.Mutex
	threads  map[ID]*memoryThread
	registry EventRegistry
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

type MemoryStoreOption func(*MemoryStore)

func WithEventRegistry(registry EventRegistry) MemoryStoreOption {
	return func(s *MemoryStore) {
		s.registry = registry
	}
}

func NewMemoryStore(opts ...MemoryStoreOption) *MemoryStore {
	store := &MemoryStore{threads: make(map[ID]*memoryThread)}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	return store
}

func (s *MemoryStore) Import(ctx context.Context, events ...Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.threads == nil {
		s.threads = make(map[ID]*memoryThread)
	}
	for _, event := range events {
		if event.ThreadID == "" {
			return fmt.Errorf("thread: imported event has no thread id")
		}
		if event.Kind == "" {
			return fmt.Errorf("thread: imported event kind is required")
		}
		if event.SchemaVersion == 0 {
			event.SchemaVersion = CurrentEventSchemaVersion
		}
		if err := s.validateEvent(event); err != nil {
			return err
		}
		branchID := event.BranchID
		if branchID == "" {
			branchID = MainBranch
			event.BranchID = branchID
		}
		stored, ok := s.threads[event.ThreadID]
		if !ok {
			stored = &memoryThread{
				id:        event.ThreadID,
				branchID:  branchID,
				branches:  map[BranchID]Branch{branchID: {ID: branchID, CreatedAt: event.At}},
				metadata:  map[string]string{},
				createdAt: event.At,
				updatedAt: event.At,
				nextSeq:   1,
			}
			s.threads[event.ThreadID] = stored
		}
		if stored.branches == nil {
			stored.branches = map[BranchID]Branch{}
		}
		if _, ok := stored.branches[branchID]; !ok {
			stored.branches[branchID] = Branch{ID: branchID, CreatedAt: event.At}
		}
		if event.Kind == EventThreadCreated {
			if event.At.Before(stored.createdAt) || stored.createdAt.IsZero() {
				stored.createdAt = event.At
			}
			stored.branchID = branchID
			var payload struct {
				Metadata map[string]string `json:"metadata,omitempty"`
			}
			if len(event.Payload) > 0 && json.Unmarshal(event.Payload, &payload) == nil && payload.Metadata != nil {
				stored.metadata = cloneMetadata(payload.Metadata)
			}
		}
		if event.Kind == EventBranchCreated {
			var payload struct {
				FromBranchID BranchID `json:"from_branch_id"`
				ToBranchID   BranchID `json:"to_branch_id"`
				ForkSeq      int64    `json:"fork_seq"`
			}
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			if payload.ToBranchID == "" {
				payload.ToBranchID = branchID
			}
			stored.branches[payload.ToBranchID] = Branch{
				ID:        payload.ToBranchID,
				Parent:    payload.FromBranchID,
				ForkSeq:   payload.ForkSeq,
				CreatedAt: event.At,
			}
		}
		if event.Kind == EventThreadArchived {
			stored.archived = true
		}
		if event.Kind == EventThreadUnarchived {
			stored.archived = false
		}
		stored.events = append(stored.events, cloneEvent(event))
		if event.Seq >= stored.nextSeq {
			stored.nextSeq = event.Seq + 1
		}
		if event.At.After(stored.updatedAt) || stored.updatedAt.IsZero() {
			stored.updatedAt = event.At
		}
	}
	return nil
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
		return nil, fmt.Errorf("%w: thread %q", ErrAlreadyExists, id)
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
	created := Event{
		ID:            NewEventID(),
		ThreadID:      id,
		BranchID:      branchID,
		Seq:           stored.nextSeq,
		SchemaVersion: CurrentEventSchemaVersion,
		Kind:          EventThreadCreated,
		Payload:       createdPayload,
		At:            now,
		Source:        params.Source,
	}
	if err := s.validateEvent(created); err != nil {
		return nil, err
	}
	stored.events = append(stored.events, created)
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
		return nil, fmt.Errorf("%w: thread %q", ErrNotFound, params.ID)
	}
	branchID := params.BranchID
	if branchID == "" {
		branchID = stored.branchID
	}
	if _, ok := stored.branches[branchID]; !ok {
		return nil, fmt.Errorf("%w: branch %q", ErrNotFound, branchID)
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
		return nil, fmt.Errorf("%w: thread %q", ErrNotFound, params.ID)
	}
	from := params.FromBranchID
	if from == "" {
		from = stored.branchID
	}
	if from == "" {
		from = MainBranch
	}
	if _, ok := stored.branches[from]; !ok {
		return nil, fmt.Errorf("%w: branch %q", ErrNotFound, from)
	}
	to := params.ToBranchID
	if to == "" {
		to = NewBranchID()
	}
	if _, ok := stored.branches[to]; ok {
		return nil, fmt.Errorf("%w: branch %q", ErrAlreadyExists, to)
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
	event := Event{Kind: EventBranchCreated, Payload: payload, Source: source, At: now}
	if err := s.validateEvent(event); err != nil {
		return nil, err
	}
	stored.appendLocked(to, event)
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
		return Stored{}, fmt.Errorf("%w: thread %q", ErrNotFound, params.ID)
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
		return fmt.Errorf("%w: thread %q", ErrNotFound, id)
	}
	stored.archived = archived
	event := Event{Kind: kind}
	if err := s.validateEvent(event); err != nil {
		return err
	}
	stored.appendLocked(stored.branchID, event)
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
		if event.SchemaVersion == 0 {
			event.SchemaVersion = CurrentEventSchemaVersion
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
		return fmt.Errorf("%w: thread %q", ErrNotFound, l.threadID)
	}
	batch := make([]Event, len(events))
	for i, event := range events {
		if event.Kind == "" {
			return fmt.Errorf("thread: event kind is required")
		}
		if err := l.store.validateEvent(event); err != nil {
			return err
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

func (s *MemoryStore) validateEvent(event Event) error {
	if s == nil || s.registry == nil {
		return nil
	}
	return s.registry.Validate(event)
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

func (s *MemoryStore) DiscardThread(ctx context.Context, id ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.threads, id)
	return nil
}
