package agentcontext

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Manager renders registered providers and owns the provider render records
// used to diff changed context between turns.
type Manager struct {
	mu        sync.Mutex
	providers []Provider
	records   map[ProviderKey]ProviderRenderRecord
	version   int64
}

// NewManager creates a context manager with the supplied providers.
func NewManager(providers ...Provider) (*Manager, error) {
	m := &Manager{
		records: make(map[ProviderKey]ProviderRenderRecord),
	}
	if err := m.Register(providers...); err != nil {
		return nil, err
	}
	return m, nil
}

// Register adds providers to the manager. Provider keys must be unique.
func (m *Manager) Register(providers ...Provider) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.registerLocked(providers...)
}

func (m *Manager) registerLocked(providers ...Provider) error {
	if m.records == nil {
		m.records = make(map[ProviderKey]ProviderRenderRecord)
	}
	seen := make(map[ProviderKey]struct{}, len(m.providers)+len(providers))
	for _, provider := range m.providers {
		seen[provider.Key()] = struct{}{}
	}
	for _, provider := range providers {
		if provider == nil {
			return fmt.Errorf("agentcontext: provider is nil")
		}
		key := provider.Key()
		if key == "" {
			return fmt.Errorf("agentcontext: provider key is required")
		}
		if _, ok := seen[key]; ok {
			return fmt.Errorf("agentcontext: duplicate provider key %q", key)
		}
		seen[key] = struct{}{}
		m.providers = append(m.providers, provider)
	}
	sort.SliceStable(m.providers, func(i, j int) bool {
		return m.providers[i].Key() < m.providers[j].Key()
	})
	return nil
}

// Records returns a defensive copy of the manager's last render records.
func (m *Manager) Records() map[ProviderKey]ProviderRenderRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(map[ProviderKey]ProviderRenderRecord, len(m.records))
	for key, record := range m.records {
		out[key] = cloneRecord(record)
	}
	return out
}

// SetRecords replaces the manager's last render records, typically after
// replaying a persisted context render on resume.
func (m *Manager) SetRecords(records map[ProviderKey]ProviderRenderRecord) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = make(map[ProviderKey]ProviderRenderRecord, len(records))
	for key, record := range records {
		m.records[key] = cloneRecord(record)
	}
	m.version++
}

// Descriptors returns side-effect-free metadata for all registered providers.
func (m *Manager) Descriptors() []ProviderDescriptor {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]ProviderDescriptor, 0, len(m.providers))
	for _, provider := range m.providers {
		out = append(out, descriptorForProvider(provider))
	}
	return out
}

// StateSnapshot returns a machine-readable copy of the last committed provider
// render state, enriched with provider descriptors when available.
func (m *Manager) StateSnapshot() StateSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	records := make(map[ProviderKey]ProviderRenderRecord, len(m.records))
	for key, record := range m.records {
		records[key] = cloneRecord(record)
	}
	descriptors := make(map[ProviderKey]ProviderDescriptor, len(m.providers))
	for _, provider := range m.providers {
		desc := descriptorForProvider(provider)
		if desc.Key != "" {
			descriptors[desc.Key] = desc
		}
	}
	return snapshotFromRecords(records, descriptors)
}

type BuildRequest struct {
	ThreadID     string
	BranchID     string
	TurnID       string
	HarnessState any
	Preference   Preference
	TokenBudget  int
	Reason       RenderReason
}

type ProviderBuildResult struct {
	ProviderKey ProviderKey
	Diff        RenderDiff
	Record      ProviderRenderRecord
	Skipped     bool
}

type BuildResult struct {
	TurnID    string
	Reason    RenderReason
	Providers []ProviderBuildResult
	Added     []ContextFragment
	Updated   []ContextFragment
	Removed   []FragmentRemoved
	Active    []ContextFragment
}

func (m *Manager) Build(ctx context.Context, req BuildRequest) (BuildResult, error) {
	render, err := m.Prepare(ctx, req)
	if err != nil {
		return BuildResult{}, err
	}
	if err := render.Commit(); err != nil {
		return BuildResult{}, err
	}
	return render.Result, nil
}

type PreparedRender struct {
	manager *Manager
	version int64
	records map[ProviderKey]ProviderRenderRecord
	Result  BuildResult
	done    bool
}

