package resource

import (
	"path/filepath"
	"strings"
)

// Namespace is an origin-local path that scopes a resource. Internally stored
// as string segments, rendered as "/"-joined for display. The agentdir loader
// strips conventional directories (commands/, agents/, skills/) into Kind;
// Namespace is the path from origin root to the agentdir root.
type Namespace struct {
	segments []string
}

// NewNamespace creates a Namespace from path segments.
func NewNamespace(segments ...string) Namespace {
	clean := make([]string, 0, len(segments))
	for _, s := range segments {
		if s = strings.TrimSpace(s); s != "" {
			clean = append(clean, s)
		}
	}
	return Namespace{segments: clean}
}

// Segments returns the namespace path segments.
func (n Namespace) Segments() []string {
	return append([]string(nil), n.segments...)
}

// Len returns the number of segments.
func (n Namespace) Len() int { return len(n.segments) }

// Last returns the final segment, or "" if empty.
func (n Namespace) Last() string {
	if len(n.segments) == 0 {
		return ""
	}
	return n.segments[len(n.segments)-1]
}

// String renders the namespace as "/"-joined segments.
func (n Namespace) String() string {
	return strings.Join(n.segments, "/")
}

// IsEmpty returns true if the namespace has no segments.
func (n Namespace) IsEmpty() bool { return len(n.segments) == 0 }

// Equal returns true if both namespaces have identical segments.
func (n Namespace) Equal(other Namespace) bool {
	if len(n.segments) != len(other.segments) {
		return false
	}
	for i, s := range n.segments {
		if s != other.segments[i] {
			return false
		}
	}
	return true
}

// SuffixMatch returns true if the given segments match the tail of this
// namespace. For example, namespace ["a","b","c"] suffix-matches ["b","c"]
// and ["c"], but not ["a","c"].
func (n Namespace) SuffixMatch(suffix []string) bool {
	if len(suffix) > len(n.segments) {
		return false
	}
	offset := len(n.segments) - len(suffix)
	for i, s := range suffix {
		if n.segments[offset+i] != s {
			return false
		}
	}
	return true
}

// Append returns a new Namespace with additional segments appended.
func (n Namespace) Append(segments ...string) Namespace {
	combined := make([]string, 0, len(n.segments)+len(segments))
	combined = append(combined, n.segments...)
	for _, s := range segments {
		if s = strings.TrimSpace(s); s != "" {
			combined = append(combined, s)
		}
	}
	return Namespace{segments: combined}
}

// ResourceID is the structured identity of a resource. Kind scopes resolution
// but is not part of the user-facing address. The canonical address is
// "<origin>:<namespace>:<name>" where namespace renders as "/"-joined segments.
type ResourceID struct {
	Kind      string    // "command", "agent", "workflow", "skill", "action", "plugin"
	Origin    string    // opaque loader token, no ":" allowed
	Namespace Namespace // origin-local namespace path
	Name      string    // leaf resource name
}

// Address returns the canonical address: "origin:namespace:name".
// If namespace is empty, returns "origin:name".
func (r ResourceID) Address() string {
	ns := r.Namespace.String()
	if ns == "" {
		return r.Origin + ":" + r.Name
	}
	return r.Origin + ":" + ns + ":" + r.Name
}

// String is an alias for Address.
func (r ResourceID) String() string { return r.Address() }

// IsZero returns true if the ResourceID has no origin and no name.
func (r ResourceID) IsZero() bool { return r.Origin == "" && r.Name == "" }

// Equal returns true if both IDs are identical.
func (r ResourceID) Equal(other ResourceID) bool {
	return r.Kind == other.Kind &&
		r.Origin == other.Origin &&
		r.Namespace.Equal(other.Namespace) &&
		r.Name == other.Name
}

// MatchesRef checks whether this ResourceID matches a user-typed reference.
// The ref is split on ":" into parts. The last part must match Name. The
// remaining parts (the qualifier) are matched against the resource's origin
// and namespace using two strategies:
//
//  1. Suffix match: qualifier is a suffix of [origin, namespace_segments...]
//  2. Origin + namespace suffix: first qualifier part matches origin, remaining
//     parts are a suffix of namespace segments
//
// Examples (for ID {Origin:"agentsdk", Namespace:["engineer"], Name:"commit"}):
//
//	"commit"                   → true  (name only)
//	"engineer:commit"          → true  (namespace suffix + name)
//	"agentsdk:engineer:commit" → true  (fully qualified)
//	"agentsdk:commit"          → true  (origin + name, namespace skipped)
//	"local:commit"             → false (origin mismatch)
func (r ResourceID) MatchesRef(ref string) bool {
	parts := strings.Split(ref, ":")
	if len(parts) == 0 {
		return false
	}
	// Last part must match name.
	if parts[len(parts)-1] != r.Name {
		return false
	}
	qualifier := parts[:len(parts)-1]
	if len(qualifier) == 0 {
		// Unqualified: name-only match.
		return true
	}

	// Strategy 1: suffix match against [origin, namespace...].
	full := make([]string, 0, 1+r.Namespace.Len())
	full = append(full, r.Origin)
	full = append(full, r.Namespace.segments...)
	if suffixSliceMatch(full, qualifier) {
		return true
	}

	// Strategy 2: first qualifier part is origin, rest is namespace suffix.
	if qualifier[0] == r.Origin {
		nsSuffix := qualifier[1:]
		if len(nsSuffix) == 0 {
			// Origin-only qualifier: matches any namespace.
			return true
		}
		return r.Namespace.SuffixMatch(nsSuffix)
	}

	return false
}

func suffixSliceMatch(full, suffix []string) bool {
	if len(suffix) > len(full) {
		return false
	}
	offset := len(full) - len(suffix)
	for i, s := range suffix {
		if full[offset+i] != s {
			return false
		}
	}
	return true
}

// DeriveOrigin returns the origin token for a SourceRef based on its Scope.
func DeriveOrigin(source SourceRef) string {
	switch source.Scope {
	case ScopeEmbedded:
		return "embedded"
	case ScopeProject:
		return "local"
	case ScopeUser:
		return "user"
	case ScopeRemote:
		return "remote"
	case ScopeGit:
		return "git"
	default:
		if source.Ecosystem != "" {
			return source.Ecosystem
		}
		return "unknown"
	}
}

// DeriveNamespace returns the namespace for a SourceRef. For project scope,
// it uses the basename of the root directory. For user scope, it returns
// "global". For embedded scope, it uses the root path. For other scopes,
// it uses the root path cleaned of leading dots and separators.
func DeriveNamespace(source SourceRef) Namespace {
	switch source.Scope {
	case ScopeUser:
		return NewNamespace("global")
	case ScopeProject:
		base := filepath.Base(source.Root)
		if base == "." || base == "/" || base == "" {
			return NewNamespace()
		}
		return NewNamespace(base)
	case ScopeEmbedded:
		root := strings.TrimPrefix(source.Root, ".")
		root = strings.TrimPrefix(root, "/")
		if root == "" {
			return NewNamespace()
		}
		return NewNamespace(root)
	default:
		if source.Root != "" {
			return NewNamespace(source.Root)
		}
		return NewNamespace()
	}
}

// DeriveResourceID builds a ResourceID from a SourceRef, kind, and name.
func DeriveResourceID(source SourceRef, kind, name string) ResourceID {
	return ResourceID{
		Kind:      kind,
		Origin:    DeriveOrigin(source),
		Namespace: DeriveNamespace(source),
		Name:      name,
	}
}
