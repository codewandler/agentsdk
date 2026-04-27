package agentcontext

import (
	"context"
	"fmt"
	"sort"
)

type Manager struct {
	providers []Provider
	records   map[ProviderKey]ProviderRenderRecord
}

func NewManager(providers ...Provider) (*Manager, error) {
	m := &Manager{
		records: make(map[ProviderKey]ProviderRenderRecord),
	}
	if err := m.Register(providers...); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) Register(providers ...Provider) error {
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

func (m *Manager) Records() map[ProviderKey]ProviderRenderRecord {
	out := make(map[ProviderKey]ProviderRenderRecord, len(m.records))
	for key, record := range m.records {
		out[key] = cloneRecord(record)
	}
	return out
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
	Providers []ProviderBuildResult
	Added     []ContextFragment
	Updated   []ContextFragment
	Removed   []FragmentRemoved
	Active    []ContextFragment
}

func (m *Manager) Build(ctx context.Context, req BuildRequest) (BuildResult, error) {
	if m.records == nil {
		m.records = make(map[ProviderKey]ProviderRenderRecord)
	}
	result := BuildResult{}
	for _, provider := range m.providers {
		providerKey := provider.Key()
		previous, hasPrevious := m.records[providerKey]
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
			clone := cloneRecord(previous)
			contextReq.Previous = &clone
		}

		if fast, ok := provider.(FingerprintingProvider); ok && hasPrevious {
			fingerprint, valid, err := fast.StateFingerprint(ctx, contextReq)
			if err != nil {
				return BuildResult{}, err
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
			return BuildResult{}, err
		}
		record, diff, err := buildProviderRecord(providerKey, previous, providerContext)
		if err != nil {
			return BuildResult{}, err
		}
		m.records[providerKey] = cloneRecord(record)

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
	return result, nil
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
