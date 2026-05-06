package runtime

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

// EventContextRenderCommitted records the latest provider render fingerprints
// so resumed runtimes can continue manager-owned context diffs.
const (
	EventContextFragmentRecorded thread.EventKind = "conversation.context_fragment"
	EventContextFragmentRemoved  thread.EventKind = "conversation.context_fragment_removed"
	EventContextSnapshotRecorded thread.EventKind = "harness.context_snapshot_recorded"
	EventContextRenderCommitted  thread.EventKind = "harness.context_render_committed"
)

// ThreadRuntime binds a live thread to capabilities, context providers, and
// replay state used by the high-level runtime engine helpers.
type ThreadRuntime struct {
	live         thread.Live
	source       thread.EventSource
	capabilities *capability.Manager
	contexts     *agentcontext.Manager
	observer     *observerRef // mutable observer for the ObservableRuntime
}

// observerRef is a mutable holder so the engine can set the observer after
// ThreadRuntime construction. The ObservableRuntime reads through it.
type observerRef struct {
	fn capability.ThreadEventObserver
}

// ThreadRuntimeOption configures a ThreadRuntime.
type ThreadRuntimeOption func(*threadRuntimeConfig)

type threadRuntimeConfig struct {
	source        thread.EventSource
	context       *agentcontext.Manager
	eventObserver capability.ThreadEventObserver
}

// WithThreadRuntimeSource tags capability events emitted by the runtime with
// the provided source identity.
func WithThreadRuntimeSource(source thread.EventSource) ThreadRuntimeOption {
	return func(c *threadRuntimeConfig) { c.source = source }
}

// WithContextManager uses an existing context manager instead of creating a new
// manager for the thread runtime.
func WithContextManager(manager *agentcontext.Manager) ThreadRuntimeOption {
	return func(c *threadRuntimeConfig) { c.context = manager }
}

// WithThreadEventObserver registers an observer that is called after thread
// events are persisted. This allows the runner event system to surface
// capability state changes, compaction, and other persisted events.
func WithThreadEventObserver(observer capability.ThreadEventObserver) ThreadRuntimeOption {
	return func(c *threadRuntimeConfig) { c.eventObserver = observer }
}

// NewThreadRuntime creates the capability and context runtime for a live thread.
// The returned runtime owns replayable capability state for that thread branch.
func NewThreadRuntime(live thread.Live, registry capability.Registry, opts ...ThreadRuntimeOption) (*ThreadRuntime, error) {
	if live == nil {
		return nil, fmt.Errorf("runtime: live thread is required")
	}
	if registry == nil {
		return nil, fmt.Errorf("runtime: capability registry is required")
	}
	cfg := threadRuntimeConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	obs := &observerRef{fn: cfg.eventObserver}
	var capRuntime capability.Runtime = capability.ObservableRuntime{
		Inner: capability.NewRuntime(live, cfg.source),
		Observer: func(ctx context.Context, events []thread.Event) {
			if obs.fn != nil {
				obs.fn(ctx, events)
			}
		},
	}
	capabilities := capability.NewManager(registry, capRuntime)
	contexts := cfg.context
	if contexts == nil {
		var err error
		contexts, err = agentcontext.NewManager()
		if err != nil {
			return nil, err
		}
	}
	if err := contexts.Register(capabilities.ContextProvider()); err != nil {
		return nil, err
	}
	return &ThreadRuntime{
		live:         live,
		source:       cfg.source,
		capabilities: capabilities,
		contexts:     contexts,
		observer:     obs,
	}, nil
}

// SetEventObserver replaces the thread event observer. This is called by the
// engine at the start of each turn so persisted events are dispatched through
// the runner event system. Pass nil to disable observation.
func (tr *ThreadRuntime) SetEventObserver(observer capability.ThreadEventObserver) {
	if tr == nil || tr.observer == nil {
		return
	}
	tr.observer.fn = observer
}

