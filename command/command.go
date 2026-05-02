// Package command provides slash-command registration, parsing, and dispatch.
package command

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Spec describes a slash command. Name and aliases do not include the leading
// slash.
type Spec struct {
	Name         string
	Aliases      []string
	Description  string
	ArgumentHint string
	Policy       Policy
}

// Policy describes who may invoke a command.
type Policy struct {
	UserCallable  bool `json:"userCallable,omitempty"`
	AgentCallable bool `json:"agentCallable,omitempty"`
	Internal      bool `json:"internal,omitempty"`
}

// Params carries parsed arguments from a slash command invocation.
type Params struct {
	Raw   string
	Args  []string
	Flags map[string]string
}

// Command is a named executable slash command.
type Command interface {
	Spec() Spec
	Execute(context.Context, Params) (Result, error)
}

// Handler adapts a function into a Command.
type Handler func(context.Context, Params) (Result, error)

type funcCommand struct {
	spec Spec
	fn   Handler
}

// New returns a command backed by fn.
func New(spec Spec, fn Handler) Command {
	return &funcCommand{spec: normalizeSpec(spec), fn: fn}
}

func (c *funcCommand) Spec() Spec { return c.spec }

func (c *funcCommand) Execute(ctx context.Context, params Params) (Result, error) {
	if c.fn == nil {
		return Handled(), nil
	}
	return c.fn(ctx, params)
}

// ResultKind describes the action a command asks its caller to perform.
type ResultKind int

const (
	ResultHandled ResultKind = iota
	ResultDisplay
	ResultAgentTurn
	ResultReset
	ResultExit
)

// DisplayMode selects the target presentation for structured command payloads.
type DisplayMode string

const (
	DisplayTerminal DisplayMode = "terminal"
	DisplayLLM      DisplayMode = "llm"
	DisplayJSON     DisplayMode = "json"
)

// Displayable is implemented by structured command payloads that know how to
// present themselves for a requested display target.
type Displayable interface {
	Display(DisplayMode) (string, error)
}

// TextPayload is a simple display payload for commands that only need to return
// text. It is still structured so command.Result does not need text-specific
// fields.
type TextPayload struct {
	Text string `json:"text"`
}

// NoticeLevel classifies generic command messages without forcing callers to
// parse terminal text.
type NoticeLevel string

const (
	NoticeInfo        NoticeLevel = "info"
	NoticeNotFound    NoticeLevel = "not_found"
	NoticeUnavailable NoticeLevel = "unavailable"
)

// NoticePayload is a generic structured payload for common command outcomes
// that do not need a domain-specific result type.
type NoticePayload struct {
	Level    NoticeLevel `json:"level"`
	Message  string      `json:"message"`
	Resource string      `json:"resource,omitempty"`
	ID       string      `json:"id,omitempty"`
}

func (p NoticePayload) Display(DisplayMode) (string, error) {
	return p.Message, nil
}

// AgentTurnPayload asks the caller to run Input as an agent turn.
type AgentTurnPayload struct {
	Input string `json:"input"`
}

// Result is the typed outcome of a command execution. Payload carries the
// structured data for the result kind; renderers turn payloads into terminal,
// LLM, or machine-readable output at the boundary.
type Result struct {
	Kind    ResultKind
	Payload any
}

// Handled indicates the command handled itself and no further action is needed.
func Handled() Result { return Result{Kind: ResultHandled} }

// Text returns a structured display result containing plain text.
func Text(text string) Result { return Display(TextPayload{Text: text}) }

// Notice returns a generic structured display message.
func Notice(message string) Result {
	return Display(NoticePayload{Level: NoticeInfo, Message: message})
}

// NotFound returns a generic structured not-found display result.
func NotFound(resource string, id string) Result {
	return Display(NoticePayload{
		Level:    NoticeNotFound,
		Message:  fmt.Sprintf("%s %q not found", resource, id),
		Resource: resource,
		ID:       id,
	})
}

// Unavailable returns a generic structured unavailable display result.
func Unavailable(message string) Result {
	return Display(NoticePayload{Level: NoticeUnavailable, Message: message})
}

