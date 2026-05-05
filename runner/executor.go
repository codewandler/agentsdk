package runner

import (
	"context"
	"errors"
	"fmt"
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
