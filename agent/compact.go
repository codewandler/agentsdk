package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/conversation"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/llmadapter/unified"
)

// ErrNothingToCompact is returned when the conversation is too short to compact.
var ErrNothingToCompact = errors.New("agent: nothing to compact")

// CompactOptions controls the compaction behavior.
type CompactOptions struct {
	KeepWindow int    // messages to preserve at the end; 0 = default (4)
	Summary    string // if non-empty, skip LLM summarization
}

// CompactionResult describes the outcome of a compaction operation.
type CompactionResult struct {
	ReplacedCount    int
	TokensBefore     int
	TokensAfter      int
	CompactionNodeID conversation.NodeID
}

const defaultKeepWindow = 4

// AutoCompactionConfig controls automatic compaction between turns.
type AutoCompactionConfig struct {
	Enabled            bool
	TokenThreshold     int     // explicit override; 0 = use model-aware default
	ContextWindowRatio float64 // fraction of model context window; 0 = default (0.8)
	KeepWindow         int     // messages to preserve; 0 = default (4)
}

const defaultAutoCompactionThreshold = 80_000
const defaultAutoCompactionContextWindowRatio = 0.8

const compactionSystemPrompt = `Summarize the following conversation concisely. Preserve:
- Key decisions and conclusions
- File paths, function names, and code identifiers mentioned
- Current task state and any pending work
- Important constraints or requirements established

Output only the summary. No preamble, no markdown headers.`

// Compact compacts the conversation history using default options.
func (a *Instance) Compact(ctx context.Context) (CompactionResult, error) {
	return a.CompactWithOptions(ctx, CompactOptions{})
}

// CompactWithOptions compacts the conversation history with the given options.
// It generates an LLM summary of older messages (unless opts.Summary is set),
// then replaces them with a single compaction node.
func (a *Instance) CompactWithOptions(ctx context.Context, opts CompactOptions) (CompactionResult, error) {
	if a == nil || a.runtime == nil {
		return CompactionResult{}, fmt.Errorf("agent: runtime is not initialized")
	}
	if a.client == nil {
		return CompactionResult{}, fmt.Errorf("agent: client is not initialized")
	}

	keepWindow := opts.KeepWindow
	if keepWindow <= 0 {
		keepWindow = defaultKeepWindow
	}

	history := a.runtime.History()
	if history == nil {
		return CompactionResult{}, fmt.Errorf("agent: history is not initialized")
	}

	messagesBefore, err := history.Messages()
	if err != nil {
		return CompactionResult{}, fmt.Errorf("agent: compact: %w", err)
	}
	tokensBefore := conversation.EstimateMessagesTokens(messagesBefore, nil)

	replaceIDs, keepCount, err := selectCompactionNodes(history, keepWindow)
	if err != nil {
		return CompactionResult{}, err
	}
	if len(replaceIDs) == 0 {
		return CompactionResult{}, ErrNothingToCompact
	}

	summary := strings.TrimSpace(opts.Summary)
	if summary == "" {
		toSummarize := compactionSummarizeMessages(messagesBefore, keepCount)
		generated, err := a.generateCompactionSummary(ctx, toSummarize)
		if err != nil {
			return CompactionResult{}, fmt.Errorf("agent: compact summary: %w", err)
		}
		summary = generated
	}

	nodeID, err := a.runtime.Compact(ctx, summary, replaceIDs...)
	if err != nil {
		return CompactionResult{}, fmt.Errorf("agent: compact: %w", err)
	}

	messagesAfter, err := history.Messages()
	if err != nil {
		return CompactionResult{
			ReplacedCount:    len(replaceIDs),
			TokensBefore:     tokensBefore,
			CompactionNodeID: nodeID,
		}, nil
	}
	tokensAfter := conversation.EstimateMessagesTokens(messagesAfter, nil)

	return CompactionResult{
		ReplacedCount:    len(replaceIDs),
		TokensBefore:     tokensBefore,
		TokensAfter:      tokensAfter,
		CompactionNodeID: nodeID,
	}, nil
}

func selectCompactionNodes(history *agentruntime.History, keepWindow int) ([]conversation.NodeID, int, error) {
	tree := history.Tree()
	if tree == nil {
		return nil, 0, fmt.Errorf("agent: conversation tree is nil")
	}
	path, err := tree.Path(history.Branch())
	if err != nil {
		return nil, 0, fmt.Errorf("agent: compact: %w", err)
	}
	if len(path) <= keepWindow {
		return nil, 0, nil
	}

	cutoff := len(path) - keepWindow
	var replaceIDs []conversation.NodeID
	for _, node := range path[:cutoff] {
		switch node.Payload.(type) {
		case conversation.CompactionEvent, *conversation.CompactionEvent:
			continue
		}
		replaceIDs = append(replaceIDs, node.ID)
	}
	return replaceIDs, keepWindow, nil
}

