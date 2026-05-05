package resource

import "sync"

// ResourceIndex provides O(1) lookup of resources by name. Resources are
// stored in a map keyed by Name; multiple resources with the same name
// (from different origins) coexist as a slice.
type ResourceIndex struct {
	mu     sync.RWMutex
	byName map[string][]ResourceID
}

// NewResourceIndex creates an empty index.
func NewResourceIndex() *ResourceIndex {
	return &ResourceIndex{byName: make(map[string][]ResourceID)}
}

// Add inserts a resource ID into the index. Duplicate IDs (same Kind,
// Origin, Namespace, Name) are silently ignored.
func (idx *ResourceIndex) Add(id ResourceID) {
	idx.mu.Lock()
	defer idx.mu.Unlock()
	existing := idx.byName[id.Name]
	for _, e := range existing {
		if e.Equal(id) {
			return
		}
	}
	idx.byName[id.Name] = append(existing, id)
}

// Lookup returns all resources with the given name and kind. If kind is
// empty, all resources with that name are returned regardless of kind.
func (idx *ResourceIndex) Lookup(kind, name string) []ResourceID {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	candidates := idx.byName[name]
	if kind == "" {
		out := make([]ResourceID, len(candidates))
		copy(out, candidates)
		return out
	}
	var out []ResourceID
	for _, c := range candidates {
		if c.Kind == kind {
			out = append(out, c)
		}
	}
	return out
}

// LookupRef returns all resources of the given kind that match a user-typed
// reference. The ref is parsed and matched via [ResourceID.MatchesRef].
func (idx *ResourceIndex) LookupRef(kind, ref string) []ResourceID {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	// Extract the name (last ":" segment) for the map lookup.
	name := ref
	if i := lastIndexByte(ref, ':'); i >= 0 {
		name = ref[i+1:]
	}
	candidates := idx.byName[name]
	var out []ResourceID
	for _, c := range candidates {
		if kind != "" && c.Kind != kind {
			continue
		}
		if c.MatchesRef(ref) {
			out = append(out, c)
		}
	}
	return out
}

// All returns every resource ID in the index.
func (idx *ResourceIndex) All() []ResourceID {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []ResourceID
	for _, ids := range idx.byName {
		out = append(out, ids...)
	}
	return out
}

// Len returns the total number of resource IDs in the index.
func (idx *ResourceIndex) Len() int {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	n := 0
	for _, ids := range idx.byName {
		n += len(ids)
	}
	return n
}

func lastIndexByte(s string, c byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == c {
			return i
		}
	}
	return -1
}