// ResumeThreadRuntime resumes a live thread, replays capability events, and
// restores the last committed context render records for manager-owned diffs.
func ResumeThreadRuntime(ctx context.Context, store thread.Store, params thread.ResumeParams, registry capability.Registry, opts ...ThreadRuntimeOption) (*ThreadRuntime, thread.Stored, error) {
	if store == nil {
		return nil, thread.Stored{}, fmt.Errorf("runtime: thread store is required")
	}
	live, err := store.Resume(ctx, params)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	stored, err := store.Read(ctx, thread.ReadParams{ID: live.ID()})
	if err != nil {
		return nil, thread.Stored{}, err
	}
	if params.Source.Type != "" || params.Source.ID != "" || params.Source.SessionID != "" {
		opts = append(opts, WithThreadRuntimeSource(params.Source))
	}
	runtime, err := NewThreadRuntime(live, registry, opts...)
	if err != nil {
		return nil, thread.Stored{}, err
	}
	events, err := stored.EventsForBranch(live.BranchID())
	if err != nil {
		return nil, thread.Stored{}, err
	}
	if err := runtime.Replay(ctx, events); err != nil {
		return nil, thread.Stored{}, err
	}
	if err := runtime.ReplayContextRenders(events); err != nil {
		return nil, thread.Stored{}, err
	}
	return runtime, stored, nil
}

func (r *ThreadRuntime) Live() thread.Live {
	if r == nil {
		return nil
	}
	return r.live
}

func (r *ThreadRuntime) CapabilityManager() *capability.Manager {
	if r == nil {
		return nil
	}
	return r.capabilities
}

func (r *ThreadRuntime) ContextManager() *agentcontext.Manager {
	if r == nil {
		return nil
	}
	return r.contexts
}

func (r *ThreadRuntime) CapabilityDescriptors() []capability.Descriptor {
	if r == nil || r.capabilities == nil {
		return nil
	}
	return r.capabilities.Descriptors()
}

func (r *ThreadRuntime) CapabilityActions() []action.Action {
	if r == nil || r.capabilities == nil {
		return nil
	}
	return r.capabilities.Actions()
}

// ContextState returns a human-readable summary of the last committed context
// render records.
func (r *ThreadRuntime) ContextState() string {
	if r == nil || r.contexts == nil {
		return "context: unavailable"
	}
	return r.contexts.LastRenderState()
}

// ContextDescriptors returns metadata for registered context providers without
// rendering them.
func (r *ThreadRuntime) ContextDescriptors() []agentcontext.ProviderDescriptor {
	if r == nil || r.contexts == nil {
		return nil
	}
	return r.contexts.Descriptors()
}

// ContextSnapshot returns a machine-readable snapshot of the last committed
// context render records.
func (r *ThreadRuntime) ContextSnapshot() agentcontext.StateSnapshot {
	if r == nil || r.contexts == nil {
		return agentcontext.StateSnapshot{}
	}
	return r.contexts.StateSnapshot()
}

func (r *ThreadRuntime) AttachCapability(ctx context.Context, spec capability.AttachSpec) (capability.Capability, error) {
	if r == nil || r.capabilities == nil {
		return nil, fmt.Errorf("runtime: thread runtime is nil")
	}
	return r.capabilities.Attach(ctx, spec)
}

func (r *ThreadRuntime) EnsureCapabilities(ctx context.Context, specs ...capability.AttachSpec) error {
	if r == nil || r.capabilities == nil {
		return fmt.Errorf("runtime: thread runtime is nil")
	}
	for _, spec := range specs {
		spec = r.normalizeCapabilitySpec(spec)
		if spec.InstanceID == "" {
			return fmt.Errorf("runtime: capability instance id is required")
		}
		if _, ok := r.capabilities.Capability(spec.InstanceID); ok {
			continue
		}
		if _, err := r.capabilities.Attach(ctx, spec); err != nil {
			return err
		}
	}
	return nil
}

func (r *ThreadRuntime) normalizeCapabilitySpec(spec capability.AttachSpec) capability.AttachSpec {
	if spec.ThreadID == "" && r.live != nil {
		spec.ThreadID = r.live.ID()
	}
	if spec.BranchID == "" && r.live != nil {
		spec.BranchID = r.live.BranchID()
	}
	return spec
}

