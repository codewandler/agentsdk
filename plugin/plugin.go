// Package plugin defines the Plugin interface and all capability sub-interfaces.
// A Plugin is a composable feature bundle that can provide tools, skills, modes,
// state context, hooks, and agent configs.
//
// When a plugin is registered with an agent, the SDK type-asserts it against each
// capability interface and installs the features it provides. A plugin may implement
// any combination of capabilities.
package plugin

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/codewandler/flai/core/cmd"
	conv "github.com/codewandler/flai/core/conversation"
	"github.com/codewandler/flai/core/hook"
	"github.com/codewandler/flai/core/mode"
	"github.com/codewandler/core/skill"
	"github.com/codewandler/core/tool"
)

// Plugin is the base marker interface. All plugins must implement this.
type Plugin interface {
	// Name returns a unique identifier for this plugin.
	// Duplicate names are rejected (first-wins, second returns error).
	Name() string
}

// ── Capability sub-interfaces ────────────────────────────────────────────────
// A plugin implements any subset of these. The agent checks each via type assertion.

// ToolsPlugin provides tools to register in the agent's tool registry.
type ToolsPlugin interface {
	Plugin
	Tools() []tool.Tool
}

// SkillsPlugin provides additional skill discovery sources.
type SkillsPlugin interface {
	Plugin
	SkillLoaders() []skill.Loader
	// PreloadSkills returns a list of skill names that should be loaded
	// immediately when this plugin is registered. This allows plugins to
	// ensure their core skills are always available.
	PreloadSkills() []string
}

// ModesPlugin provides mode definitions to register with the agent.
type ModesPlugin interface {
	Plugin
	Modes() []mode.Mode
}

// StatePlugin provides StateProviders that contribute to the STATE section.
type StatePlugin interface {
	Plugin
	StateProviders() []StateProvider
}

// HeadPlugin provides HeadProviders that contribute to the HEAD section.
// Used by the SkillsPlugin to inject loaded skill content into system messages.
type HeadPlugin interface {
	Plugin
	HeadProviders() []HeadProvider
}

// HooksPlugin installs hooks on the agent at registration time.
type HooksPlugin interface {
	Plugin
	InstallHooks(installer hook.HookInstaller)
}

// AgentsPlugin provides named agent configurations for sub-agent spawning.
type AgentsPlugin interface {
	Plugin
	AgentConfigs() []AgentConfig
}

// ExtraPlugin provides key-value pairs injected into ctx.Extra() for tool access.
// Use this so tools can retrieve plugin-scoped state without requiring the agent
// to know about every tool's dependencies.
type ExtraPlugin interface {
	Plugin
	Extra() map[string]any
}

// CommandsPlugin provides slash commands from a plugin.
// Commands are collected by the [sdk.App] at construction time and registered
// lazily via [App.RegisterPluginCommands]. Disk .agents/commands/ files take
// priority by being registered first.
type CommandsPlugin interface {
	Plugin
	Commands() []cmd.Command
}

// ── Context providers ────────────────────────────────────────────────────────

// StateProvider contributes event-sourced state to the agent context.
// Instead of rendering on every request, providers emit (event, newState)
// pairs only when their state changes. The loop collects pending events
// once per turn, injects them as a synthetic state_poll tool call, and
// persists them to history.
type StateProvider interface {
	// Key uniquely identifies this provider's contribution.
	Key() string

	// Priority controls ordering within the state_poll output (higher = earlier).
	Priority() int

	// PendingEvents returns all (event, newState) pairs accumulated since the
	// last Drain call. Returns nil if nothing has changed. Safe to call
	// multiple times — does not drain the queue.
	//
	// For StaticStateProvider: initial event is seeded lazily on first call
	// using context.Background(). Subsequent events are produced eagerly by
	// Invalidate(ctx, event).
	PendingEvents() []conv.StateEvent

	// Drain clears the pending event queue. Called by the loop after the
	// poll has been successfully persisted to history.
	// Must be safe to call concurrently with event emission.
	Drain()
}