func compactionSummarizeMessages(all []unified.Message, keepCount int) []unified.Message {
	if keepCount >= len(all) {
		return nil
	}
	return all[:len(all)-keepCount]
}

func (a *Instance) generateCompactionSummary(ctx context.Context, messages []unified.Message) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages to summarize")
	}

	var transcript strings.Builder
	for _, msg := range messages {
		fmt.Fprintf(&transcript, "[%s]", msg.Role)
		if msg.Name != "" {
			fmt.Fprintf(&transcript, " (%s)", msg.Name)
		}
		transcript.WriteString(": ")
		for _, part := range msg.Content {
			switch p := part.(type) {
			case unified.TextPart:
				transcript.WriteString(p.Text)
			case *unified.TextPart:
				if p != nil {
					transcript.WriteString(p.Text)
				}
			}
		}
		transcript.WriteByte('\n')
	}

	maxTokens := 1024
	req := unified.Request{
		Model:           a.inference.Model,
		MaxOutputTokens: &maxTokens,
		Instructions: []unified.Instruction{{
			Kind:    unified.InstructionSystem,
			Content: []unified.ContentPart{unified.TextPart{Text: compactionSystemPrompt}},
		}},
		Messages: []unified.Message{{
			Role:    unified.RoleUser,
			Content: []unified.ContentPart{unified.TextPart{Text: transcript.String()}},
		}},
		Stream: false,
	}

	events, err := a.client.Request(ctx, req)
	if err != nil {
		return "", err
	}

	var text strings.Builder
	for event := range events {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		switch ev := event.(type) {
		case unified.TextDeltaEvent:
			text.WriteString(ev.Text)
		case unified.ErrorEvent:
			if ev.Err != nil {
				return "", ev.Err
			}
		}
	}

	result := strings.TrimSpace(text.String())
	if result == "" {
		return "", fmt.Errorf("empty summary from model")
	}
	return result, nil
}

func (a *Instance) autoCompactionThreshold() int {
	if a.autoCompaction.TokenThreshold > 0 {
		return a.autoCompaction.TokenThreshold
	}
	if a.contextWindow > 0 {
		ratio := a.autoCompaction.ContextWindowRatio
		if ratio <= 0 {
			ratio = defaultAutoCompactionContextWindowRatio
		}
		if ratio > 1 {
			ratio = 1
		}
		return int(float64(a.contextWindow) * ratio)
	}
	return defaultAutoCompactionThreshold
}

func (a *Instance) maybeAutoCompact(ctx context.Context) {
	if !a.autoCompaction.Enabled {
		return
	}
	threshold := a.autoCompactionThreshold()

	tokens, err := a.estimateProjectedTokens()
	if err != nil {
		return
	}
	if tokens < threshold {
		return
	}

	keepWindow := a.autoCompaction.KeepWindow
	if keepWindow <= 0 {
		keepWindow = defaultKeepWindow
	}

	result, err := a.CompactWithOptions(ctx, CompactOptions{KeepWindow: keepWindow})
	if err != nil {
		if a.verbose {
			fmt.Fprintf(a.Out(), "[auto-compact failed: %v]\n", err)
		}
		return
	}

	saved := result.TokensBefore - result.TokensAfter
	fmt.Fprintf(a.Out(), "[auto-compacted: replaced %d messages, ~%d tokens saved]\n",
		result.ReplacedCount, saved)

	a.emitAutoCompactionEvent(result, tokens, threshold)
}

func (a *Instance) emitAutoCompactionEvent(result CompactionResult, estimatedTokens, threshold int) {
	payload := map[string]any{
		"trigger":          "token_threshold",
		"estimated_tokens": estimatedTokens,
		"threshold":        threshold,
		"context_window":   a.contextWindow,
		"replaced_count":   result.ReplacedCount,
		"tokens_before":    result.TokensBefore,
		"tokens_after":     result.TokensAfter,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	event := thread.Event{
		Kind:    "conversation.auto_compaction",
		Payload: raw,
	}
	if a.threadRuntime != nil && a.threadRuntime.Live() != nil {
		_ = a.threadRuntime.Live().Append(context.Background(), event)
	} else if a.history != nil {
		_ = a.history.AppendThreadEvents(context.Background(), event)
	}
}

func (a *Instance) estimateProjectedTokens() (int, error) {
	if a == nil || a.runtime == nil {
		return 0, fmt.Errorf("agent: runtime is not initialized")
	}
	history := a.runtime.History()
	if history == nil {
		return 0, fmt.Errorf("agent: history is not initialized")
	}
	messages, err := history.Messages()
	if err != nil {
		return 0, err
	}
	return conversation.EstimateMessagesTokens(messages, nil), nil
}
