package agentcontext

import "sort"

type ProviderRenderRecord struct {
	ProviderKey ProviderKey
	Fingerprint string
	Snapshot    *ProviderSnapshot
	Fragments   map[FragmentKey]RenderedFragmentRecord
}

type RenderedFragmentRecord struct {
	Key         FragmentKey
	Fingerprint string
	Fragment    ContextFragment
	Removed     bool
}

type FragmentRemoved struct {
	ProviderKey         ProviderKey
	FragmentKey         FragmentKey
	PreviousFingerprint string
}

type RenderDiff struct {
	Added     []ContextFragment
	Updated   []ContextFragment
	Removed   []FragmentRemoved
	Unchanged []RenderedFragmentRecord
}

func (r ProviderRenderRecord) ActiveFragments() []ContextFragment {
	if len(r.Fragments) == 0 {
		return nil
	}
	keys := make([]FragmentKey, 0, len(r.Fragments))
	for key, record := range r.Fragments {
		if record.Removed {
			continue
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	out := make([]ContextFragment, 0, len(keys))
	for _, key := range keys {
		out = append(out, r.Fragments[key].Fragment)
	}
	return out
}
