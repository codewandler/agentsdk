package harness

import (
	"os"
	"testing"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/stretchr/testify/require"
)

func TestSessionInfoPayloadTerminalGolden(t *testing.T) {
	payload := SessionInfoPayload{Info: SessionInfo{
		SessionID:     "session_123",
		AgentName:     "coder",
		ThreadID:      thread.ID("thread_123"),
		BranchID:      thread.BranchID("branch_123"),
		ThreadBacked:  true,
		ParamsSummary: "model=test/model max_tokens=1000",
	}}

	text := renderPayload(t, payload, command.DisplayTerminal)

	requireGolden(t, "testdata/session_info_terminal.golden", text)
}

func TestWorkflowDefinitionPayloadTerminalGolden(t *testing.T) {
	payload := WorkflowDefinitionPayload{Definition: workflow.Definition{
		Name:        "ask_flow",
		Description: "Ask the default agent",
		Steps: []workflow.Step{
			{ID: "ask", Action: workflow.ActionRef{Name: "ask_agent"}},
			{ID: "summarize", Action: workflow.ActionRef{Name: "summarize"}, DependsOn: []string{"ask"}},
		},
	}}

	text := renderPayload(t, payload, command.DisplayTerminal)

	requireGolden(t, "testdata/workflow_definition_terminal.golden", text)
}

func TestHarnessStructuredPayloadsRenderJSON(t *testing.T) {
	sessionJSON := renderPayload(t, SessionInfoPayload{Info: SessionInfo{SessionID: "session_123", AgentName: "coder"}}, command.DisplayJSON)
	require.JSONEq(t, `{
		"Info": {
			"SessionID": "session_123",
			"AgentName": "coder",
			"ThreadID": "",
			"BranchID": "",
			"ThreadBacked": false,
			"ParamsSummary": ""
		}
	}`, sessionJSON)

	workflowJSON := renderPayload(t, WorkflowListPayload{Definitions: []workflow.Definition{{Name: "ask_flow", Description: "Ask the agent"}}}, command.DisplayJSON)
	require.JSONEq(t, `{
		"Definitions": [{
			"Name": "ask_flow",
			"Description": "Ask the agent",
			"Steps": null
		}]
	}`, workflowJSON)
}

func renderPayload(t *testing.T, payload any, mode command.DisplayMode) string {
	t.Helper()
	text, err := command.Render(command.Display(payload), mode)
	require.NoError(t, err)
	return text
}

func requireGolden(t *testing.T, path string, got string) {
	t.Helper()
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), got+"\n")
}
