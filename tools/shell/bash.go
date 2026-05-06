// Package shell provides the bash tool.
package shell

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/cmdrisk"
)

const (
	defaultTimeout = 30
	maxTimeout     = 300
	maxOutputSize  = 50 * 1024 // 50 KB
)

// BashParams are the parameters for the bash tool.
// Cmd accepts either a single string or an array of strings (handled by StringSliceParam).
type BashParams struct {
	Cmd      tool.StringSliceParam `json:"cmd" jsonschema:"description=Shell command(s) to execute. A single string runs one command. An array runs commands sequentially in order."`
	Timeout  int                   `json:"timeout,omitempty" jsonschema:"description=Timeout in seconds (default 30 max 300)"`
	Workdir  string                `json:"workdir,omitempty" jsonschema:"description=Working directory for command execution. Defaults to the agent project root — omit unless running outside the project directory"`
	FailFast bool                  `json:"failfast,omitempty" jsonschema:"description=When true stop at the first failing command and skip the rest (only meaningful when cmd is an array)."`
}

// bashRun holds the result of a single command execution.
// It is an internal type used to build the BlocksResult returned to the LLM.
type bashRun struct {
	Command  string
	Workdir  string
	Stdout   string
	Stderr   string
	ExitCode int
	Duration time.Duration
	TimedOut bool
}

func (r bashRun) isError() bool { return r.ExitCode != 0 }

// formatText renders a single command result as plain text for the LLM.
func (r bashRun) formatText(index, total int) string {
	var sb strings.Builder
	if total > 1 {
		fmt.Fprintf(&sb, "=== command %d: %s ===\n", index+1, r.Command)
	}
	fmt.Fprintf(&sb, "[exit: %d] [duration: %.1fs]", r.ExitCode, r.Duration.Seconds())
	if r.TimedOut {
		sb.WriteString(" [timed out]")
	}
	if r.Workdir != "" {
		fmt.Fprintf(&sb, " [dir: %s]", r.Workdir)
	}
	if r.Stdout != "" {
		sb.WriteString("\n=== STDOUT ===\n")
		sb.WriteString(r.Stdout)
	}
	if r.Stderr != "" {
		sb.WriteString("\n=== STDERR ===\n")
		sb.WriteString(r.Stderr)
	}
	return sb.String()
}

// buildResult converts a slice of bashRun values into a BlocksResult.
// Blocks contains the LLM-visible text. DisplayBlocks contains one CommandBlock
// per command for rich TUI rendering.
func buildResult(runs []bashRun) tool.Result {
	b := tool.NewResult()

	// Determine overall error state.
	anyError := false
	for _, r := range runs {
		if r.isError() {
			anyError = true
			break
		}
	}
	if anyError {
		b.WithError()
	}

	// LLM-visible text: summary for multi-command arrays, then detailed output.
	var parts []string
	if len(runs) > 1 {
		parts = append(parts, formatRunSummary(runs))
	}
	for i, r := range runs {
		parts = append(parts, r.formatText(i, len(runs)))
	}
	b.Text(strings.Join(parts, "\n\n"))

	// Display-only: one CommandBlock per command.
	for _, r := range runs {
		b.Display(tool.CommandBlock{
			Command:  r.Command,
			Workdir:  r.Workdir,
			Stdout:   r.Stdout,
			Stderr:   r.Stderr,
			ExitCode: r.ExitCode,
			Duration: r.Duration,
			TimedOut: r.TimedOut,
		})
	}

	return b.Build()
}

func formatRunSummary(runs []bashRun) string {
	var sb strings.Builder
	sb.WriteString("Summary:")
	for i, r := range runs {
		marker := "✓"
		if r.isError() {
			marker = "✗"
		}
		fmt.Fprintf(&sb, "\n  %s command %d exited %d in %.1fs", marker, i+1, r.ExitCode, r.Duration.Seconds())
		if r.TimedOut {
			sb.WriteString(" (timed out)")
		}
	}
	return sb.String()
}

// Option configures the bash tool.
type Option func(*bashConfig)

type bashConfig struct {
	analyzer *cmdrisk.Analyzer
}

// WithRiskAnalyzer sets the cmdrisk analyzer for bash command risk assessment.
// When set, the bash tool's DeclareIntent uses cmdrisk to extract structured
// intent (targets, behaviors, confidence) from shell commands.
func WithRiskAnalyzer(a *cmdrisk.Analyzer) Option {
	return func(c *bashConfig) { c.analyzer = a }
}

// Tools returns the bash tool.
func Tools(opts ...Option) []tool.Tool {
	var cfg bashConfig
	for _, o := range opts {
		o(&cfg)
	}
	const bashGuidance = `When cmd is an array commands run sequentially. Use failfast=true to stop on first non-zero exit.
Output is truncated at 50 KB — pipe through head/tail for very long output.
Prefer native tools over bash for file ops: file_read not cat, grep not bash+grep, glob not find.
Never cd into the project root — workdir already defaults there.`

	return []tool.Tool{
		tool.New("bash",
			"Execute a shell command using bash. Returns stdout, stderr, and exit code. "+
				"Use for builds, tests, git operations, and pipelines. "+
				"Do NOT use bash as a substitute for native tools: use grep for search, "+
				"file_read for reading, glob for finding files, dir_list for listings.",
			func(ctx tool.Ctx, p BashParams) (tool.Result, error) {
				commands := []string(p.Cmd)

				if len(commands) == 0 {
					return nil, fmt.Errorf("cmd must be provided")
				}

				timeout := p.Timeout
				if timeout < 1 {
					timeout = defaultTimeout
				}
				if timeout > maxTimeout {
					timeout = maxTimeout
				}

				workdir := p.Workdir
				if workdir == "" {
					workdir = ctx.WorkDir()
				}

				return runSequential(ctx, commands, workdir, timeout, p.FailFast, ctx.Output())
			},
			tool.WithGuidance[BashParams](bashGuidance),
			bashIntent(cfg.analyzer),
		),
	}
}