func (m *Manager) Prepare(ctx context.Context, req BuildRequest) (*PreparedRender, error) {
	m.mu.Lock()
	if m.records == nil {
		m.records = make(map[ProviderKey]ProviderRenderRecord)
	}
	providers := append([]Provider(nil), m.providers...)
	previousRecords := make(map[ProviderKey]ProviderRenderRecord, len(m.records))
	for key, record := range m.records {
		previousRecords[key] = cloneRecord(record)
	}
	version := m.version
	m.mu.Unlock()

	result := BuildResult{TurnID: req.TurnID, Reason: req.Reason}
	nextRecords := make(map[ProviderKey]ProviderRenderRecord, len(previousRecords))
	for key, record := range previousRecords {
		nextRecords[key] = cloneRecord(record)
	}
	for _, provider := range providers {
		providerKey := provider.Key()
		previous, hasPrevious := previousRecords[providerKey]
		contextReq := Request{
			ThreadID:     req.ThreadID,
			BranchID:     req.BranchID,
			TurnID:       req.TurnID,
			HarnessState: req.HarnessState,
			Preference:   req.Preference,
			TokenBudget:  req.TokenBudget,
			Reason:       req.Reason,
		}
		if hasPrevious {
			contextReq.Previous = &previous
		}

		if fast, ok := provider.(FingerprintingProvider); ok && hasPrevious {
			fingerprint, valid, err := fast.StateFingerprint(ctx, contextReq)
			if err != nil {
				return nil, err
			}
			if valid && fingerprint != "" && fingerprint == previous.Fingerprint {
				record := cloneRecord(previous)
				result.Providers = append(result.Providers, ProviderBuildResult{
					ProviderKey: providerKey,
					Record:      record,
					Skipped:     true,
				})
				result.Active = append(result.Active, record.ActiveFragments()...)
				continue
			}
		}

		providerContext, err := provider.GetContext(ctx, contextReq)
		if err != nil {
			return nil, err
		}
		record, diff, err := buildProviderRecord(providerKey, previous, providerContext)
		if err != nil {
			return nil, err
		}
		nextRecords[providerKey] = cloneRecord(record)

		result.Providers = append(result.Providers, ProviderBuildResult{
			ProviderKey: providerKey,
			Diff:        diff,
			Record:      record,
		})
		result.Added = append(result.Added, diff.Added...)
		result.Updated = append(result.Updated, diff.Updated...)
		result.Removed = append(result.Removed, diff.Removed...)
		result.Active = append(result.Active, record.ActiveFragments()...)
	}
	return &PreparedRender{manager: m, version: version, records: nextRecords, Result: result}, nil
}

func (r *PreparedRender) Commit() error {
	if r == nil || r.manager == nil {
		return fmt.Errorf("agentcontext: prepared render is nil")
	}
	r.manager.mu.Lock()
	defer r.manager.mu.Unlock()
	if r.done {
		return fmt.Errorf("agentcontext: prepared render already closed")
	}
	if r.manager.version != r.version {
		return fmt.Errorf("agentcontext: render records changed before commit")
	}
	r.manager.records = make(map[ProviderKey]ProviderRenderRecord, len(r.records))
	for key, record := range r.records {
		r.manager.records[key] = cloneRecord(record)
	}
	r.manager.version++
	r.done = true
	return nil
}

func (r *PreparedRender) Records() map[ProviderKey]ProviderRenderRecord {
	if r == nil {
		return nil
	}
	out := make(map[ProviderKey]ProviderRenderRecord, len(r.records))
	for key, record := range r.records {
		out[key] = cloneRecord(record)
	}
	return out
}

func (r *PreparedRender) Rollback() {
	if r == nil {
		return
	}
	r.done = true
}

func buildProviderRecord(providerKey ProviderKey, previous ProviderRenderRecord, providerContext ProviderContext) (ProviderRenderRecord, RenderDiff, error) {
	fragments, err := normalizeFragments(providerContext.Fragments)
	if err != nil {
		return ProviderRenderRecord{}, RenderDiff{}, err
	}

	next := ProviderRenderRecord{
		ProviderKey: providerKey,
		Snapshot:    providerContext.Snapshot,
		Fragments:   make(map[FragmentKey]RenderedFragmentRecord, len(fragments)),
	}
	diff := RenderDiff{}

	for _, fragment := range fragments {
		fingerprint := FragmentFingerprint(fragment)
		fragment.Fingerprint = fingerprint
		record := RenderedFragmentRecord{
			Key:         fragment.Key,
			Fingerprint: fingerprint,
			Fragment:    fragment,
		}
		next.Fragments[fragment.Key] = record

		prev, ok := previous.Fragments[fragment.Key]
		switch {
		case !ok || prev.Removed:
			diff.Added = append(diff.Added, fragment)
		case prev.Fingerprint != fingerprint:
			diff.Updated = append(diff.Updated, fragment)
		default:
			diff.Unchanged = append(diff.Unchanged, record)
		}
	}

	for key, prev := range previous.Fragments {
		if prev.Removed {
			continue
		}
		if _, ok := next.Fragments[key]; ok {
			continue
		}
		diff.Removed = append(diff.Removed, FragmentRemoved{
			ProviderKey:         providerKey,
			FragmentKey:         key,
			PreviousFingerprint: prev.Fingerprint,
		})
		next.Fragments[key] = RenderedFragmentRecord{
			Key:         key,
			Fingerprint: prev.Fingerprint,
			Removed:     true,
		}
	}

	if providerContext.Fingerprint != "" {
		next.Fingerprint = providerContext.Fingerprint
	} else {
		next.Fingerprint = ProviderFingerprint(fragments)
	}
	return next, diff, nil
}

func normalizeFragments(fragments []ContextFragment) ([]ContextFragment, error) {
	out := append([]ContextFragment(nil), fragments...)
	seen := make(map[FragmentKey]struct{}, len(out))
	for _, fragment := range out {
		if fragment.Key == "" {
			return nil, fmt.Errorf("agentcontext: fragment key is required")
		}
		if _, ok := seen[fragment.Key]; ok {
			return nil, fmt.Errorf("agentcontext: duplicate fragment key %q", fragment.Key)
		}
		seen[fragment.Key] = struct{}{}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func cloneRecord(record ProviderRenderRecord) ProviderRenderRecord {
	out := record
	if record.Fragments != nil {
		out.Fragments = make(map[FragmentKey]RenderedFragmentRecord, len(record.Fragments))
		for key, fragment := range record.Fragments {
			out.Fragments[key] = fragment
		}
	}
	return out
}
