package agentcontext

import (
	"context"
	"time"

	"github.com/codewandler/llmadapter/unified"
)

type ProviderKey string
type FragmentKey string

type Preference string

const (
	PreferChanges Preference = "changes"
	PreferFull    Preference = "full"
)

type RenderReason string

const (
	RenderInitial           RenderReason = "initial"
	RenderTurn              RenderReason = "turn"
	RenderToolFollowup      RenderReason = "tool_followup"
	RenderResume            RenderReason = "resume"
	RenderCompaction        RenderReason = "compaction"
	RenderBranchSwitch      RenderReason = "branch_switch"
	RenderForcedFullRefresh RenderReason = "forced_full_refresh"
)

type FragmentAuthority string

const (
	AuthorityDeveloper FragmentAuthority = "developer"
	AuthorityUser      FragmentAuthority = "user"
	AuthorityTool      FragmentAuthority = "tool"
)

type CacheScope string

const (
	CacheNone   CacheScope = "none"
	CacheTurn   CacheScope = "turn"
	CacheBranch CacheScope = "branch"
	CacheThread CacheScope = "thread"
)

type CachePolicy struct {
	Stable bool
	MaxAge time.Duration
	Scope  CacheScope
}

type ContextFragment struct {
	Key         FragmentKey
	Role        unified.Role
	StartMarker string
	EndMarker   string
	Content     string
	Fingerprint string
	Authority   FragmentAuthority
	CachePolicy CachePolicy
}

type ProviderSnapshot struct {
	Fingerprint string
	Data        []byte
}

type ProviderContext struct {
	Fragments   []ContextFragment
	Snapshot    *ProviderSnapshot
	Fingerprint string
}

type Request struct {
	ThreadID     string
	BranchID     string
	TurnID       string
	HarnessState any
	Preference   Preference
	Previous     *ProviderRenderRecord
	TokenBudget  int
	Reason       RenderReason
}

type Provider interface {
	Key() ProviderKey
	GetContext(context.Context, Request) (ProviderContext, error)
}

type FingerprintingProvider interface {
	Provider
	StateFingerprint(context.Context, Request) (fingerprint string, ok bool, err error)
}
