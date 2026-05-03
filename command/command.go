// Package command provides slash-command registration, parsing, and dispatch.
package command

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Policy describes who may invoke a command.
type Policy struct {
	UserCallable  bool `json:"userCallable,omitempty"`
	AgentCallable bool `json:"agentCallable,omitempty"`
	Internal      bool `json:"internal,omitempty"`
}

func UserPolicy() Policy { return Policy{UserCallable: true} }

func AgentPolicy() Policy { return Policy{AgentCallable: true} }

func InternalPolicy() Policy { return Policy{Internal: true} }

func TrustedPolicy() Policy { return Policy{Internal: true} }

// Params carries parsed arguments from a slash command invocation.
type Params struct {
	Raw   string
	Args  []string
	Flags map[string]string
}

// Command is a named executable slash command.
type Command interface {
	Descriptor() Descriptor
	Execute(context.Context, Params) (Result, error)
}

// Handler adapts a function into a Command.
type Handler func(context.Context, Params) (Result, error)

type funcCommand struct {
	desc Descriptor
	fn   Handler
}

// New returns a command backed by fn.
func New(desc Descriptor, fn Handler) Command {
	desc = normalizeDescriptor(desc)
	desc.Executable = true
	return &funcCommand{desc: desc, fn: fn}
}

func (c *funcCommand) Descriptor() Descriptor { return cloneDescriptor(c.desc) }

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
		desc := normalizeDescriptor(cmd.Descriptor())
		if desc.Name == "" {
			return fmt.Errorf("command: command name is required")
		}
		if _, exists := r.index[desc.Name]; exists {
			return ErrDuplicate{Name: desc.Name}
		}
		for _, alias := range desc.Aliases {
			if alias == "" {
				continue
			}
			if _, exists := r.index[alias]; exists {
				return ErrDuplicate{Name: alias}
			}
		}
		r.index[desc.Name] = cmd
		for _, alias := range desc.Aliases {
			if alias != "" {
				r.index[alias] = cmd
			}
		}
		r.ordered = append(r.ordered, cmd)
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

// Descriptors returns command descriptors in registration order.
func (r *Registry) Descriptors() []Descriptor {
	if r == nil {
		return nil
	}
	out := make([]Descriptor, 0, len(r.ordered))
	for _, cmd := range r.ordered {
		if cmd != nil {
			out = append(out, cmd.Descriptor())
		}
	}
	return out
}

type mapExecutable interface {
	ExecuteMap(context.Context, []string, map[string]any) (Result, error)
}

// ExecuteMap dispatches structured input by command path.
func (r *Registry) ExecuteMap(ctx context.Context, path []string, input map[string]any) (Result, error) {
	clean := cleanPath(path)
	if len(clean) == 0 {
		return Result{}, ValidationError{Code: ValidationInvalidSpec, Message: "command: command path is required"}
	}
	cmd, ok := r.Get(clean[0])
	if !ok {
		return Result{}, ErrUnknown{Name: clean[0]}
	}
	if exec, ok := cmd.(mapExecutable); ok {
		return exec.ExecuteMap(ctx, clean, input)
	}
	if len(clean) > 1 {
		return Result{}, ValidationError{Path: clean[:1], Code: ValidationUnknownSubcommand, Field: clean[1], Message: fmt.Sprintf("unknown subcommand %q", clean[1])}
	}
	if len(input) > 0 {
		return Result{}, ValidationError{Path: clean, Code: ValidationInvalidSpec, Message: "command: structured input is not supported by this command"}
	}
	return cmd.Execute(ctx, Params{})
}

func cleanPath(path []string) []string {
	clean := make([]string, 0, len(path))
	for _, part := range path {
		part = strings.TrimPrefix(strings.TrimSpace(part), "/")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return clean
}

// UserCommands returns commands visible/callable from user-facing interfaces.
func (r *Registry) UserCommands() []Command {
	return filterCommands(r.All(), func(desc Descriptor) bool { return desc.UserCallable() })
}

// AgentCommands returns commands explicitly exposed for agent/tool invocation.
func (r *Registry) AgentCommands() []Command {
	return filterCommands(r.All(), func(desc Descriptor) bool { return desc.AgentCallable() })
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
	if !cmd.Descriptor().UserCallable() {
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
	if !cmd.Descriptor().AgentCallable() {
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
		return cmds[i].Descriptor().Name < cmds[j].Descriptor().Name
	})
	var b strings.Builder
	b.WriteString("Commands:\n")
	for _, cmd := range cmds {
		desc := cmd.Descriptor()
		if desc.Name == "" {
			continue
		}
		fmt.Fprintf(&b, "/%s", desc.Name)
		if desc.ArgumentHint != "" {
			fmt.Fprintf(&b, " %s", desc.ArgumentHint)
		}
		if len(desc.Aliases) > 0 {
			fmt.Fprintf(&b, " (aliases: %s)", strings.Join(desc.Aliases, ", "))
		}
		if desc.Description != "" {
			fmt.Fprintf(&b, " - %s", desc.Description)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func normalizeDescriptor(desc Descriptor) Descriptor {
	desc.Name = strings.TrimPrefix(strings.TrimSpace(desc.Name), "/")
	for i, alias := range desc.Aliases {
		desc.Aliases[i] = strings.TrimPrefix(strings.TrimSpace(alias), "/")
	}
	if len(desc.Path) == 0 && desc.Name != "" {
		desc.Path = []string{desc.Name}
	}
	if desc.Name != "" && len(desc.Path) > 0 {
		desc.Path[0] = strings.TrimPrefix(strings.TrimSpace(desc.Path[0]), "/")
	}
	return desc
}

func cloneDescriptor(desc Descriptor) Descriptor {
	desc.Path = append([]string(nil), desc.Path...)
	desc.Aliases = append([]string(nil), desc.Aliases...)
	desc.Args = append([]ArgDescriptor(nil), desc.Args...)
	desc.Flags = append([]FlagDescriptor(nil), desc.Flags...)
	desc.Input.Fields = append([]InputFieldDescriptor(nil), desc.Input.Fields...)
	desc.Output = cloneOutputDescriptor(desc.Output)
	desc.Subcommands = cloneDescriptors(desc.Subcommands)
	return desc
}

func cloneDescriptors(descs []Descriptor) []Descriptor {
	if len(descs) == 0 {
		return nil
	}
	out := make([]Descriptor, len(descs))
	for i, desc := range descs {
		out[i] = cloneDescriptor(desc)
	}
	return out
}

// UserCallable reports whether this command descriptor should be available in user-facing UIs.
func (d Descriptor) UserCallable() bool {
	return !d.Policy.Internal && (d.Policy.UserCallable || (!d.Policy.AgentCallable && !d.Policy.Internal))
}

// AgentCallable reports whether this command descriptor may be invoked by an agent command tool.
func (d Descriptor) AgentCallable() bool {
	return !d.Policy.Internal && d.Policy.AgentCallable
}

// InternalCallable reports whether this command descriptor is internal/trusted-only.
func (d Descriptor) InternalCallable() bool {
	return d.Policy.Internal
}

func filterCommands(commands []Command, keep func(Descriptor) bool) []Command {
	var out []Command
	for _, cmd := range commands {
		if cmd != nil && keep(cmd.Descriptor()) {
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
