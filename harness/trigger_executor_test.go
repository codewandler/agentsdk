package harness

import (
	"context"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/trigger"
	"github.com/codewandler/agentsdk/workflow"
	"github.com/stretchr/testify/require"
)

func TestTriggerExecutorStartsWorkflowAndPersistsTriggerMetadata(t *testing.T) {
	application, err := app.New(
		app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model"}}),
		app.WithActions(action.New(action.Spec{Name: "echo.trigger"}, func(_ action.Ctx, input any) action.Result {
			return action.OK(input)
		})),
		app.WithWorkflows(workflow.Definition{Name: "echo_flow", Steps: []workflow.Step{{ID: "echo", Action: workflow.ActionRef{Name: "echo.trigger"}}}}),
	)
	require.NoError(t, err)
	service := NewService(application)
	executor := TriggerExecutor{Service: service, StoreDir: t.TempDir()}

	result, err := executor.ExecuteTrigger(context.Background(), trigger.Execution{Rule: trigger.Rule{
		ID:      "daily",
		Target:  trigger.Target{Kind: trigger.TargetWorkflow, WorkflowName: "echo_flow", Input: "hello"},
		Session: trigger.SessionPolicy{Mode: trigger.SessionTriggerOwned, AgentName: "coder"},
	}, Event: trigger.Event{ID: "evt_1", Type: trigger.EventTypeInterval, SourceID: "daily", At: time.Now()}})

	require.NoError(t, err)
	require.Equal(t, trigger.TargetWorkflow, result.TargetKind)
	require.Equal(t, "echo_flow", result.TargetName)
	require.NotEmpty(t, result.SessionID)
	require.NotEmpty(t, result.WorkflowRunID)
	session, ok := service.Session("trigger-daily")
	require.True(t, ok)
	state, ok, err := session.WorkflowRunState(context.Background(), workflow.RunID(result.WorkflowRunID))
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "trigger", state.Metadata.Trigger)
	require.Equal(t, []string{"trigger", "daily"}, state.Metadata.CommandPath)
}

func TestTriggerExecutorRunsAgentPromptAndRejectsDirectAction(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	application, err := app.New(app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model"}}), app.WithAgentOptions(agent.WithClient(client)))
	require.NoError(t, err)
	service := NewService(application)
	executor := TriggerExecutor{Service: service, StoreDir: t.TempDir()}

	_, err = executor.ExecuteTrigger(context.Background(), trigger.Execution{Rule: trigger.Rule{
		ID:      "prompt",
		Target:  trigger.Target{Kind: trigger.TargetAgentPrompt, AgentName: "coder", Prompt: "hello from trigger"},
		Session: trigger.SessionPolicy{Mode: trigger.SessionTriggerOwned, AgentName: "coder"},
	}, Event: trigger.Event{ID: "evt_1", Type: trigger.EventTypeInterval}})
	require.NoError(t, err)
	require.Len(t, client.Requests(), 1)

	_, err = executor.ExecuteTrigger(context.Background(), trigger.Execution{Rule: trigger.Rule{
		ID:     "action",
		Target: trigger.Target{Kind: trigger.TargetAction, ActionName: "unsafe"},
	}, Event: trigger.Event{ID: "evt_2", Type: trigger.EventTypeInterval}})
	require.ErrorContains(t, err, "direct action targets")
}
