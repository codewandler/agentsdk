package tool

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/codewandler/agentsdk/action"
	"github.com/invopop/jsonschema"
)

var rawMessageType = reflect.TypeOf(json.RawMessage{})
var toolResultType = reflect.TypeOf((*Result)(nil)).Elem()

// ActionBacked is implemented by tools projected from action.Action values. It
// allows callers to apply action middleware even after they only have the
// model-facing Tool projection.
type ActionBacked interface {
	Tool
	Action() action.Action
	WithAction(action.Action) Tool
}

// ActionOption configures a Tool projection over an action.Action.
type ActionOption func(*actionTool)

// WithActionGuidance sets the LLM-facing guidance string for an action-backed tool.
func WithActionGuidance(guidance string) ActionOption {
	return func(t *actionTool) { t.guidance = guidance }
}

// WithActionResultMapper overrides how action.Result values are projected into
// model-facing tool results.
func WithActionResultMapper(mapper func(action.Result) (Result, error)) ActionOption {
	return func(t *actionTool) {
		if mapper != nil {
			t.mapResult = mapper
		}
	}
}

// WithActionMiddleware applies action middleware before projecting the action as
// a tool. This is the preferred way to attach execution middleware to
// action-backed tools at construction time.
func WithActionMiddleware(middleware ...action.Middleware) ActionOption {
	return func(t *actionTool) {
		if t.action != nil {
			t.action = action.Apply(t.action, middleware...)
		}
	}
}

// FromAction exposes an action as an LLM-callable Tool.
//
// JSON arguments are decoded using the action input Type when present, then the
// action is executed. The returned action.Result is projected into the existing
// tool.Result contract so current runners and providers can consume it.
func FromAction(a action.Action, opts ...ActionOption) Tool {
	t := &actionTool{action: a, mapResult: defaultActionResultMapper}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

type actionTool struct {
	action    action.Action
	guidance  string
	mapResult func(action.Result) (Result, error)
}

func (t *actionTool) Name() string {
	if t == nil || t.action == nil {
		return ""
	}
	return t.action.Spec().Name
}

func (t *actionTool) Description() string {
	if t == nil || t.action == nil {
		return ""
	}
	return t.action.Spec().Description
}

func (t *actionTool) Schema() *jsonschema.Schema {
	if t == nil || t.action == nil {
		return nil
	}
	return t.action.Spec().Input.Schema
}

func (t *actionTool) Guidance() string {
	if t == nil {
		return ""
	}
	return t.guidance
}

func (t *actionTool) Execute(ctx Ctx, input json.RawMessage) (Result, error) {
	if t == nil || t.action == nil {
		return nil, fmt.Errorf("tool: nil action")
	}

	value, err := decodeActionInput(t.action.Spec().Input, input)
	if err != nil {
		return nil, fmt.Errorf("parse %s input: %w", t.Name(), err)
	}
	res := t.action.Execute(ctx, value)
	return t.mapResult(res)
}

// Action returns the underlying action for action-backed tools.
func (t *actionTool) Action() action.Action {
	if t == nil {
		return nil
	}
	return t.action
}

// WithAction returns a copy of t projected from a different action while
// preserving tool-specific projection options such as guidance and result
// mapping.
func (t *actionTool) WithAction(a action.Action) Tool {
	if t == nil {
		return FromAction(a)
	}
	clone := *t
	clone.action = a
	return &clone
}

// ApplyAction applies action middleware to action-backed tools. Legacy tools are
// returned unchanged so callers can use this during migration without knowing
// which tools have already moved to action-backed construction.
func ApplyAction(t Tool, middleware ...action.Middleware) Tool {
	if at, ok := t.(ActionBacked); ok {
		return at.WithAction(action.Apply(at.Action(), middleware...))
	}
	return t
}

func decodeActionInput(typ action.Type, input json.RawMessage) (any, error) {
	if len(input) == 0 || string(input) == "null" {
		input = json.RawMessage(`{}`)
	}
	if typ.GoType == nil {
		return input, nil
	}
	return typ.DecodeJSON(input)
}

func defaultActionResultMapper(res action.Result) (Result, error) {
	if err := res.Err(); err != nil {
		return Error(err.Error()), nil
	}
	if res.Data == nil {
		return Text(""), nil
	}
	if toolResult, ok := res.Data.(Result); ok {
		return toolResult, nil
	}
	if stringer, ok := res.Data.(fmt.Stringer); ok {
		return Text(stringer.String()), nil
	}
	if raw, err := json.MarshalIndent(res.Data, "", "  "); err == nil {
		return Text(string(raw)), nil
	}
	return Text(fmt.Sprint(res.Data)), nil
}

// ToAction adapts an existing Tool into an action.Action for migration paths
// that need to orchestrate legacy tools through the action abstraction.
//
// The resulting action expects json.RawMessage, []byte, string, nil, or any
// JSON-marshalable value as input. It requires an action context that also
// satisfies tool.Ctx because legacy tools depend on tool-specific context
// methods such as WorkDir and Extra.
func ToAction(t Tool) action.Action {
	if t == nil {
		return action.New(action.Spec{}, func(action.Ctx, any) action.Result {
			return action.Failed(fmt.Errorf("tool: nil tool"))
		})
	}
	return action.New(action.Spec{
		Name:        t.Name(),
		Description: t.Description(),
		Input:       action.Type{GoType: rawMessageType, Schema: t.Schema()},
		Output:      action.Type{GoType: toolResultType},
	}, func(ctx action.Ctx, input any) action.Result {
		toolCtx, ok := ctx.(Ctx)
		if !ok {
			return action.Failed(fmt.Errorf("tool action %q requires tool.Ctx", t.Name()))
		}
		raw, err := encodeToolActionInput(input)
		if err != nil {
			return action.Failed(err)
		}
		res, err := t.Execute(toolCtx, raw)
		if err != nil {
			return action.Failed(err)
		}
		return action.OK(res)
	})
}

func encodeToolActionInput(input any) (json.RawMessage, error) {
	switch v := input.(type) {
	case nil:
		return json.RawMessage(`{}`), nil
	case json.RawMessage:
		return v, nil
	case []byte:
		return json.RawMessage(v), nil
	case string:
		return json.RawMessage(v), nil
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		return raw, nil
	}
}

var (
	_ Tool         = (*actionTool)(nil)
	_ ActionBacked = (*actionTool)(nil)
)