func (r *ThreadRuntime) Replay(ctx context.Context, events []thread.Event) error {
	if r == nil || r.capabilities == nil {
		return fmt.Errorf("runtime: thread runtime is nil")
	}
	return r.capabilities.Replay(ctx, events)
}

func (r *ThreadRuntime) ReplayContextRenders(events []thread.Event) error {
	if r == nil || r.contexts == nil {
		return fmt.Errorf("runtime: thread runtime is nil")
	}
	var latest contextRenderCommitted
	var ok bool
	records := map[agentcontext.ProviderKey]agentcontext.ProviderRenderRecord{}
	for _, event := range events {
		switch event.Kind {
		case EventContextFragmentRecorded:
			if err := applyContextFragmentRecorded(records, event.Payload); err != nil {
				return err
			}
		case EventContextFragmentRemoved:
			if err := applyContextFragmentRemoved(records, event.Payload); err != nil {
				return err
			}
		case EventContextRenderCommitted:
			var payload contextRenderCommitted
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				return err
			}
			latest = payload
			ok = true
		}
	}
	if ok {
		r.contexts.SetRecords(latest.Records)
	} else if len(records) > 0 {
		r.contexts.SetRecords(records)
	}
	return nil
}

func (r *ThreadRuntime) Tools() []tool.Tool {
	if r == nil || r.capabilities == nil {
		return nil
	}
	return r.capabilities.Tools()
}

func (r *ThreadRuntime) PrepareRequest(ctx context.Context, meta runner.RequestPrepareMeta, req conversation.Request) (runner.PreparedRequest, error) {
	if r == nil || r.contexts == nil || r.live == nil {
		return runner.PreparedRequest{Request: req}, nil
	}
	reason := agentcontext.RenderTurn
	if meta.Step > 1 {
		reason = agentcontext.RenderToolFollowup
	}
	render, err := r.contexts.Prepare(ctx, agentcontext.BuildRequest{
		ThreadID: string(r.live.ID()),
		BranchID: string(r.live.BranchID()),
		TurnID:   fmt.Sprintf("step_%d", meta.Step),
		Reason:   reason,
	})
	if err != nil {
		return runner.PreparedRequest{}, err
	}
	out := injectSystemContextDiff(req, render.Result)
	return runner.PreparedRequest{
		Request:      out,
		ThreadEvents: r.contextRenderEvents(render),
		Commit: func(ctx context.Context) error {
			return render.Commit()
		},
		Rollback: func(context.Context) {
			render.Rollback()
		},
	}, nil
}

func (r *ThreadRuntime) Compact(ctx context.Context, history *History, summary string, replaces ...conversation.NodeID) (conversation.NodeID, error) {
	if r == nil || r.live == nil || r.contexts == nil {
		return "", fmt.Errorf("runtime: thread runtime is nil")
	}
	if history == nil {
		return "", fmt.Errorf("runtime: history is required")
	}
	id, err := history.CompactContext(ctx, summary, replaces...)
	if err != nil {
		return "", err
	}
	_, err = r.renderAndCommitContext(ctx, agentcontext.BuildRequest{
		ThreadID:   string(r.live.ID()),
		BranchID:   string(r.live.BranchID()),
		TurnID:     string(id),
		Preference: agentcontext.PreferFull,
		Reason:     agentcontext.RenderCompaction,
	})
	return id, err
}

type contextRenderCommitted struct {
	Records map[agentcontext.ProviderKey]agentcontext.ProviderRenderRecord `json:"records"`
}

type contextFragmentRecorded struct {
	ProviderKey string                   `json:"provider_key"`
	FragmentKey string                   `json:"fragment_key"`
	Role        unified.Role             `json:"role,omitempty"`
	Authority   string                   `json:"authority,omitempty"`
	StartMarker string                   `json:"start_marker,omitempty"`
	EndMarker   string                   `json:"end_marker,omitempty"`
	Content     string                   `json:"content"`
	Fingerprint string                   `json:"fingerprint"`
	CachePolicy agentcontext.CachePolicy `json:"cache_policy,omitempty"`
}