// runSingle executes one bash command and returns a bashRun.
// Output is written to w in real time when w is not io.Discard.
func runSingle(ctx context.Context, command, workdir string, timeoutSecs int, w io.Writer) (bashRun, error) {
	execCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	start := time.Now()
	cmd := exec.CommandContext(execCtx, "bash", "-c", command)
	// Kill the entire process group on cancellation so child processes
	// (e.g. git, tail in pipelines) don't keep pipes open and block scanners.
	applyProcGroupKill(cmd)
	if workdir != "" {
		cmd.Dir = workdir
	}

	// When a real output writer is available, tee output for real-time streaming.
	if w != nil && w != io.Discard {
		return runSingleStreaming(execCtx, cmd, command, workdir, timeoutSecs, start, w)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	dur := time.Since(start)

	return buildRun(command, workdir, timeoutSecs, stdout.String(), stderr.String(), err, dur, execCtx), nil
}

// runSingleStreaming runs a command with real-time output streaming.
// Stdout and stderr are tee'd to w while also being captured for the final result.
func runSingleStreaming(ctx context.Context, cmd *exec.Cmd, command, workdir string, timeoutSecs int, start time.Time, w io.Writer) (bashRun, error) {
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return bashRun{}, fmt.Errorf("stdout pipe: %w", err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return bashRun{}, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return bashRun{}, fmt.Errorf("start command: %w", err)
	}

	var stdoutBuf, stderrBuf strings.Builder
	var wg sync.WaitGroup
	wg.Add(2)

	// Tee pipe output to both the capture buffer and the streaming writer.
	copyAndCapture := func(r io.Reader, buf *strings.Builder) {
		defer wg.Done()
		// Use a scanner so output is flushed line-by-line to the writer,
		// giving the presentation layer timely chunks.
		scanner := bufio.NewScanner(r)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // up to 1MB per line
		for scanner.Scan() {
			line := scanner.Text()
			buf.WriteString(line)
			buf.WriteString("\n")
			_, _ = fmt.Fprintln(w, line)
		}
	}

	go copyAndCapture(stdoutPipe, &stdoutBuf)
	go copyAndCapture(stderrPipe, &stderrBuf)

	wg.Wait()
	cmdErr := cmd.Wait()
	dur := time.Since(start)

	return buildRun(command, workdir, timeoutSecs,
		strings.TrimSuffix(stdoutBuf.String(), "\n"),
		strings.TrimSuffix(stderrBuf.String(), "\n"),
		cmdErr, dur, ctx), nil
}

// buildRun constructs a bashRun from collected output and command error.
func buildRun(command, workdir string, timeoutSecs int, stdout, stderr string, err error, dur time.Duration, ctx context.Context) bashRun {
	r := bashRun{
		Command:  command,
		Workdir:  workdir,
		Duration: dur,
	}

	if ctx.Err() == context.DeadlineExceeded {
		r.ExitCode = -1
		r.TimedOut = true
		out := truncateOutput(stdout)
		if out != "" {
			r.Stdout = out + fmt.Sprintf("\n[timed out after %d seconds]", timeoutSecs)
		} else {
			r.Stdout = fmt.Sprintf("[timed out after %d seconds]", timeoutSecs)
		}
		if stderr != "" {
			r.Stderr = truncateOutput(stderr)
		}
	} else if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			r.ExitCode = exitErr.ExitCode()
		} else {
			return bashRun{Command: command, Workdir: workdir, Duration: dur, ExitCode: -1, Stdout: fmt.Sprintf("[error: %s]", err.Error())}
		}
		r.Stdout = truncateOutput(stdout)
		if stderr != "" {
			r.Stderr = truncateOutput(stderr)
		}
	} else {
		r.Stdout = truncateOutput(stdout)
		if stderr != "" {
			r.Stderr = truncateOutput(stderr)
		}
	}

	return r
}

// runSequential runs multiple commands sequentially, optionally stopping at first failure.
func runSequential(ctx context.Context, commands []string, workdir string, timeoutSecs int, failFast bool, w io.Writer) (tool.Result, error) {
	runs := make([]bashRun, 0, len(commands))

	for _, cmd := range commands {
		r, err := runSingle(ctx, cmd, workdir, timeoutSecs, w)
		if err != nil {
			// Infrastructure error — stop execution.
			runs = append(runs, bashRun{
				Command:  cmd,
				Workdir:  workdir,
				Stdout:   fmt.Sprintf("[error: %s]", err.Error()),
				ExitCode: -1,
			})
			break
		}
		runs = append(runs, r)

		if failFast && r.ExitCode != 0 {
			break
		}
	}

	return buildResult(runs), nil
}

func truncateOutput(s string) string {
	if len(s) <= maxOutputSize {
		return s
	}
	return s[:maxOutputSize] + fmt.Sprintf("\n\n... (truncated, showing first %d bytes of %d)", maxOutputSize, len(s))
}
