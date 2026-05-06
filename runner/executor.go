package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

const (
	canceledToolOutput = "[Canceled]"
	timedOutToolOutput = "[Timed out]"
)

type defaultToolExecutor struct {
	tools   map[string]tool.Tool
	toolCtx tool.Ctx
	timeout time.Duration
	emit    func(Event)
}

func newDefaultToolExecutor(tools []tool.Tool, toolCtx tool.Ctx, timeout time.Duration, emit func(Event)) *defaultToolExecutor {
	toolMap := make(map[string]tool.Tool, len(tools))
	for _, t := range tools {
		if t != nil {
			toolMap[t.Name()] = t
		}
	}
	return &defaultToolExecutor{tools: toolMap, toolCtx: toolCtx, timeout: timeout, emit: emit}
}

func (e *defaultToolExecutor) ExecuteTool(ctx context.Context, call unified.ToolCall) unified.ToolResult {
	if err := ctx.Err(); err != nil {
		return toolResultFromContext(call, err)
	}
	t, ok := e.tools[call.Name]
	if !ok {
		return toolResult(call, fmt.Sprintf("tool %q not found", call.Name), true)
	}

	execCtx := ctx
	cancel := func() {}
	if e.timeout > 0 {
		execCtx, cancel = context.WithTimeout(ctx, e.timeout)
	}
	defer cancel()

	toolCtx := withContextAndStreaming(e.toolCtx, execCtx, call, e.emit)
	res, err := t.Execute(toolCtx, call.Arguments)
	if err != nil {
		return toolResultFromError(call, execCtx, res, err)
	}
	if res == nil {
		// Tool returned (nil, nil). If the context expired during
		// execution, report that instead of an empty result.
		if err := execCtx.Err(); err != nil {
			return toolResultFromContext(call, err)
		}
		return toolResult(call, "", false)
	}
	// Always return the tool's result, even if the context expired.
	// The tool already had the chance to observe the cancellation and
	// include partial output (as bash does). Discarding a valid result
	// loses information the LLM needs.
	return toolResult(call, res.String(), res.IsError())
}

func toolResultFromError(call unified.ToolCall, ctx context.Context, partialResult tool.Result, err error) unified.ToolResult {
	isCanceled := errors.Is(err, context.Canceled) ||
		(ctx != nil && errors.Is(ctx.Err(), context.Canceled))
	isTimedOut := errors.Is(err, context.DeadlineExceeded) ||
		(ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded))

	if !isCanceled && !isTimedOut {
		// Regular error — no timeout/cancel involved.
		return toolResult(call, err.Error(), true)
	}

	// Timeout or cancellation. If the tool returned a partial result
	// alongside the error, preserve it so the LLM sees what was
	// produced before the interruption.
	label := timedOutToolOutput
	if isCanceled {
		label = canceledToolOutput
	}
	if partialResult != nil {
		partial := partialResult.String()
		if partial != "" {
			return toolResult(call, partial+"\n\n"+label, true)
		}
	}
	return toolResult(call, label, true)
}

func toolResultFromContext(call unified.ToolCall, err error) unified.ToolResult {
	if errors.Is(err, context.DeadlineExceeded) {
		return toolResult(call, timedOutToolOutput, true)
	}
	return toolResult(call, canceledToolOutput, true)
}

func toolResult(call unified.ToolCall, output string, isError bool) unified.ToolResult {
	return unified.ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Content:    []unified.ContentPart{unified.TextPart{Text: output}},
		IsError:    isError,
	}
}

func textFromParts(parts []unified.ContentPart) string {
	var out []string
	for _, part := range parts {
		if text, ok := part.(unified.TextPart); ok {
			out = append(out, text.Text)
		}
	}
	return strings.Join(out, "\n")
}

func withContext(base tool.Ctx, ctx context.Context) tool.Ctx {
	if base == nil {
		return &basicToolCtx{BaseCtx: action.BaseCtx{Context: ctx}, extra: map[string]any{}}
	}
	return tool.WrapCtx(base, ctx)
}

// withContextAndStreaming wraps the base tool context with Output and Emit
// wired to the runner event system via emit. The Output writer emits
// ToolOutputDeltaEvent for each line written. Emit bridges action events
// (StatusEvent, OutputEvent) to the corresponding runner events.
func withContextAndStreaming(base tool.Ctx, ctx context.Context, call unified.ToolCall, emit func(Event)) tool.Ctx {
	if emit == nil {
		return withContext(base, ctx)
	}

	output := &toolOutputWriter{
		callID: call.ID,
		name:   call.Name,
		emit:   emit,
	}
	emitter := func(event action.Event) {
		switch ev := event.(type) {
		case action.StatusEvent:
			emit(ToolStatusEvent{
				CallID:   call.ID,
				Name:     call.Name,
				Progress: ev.Progress,
				Message:  ev.Message,
			})
		case action.OutputEvent:
			emit(ToolOutputDeltaEvent{
				CallID: call.ID,
				Name:   call.Name,
				Stream: ev.Stream,
				Chunk:  string(ev.Chunk),
			})
		}
	}

	if base == nil {
		return &streamingToolCtx{
			basicToolCtx: basicToolCtx{BaseCtx: action.BaseCtx{Context: ctx}, extra: map[string]any{}},
			output:       output,
			emit:         emitter,
		}
	}
	wrapped := tool.WrapCtx(base, ctx)
	return &streamingToolCtx{
		wrappedToolCtx: wrapped,
		output:         output,
		emit:           emitter,
	}
}

// streamingToolCtx overrides Output() and Emit() on a wrapped tool context.
type streamingToolCtx struct {
	// Exactly one of these is set.
	basicToolCtx   basicToolCtx
	wrappedToolCtx tool.Ctx

	output io.Writer
	emit   func(action.Event)
}

func (c *streamingToolCtx) base() tool.Ctx {
	if c.wrappedToolCtx != nil {
		return c.wrappedToolCtx
	}
	return &c.basicToolCtx
}

func (c *streamingToolCtx) Deadline() (time.Time, bool) { return c.base().Deadline() }
func (c *streamingToolCtx) Done() <-chan struct{}        { return c.base().Done() }
func (c *streamingToolCtx) Err() error                   { return c.base().Err() }
func (c *streamingToolCtx) Value(key any) any            { return c.base().Value(key) }
func (c *streamingToolCtx) WorkDir() string              { return c.base().WorkDir() }
func (c *streamingToolCtx) AgentID() string              { return c.base().AgentID() }
func (c *streamingToolCtx) SessionID() string            { return c.base().SessionID() }
func (c *streamingToolCtx) Extra() map[string]any        { return c.base().Extra() }
func (c *streamingToolCtx) Output() io.Writer            { return c.output }
func (c *streamingToolCtx) Emit(event action.Event)      { c.emit(event) }

// toolOutputWriter is an io.Writer that emits ToolOutputDeltaEvent for each
// newline-delimited line written. Partial lines (without trailing newline)
// are emitted immediately as-is.
type toolOutputWriter struct {
	callID string
	name   string
	emit   func(Event)
	buf    []byte
}

func (w *toolOutputWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		w.emit(ToolOutputDeltaEvent{
			CallID: w.callID,
			Name:   w.name,
			Stream: "stdout",
			Chunk:  line,
		})
	}
	return n, nil
}

// Flush emits any remaining buffered content that didn't end with a newline.
func (w *toolOutputWriter) Flush() {
	if len(w.buf) > 0 {
		w.emit(ToolOutputDeltaEvent{
			CallID: w.callID,
			Name:   w.name,
			Stream: "stdout",
			Chunk:  string(w.buf),
		})
		w.buf = nil
	}
}
