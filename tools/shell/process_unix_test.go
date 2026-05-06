//go:build !windows

package shell

import (
	"context"
	"io"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestApplyProcGroupKill_ConfiguresProcessGroup(t *testing.T) {
	cmd := exec.Command("bash", "-c", "echo ok")

	applyProcGroupKill(cmd)

	require.NotNil(t, cmd.SysProcAttr)
	require.True(t, cmd.SysProcAttr.Setpgid)
	require.NotNil(t, cmd.Cancel)
	require.Equal(t, 2*time.Second, cmd.WaitDelay)
}

func TestRunSingle_TimeoutKillsChildProcessGroupAndReturnsPromptly(t *testing.T) {
	start := time.Now()
	done := make(chan struct {
		run bashRun
		err error
	}, 1)

	go func() {
		r, err := runSingle(context.Background(), "sleep 30 & wait", "", 1, io.Discard)
		done <- struct {
			run bashRun
			err error
		}{run: r, err: err}
	}()

	select {
	case got := <-done:
		require.NoError(t, got.err)
		require.True(t, got.run.TimedOut)
		require.Equal(t, -1, got.run.ExitCode)
		require.Contains(t, got.run.Stdout, "[timed out after 1 seconds]")
		require.Less(t, time.Since(start), 4*time.Second,
			"runSingle should return shortly after the timeout instead of hanging on orphaned child processes")
	case <-time.After(4 * time.Second):
		t.Fatal("runSingle hung after timeout; child process likely kept stdout/stderr pipes open")
	}
}
