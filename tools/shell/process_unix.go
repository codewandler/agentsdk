//go:build !windows

package shell

import (
	"os/exec"
	"syscall"
	"time"
)

// applyProcGroupKill configures cmd to run in its own process group and to
// kill the entire group when the execution context is cancelled or times out.
//
// Without this, exec.CommandContext only kills the direct bash process.
// Any child processes spawned by bash (e.g. git clone, tail in a pipeline)
// become orphans: they keep the stdout/stderr pipe write-ends open, which
// prevents the scanner goroutines in runSingleStreaming from getting EOF.
// The result is that dispatch() blocks in wg.Wait() indefinitely, so
// AppendToolResult is never called, and the conversation history ends up with
// a tool_use block that has no corresponding tool_result — causing Anthropic
// to return HTTP 400 on the next request.
func applyProcGroupKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID kills the entire process group (pgid == bash's pid
		// because Setpgid: true puts bash in its own process group).
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	// Allow a short window for Wait to return after SIGKILL before Go sends
	// a second SIGKILL. In practice the group is dead immediately.
	cmd.WaitDelay = 2 * time.Second
}
