package harness

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

func TestSessionAgentCommandToolExecutesAgentCallableCommand(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)
	tk := session.AgentCommandProjection().Tools[0]

	require.Equal(t, AgentCommandToolName, tk.Name())
	require.NotEmpty(t, tk.Description())
	require.NotEmpty(t, tk.Guidance())
	require.NotNil(t, tk.Schema())

	res, err := tk.Execute(minimalToolCtx{Context: context.Background()}, json.RawMessage(`{"path":["workflow","show"],"input":{"name":"ask_flow"}}`))

	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "ask_flow")
}

func TestSessionAgentCommandToolRejectsNonAgentCallableCommand(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)
	tk := session.AgentCommandProjection().Tools[0]

	res, err := tk.Execute(minimalToolCtx{Context: context.Background()}, json.RawMessage(`{"path":["workflow","start"],"input":{"name":"ask_flow"}}`))

	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "not callable")
}

func TestSessionAgentCommandToolReportsMissingPathAsToolError(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)
	tk := session.AgentCommandProjection().Tools[0]

	res, err := tk.Execute(minimalToolCtx{Context: context.Background()}, json.RawMessage(`{}`))

	require.NoError(t, err)
	require.True(t, res.IsError())
	var validation command.ValidationError
	_, execErr := session.ExecuteAgentCommandEnvelope(context.Background(), CommandEnvelope{})
	require.ErrorAs(t, execErr, &validation)
	require.Contains(t, res.String(), "command envelope path is required")
}

func TestSessionAgentCommandToolSchemaUsesCommandEnvelope(t *testing.T) {
	schema := newCommandEnvelopeTestSession(t).AgentCommandProjection().Tools[0].Schema()

	require.Equal(t, "object", schema.Type)
	path, ok := schema.Properties.Get("path")
	require.True(t, ok)
	require.Equal(t, "array", path.Type)
	require.NotNil(t, path.Items)
	require.Equal(t, "string", path.Items.Type)
	input, ok := schema.Properties.Get("input")
	require.True(t, ok)
	require.Equal(t, "object", input.Type)
	require.Contains(t, schema.Required, "path")
}

func TestSessionAgentCommandToolParseErrorIsInfrastructureError(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)
	tk := session.AgentCommandProjection().Tools[0]

	_, err := tk.Execute(minimalToolCtx{Context: context.Background()}, json.RawMessage(`not-json`))

	require.Error(t, err)
	require.Contains(t, err.Error(), "parse "+AgentCommandToolName+" input")
}

var _ tool.Ctx = minimalToolCtx{}

type minimalToolCtx struct {
	context.Context
}

func (c minimalToolCtx) WorkDir() string       { return "" }
func (c minimalToolCtx) AgentID() string       { return "coder" }
func (c minimalToolCtx) SessionID() string     { return "session" }
func (c minimalToolCtx) Extra() map[string]any { return nil }
