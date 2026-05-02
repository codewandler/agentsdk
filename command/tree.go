package command

import (
	"context"
	"fmt"
	"strings"
)

// TreeHandler handles a validated command tree invocation.
type TreeHandler func(context.Context, Invocation) (Result, error)

// NodeOption configures a command tree node.
type NodeOption interface {
	applyNodeOption(*treeNode)
}

type nodeOptionFunc func(*treeNode)

func (fn nodeOptionFunc) applyNodeOption(n *treeNode) { fn(n) }

// Description sets command node description metadata.
func Description(description string) NodeOption {
	return nodeOptionFunc(func(n *treeNode) { n.spec.Description = strings.TrimSpace(description) })
}

// ArgumentHint sets compact usage hint metadata for a command node.
func ArgumentHint(hint string) NodeOption {
	return nodeOptionFunc(func(n *treeNode) { n.spec.ArgumentHint = strings.TrimSpace(hint) })
}

// Alias adds aliases for a command node.
func Alias(aliases ...string) NodeOption {
	return nodeOptionFunc(func(n *treeNode) {
		for _, alias := range aliases {
			alias = strings.TrimPrefix(strings.TrimSpace(alias), "/")
			if alias != "" {
				n.spec.Aliases = append(n.spec.Aliases, alias)
			}
		}
	})
}

// WithPolicy sets caller policy metadata for a command node.
func WithPolicy(policy Policy) NodeOption {
	return nodeOptionFunc(func(n *treeNode) { n.spec.Policy = policy })
}

// ArgSpec declares a positional argument accepted by a tree node.
type ArgSpec struct {
	Name        string
	Description string
	IsRequired  bool
	IsVariadic  bool
}

// Arg declares a positional argument.
func Arg(name string) ArgSpec { return ArgSpec{Name: strings.TrimSpace(name)} }

// Describe sets the argument description.
func (a ArgSpec) Describe(description string) ArgSpec {
	a.Description = strings.TrimSpace(description)
	return a
}

// Required marks the argument as required.
func (a ArgSpec) Required() ArgSpec { a.IsRequired = true; return a }

// Variadic marks the argument as consuming all remaining positional values.
func (a ArgSpec) Variadic() ArgSpec { a.IsVariadic = true; return a }

func (a ArgSpec) applyNodeOption(n *treeNode) { n.args = append(n.args, a) }

// FlagSpec declares a named flag accepted by a tree node.
type FlagSpec struct {
	Name        string
	Description string
	IsRequired  bool
	EnumValues  []string
}

// Flag declares a named flag.
func Flag(name string) FlagSpec { return FlagSpec{Name: strings.TrimSpace(name)} }

// Describe sets the flag description.
func (f FlagSpec) Describe(description string) FlagSpec {
	f.Description = strings.TrimSpace(description)
	return f
}

// Required marks the flag as required.
func (f FlagSpec) Required() FlagSpec { f.IsRequired = true; return f }

// Enum restricts the flag to one of values.
func (f FlagSpec) Enum(values ...string) FlagSpec {
	f.EnumValues = append([]string(nil), values...)
	return f
}

func (f FlagSpec) applyNodeOption(n *treeNode) { n.flags = append(n.flags, f) }

// Invocation carries a validated command tree invocation to a handler.
type Invocation struct {
	Path  []string
	Raw   string
	Args  map[string][]string
	Flags map[string]string
}

// Arg returns the argument named name. Variadic arguments are joined with spaces.
func (i Invocation) Arg(name string) string {
	vals := i.Args[name]
	if len(vals) == 0 {
		return ""
	}
	return strings.Join(vals, " ")
}

// ArgValues returns all values for the argument named name.
func (i Invocation) ArgValues(name string) []string {
	return append([]string(nil), i.Args[name]...)
}

// Flag returns the flag named name.
func (i Invocation) Flag(name string) string { return i.Flags[name] }

// CommandInput executes a tree command without converting structured input into slash-command text.
type CommandInput struct {
	Path  []string
	Args  map[string]any
	Flags map[string]any
}

// Descriptor describes a command tree node for help, UI, and machine-readable discovery.
type Descriptor struct {
	Name         string
	Path         []string
	Description  string
	ArgumentHint string
	Args         []ArgDescriptor
	Flags        []FlagDescriptor
	Subcommands  []Descriptor
}