type contextFragmentRemovedRecorded struct {
	ProviderKey         string `json:"provider_key"`
	FragmentKey         string `json:"fragment_key"`
	PreviousFingerprint string `json:"previous_fingerprint"`
}

type contextSnapshotRecorded struct {
	TurnID    string                         `json:"turn_id,omitempty"`
	Reason    agentcontext.RenderReason      `json:"reason,omitempty"`
	Providers map[string]providerSnapshotRef `json:"providers,omitempty"`
}

type providerSnapshotRef struct {
	Fingerprint string `json:"fingerprint,omitempty"`
	Inline      []byte `json:"inline,omitempty"`
}

func applyContextFragmentRecorded(records map[agentcontext.ProviderKey]agentcontext.ProviderRenderRecord, raw json.RawMessage) error {
	var payload contextFragmentRecorded
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	providerKey := agentcontext.ProviderKey(payload.ProviderKey)
	fragmentKey := agentcontext.FragmentKey(payload.FragmentKey)
	record := records[providerKey]
	record.ProviderKey = providerKey
	if record.Fragments == nil {
		record.Fragments = map[agentcontext.FragmentKey]agentcontext.RenderedFragmentRecord{}
	}
	fragment := agentcontext.ContextFragment{
		Key:         fragmentKey,
		Role:        payload.Role,
		StartMarker: payload.StartMarker,
		EndMarker:   payload.EndMarker,
		Content:     payload.Content,
		Fingerprint: payload.Fingerprint,
		Authority:   agentcontext.FragmentAuthority(payload.Authority),
		CachePolicy: payload.CachePolicy,
	}
	if fragment.Fingerprint == "" {
		fragment.Fingerprint = agentcontext.FragmentFingerprint(fragment)
	}
	record.Fragments[fragmentKey] = agentcontext.RenderedFragmentRecord{
		Key:         fragmentKey,
		Fingerprint: fragment.Fingerprint,
		Fragment:    fragment,
	}
	record.Fingerprint = agentcontext.ProviderFingerprint(record.ActiveFragments())
	records[providerKey] = record
	return nil
}

func applyContextFragmentRemoved(records map[agentcontext.ProviderKey]agentcontext.ProviderRenderRecord, raw json.RawMessage) error {
	var payload contextFragmentRemovedRecorded
	if err := json.Unmarshal(raw, &payload); err != nil {
		return err
	}
	providerKey := agentcontext.ProviderKey(payload.ProviderKey)
	fragmentKey := agentcontext.FragmentKey(payload.FragmentKey)
	record := records[providerKey]
	record.ProviderKey = providerKey
	if record.Fragments == nil {
		record.Fragments = map[agentcontext.FragmentKey]agentcontext.RenderedFragmentRecord{}
	}
	record.Fragments[fragmentKey] = agentcontext.RenderedFragmentRecord{
		Key:         fragmentKey,
		Fingerprint: payload.PreviousFingerprint,
		Removed:     true,
	}
	record.Fingerprint = agentcontext.ProviderFingerprint(record.ActiveFragments())
	records[providerKey] = record
	return nil
}

func (r *ThreadRuntime) renderAndCommitContext(ctx context.Context, req agentcontext.BuildRequest) (agentcontext.BuildResult, error) {
	if r == nil || r.contexts == nil || r.live == nil {
		return agentcontext.BuildResult{}, fmt.Errorf("runtime: thread runtime is nil")
	}
	render, err := r.contexts.Prepare(ctx, req)
	if err != nil {
		return agentcontext.BuildResult{}, err
	}
	if err := r.appendContextRenderCommitted(ctx, render); err != nil {
		render.Rollback()
		return agentcontext.BuildResult{}, err
	}
	if err := render.Commit(); err != nil {
		return agentcontext.BuildResult{}, err
	}
	return render.Result, nil
}

func (r *ThreadRuntime) appendContextRenderCommitted(ctx context.Context, render *agentcontext.PreparedRender) error {
	if r == nil || r.live == nil || render == nil {
		return nil
	}
	return r.live.Append(ctx, r.contextRenderEvents(render)...)
}

