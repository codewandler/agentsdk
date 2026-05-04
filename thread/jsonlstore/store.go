package jsonlstore

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/codewandler/agentsdk/thread"
)

type Store struct {
	dir      string
	mu       sync.Mutex
	registry thread.EventRegistry
}

type record struct {
	Version       int                `json:"version"`
	ID            thread.EventID     `json:"id"`
	ThreadID      thread.ID          `json:"thread_id"`
	BranchID      thread.BranchID    `json:"branch_id"`
	NodeID        thread.NodeID      `json:"node_id,omitempty"`
	ParentNodeID  thread.NodeID      `json:"parent_node_id,omitempty"`
	Seq           int64              `json:"seq"`
	Kind          thread.EventKind   `json:"kind"`
	SchemaVersion int                `json:"schema_version,omitempty"`
	Payload       json.RawMessage    `json:"payload,omitempty"`
	At            time.Time          `json:"at"`
	Source        thread.EventSource `json:"source,omitempty"`
	CausationID   thread.EventID     `json:"causation_id,omitempty"`
	CorrelationID string             `json:"correlation_id,omitempty"`
}

type Option func(*Store)

func WithEventRegistry(registry thread.EventRegistry) Option {
	return func(s *Store) {
		s.registry = registry
	}
}

func Open(dir string, opts ...Option) *Store {
	store := &Store{dir: dir}
	for _, opt := range opts {
		if opt != nil {
			opt(store)
		}
	}
	return store
}

func (s *Store) Create(ctx context.Context, params thread.CreateParams) (thread.Live, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	memory, err := s.loadMemory(ctx)
	if err != nil {
		return nil, err
	}
	live, err := memory.Create(ctx, params)
	if err != nil {
		return nil, err
	}
	stored, err := memory.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		return nil, err
	}
	if err := s.rewrite(ctx, memory); err != nil {
		return nil, err
	}
	return &liveThread{store: s, memory: memory, threadID: live.ID(), branchID: live.BranchID(), source: params.Source, nextSeq: nextSeq(stored.Events)}, nil
}

func (s *Store) Resume(ctx context.Context, params thread.ResumeParams) (thread.Live, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	memory, err := s.loadMemory(ctx)
	if err != nil {
		return nil, err
	}
	live, err := memory.Resume(ctx, params)
	if err != nil {
		return nil, err
	}
	stored, err := memory.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		return nil, err
	}
	return &liveThread{store: s, memory: memory, threadID: live.ID(), branchID: live.BranchID(), source: params.Source, nextSeq: nextSeq(stored.Events)}, nil
}

func (s *Store) Fork(ctx context.Context, params thread.ForkParams) (thread.Live, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	memory, err := s.loadMemory(ctx)
	if err != nil {
		return nil, err
	}
	live, err := memory.Fork(ctx, params)
	if err != nil {
		return nil, err
	}
	if err := s.rewrite(ctx, memory); err != nil {
		return nil, err
	}
	stored, err := memory.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		return nil, err
	}
	return &liveThread{store: s, memory: memory, threadID: live.ID(), branchID: live.BranchID(), source: params.Source, nextSeq: nextSeq(stored.Events)}, nil
}

func (s *Store) Read(ctx context.Context, params thread.ReadParams) (thread.Stored, error) {
	memory, err := s.loadMemory(ctx)
	if err != nil {
		return thread.Stored{}, err
	}
	return memory.Read(ctx, params)
}

func (s *Store) List(ctx context.Context, params thread.ListParams) (thread.Page, error) {
	memory, err := s.loadMemory(ctx)
	if err != nil {
		return thread.Page{}, err
	}
	return memory.List(ctx, params)
}

func (s *Store) Archive(ctx context.Context, id thread.ID) error {
	memory, err := s.loadMemory(ctx)
	if err != nil {
		return err
	}
	if err := memory.Archive(ctx, id); err != nil {
		return err
	}
	return s.rewrite(ctx, memory)
}

func (s *Store) Unarchive(ctx context.Context, id thread.ID) error {
	memory, err := s.loadMemory(ctx)
	if err != nil {
		return err
	}
	if err := memory.Unarchive(ctx, id); err != nil {
		return err
	}
	return s.rewrite(ctx, memory)
}

func (s *Store) ensure() error {
	if s == nil || s.dir == "" {
		return fmt.Errorf("thread/jsonlstore: dir is required")
	}
	return nil
}

func (s *Store) path(id thread.ID) string {
	return filepath.Join(s.dir, string(id)+".jsonl")
}