// ArgDescriptor describes one positional argument.
type ArgDescriptor struct {
	Name        string
	Description string
	Required    bool
	Variadic    bool
}

// FlagDescriptor describes one named flag.
type FlagDescriptor struct {
	Name        string
	Description string
	Required    bool
	EnumValues  []string
}

// ValidationErrorCode identifies a command tree validation failure.
type ValidationErrorCode string

const (
	ValidationMissingArg        ValidationErrorCode = "missing_arg"
	ValidationTooManyArgs       ValidationErrorCode = "too_many_args"
	ValidationUnknownArg        ValidationErrorCode = "unknown_arg"
	ValidationUnknownFlag       ValidationErrorCode = "unknown_flag"
	ValidationMissingFlag       ValidationErrorCode = "missing_flag"
	ValidationInvalidFlagValue  ValidationErrorCode = "invalid_flag_value"
	ValidationInvalidArgValue   ValidationErrorCode = "invalid_arg_value"
	ValidationUnknownSubcommand ValidationErrorCode = "unknown_subcommand"
	ValidationInvalidSpec       ValidationErrorCode = "invalid_spec"
)

// ValidationError describes invalid command input or an invalid command spec.
type ValidationError struct {
	Path    []string
	Code    ValidationErrorCode
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	if e.Field != "" {
		return fmt.Sprintf("command: %s: %s", e.Code, e.Field)
	}
	return fmt.Sprintf("command: %s", e.Code)
}

// HelpPayload displays descriptor-derived command usage, optionally with a validation error.
type HelpPayload struct {
	Descriptor Descriptor
	Error      *ValidationError
}

func (p HelpPayload) Display(DisplayMode) (string, error) {
	var b strings.Builder
	if p.Error != nil && p.Error.Message != "" {
		b.WriteString(p.Error.Message)
		b.WriteByte('\n')
	}
	writeUsage(&b, p.Descriptor)
	return strings.TrimRight(b.String(), "\n"), nil
}

func writeUsage(b *strings.Builder, desc Descriptor) {
	fmt.Fprintf(b, "usage: /%s", strings.Join(desc.Path, " "))
	if len(desc.Subcommands) > 0 {
		fmt.Fprintf(b, " <%s>", strings.Join(descriptorNames(desc.Subcommands), "|"))
	}
	writeArgsUsage(b, desc.Args)
	writeFlagsUsage(b, desc.Flags)
	b.WriteByte('\n')
	for _, sub := range desc.Subcommands {
		fmt.Fprintf(b, "  /%s", strings.Join(sub.Path, " "))
		writeArgsUsage(b, sub.Args)
		writeFlagsUsage(b, sub.Flags)
		if sub.Description != "" {
			fmt.Fprintf(b, " - %s", sub.Description)
		}
		b.WriteByte('\n')
	}
}

func writeArgsUsage(b *strings.Builder, args []ArgDescriptor) {
	for _, arg := range args {
		name := arg.Name
		if arg.Variadic {
			name += "..."
		}
		if arg.Required {
			fmt.Fprintf(b, " <%s>", name)
		} else {
			fmt.Fprintf(b, " [%s]", name)
		}
	}
}

func writeFlagsUsage(b *strings.Builder, flags []FlagDescriptor) {
	for _, flag := range flags {
		value := flag.Name
		if len(flag.EnumValues) > 0 {
			value = strings.Join(flag.EnumValues, "|")
		}
		part := fmt.Sprintf("--%s <%s>", flag.Name, value)
		if flag.Required {
			fmt.Fprintf(b, " %s", part)
		} else {
			fmt.Fprintf(b, " [%s]", part)
		}
	}
}

func descriptorNames(descs []Descriptor) []string {
	names := make([]string, 0, len(descs))
	for _, desc := range descs {
		if desc.Name != "" {
			names = append(names, desc.Name)
		}
	}
	return names
}

type treeNode struct {
	spec     Spec
	handler  TreeHandler
	args     []ArgSpec
	flags    []FlagSpec
	children []*treeNode
	index    map[string]*treeNode
	parent   *treeNode
}

// Tree is a declarative command tree that implements Command.
type Tree struct {
	root *treeNode
	err  error
}

