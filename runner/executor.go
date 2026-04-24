package runner

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

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
}

func newDefaultToolExecutor(tools []tool.Tool, toolCtx tool.Ctx, timeout time.Duration) *defaultToolExecutor {
	toolMap := make(map[string]tool.Tool, len(tools))
	for _, t := range tools {
		if t != nil {
			toolMap[t.Name()] = t
		}
	}
	return &defaultToolExecutor{tools: toolMap, toolCtx: toolCtx, timeout: timeout}
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

	toolCtx := withContext(e.toolCtx, execCtx)
	res, err := t.Execute(toolCtx, call.Arguments)
	if err != nil {
		return toolResultFromError(call, execCtx, err)
	}
	if err := execCtx.Err(); err != nil {
		return toolResultFromContext(call, err)
	}
	if res == nil {
		return toolResult(call, "", false)
	}
	return toolResult(call, res.String(), res.IsError())
}

func toolResultFromError(call unified.ToolCall, ctx context.Context, err error) unified.ToolResult {
	switch {
	case errors.Is(err, context.Canceled):
		return toolResult(call, canceledToolOutput, true)
	case errors.Is(err, context.DeadlineExceeded):
		return toolResult(call, timedOutToolOutput, true)
	case ctx != nil && errors.Is(ctx.Err(), context.Canceled):
		return toolResult(call, canceledToolOutput, true)
	case ctx != nil && errors.Is(ctx.Err(), context.DeadlineExceeded):
		return toolResult(call, timedOutToolOutput, true)
	default:
		return toolResult(call, err.Error(), true)
	}
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

type contextToolCtx struct {
	tool.Ctx
	context.Context
}

func (c *contextToolCtx) Deadline() (time.Time, bool) { return c.Context.Deadline() }
func (c *contextToolCtx) Done() <-chan struct{}       { return c.Context.Done() }
func (c *contextToolCtx) Err() error                  { return c.Context.Err() }
func (c *contextToolCtx) Value(key any) any           { return c.Context.Value(key) }

func withContext(base tool.Ctx, ctx context.Context) tool.Ctx {
	if base == nil {
		return &basicToolCtx{Context: ctx, extra: map[string]any{}}
	}
	return &contextToolCtx{Ctx: base, Context: ctx}
}
