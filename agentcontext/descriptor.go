package agentcontext

import "sort"

// ProviderDescriptor is side-effect-free metadata about a context provider. It is
// intended for discovery/debug surfaces; rendering context remains owned by the
// Provider interface and Manager.
type ProviderDescriptor struct {
	Key         ProviderKey `json:"key"`
	Description string      `json:"description,omitempty"`
	Lifecycle   string      `json:"lifecycle,omitempty"`
	Scope       CacheScope  `json:"scope,omitempty"`
	CachePolicy CachePolicy `json:"cache_policy,omitempty"`
}

// DescribedProvider can publish metadata without rendering context.
type DescribedProvider interface {
	Provider
	Descriptor() ProviderDescriptor
}

// FragmentState is a machine-readable view of the last committed fragment
// render state for non-terminal channels and debugging tools.
type FragmentState struct {
	Key         FragmentKey       `json:"key"`
	Role        string            `json:"role,omitempty"`
	Authority   FragmentAuthority `json:"authority,omitempty"`
	Fingerprint string            `json:"fingerprint,omitempty"`
	CachePolicy CachePolicy       `json:"cache_policy,omitempty"`
	Removed     bool              `json:"removed,omitempty"`
	Content     string            `json:"content,omitempty"`
}

// ProviderState is one provider's last committed render state.
type ProviderState struct {
	Descriptor          ProviderDescriptor `json:"descriptor"`
	Fingerprint         string             `json:"fingerprint,omitempty"`
	SnapshotFingerprint string             `json:"snapshot_fingerprint,omitempty"`
	Fragments           []FragmentState    `json:"fragments,omitempty"`
}

// StateSnapshot is a stable, JSON-friendly context manager inspection shape.
type StateSnapshot struct {
	Providers []ProviderState `json:"providers"`
}

func descriptorForProvider(provider Provider) ProviderDescriptor {
	if provider == nil {
		return ProviderDescriptor{}
	}
	if described, ok := provider.(DescribedProvider); ok {
		desc := described.Descriptor()
		if desc.Key == "" {
			desc.Key = provider.Key()
		}
		return desc
	}
	return ProviderDescriptor{Key: provider.Key()}
}

func snapshotFromRecords(records map[ProviderKey]ProviderRenderRecord, descriptors map[ProviderKey]ProviderDescriptor) StateSnapshot {
	keys := make([]ProviderKey, 0, len(records))
	for key := range records {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	out := StateSnapshot{Providers: make([]ProviderState, 0, len(keys))}
	for _, key := range keys {
		record := records[key]
		desc := descriptors[key]
		if desc.Key == "" {
			desc.Key = key
		}
		state := ProviderState{Descriptor: desc, Fingerprint: record.Fingerprint}
		if record.Snapshot != nil {
			state.SnapshotFingerprint = record.Snapshot.Fingerprint
		}
		fragmentKeys := make([]FragmentKey, 0, len(record.Fragments))
		for fragmentKey := range record.Fragments {
			fragmentKeys = append(fragmentKeys, fragmentKey)
		}
		sort.Slice(fragmentKeys, func(i, j int) bool { return fragmentKeys[i] < fragmentKeys[j] })
		for _, fragmentKey := range fragmentKeys {
			fragment := record.Fragments[fragmentKey]
			state.Fragments = append(state.Fragments, FragmentState{
				Key:         fragment.Key,
				Role:        string(fragment.Fragment.Role),
				Authority:   fragment.Fragment.Authority,
				Fingerprint: fragment.Fingerprint,
				CachePolicy: fragment.Fragment.CachePolicy,
				Removed:     fragment.Removed,
				Content:     fragment.Fragment.Content,
			})
		}
		out.Providers = append(out.Providers, state)
	}
	return out
}