// NewTree creates a command tree rooted at name.
func NewTree(name string, opts ...NodeOption) *Tree {
	node := &treeNode{spec: Spec{Name: strings.TrimPrefix(strings.TrimSpace(name), "/")}, index: map[string]*treeNode{}}
	applyNodeOptions(node, opts...)
	return &Tree{root: node}
}

var _ Command = (*Tree)(nil)

// Spec returns the root command spec.
func (t *Tree) Spec() Spec {
	if t == nil || t.root == nil {
		return Spec{}
	}
	return t.root.spec
}

// Handle sets the root handler.
func (t *Tree) Handle(handler TreeHandler, opts ...NodeOption) *Tree {
	if t == nil || t.root == nil {
		return t
	}
	t.root.handler = handler
	applyNodeOptions(t.root, opts...)
	if err := validateNodeSpec(t.root); err != nil && t.err == nil {
		t.err = err
	}
	return t
}

// Sub adds a subcommand to the root and records any construction error on the tree.
func (t *Tree) Sub(name string, handler TreeHandler, opts ...NodeOption) *Tree {
	if t == nil || t.root == nil {
		return t
	}
	if _, err := addSubNode(t.root, name, handler, opts...); err != nil && t.err == nil {
		t.err = err
	}
	return t
}

// Build validates and returns the tree.
func (t *Tree) Build() (*Tree, error) {
	if t == nil || t.root == nil {
		return nil, ValidationError{Code: ValidationInvalidSpec, Message: "command: tree is nil"}
	}
	if t.err != nil {
		return nil, t.err
	}
	if err := validateTree(t.root); err != nil {
		return nil, err
	}
	return t, nil
}

// Execute validates params and dispatches to the matching tree node.
func (t *Tree) Execute(ctx context.Context, params Params) (Result, error) {
	if t == nil || t.root == nil {
		return Result{}, ValidationError{Code: ValidationInvalidSpec, Message: "command: tree is nil"}
	}
	if t.err != nil {
		return Result{}, t.err
	}
	node, rest, unknown := resolveNode(t.root, params.Args)
	if unknown != "" {
		err := ValidationError{Path: node.path(), Code: ValidationUnknownSubcommand, Field: unknown, Message: fmt.Sprintf("unknown subcommand %q", unknown)}
		return Display(HelpPayload{Descriptor: node.descriptor(), Error: &err}), nil
	}
	inv, validation := node.invocation(params, rest)
	if validation != nil {
		return Display(HelpPayload{Descriptor: node.descriptor(), Error: validation}), nil
	}
	if node.handler == nil {
		return Display(HelpPayload{Descriptor: node.descriptor()}), nil
	}
	return node.handler(ctx, inv)
}

// ExecuteInput validates structured input and dispatches to the matching tree node.
func (t *Tree) ExecuteInput(ctx context.Context, input CommandInput) (Result, error) {
	if t == nil || t.root == nil {
		return Result{}, ValidationError{Code: ValidationInvalidSpec, Message: "command: tree is nil"}
	}
	if t.err != nil {
		return Result{}, t.err
	}
	node, validation := t.resolveInputPath(input.Path)
	if validation != nil {
		return Display(HelpPayload{Descriptor: t.root.descriptor(), Error: validation}), nil
	}
	inv, validation := node.structuredInvocation(input)
	if validation != nil {
		return Display(HelpPayload{Descriptor: node.descriptor(), Error: validation}), nil
	}
	if node.handler == nil {
		return Display(HelpPayload{Descriptor: node.descriptor()}), nil
	}
	return node.handler(ctx, inv)
}

// ExecuteMap validates structured input by command path. Input keys are split into
// declared args and flags by the target node's command spec.
func (t *Tree) ExecuteMap(ctx context.Context, path []string, input map[string]any) (Result, error) {
	if t == nil || t.root == nil {
		return Result{}, ValidationError{Code: ValidationInvalidSpec, Message: "command: tree is nil"}
	}
	if t.err != nil {
		return Result{}, t.err
	}
	node, validation := t.resolveInputPath(path)
	if validation != nil {
		return Display(HelpPayload{Descriptor: t.root.descriptor(), Error: validation}), nil
	}
	cmdInput, validation := node.commandInputFromMap(path, input)
	if validation != nil {
		return Display(HelpPayload{Descriptor: node.descriptor(), Error: validation}), nil
	}
	return t.ExecuteInput(ctx, cmdInput)
}

