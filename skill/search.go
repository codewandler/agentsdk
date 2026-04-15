package skill

import (
	"context"
	"encoding/json"
)

// SearchResult is a single skill discovered via a remote search provider.
type SearchResult struct {
	Name          string          `json:"name"`
	Description   string          `json:"description"`
	Source        string          `json:"source"`
	URL           string          `json:"url"`
	Tags          []string        `json:"tags,omitempty"`
	InstallerName string          `json:"installer"`
	InstallRef    json.RawMessage `json:"install_ref"`
}

// SearchOptions configures a skill search request.
type SearchOptions struct {
	MaxResults int
}

// Searcher is implemented by remote skill search backends.
type Searcher interface {
	Search(ctx context.Context, query string, opts SearchOptions) ([]SearchResult, error)
	Name() string
}

// SearcherKey is the Extra() key for a Searcher.
const SearcherKey = "skill_searcher"