func (r *ThreadRuntime) contextRenderEvents(render *agentcontext.PreparedRender) []thread.Event {
	if r == nil || render == nil {
		return nil
	}
	result := render.Result
	events := make([]thread.Event, 0, len(result.Added)+len(result.Updated)+len(result.Removed)+1)
	for _, fragment := range append(append([]agentcontext.ContextFragment(nil), result.Added...), result.Updated...) {
		payload, err := json.Marshal(contextFragmentRecorded{
			ProviderKey: string(providerKeyForFragment(result, fragment.Key)),
			FragmentKey: string(fragment.Key),
			Role:        fragment.Role,
			Authority:   string(fragment.Authority),
			StartMarker: fragment.StartMarker,
			EndMarker:   fragment.EndMarker,
			Content:     fragment.Content,
			Fingerprint: fragment.Fingerprint,
			CachePolicy: fragment.CachePolicy,
		})
		if err != nil {
			continue
		}
		events = append(events, thread.Event{
			Kind:    EventContextFragmentRecorded,
			Payload: payload,
			Source:  r.source,
		})
	}
	for _, removed := range result.Removed {
		payload, err := json.Marshal(contextFragmentRemovedRecorded{
			ProviderKey:         string(removed.ProviderKey),
			FragmentKey:         string(removed.FragmentKey),
			PreviousFingerprint: removed.PreviousFingerprint,
		})
		if err != nil {
			continue
		}
		events = append(events, thread.Event{
			Kind:    EventContextFragmentRemoved,
			Payload: payload,
			Source:  r.source,
		})
	}
	if payload, err := json.Marshal(contextSnapshotFromRender(render)); err == nil {
		events = append(events, thread.Event{
			Kind:    EventContextSnapshotRecorded,
			Payload: payload,
			Source:  r.source,
		})
	}
	payload, err := json.Marshal(contextRenderCommitted{Records: render.Records()})
	if err != nil {
		return events
	}
	events = append(events, thread.Event{
		Kind:    EventContextRenderCommitted,
		Payload: payload,
		Source:  r.source,
	})
	return events
}

func contextSnapshotFromRender(render *agentcontext.PreparedRender) contextSnapshotRecorded {
	if render == nil {
		return contextSnapshotRecorded{}
	}
	snapshot := contextSnapshotRecorded{
		TurnID:    render.Result.TurnID,
		Reason:    render.Result.Reason,
		Providers: map[string]providerSnapshotRef{},
	}
	for _, provider := range render.Result.Providers {
		ref := providerSnapshotRef{Fingerprint: provider.Record.Fingerprint}
		if provider.Record.Snapshot != nil {
			if ref.Fingerprint == "" {
				ref.Fingerprint = provider.Record.Snapshot.Fingerprint
			}
			ref.Inline = append([]byte(nil), provider.Record.Snapshot.Data...)
		}
		snapshot.Providers[string(provider.ProviderKey)] = ref
	}
	return snapshot
}

func providerKeyForFragment(result agentcontext.BuildResult, key agentcontext.FragmentKey) agentcontext.ProviderKey {
	for _, provider := range result.Providers {
		if _, ok := provider.Record.Fragments[key]; ok {
			return provider.ProviderKey
		}
	}
	return ""
}

func (c *TurnConfig) addThreadRuntime(runtime *ThreadRuntime) error {
	capabilityTools := runtime.Tools()
	if len(capabilityTools) > 0 {
		merged, err := appendTools(c.Tools, capabilityTools)
		if err != nil {
			return err
		}
		c.Tools = merged
		c.Request.Tools = mergeUnifiedTools(c.Request.Tools, tool.UnifiedToolsFrom(merged))
	}
	c.RequestPreparer = chainRequestPreparers(c.RequestPreparer, runtime.PrepareRequest)
	return nil
}

func (c *TurnConfig) addContextManager(manager *agentcontext.Manager) {
	c.RequestPreparer = chainRequestPreparers(c.RequestPreparer, standaloneContextPreparer(manager))
}

