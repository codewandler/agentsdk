package resource

import (
	"fmt"
	"strings"
	"sync"
)

// ResolverPolicy decides which resource to pick when multiple candidates
// match an unqualified or partially-qualified reference.
type ResolverPolicy interface {
	Resolve(kind string, ref string, candidates []ResourceID) (ResourceID, error)
}

// PrecedencePolicy picks the candidate whose origin appears earliest in the
// precedence list. If multiple candidates share the same origin, or if no
// candidate's origin is in the list, it falls back to ErrorPolicy.
type PrecedencePolicy struct {
	// Order lists origins from highest to lowest priority.
	// Default: ["explicit", "local", "user", "embedded"]
	Order []string
}

// DefaultPrecedenceOrder is the default origin precedence.
var DefaultPrecedenceOrder = []string{"explicit", "local", "user", "embedded"}

func (p PrecedencePolicy) Resolve(kind string, ref string, candidates []ResourceID) (ResourceID, error) {
	order := p.Order
	if len(order) == 0 {
		order = DefaultPrecedenceOrder
	}
	// Build a rank map: origin → position (lower = higher priority).
	rank := make(map[string]int, len(order))
	for i, o := range order {
		rank[o] = i
	}
	best := -1
	bestRank := len(order) + 1
	tied := false
	for i, c := range candidates {
		r, ok := rank[c.Origin]
		if !ok {
			r = len(order) // unranked origins sort last
		}
		if r < bestRank {
			best = i
			bestRank = r
			tied = false
		} else if r == bestRank {
			tied = true
		}
	}
	if best < 0 || tied {
		return ResourceID{}, ambiguityError(kind, ref, candidates)
	}
	return candidates[best], nil
}

// ErrorPolicy always returns an error listing all candidates.
type ErrorPolicy struct{}

func (ErrorPolicy) Resolve(kind string, ref string, candidates []ResourceID) (ResourceID, error) {
	return ResourceID{}, ambiguityError(kind, ref, candidates)
}

func ambiguityError(kind, ref string, candidates []ResourceID) error {
	var b strings.Builder
	fmt.Fprintf(&b, "ambiguous %s %q matches %d resources:", kind, ref, len(candidates))
	for _, c := range candidates {
		fmt.Fprintf(&b, "\n  - %s", c.Address())
	}
	return fmt.Errorf("%s", b.String())
}

// Resolver resolves user-typed references to canonical ResourceIDs using an
// index, aliases, a resolved cache, and a pluggable policy for ambiguity.
type Resolver struct {
	mu       sync.RWMutex
	index    *ResourceIndex
	policy   ResolverPolicy
	aliases  map[string]string // user-configured rewrites
	resolved map[string]string // cached policy decisions (ref → address)
}

// ResolverConfig configures a new Resolver.
type ResolverConfig struct {
	Index   *ResourceIndex
	Policy  ResolverPolicy
	Aliases map[string]string
}

// NewResolver creates a Resolver. If Policy is nil, PrecedencePolicy with
// default order is used. If Index is nil, an empty index is created.
func NewResolver(cfg ResolverConfig) *Resolver {
	idx := cfg.Index
	if idx == nil {
		idx = NewResourceIndex()
	}
	policy := cfg.Policy
	if policy == nil {
		policy = PrecedencePolicy{}
	}
	aliases := make(map[string]string, len(cfg.Aliases))
	for k, v := range cfg.Aliases {
		aliases[k] = v
	}
	return &Resolver{
		index:    idx,
		policy:   policy,
		aliases:  aliases,
		resolved: make(map[string]string),
	}
}

// Resolve looks up a resource by kind and user-typed ref.
//
// Resolution order:
//  1. Check aliases → rewrite ref
//  2. Check resolved cache → return cached result
//  3. Lookup in index by ref
//  4. Single match → cache and return
//  5. Multiple matches → apply policy
func (r *Resolver) Resolve(kind, ref string) (ResourceID, error) {
	r.mu.RLock()
	// 1. Alias rewrite.
	if alias, ok := r.aliases[ref]; ok {
		ref = alias
	}
	// 2. Resolved cache.
	cacheKey := kind + ":" + ref
	if cached, ok := r.resolved[cacheKey]; ok {
		// Find the cached ID in the index.
		candidates := r.index.LookupRef(kind, cached)
		r.mu.RUnlock()
		if len(candidates) == 1 {
			return candidates[0], nil
		}
		// Cache is stale — fall through to re-resolve.
	} else {
		r.mu.RUnlock()
	}

	// 3. Index lookup.
	candidates := r.index.LookupRef(kind, ref)
	if len(candidates) == 0 {
		return ResourceID{}, fmt.Errorf("no %s found matching %q", kind, ref)
	}

	// 4. Single match.
	if len(candidates) == 1 {
		r.cacheResolution(cacheKey, candidates[0].Address())
		return candidates[0], nil
	}

	// 5. Policy.
	winner, err := r.policy.Resolve(kind, ref, candidates)
	if err != nil {
		return ResourceID{}, err
	}
	r.cacheResolution(cacheKey, winner.Address())
	return winner, nil
}

// SetAlias adds or updates a user-configured alias.
func (r *Resolver) SetAlias(name, target string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.aliases[name] = target
}

// Aliases returns a copy of the current alias map.
func (r *Resolver) Aliases() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.aliases))
	for k, v := range r.aliases {
		out[k] = v
	}
	return out
}

// Resolved returns a copy of the cached resolution map.
func (r *Resolver) Resolved() map[string]string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make(map[string]string, len(r.resolved))
	for k, v := range r.resolved {
		out[k] = v
	}
	return out
}

// Index returns the underlying resource index.
func (r *Resolver) Index() *ResourceIndex { return r.index }

func (r *Resolver) cacheResolution(key, address string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resolved[key] = address
}
