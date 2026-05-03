package harness

import (
	"encoding/json"
	"fmt"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/tool"
	"github.com/invopop/jsonschema"
)

const AgentCommandToolName = "session_command"

func (s *Session) agentCommandTool() tool.Tool {
	return &agentCommandTool{Session: s}
}

type agentCommandTool struct {
	Session *Session
}

func (t *agentCommandTool) Name() string { return AgentCommandToolName }

func (t *agentCommandTool) Description() string {
	return "Execute an agent-callable command from the active harness session command catalog."
}

func (t *agentCommandTool) Guidance() string {
	return "Use only command paths and input shapes from the provided agent command catalog. Commands not marked agent-callable are rejected."
}

func (t *agentCommandTool) Schema() *jsonschema.Schema {
	return jsonSchemaFromCommandSchema(CommandEnvelopeSchema())
}

func (t *agentCommandTool) Execute(ctx tool.Ctx, input json.RawMessage) (tool.Result, error) {
	var envelope CommandEnvelope
	if len(input) > 0 && string(input) != "null" {
		if err := json.Unmarshal(input, &envelope); err != nil {
			return nil, fmt.Errorf("parse %s input: %w", t.Name(), err)
		}
	}
	result, err := t.Session.ExecuteAgentCommandEnvelope(ctx, envelope)
	if err != nil {
		return tool.Error(err.Error()), nil
	}
	text, err := command.Render(result, command.DisplayLLM)
	if err != nil {
		return nil, fmt.Errorf("render %s result: %w", t.Name(), err)
	}
	return tool.Text(text), nil
}

func jsonSchemaFromCommandSchema(schema command.JSONSchema) *jsonschema.Schema {
	out := &jsonschema.Schema{
		Type:        schema.Type,
		Description: schema.Description,
		Required:    append([]string(nil), schema.Required...),
	}
	if schema.Items != nil {
		out.Items = jsonSchemaFromCommandSchema(*schema.Items)
	}
	if len(schema.Properties) > 0 {
		out.Properties = jsonschema.NewProperties()
		for name, prop := range schema.Properties {
			out.Properties.Set(name, jsonSchemaFromCommandSchema(prop))
		}
	}
	return out
}