func standaloneContextPreparer(manager *agentcontext.Manager) runner.RequestPreparer {
	return func(ctx context.Context, meta runner.RequestPrepareMeta, req conversation.Request) (runner.PreparedRequest, error) {
		if manager == nil {
			return runner.PreparedRequest{Request: req}, nil
		}
		reason := agentcontext.RenderTurn
		if meta.Step > 1 {
			reason = agentcontext.RenderToolFollowup
		}
		render, err := manager.Prepare(ctx, agentcontext.BuildRequest{
			TurnID: fmt.Sprintf("step_%d", meta.Step),
			Reason: reason,
		})
		if err != nil {
			return runner.PreparedRequest{}, err
		}
		out := injectSystemContextDiff(req, render.Result)
		return runner.PreparedRequest{
			Request: out,
			Commit: func(context.Context) error {
				return render.Commit()
			},
			Rollback: func(context.Context) {
				render.Rollback()
			},
		}, nil
	}
}

func appendTools(base []tool.Tool, extra []tool.Tool) ([]tool.Tool, error) {
	out := append([]tool.Tool(nil), base...)
	seen := make(map[string]struct{}, len(base)+len(extra))
	for _, t := range base {
		if t == nil {
			continue
		}
		name := t.Name()
		if name == "" {
			return nil, fmt.Errorf("runtime: tool name is required")
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("runtime: duplicate tool %q", name)
		}
		seen[name] = struct{}{}
	}
	for _, t := range extra {
		if t == nil {
			continue
		}
		name := t.Name()
		if name == "" {
			return nil, fmt.Errorf("runtime: tool name is required")
		}
		if _, ok := seen[name]; ok {
			return nil, fmt.Errorf("runtime: duplicate tool %q", name)
		}
		seen[name] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}

func mergeUnifiedTools(base []unified.Tool, generated []unified.Tool) []unified.Tool {
	out := append([]unified.Tool(nil), base...)
	seen := make(map[string]struct{}, len(base)+len(generated))
	for _, spec := range base {
		if spec.Name != "" {
			seen[spec.Name] = struct{}{}
		}
	}
	for _, spec := range generated {
		if spec.Name == "" {
			continue
		}
		if _, ok := seen[spec.Name]; ok {
			continue
		}
		seen[spec.Name] = struct{}{}
		out = append(out, spec)
	}
	return out
}

func chainRequestPreparers(first runner.RequestPreparer, second runner.RequestPreparer) runner.RequestPreparer {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return func(ctx context.Context, meta runner.RequestPrepareMeta, req conversation.Request) (runner.PreparedRequest, error) {
		prepared, err := first(ctx, meta, req)
		if err != nil {
			return runner.PreparedRequest{}, err
		}
		next, err := second(ctx, meta, prepared.Request)
		if err != nil {
			if prepared.Rollback != nil {
				prepared.Rollback(ctx)
			}
			return runner.PreparedRequest{}, err
		}
		return runner.PreparedRequest{
			Request:      next.Request,
			ThreadEvents: append(append([]thread.Event(nil), prepared.ThreadEvents...), next.ThreadEvents...),
			Commit: func(ctx context.Context) error {
				if prepared.Commit != nil {
					if err := prepared.Commit(ctx); err != nil {
						if next.Rollback != nil {
							next.Rollback(ctx)
						}
						return err
					}
				}
				if next.Commit != nil {
					return next.Commit(ctx)
				}
				return nil
			},
			Rollback: func(ctx context.Context) {
				if next.Rollback != nil {
					next.Rollback(ctx)
				}
				if prepared.Rollback != nil {
					prepared.Rollback(ctx)
				}
			},
		}, nil
	}
}

func renderContextFragment(fragment agentcontext.ContextFragment) string {
	content := strings.TrimSpace(fragment.Content)
	if content == "" {
		return ""
	}
	start := strings.TrimSpace(fragment.StartMarker)
	end := strings.TrimSpace(fragment.EndMarker)
	if start != "" && end != "" {
		return start + "\n" + content + "\n" + end
	}
	if start != "" {
		return start + "\n" + content
	}
	if end != "" {
		return content + "\n" + end
	}
	return content
}

func renderContextDiff(result agentcontext.BuildResult) (string, bool) {
	var b strings.Builder
	var hasDiff bool
	for _, provider := range result.Providers {
		if len(provider.Diff.Added) == 0 && len(provider.Diff.Updated) == 0 && len(provider.Diff.Removed) == 0 {
			continue
		}
		hasDiff = true
		if b.Len() == 0 {
			b.WriteString("<system-context>\n")
		}
		writeXMLStart(&b, "provider", map[string]string{
			"key": string(provider.ProviderKey),
		}, 1)
		for _, fragment := range provider.Diff.Added {
			writeContextFragmentDiff(&b, "added", fragment, 2)
		}
		for _, fragment := range provider.Diff.Updated {
			writeContextFragmentDiff(&b, "updated", fragment, 2)
		}
		for _, removed := range provider.Diff.Removed {
			writeContextFragmentRemoval(&b, removed, 2)
		}
		writeXMLEnd(&b, "provider", 1)
	}
	if !hasDiff {
		return "", false
	}
	b.WriteString("</system-context>")
	return b.String(), true
}

func writeContextFragmentDiff(b *strings.Builder, tag string, fragment agentcontext.ContextFragment, indent int) {
	attrs := map[string]string{
		"fragment":  string(fragment.Key),
		"role":      string(fragment.Role),
		"authority": string(fragment.Authority),
	}
	writeXMLStart(b, tag, attrs, indent)
	writeXMLText(b, renderContextFragment(fragment), indent+1)
	writeXMLEnd(b, tag, indent)
}

func writeContextFragmentRemoval(b *strings.Builder, removed agentcontext.FragmentRemoved, indent int) {
	writeXMLStart(b, "removed", map[string]string{
		"fragment":    string(removed.FragmentKey),
		"fingerprint": removed.PreviousFingerprint,
	}, indent)
	writeXMLEnd(b, "removed", indent)
}

func writeXMLStart(b *strings.Builder, tag string, attrs map[string]string, indent int) {
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteByte('<')
	b.WriteString(tag)
	keys := make([]string, 0, len(attrs))
	for key, value := range attrs {
		if value == "" {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		b.WriteByte(' ')
		b.WriteString(key)
		b.WriteString("=\"")
		var escaped strings.Builder
		_ = xml.EscapeText(&escaped, []byte(attrs[key]))
		b.WriteString(escaped.String())
		b.WriteByte('"')
	}
	b.WriteString(">\n")
}

func writeXMLEnd(b *strings.Builder, tag string, indent int) {
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString("</")
	b.WriteString(tag)
	b.WriteString(">\n")
}

func writeXMLText(b *strings.Builder, text string, indent int) {
	if text == "" {
		return
	}
	b.WriteString(strings.Repeat("  ", indent))
	b.WriteString(escapeXMLText(text))
	b.WriteByte('\n')
}

func escapeXMLText(text string) string {
	text = strings.ReplaceAll(text, "&", "&amp;")
	text = strings.ReplaceAll(text, "<", "&lt;")
	text = strings.ReplaceAll(text, ">", "&gt;")
	return text
}

func injectSystemContextDiff(req conversation.Request, result agentcontext.BuildResult) conversation.Request {
	diff, ok := renderContextDiff(result)
	if !ok || len(req.Messages) == 0 {
		return req
	}
	out := req
	out.Messages = append([]unified.Message(nil), req.Messages...)
	last := len(out.Messages) - 1
	msg := out.Messages[last]
	if msg.Role == unified.RoleTool {
		out.Messages = append(out.Messages, unified.Message{
			Role: unified.RoleUser,
			Name: "context",
			Content: []unified.ContentPart{
				unified.TextPart{Text: diff},
			},
		})
		return out
	}
	msg.Content = append([]unified.ContentPart{unified.TextPart{Text: diff}}, msg.Content...)
	out.Messages[last] = msg
	return out
}
