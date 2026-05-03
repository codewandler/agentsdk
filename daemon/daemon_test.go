package daemon

import (
	"context"
	"testing"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/harness"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/stretchr/testify/require"
)

func TestHostWrapsHarnessServiceLifecycleAndStatus(t *testing.T) {
	application, err := app.New(app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model"}}))
	require.NoError(t, err)
	service := harness.NewService(application)
	storeDir := t.TempDir()

	host, err := New(Config{Service: service, SessionsDir: storeDir})
	require.NoError(t, err)
	session, err := host.OpenSession(context.Background(), harness.SessionOpenRequest{
		Name:         "daily",
		AgentName:    "coder",
		AgentOptions: []agent.Option{agent.WithClient(runnertest.NewClient())},
	})
	require.NoError(t, err)
	require.NotEmpty(t, session.SessionID())
	require.True(t, session.Info().ThreadBacked)

	status := host.Status()
	require.Equal(t, Mode, status.Mode)
	require.Equal(t, "ok", status.Health)
	require.False(t, status.Closed)
	require.Equal(t, 1, status.ActiveSessions)
	require.Equal(t, storeDir, status.Storage.SessionsDir)
	require.Len(t, status.Sessions, 1)
	require.Equal(t, "daily", status.Sessions[0].Name)

	require.NoError(t, host.Shutdown(context.Background()))
	status = host.Status()
	require.Equal(t, "closed", status.Health)
	require.True(t, status.Closed)
	_, err = host.OpenSession(context.Background(), harness.SessionOpenRequest{Name: "after-close", AgentName: "coder"})
	require.ErrorContains(t, err, "host is closed")
}

func TestHostResumeRequiresSessionAndUsesStorageDir(t *testing.T) {
	application, err := app.New(app.WithAgentSpec(agent.Spec{Name: "coder", Inference: agent.InferenceOptions{Model: "test/model"}}))
	require.NoError(t, err)
	host, err := New(Config{Service: harness.NewService(application), SessionsDir: t.TempDir()})
	require.NoError(t, err)

	_, err = host.ResumeSession(context.Background(), harness.SessionOpenRequest{AgentName: "coder"})
	require.ErrorContains(t, err, "resume session is required")
}