// Display asks the caller to render payload to the user.
func Display(payload any) Result { return Result{Kind: ResultDisplay, Payload: payload} }

// AgentTurn asks the caller to run input as an agent turn.
func AgentTurn(input string) Result {
	return Result{Kind: ResultAgentTurn, Payload: AgentTurnPayload{Input: input}}
}

// Reset asks the caller to reset the active agent/session.
func Reset() Result { return Result{Kind: ResultReset} }

// Exit asks the caller to exit the current interactive loop.
func Exit() Result { return Result{Kind: ResultExit} }

// Render renders a display result payload for mode. Non-display results render
// to an empty string.
func Render(result Result, mode DisplayMode) (string, error) {
	if result.Kind != ResultDisplay {
		return "", nil
	}
	return RenderPayload(result.Payload, mode)
}

// RenderPayload renders one structured command payload for mode.
func RenderPayload(payload any, mode DisplayMode) (string, error) {
	if mode == DisplayJSON {
		return RenderJSON(payload)
	}
	switch p := payload.(type) {
	case nil:
		return "", nil
	case TextPayload:
		return p.Text, nil
	case *TextPayload:
		if p == nil {
			return "", nil
		}
		return p.Text, nil
	case NoticePayload:
		return p.Display(mode)
	case *NoticePayload:
		if p == nil {
			return "", nil
		}
		return p.Display(mode)
	case Displayable:
		return p.Display(mode)
	default:
		return "", fmt.Errorf("command: no renderer for payload %T", payload)
	}
}