func (s *Store) loadMemory(ctx context.Context) (*thread.MemoryStore, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	memory := thread.NewMemoryStore(thread.WithEventRegistry(s.registry))
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return memory, nil
		}
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
			continue
		}
		if err := loadThreadFile(ctx, memory, filepath.Join(s.dir, entry.Name())); err != nil {
			return nil, err
		}
	}
	return memory, nil
}

func loadThreadFile(ctx context.Context, memory *thread.MemoryStore, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var events []thread.Event
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			return err
		}
		events = append(events, decode(rec))
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return memory.Import(ctx, events...)
}

func (s *Store) rewrite(ctx context.Context, memory *thread.MemoryStore) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ensure(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	page, err := memory.List(ctx, thread.ListParams{IncludeArchived: true})
	if err != nil {
		return err
	}
	for _, stored := range page.Threads {
		if err := writeThreadFile(s.path(stored.ID), stored.Events); err != nil {
			return err
		}
	}
	return nil
}

func writeThreadFile(path string, events []thread.Event) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, event := range events {
		if err := enc.Encode(encode(event)); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

type liveThread struct {
	store    *Store
	memory   *thread.MemoryStore
	threadID thread.ID
	branchID thread.BranchID
	source   thread.EventSource
	nextSeq  int64
	closed   bool
}

func (l *liveThread) ID() thread.ID             { return l.threadID }
func (l *liveThread) BranchID() thread.BranchID { return l.branchID }

func (l *liveThread) Append(ctx context.Context, events ...thread.Event) error {
	if l.closed {
		return fmt.Errorf("thread/jsonlstore: live thread %q is closed", l.threadID)
	}
	if len(events) == 0 {
		return nil
	}
	live, err := l.memory.Resume(ctx, thread.ResumeParams{ID: l.threadID, BranchID: l.branchID, Source: l.source})
	if err != nil {
		return err
	}
	if err := live.Append(ctx, events...); err != nil {
		return err
	}
	stored, err := l.memory.Read(ctx, thread.ReadParams{ID: l.threadID})
	if err != nil {
		return err
	}
	var appended []thread.Event
	for _, event := range stored.Events {
		if event.Seq >= l.nextSeq {
			appended = append(appended, event)
		}
	}
	if len(appended) == 0 {
		return nil
	}
	if err := appendThreadFile(l.store.path(l.threadID), appended); err != nil {
		return err
	}
	l.nextSeq = nextSeq(stored.Events)
	return nil
}

func (l *liveThread) Flush(context.Context) error { return nil }

func (l *liveThread) Shutdown(context.Context) error {
	l.closed = true
	return nil
}

func (l *liveThread) Discard(ctx context.Context) error {
	l.closed = true
	if err := os.Remove(l.store.path(l.threadID)); err != nil && !os.IsNotExist(err) {
		return err
	}
	return l.memory.DiscardThread(ctx, l.threadID)
}

func appendThreadFile(path string, events []thread.Event) error {
	if len(events) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, event := range events {
		if err := enc.Encode(encode(event)); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func encode(event thread.Event) record {
	return record{
		Version:       1,
		ID:            event.ID,
		ThreadID:      event.ThreadID,
		BranchID:      event.BranchID,
		NodeID:        event.NodeID,
		ParentNodeID:  event.ParentNodeID,
		Seq:           event.Seq,
		Kind:          event.Kind,
		SchemaVersion: normalizedEventSchemaVersion(event.SchemaVersion),
		Payload:       append(json.RawMessage(nil), event.Payload...),
		At:            event.At,
		Source:        event.Source,
		CausationID:   event.CausationID,
		CorrelationID: event.CorrelationID,
	}
}

func decode(rec record) thread.Event {
	return thread.Event{
		ID:            rec.ID,
		ThreadID:      rec.ThreadID,
		BranchID:      rec.BranchID,
		NodeID:        rec.NodeID,
		ParentNodeID:  rec.ParentNodeID,
		Seq:           rec.Seq,
		Kind:          rec.Kind,
		SchemaVersion: normalizedEventSchemaVersion(rec.SchemaVersion),
		Payload:       append(json.RawMessage(nil), rec.Payload...),
		At:            rec.At,
		Source:        rec.Source,
		CausationID:   rec.CausationID,
		CorrelationID: rec.CorrelationID,
	}
}

func nextSeq(events []thread.Event) int64 {
	var maxSeq int64
	for _, event := range events {
		if event.Seq > maxSeq {
			maxSeq = event.Seq
		}
	}
	return maxSeq + 1
}

func normalizedEventSchemaVersion(version int) int {
	if version <= 0 {
		return thread.CurrentEventSchemaVersion
	}
	return version
}