// WritableStateProvider is a StateProvider whose state the LLM can modify
// through mutation tools. Mutation tools enqueue a StateEvent and return
// the partial update as their tool result.
type WritableStateProvider interface {
	StateProvider

	// MutationTools returns tools the LLM can call to modify this provider's
	// state. Each tool must update internal state, enqueue a StateEvent, and
	// return the partial update (NOT the full state) as its tool.Result.
	MutationTools() []tool.Tool

	// GetTool returns a read-only tool to retrieve current state without
	// affecting the event queue. May return nil if not needed.
	GetTool() tool.Tool
}

// HeadProvider contributes content to the HEAD (system) section.
// Head content is placed in system messages and is never decayed.
// Typically used by SkillsPlugin to inject loaded skill content.
type HeadProvider interface {
	// HeadKey uniquely identifies this provider's contribution.
	HeadKey() string

	// HeadPriority controls ordering within the HEAD providers (higher = earlier).
	HeadPriority() int

	// RenderHead produces the system message text for this provider.
	// Called on every LLM request.
	RenderHead(ctx context.Context) string

	// HeadBlock returns this provider's contribution as a structured HeadBlock.
	// Empty text should return a zero-value HeadBlock (skipped by callers).
	HeadBlock(ctx context.Context) conv.HeadBlock
}

// ── Supporting types ─────────────────────────────────────────────────────────

// AgentConfig is a named predefined agent profile used by the agent_run tool.
type AgentConfig struct {
	// Name is the unique identifier for this config (e.g. "researcher").
	Name string

	// Model tier alias: "FAST", "STANDARD", "POWERFUL".
	Model string

	// SystemPrompt is the system prompt for agents using this config.
	SystemPrompt string

	// ActiveTools is a list of glob patterns for initially active tools.
	ActiveTools []string

	// MaxIterations caps the loop iterations. 0 uses the default.
	MaxIterations int
}

// ── StaticStateProvider ───────────────────────────────────────────────────────

// StaticStateProvider wraps a render function into the StateProvider interface.
// Emits one "initial state" event on the first PendingEvents() call.
// After Drain(), emits nothing unless Invalidate() is called.
type StaticStateProvider struct {
	key      string
	priority int
	fn       func(context.Context) string
	mu       sync.Mutex
	pending  []conv.StateEvent
	seeded   bool
}

// NewStaticStateProvider creates a StaticStateProvider from a render function.
func NewStaticStateProvider(key string, priority int, fn func(context.Context) string) *StaticStateProvider {
	return &StaticStateProvider{key: key, priority: priority, fn: fn}
}

// NewFuncStateProvider is an alias for NewStaticStateProvider.
func NewFuncStateProvider(key string, priority int, fn func(context.Context) string) *StaticStateProvider {
	return NewStaticStateProvider(key, priority, fn)
}

func (p *StaticStateProvider) Key() string   { return p.key }
func (p *StaticStateProvider) Priority() int { return p.priority }

// PendingEvents returns queued events. On the first call (before any Invalidate),
// seeds with an "initial state" event rendered via context.Background().
func (p *StaticStateProvider) PendingEvents() []conv.StateEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.seeded {
		p.seeded = true
		newState := strings.TrimSpace(p.fn(context.Background()))
		if newState != "" {
			p.pending = append(p.pending, conv.StateEvent{
				Event:    "initial state",
				NewState: newState,
				Time:     time.Now(),
			})
		}
	}
	if len(p.pending) == 0 {
		return nil
	}
	result := make([]conv.StateEvent, len(p.pending))
	copy(result, p.pending)
	return result
}

// Drain clears the pending event queue.
func (p *StaticStateProvider) Drain() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending = nil
}

// Invalidate marks this provider as changed. fn(ctx) is called eagerly and the
// result is stored as a pending StateEvent. No event is enqueued if the
// rendered state is empty.
func (p *StaticStateProvider) Invalidate(ctx context.Context, event string) {
	newState := strings.TrimSpace(p.fn(ctx))
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.seeded {
		p.seeded = true
	}
	if newState != "" {
		p.pending = append(p.pending, conv.StateEvent{
			Event:    event,
			NewState: newState,
			Time:     time.Now(),
		})
	}
}