// Descriptor returns descriptor metadata for the tree.
func (t *Tree) Descriptor() Descriptor {
	if t == nil || t.root == nil {
		return Descriptor{}
	}
	return t.root.descriptor()
}

func addSubNode(parent *treeNode, name string, handler TreeHandler, opts ...NodeOption) (*treeNode, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "/")
	if name == "" {
		return nil, ValidationError{Path: parent.path(), Code: ValidationInvalidSpec, Message: "command: subcommand name is required"}
	}
	if parent.index == nil {
		parent.index = map[string]*treeNode{}
	}
	if _, exists := parent.index[name]; exists {
		return nil, ErrDuplicate{Name: name}
	}
	node := &treeNode{spec: Spec{Name: name}, handler: handler, parent: parent, index: map[string]*treeNode{}}
	applyNodeOptions(node, opts...)
	for _, alias := range node.spec.Aliases {
		if alias == "" {
			continue
		}
		if _, exists := parent.index[alias]; exists {
			return nil, ErrDuplicate{Name: alias}
		}
	}
	if err := validateNodeSpec(node); err != nil {
		return nil, err
	}
	parent.children = append(parent.children, node)
	parent.index[node.spec.Name] = node
	for _, alias := range node.spec.Aliases {
		if alias != "" {
			parent.index[alias] = node
		}
	}
	return node, nil
}

func applyNodeOptions(node *treeNode, opts ...NodeOption) {
	for _, opt := range opts {
		if opt != nil {
			opt.applyNodeOption(node)
		}
	}
	node.spec = normalizeSpec(node.spec)
}

func validateTree(node *treeNode) error {
	if node == nil {
		return ValidationError{Code: ValidationInvalidSpec, Message: "command: tree is nil"}
	}
	if err := validateNodeSpec(node); err != nil {
		return err
	}
	for _, child := range node.children {
		if err := validateTree(child); err != nil {
			return err
		}
	}
	return nil
}

func validateNodeSpec(node *treeNode) error {
	seenArgs := map[string]bool{}
	for i, arg := range node.args {
		if arg.Name == "" {
			return ValidationError{Path: node.path(), Code: ValidationInvalidSpec, Message: "command: argument name is required"}
		}
		if seenArgs[arg.Name] {
			return ValidationError{Path: node.path(), Code: ValidationInvalidSpec, Field: arg.Name, Message: fmt.Sprintf("command: duplicate argument %q", arg.Name)}
		}
		seenArgs[arg.Name] = true
		if arg.IsVariadic && i != len(node.args)-1 {
			return ValidationError{Path: node.path(), Code: ValidationInvalidSpec, Field: arg.Name, Message: fmt.Sprintf("command: variadic argument %q must be last", arg.Name)}
		}
	}
	seenFlags := map[string]bool{}
	for _, flag := range node.flags {
		if flag.Name == "" {
			return ValidationError{Path: node.path(), Code: ValidationInvalidSpec, Message: "command: flag name is required"}
		}
		if seenFlags[flag.Name] {
			return ValidationError{Path: node.path(), Code: ValidationInvalidSpec, Field: flag.Name, Message: fmt.Sprintf("command: duplicate flag %q", flag.Name)}
		}
		seenFlags[flag.Name] = true
	}
	return nil
}

func resolveNode(root *treeNode, args []string) (*treeNode, []string, string) {
	node := root
	remaining := append([]string(nil), args...)
	for len(remaining) > 0 {
		if len(node.children) == 0 {
			return node, remaining, ""
		}
		next, ok := node.index[remaining[0]]
		if !ok {
			return node, remaining, remaining[0]
		}
		node = next
		remaining = remaining[1:]
	}
	return node, remaining, ""
}

func (t *Tree) resolveInputPath(path []string) (*treeNode, *ValidationError) {
	clean := make([]string, 0, len(path))
	for _, part := range path {
		part = strings.TrimSpace(part)
		if part != "" {
			clean = append(clean, part)
		}
	}
	if len(clean) > 0 && clean[0] == t.root.spec.Name {
		clean = clean[1:]
	}
	node := t.root
	for _, part := range clean {
		next, ok := node.index[part]
		if !ok {
			err := ValidationError{Path: node.path(), Code: ValidationUnknownSubcommand, Field: part, Message: fmt.Sprintf("unknown subcommand %q", part)}
			return node, &err
		}
		node = next
	}
	return node, nil
}