// RenderJSON renders payload as indented JSON for machine-readable command surfaces.
func RenderJSON(payload any) (string, error) {
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// AgentTurnInput returns the prompt carried by an agent-turn result.
func AgentTurnInput(result Result) (string, bool) {
	if result.Kind != ResultAgentTurn {
		return "", false
	}
	switch p := result.Payload.(type) {
	case AgentTurnPayload:
		return p.Input, true
	case *AgentTurnPayload:
		if p == nil {
			return "", false
		}
		return p.Input, true
	default:
		return "", false
	}
}

// Registry stores commands and resolves names and aliases.
type Registry struct {
	index   map[string]Command
	ordered []Command
}

// NewRegistry returns an empty command registry.
func NewRegistry() *Registry {
	return &Registry{index: map[string]Command{}}
}

// Register adds commands to the registry. Duplicate names and aliases are
// explicit errors.
func (r *Registry) Register(commands ...Command) error {
	if r.index == nil {
		r.index = map[string]Command{}
	}
	for _, cmd := range commands {
		if cmd == nil {
			continue
		}
		spec := normalizeSpec(cmd.Spec())
		if spec.Name == "" {
			return fmt.Errorf("command: command name is required")
		}
		if _, exists := r.index[spec.Name]; exists {
			return ErrDuplicate{Name: spec.Name}
		}
		for _, alias := range spec.Aliases {
			if alias == "" {
				continue
			}
			if _, exists := r.index[alias]; exists {
				return ErrDuplicate{Name: alias}
			}
		}
		wrapped := New(spec, cmd.Execute)
		r.index[spec.Name] = wrapped
		for _, alias := range spec.Aliases {
			if alias != "" {
				r.index[alias] = wrapped
			}
		}
		r.ordered = append(r.ordered, wrapped)
	}
	return nil
}

// Get resolves name against primary names and aliases.
func (r *Registry) Get(name string) (Command, bool) {
	if r == nil {
		return nil, false
	}
	cmd, ok := r.index[strings.TrimPrefix(strings.TrimSpace(name), "/")]
	return cmd, ok
}

// All returns commands in registration order, without alias duplicates.
func (r *Registry) All() []Command {
	if r == nil {
		return nil
	}
	out := make([]Command, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// UserCommands returns commands visible/callable from user-facing interfaces.
func (r *Registry) UserCommands() []Command {
	return filterCommands(r.All(), func(spec Spec) bool { return spec.UserCallable() })
}

// AgentCommands returns commands explicitly exposed for agent/tool invocation.
func (r *Registry) AgentCommands() []Command {
	return filterCommands(r.All(), func(spec Spec) bool { return spec.AgentCallable() })
}

// Execute parses line and dispatches to the matching command.
func (r *Registry) Execute(ctx context.Context, line string) (Result, error) {
	name, params, err := Parse(line)
	if err != nil {
		return Result{}, err
	}
	cmd, ok := r.Get(name)
	if !ok {
		return Result{}, ErrUnknown{Name: name}
	}
	return cmd.Execute(ctx, params)
}

// ExecuteUser parses and dispatches a user-callable command.
func (r *Registry) ExecuteUser(ctx context.Context, line string) (Result, error) {
	name, params, err := Parse(line)
	if err != nil {
		return Result{}, err
	}
	cmd, ok := r.Get(name)
	if !ok {
		return Result{}, ErrUnknown{Name: name}
	}
	if !cmd.Spec().UserCallable() {
		return Result{}, ErrNotCallable{Name: name, Caller: "user"}
	}
	return cmd.Execute(ctx, params)
}

// ExecuteAgent parses and dispatches an agent-callable command.
func (r *Registry) ExecuteAgent(ctx context.Context, line string) (Result, error) {
	name, params, err := Parse(line)
	if err != nil {
		return Result{}, err
	}
	cmd, ok := r.Get(name)
	if !ok {
		return Result{}, ErrUnknown{Name: name}
	}
	if !cmd.Spec().AgentCallable() {
		return Result{}, ErrNotCallable{Name: name, Caller: "agent"}
	}
	return cmd.Execute(ctx, params)
}

// HelpText returns a compact command listing.
func (r *Registry) HelpText() string {
	cmds := r.UserCommands()
	if len(cmds) == 0 {
		return "No commands registered."
	}
	sort.SliceStable(cmds, func(i, j int) bool {
		return cmds[i].Spec().Name < cmds[j].Spec().Name
	})
	var b strings.Builder
	b.WriteString("Commands:\n")
	for _, cmd := range cmds {
		spec := cmd.Spec()
		if spec.Name == "" {
			continue
		}
		fmt.Fprintf(&b, "/%s", spec.Name)
		if spec.ArgumentHint != "" {
			fmt.Fprintf(&b, " %s", spec.ArgumentHint)
		}
		if len(spec.Aliases) > 0 {
			fmt.Fprintf(&b, " (aliases: %s)", strings.Join(spec.Aliases, ", "))
		}
		if spec.Description != "" {
			fmt.Fprintf(&b, " - %s", spec.Description)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func normalizeSpec(spec Spec) Spec {
	spec.Name = strings.TrimPrefix(strings.TrimSpace(spec.Name), "/")
	for i, alias := range spec.Aliases {
		spec.Aliases[i] = strings.TrimPrefix(strings.TrimSpace(alias), "/")
	}
	return spec
}

// UserCallable reports whether command should be available in user-facing UIs.
func (s Spec) UserCallable() bool {
	return !s.Policy.Internal && (s.Policy.UserCallable || (!s.Policy.AgentCallable && !s.Policy.Internal))
}

// AgentCallable reports whether command may be invoked by an agent command tool.
func (s Spec) AgentCallable() bool {
	return !s.Policy.Internal && s.Policy.AgentCallable
}

func filterCommands(commands []Command, keep func(Spec) bool) []Command {
	var out []Command
	for _, cmd := range commands {
		if cmd != nil && keep(cmd.Spec()) {
			out = append(out, cmd)
		}
	}
	return out
}

// ErrDuplicate is returned when a command name or alias is already registered.
type ErrDuplicate struct {
	Name string
}

func (e ErrDuplicate) Error() string {
	return fmt.Sprintf("command: %q is already registered", e.Name)
}

// ErrUnknown is returned when a command cannot be found.
type ErrUnknown struct {
	Name string
}

func (e ErrUnknown) Error() string {
	return fmt.Sprintf("command: unknown command %q", e.Name)
}

// ErrNotCallable is returned when a command exists but is not available to a
// caller class.
type ErrNotCallable struct {
	Name   string
	Caller string
}

func (e ErrNotCallable) Error() string {
	return fmt.Sprintf("command: %q is not callable by %s", e.Name, e.Caller)
}
