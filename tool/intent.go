package tool

import (
	"encoding/json"

	"github.com/codewandler/agentsdk/action"
)

// Intent describes what a tool call is about to do. It aliases the
// surface-neutral action intent model during migration.
type Intent = action.Intent

// IntentOperation is a single resource+operation pair.
type IntentOperation = action.IntentOperation

// IntentResource identifies a resource being acted upon.
type IntentResource = action.IntentResource

// IntentProvider is an optional interface a Tool can implement to declare
// what it's about to do before execution. This enables risk assessment,
// approval gates, and audit without reverse-engineering tool semantics.
//
// Tools that don't implement IntentProvider are treated as opaque
// (Intent.Opaque = true, Confidence = "low").
type IntentProvider interface {
	// DeclareIntent inspects the raw input and returns the intent.
	// Called before Execute, with the same raw JSON.
	//
	// DeclareIntent must be side-effect-free and fast. It must not
	// perform the actual operation — only describe what would happen.
	DeclareIntent(ctx Ctx, input json.RawMessage) (Intent, error)
}

// ExtractIntent gets the intent from a tool, then walks the middleware
// chain outward letting each layer amend it via OnIntent.
func ExtractIntent(t Tool, ctx Ctx, input json.RawMessage) Intent {
	// 1. Get base intent from innermost IntentProvider.
	target := Innermost(t)
	var intent Intent
	if provider, ok := target.(IntentProvider); ok {
		var err error
		intent, err = provider.DeclareIntent(ctx, input)
		if err != nil {
			intent = opaqueToolIntent(target)
		}
	} else {
		intent = opaqueToolIntent(target)
	}
	if intent.Tool == "" && target != nil {
		intent.Tool = target.Name()
	}
	if intent.Action == "" {
		intent.Action = intent.Tool
	}
	if intent.Class == "" && intent.ToolClass != "" {
		intent.Class = intent.ToolClass
	}
	if intent.ToolClass == "" && intent.Class != "" {
		intent.ToolClass = intent.Class
	}

	// 2. Collect middleware layers (outermost-first via Unwrap walk).
	var layers []*hookedTool
	cur := t
	for {
		if ht, ok := cur.(*hookedTool); ok {
			layers = append(layers, ht)
			cur = ht.inner
		} else {
			break
		}
	}

	// 3. Reverse to inside-out order: innermost middleware amends first,
	// outermost gets the final say.
	for i, j := 0, len(layers)-1; i < j; i, j = i+1, j-1 {
		layers[i], layers[j] = layers[j], layers[i]
	}

	// 4. Let each middleware layer amend the intent.
	// Each layer gets its own empty CallState — OnIntent is not part of
	// the Execute flow, so there's no shared state to carry.
	for _, ht := range layers {
		intent = ht.onIntent(ctx, intent, nil)
	}

	return intent
}

func opaqueToolIntent(t Tool) Intent {
	name := ""
	if t != nil {
		name = t.Name()
	}
	return Intent{Action: name, Tool: name, Class: "unknown", ToolClass: "unknown", Opaque: true, Confidence: "low"}
}