func (n *treeNode) invocation(params Params, positional []string) (Invocation, *ValidationError) {
	inv := Invocation{Path: n.path(), Raw: params.Raw, Args: map[string][]string{}, Flags: map[string]string{}}
	flagsByName := map[string]FlagSpec{}
	for _, flag := range n.flags {
		flagsByName[flag.Name] = flag
	}
	for name, value := range params.Flags {
		flag, ok := flagsByName[name]
		if !ok {
			err := ValidationError{Path: inv.Path, Code: ValidationUnknownFlag, Field: name, Message: fmt.Sprintf("unknown flag --%s", name)}
			return Invocation{}, &err
		}
		if value == "" || value == "true" {
			err := ValidationError{Path: inv.Path, Code: ValidationMissingFlag, Field: name, Message: fmt.Sprintf("missing value for --%s", name)}
			return Invocation{}, &err
		}
		if len(flag.EnumValues) > 0 && !contains(flag.EnumValues, value) {
			err := ValidationError{Path: inv.Path, Code: ValidationInvalidFlagValue, Field: name, Message: fmt.Sprintf("invalid value %q for --%s", value, name)}
			return Invocation{}, &err
		}
		inv.Flags[name] = value
	}
	for _, flag := range n.flags {
		if flag.IsRequired && inv.Flags[flag.Name] == "" {
			err := ValidationError{Path: inv.Path, Code: ValidationMissingFlag, Field: flag.Name, Message: fmt.Sprintf("missing required flag --%s", flag.Name)}
			return Invocation{}, &err
		}
	}
	pos := 0
	for _, arg := range n.args {
		if arg.IsVariadic {
			vals := append([]string(nil), positional[pos:]...)
			if arg.IsRequired && len(vals) == 0 {
				err := ValidationError{Path: inv.Path, Code: ValidationMissingArg, Field: arg.Name, Message: fmt.Sprintf("missing required argument %q", arg.Name)}
				return Invocation{}, &err
			}
			if len(vals) > 0 {
				inv.Args[arg.Name] = vals
			}
			pos = len(positional)
			break
		}
		if pos >= len(positional) {
			if arg.IsRequired {
				err := ValidationError{Path: inv.Path, Code: ValidationMissingArg, Field: arg.Name, Message: fmt.Sprintf("missing required argument %q", arg.Name)}
				return Invocation{}, &err
			}
			continue
		}
		inv.Args[arg.Name] = []string{positional[pos]}
		pos++
	}
	if pos < len(positional) {
		err := ValidationError{Path: inv.Path, Code: ValidationTooManyArgs, Message: fmt.Sprintf("too many arguments for /%s", strings.Join(inv.Path, " "))}
		return Invocation{}, &err
	}
	return inv, nil
}

