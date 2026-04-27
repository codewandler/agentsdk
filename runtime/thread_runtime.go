package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

type ThreadRuntime struct {
	live         thread.Live
	source       thread.EventSource
	capabilities *capability.Manager
	contexts     *agentcontext.Manager
}

type ThreadRuntimeOption func(*threadRuntimeConfig)

type threadRuntimeConfig struct {
	source  thread.EventSource
	context *agentcontext.Manager
}

func WithThreadRuntimeSource(source thread.EventSource) ThreadRuntimeOption {
	return func(c *threadRuntimeConfig) { c.source = source }
}

func WithContextManager(manager *agentcontext.Manager) ThreadRuntimeOption {
	return func(c *threadRuntimeConfig) { c.context = manager }
}

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
	runtime := capability.NewRuntime(live, cfg.source)
	capabilities := capability.NewManager(registry, runtime)
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
	}, nil
}

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

func (r *ThreadRuntime) AttachCapability(ctx context.Context, spec capability.AttachSpec) (capability.Capability, error) {
	if r == nil || r.capabilities == nil {
		return nil, fmt.Errorf("runtime: thread runtime is nil")
	}
	return r.capabilities.Attach(ctx, spec)
}

func (r *ThreadRuntime) Replay(ctx context.Context, events []thread.Event) error {
	if r == nil || r.capabilities == nil {
		return fmt.Errorf("runtime: thread runtime is nil")
	}
	return r.capabilities.Replay(ctx, events)
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
	injection := contextInjectionForRender(render.Result, meta.NativeContinuation)
	out := req
	if len(injection.Instructions) > 0 {
		out.Instructions = append(append([]unified.Instruction(nil), injection.Instructions...), req.Instructions...)
	}
	if len(injection.Messages) > 0 {
		out.Messages = append(append([]unified.Message(nil), injection.Messages...), req.Messages...)
	}
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
			Request: next.Request,
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

type contextInjection struct {
	Instructions []unified.Instruction
	Messages     []unified.Message
}

func contextInjectionForRender(result agentcontext.BuildResult, nativeContinuation bool) contextInjection {
	if nativeContinuation {
		return contextInjection{
			Messages: contextRemovalMessages(result.Removed),
		}.appendFragments(append(append([]agentcontext.ContextFragment(nil), result.Added...), result.Updated...))
	}
	return contextInjection{}.appendFragments(result.Active)
}

func (i contextInjection) appendFragments(fragments []agentcontext.ContextFragment) contextInjection {
	for _, fragment := range fragments {
		content := renderContextFragment(fragment)
		if content == "" {
			continue
		}
		if fragment.Authority == agentcontext.AuthorityDeveloper || fragment.Role == unified.RoleSystem {
			kind := unified.InstructionDeveloper
			if fragment.Role == unified.RoleSystem {
				kind = unified.InstructionSystem
			}
			i.Instructions = append(i.Instructions, unified.Instruction{
				Kind:    kind,
				Name:    "context",
				Content: []unified.ContentPart{unified.TextPart{Text: content}},
				Meta: map[string]any{
					"context_fragment": string(fragment.Key),
					"authority":        string(fragment.Authority),
				},
			})
			continue
		}
		role := fragment.Role
		if role == "" || role == unified.RoleTool {
			role = unified.RoleUser
		}
		i.Messages = append(i.Messages, unified.Message{
			Role:    role,
			Name:    "context",
			Content: []unified.ContentPart{unified.TextPart{Text: content}},
			Meta: map[string]any{
				"context_fragment": string(fragment.Key),
				"authority":        string(fragment.Authority),
			},
		})
	}
	return i
}

func contextRemovalMessages(removed []agentcontext.FragmentRemoved) []unified.Message {
	messages := make([]unified.Message, 0, len(removed))
	for _, fragment := range removed {
		messages = append(messages, unified.Message{
			Role: unified.RoleUser,
			Name: "context",
			Content: []unified.ContentPart{unified.TextPart{
				Text: fmt.Sprintf("Context fragment removed: %s", fragment.FragmentKey),
			}},
			Meta: map[string]any{
				"context_fragment": string(fragment.FragmentKey),
				"context_removed":  true,
			},
		})
	}
	return messages
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