func (n *treeNode) structuredInvocation(input CommandInput) (Invocation, *ValidationError) {
	inv := Invocation{Path: n.path(), Args: map[string][]string{}, Flags: map[string]string{}}
	argNames := map[string]ArgSpec{}
	for _, arg := range n.args {
		argNames[arg.Name] = arg
	}
	for name, value := range input.Args {
		arg, ok := argNames[name]
		if !ok {
			err := ValidationError{Path: inv.Path, Code: ValidationUnknownArg, Field: name, Message: fmt.Sprintf("unknown argument %q", name)}
			return Invocation{}, &err
		}
		vals, ok := structuredValues(value)
		if !ok {
			err := ValidationError{Path: inv.Path, Code: ValidationInvalidArgValue, Field: name, Message: fmt.Sprintf("invalid value for argument %q", name)}
			return Invocation{}, &err
		}
		if !arg.IsVariadic && len(vals) > 1 {
			err := ValidationError{Path: inv.Path, Code: ValidationTooManyArgs, Field: name, Message: fmt.Sprintf("too many values for argument %q", name)}
			return Invocation{}, &err
		}
		if len(vals) > 0 {
			inv.Args[name] = vals
		}
	}
	for _, arg := range n.args {
		if arg.IsRequired && len(inv.Args[arg.Name]) == 0 {
			err := ValidationError{Path: inv.Path, Code: ValidationMissingArg, Field: arg.Name, Message: fmt.Sprintf("missing required argument %q", arg.Name)}
			return Invocation{}, &err
		}
	}
	flagNames := map[string]FlagSpec{}
	for _, flag := range n.flags {
		flagNames[flag.Name] = flag
	}
	for name, value := range input.Flags {
		flag, ok := flagNames[name]
		if !ok {
			err := ValidationError{Path: inv.Path, Code: ValidationUnknownFlag, Field: name, Message: fmt.Sprintf("unknown flag --%s", name)}
			return Invocation{}, &err
		}
		values, ok := structuredValues(value)
		if !ok || len(values) != 1 || values[0] == "" {
			err := ValidationError{Path: inv.Path, Code: ValidationInvalidFlagValue, Field: name, Message: fmt.Sprintf("invalid value for --%s", name)}
			return Invocation{}, &err
		}
		if len(flag.EnumValues) > 0 && !contains(flag.EnumValues, values[0]) {
			err := ValidationError{Path: inv.Path, Code: ValidationInvalidFlagValue, Field: name, Message: fmt.Sprintf("invalid value %q for --%s", values[0], name)}
			return Invocation{}, &err
		}
		inv.Flags[name] = values[0]
	}
	for _, flag := range n.flags {
		if flag.IsRequired && inv.Flags[flag.Name] == "" {
			err := ValidationError{Path: inv.Path, Code: ValidationMissingFlag, Field: flag.Name, Message: fmt.Sprintf("missing required flag --%s", flag.Name)}
			return Invocation{}, &err
		}
	}
	return inv, nil
}

func (n *treeNode) commandInputFromMap(path []string, input map[string]any) (CommandInput, *ValidationError) {
	cmdInput := CommandInput{Path: path, Args: map[string]any{}, Flags: map[string]any{}}
	argNames := map[string]bool{}
	for _, arg := range n.args {
		argNames[arg.Name] = true
	}
	flagNames := map[string]bool{}
	for _, flag := range n.flags {
		flagNames[flag.Name] = true
	}
	for name, value := range input {
		switch {
		case argNames[name]:
			cmdInput.Args[name] = value
		case flagNames[name]:
			cmdInput.Flags[name] = value
		default:
			err := ValidationError{Path: n.path(), Code: ValidationUnknownArg, Field: name, Message: fmt.Sprintf("unknown command input %q", name)}
			return CommandInput{}, &err
		}
	}
	return cmdInput, nil
}

func (n *treeNode) descriptor() Descriptor {
	desc := Descriptor{Name: n.spec.Name, Path: n.path(), Description: n.spec.Description, ArgumentHint: n.spec.ArgumentHint}
	for _, arg := range n.args {
		desc.Args = append(desc.Args, ArgDescriptor{Name: arg.Name, Description: arg.Description, Required: arg.IsRequired, Variadic: arg.IsVariadic})
	}
	for _, flag := range n.flags {
		desc.Flags = append(desc.Flags, FlagDescriptor{Name: flag.Name, Description: flag.Description, Required: flag.IsRequired, EnumValues: append([]string(nil), flag.EnumValues...)})
	}
	for _, child := range n.children {
		desc.Subcommands = append(desc.Subcommands, child.descriptor())
	}
	return desc
}

func (n *treeNode) path() []string {
	if n == nil {
		return nil
	}
	var rev []string
	for cur := n; cur != nil; cur = cur.parent {
		if cur.spec.Name != "" {
			rev = append(rev, cur.spec.Name)
		}
	}
	path := make([]string, len(rev))
	for i := range rev {
		path[i] = rev[len(rev)-1-i]
	}
	return path
}

func structuredValues(value any) ([]string, bool) {
	switch v := value.(type) {
	case nil:
		return nil, true
	case string:
		if v == "" {
			return nil, true
		}
		return []string{v}, true
	case fmt.Stringer:
		return []string{v.String()}, true
	case []string:
		return append([]string(nil), v...), true
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			vals, ok := structuredValues(item)
			if !ok || len(vals) > 1 {
				return nil, false
			}
			out = append(out, vals...)
		}
		return out, true
	default:
		return []string{fmt.Sprint(value)}, true
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
