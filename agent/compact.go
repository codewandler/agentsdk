package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/agentconfig"
	"github.com/codewandler/agentsdk/conversation"
	agentruntime "github.com/codewandler/agentsdk/runtime"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/unified"
)

// ErrNothingToCompact is returned when the conversation is too short to compact.
var ErrNothingToCompact = errors.New("agent: nothing to compact")

// CompactOptions controls the compaction behavior.
type CompactOptions struct {
	KeepWindow int    // messages to preserve at the end; 0 = default (4)
	Summary    string // if non-empty, skip LLM summarization
	Trigger    CompactionTrigger
	Reason     string

	EstimatedTokens int
	ThresholdTokens int
}

// CompactionResult describes the outcome of a compaction operation.
type CompactionResult struct {
	ReplacedCount       int
	TokensBefore        int
	TokensAfter         int
	SavedTokens         int
	CompactionNodeID    conversation.NodeID
	Summary             string
	Trigger             CompactionTrigger
	Reason              string
	EstimatedTokens     int
	ThresholdTokens     int
	ContextWindow       int
	ContextWindowRatio  float64
	ContextWindowSource string
	KeepWindow          int
}

// CompactionPolicy is the effective auto-compaction policy for an agent.
type CompactionPolicy struct {
	Enabled             bool
	KeepWindow          int
	ContextWindowRatio  float64
	ContextWindow       int
	ContextWindowSource string
	ThresholdTokens     int
	Fallback            bool
}

const defaultKeepWindow = 4
const defaultAutoCompactionFallbackContextWindow = 100_000
const defaultAutoCompactionContextWindowRatio = 0.85

// AutoCompactionConfig controls automatic compaction between turns.
// The canonical definition is in [agentconfig.AutoCompactionConfig].
type AutoCompactionConfig = agentconfig.AutoCompactionConfig

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
	if a.route.client == nil {
		return CompactionResult{}, fmt.Errorf("agent: client is not initialized")
	}
	trigger := opts.Trigger
	if trigger == "" {
		trigger = CompactionTriggerManual
	}
	reason := strings.TrimSpace(opts.Reason)
	if reason == "" {
		reason = "manual"
		if trigger == CompactionTriggerAuto {
			reason = "context_window_ratio"
		}
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
	policy := a.CompactionPolicy()
	estimated := opts.EstimatedTokens
	if estimated <= 0 {
		estimated = tokensBefore
	}
	threshold := opts.ThresholdTokens
	if threshold <= 0 {
		threshold = policy.ThresholdTokens
	}

	replaceIDs, keepCount, err := selectCompactionNodes(history, keepWindow)
	if err != nil {
		return CompactionResult{}, err
	}
	if len(replaceIDs) == 0 {
		a.emitCompactionEvent(CompactionEvent{Type: CompactionEventSkipped, Trigger: trigger, Reason: reason, EstimatedTokens: estimated, ThresholdTokens: threshold, ContextWindow: policy.ContextWindow, ContextWindowRatio: policy.ContextWindowRatio, ContextWindowSource: policy.ContextWindowSource, KeepWindow: keepWindow})
		return CompactionResult{}, ErrNothingToCompact
	}

	a.emitCompactionEvent(CompactionEvent{Type: CompactionEventStarted, Trigger: trigger, Reason: reason, EstimatedTokens: estimated, ThresholdTokens: threshold, ContextWindow: policy.ContextWindow, ContextWindowRatio: policy.ContextWindowRatio, ContextWindowSource: policy.ContextWindowSource, KeepWindow: keepWindow, ReplacedCount: len(replaceIDs), TokensBefore: tokensBefore})

	summary := strings.TrimSpace(opts.Summary)
	if summary == "" {
		toSummarize := compactionSummarizeMessages(messagesBefore, keepCount)
		generated, err := a.generateCompactionSummary(ctx, trigger, reason, toSummarize)
		if err != nil {
			a.emitCompactionEvent(CompactionEvent{Type: CompactionEventFailed, Trigger: trigger, Reason: reason, Stage: "summary", Err: err, EstimatedTokens: estimated, ThresholdTokens: threshold, ContextWindow: policy.ContextWindow, ContextWindowRatio: policy.ContextWindowRatio, ContextWindowSource: policy.ContextWindowSource, KeepWindow: keepWindow})
			return CompactionResult{}, fmt.Errorf("agent: compact summary: %w", err)
		}
		summary = generated
	} else {
		a.emitCompactionEvent(CompactionEvent{Type: CompactionEventSummaryCompleted, Trigger: trigger, Reason: reason, Summary: summary, EstimatedTokens: estimated, ThresholdTokens: threshold, ContextWindow: policy.ContextWindow, ContextWindowRatio: policy.ContextWindowRatio, ContextWindowSource: policy.ContextWindowSource, KeepWindow: keepWindow})
	}

	nodeID, err := a.runtime.Compact(ctx, summary, replaceIDs...)
	if err != nil {
		a.emitCompactionEvent(CompactionEvent{Type: CompactionEventFailed, Trigger: trigger, Reason: reason, Stage: "commit", Err: err, EstimatedTokens: estimated, ThresholdTokens: threshold, ContextWindow: policy.ContextWindow, ContextWindowRatio: policy.ContextWindowRatio, ContextWindowSource: policy.ContextWindowSource, KeepWindow: keepWindow})
		return CompactionResult{}, fmt.Errorf("agent: compact: %w", err)
	}

	messagesAfter, err := history.Messages()
	tokensAfter := 0
	if err == nil {
		tokensAfter = conversation.EstimateMessagesTokens(messagesAfter, nil)
	}
	result := CompactionResult{
		ReplacedCount:       len(replaceIDs),
		TokensBefore:        tokensBefore,
		TokensAfter:         tokensAfter,
		SavedTokens:         tokensBefore - tokensAfter,
		CompactionNodeID:    nodeID,
		Summary:             summary,
		Trigger:             trigger,
		Reason:              reason,
		EstimatedTokens:     estimated,
		ThresholdTokens:     threshold,
		ContextWindow:       policy.ContextWindow,
		ContextWindowRatio:  policy.ContextWindowRatio,
		ContextWindowSource: policy.ContextWindowSource,
		KeepWindow:          keepWindow,
	}
	a.emitCompactionEvent(compactionCommittedEvent(result))
	return result, nil
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

func (a *Instance) generateCompactionSummary(ctx context.Context, trigger CompactionTrigger, reason string, messages []unified.Message) (string, error) {
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
		Stream: true,
	}
	if a.requestObserver != nil {
		a.requestObserver(ctx, req)
	}

	events, err := a.route.client.Request(ctx, req)
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
			a.emitCompactionEvent(CompactionEvent{Type: CompactionEventSummaryDelta, Trigger: trigger, Reason: reason, SummaryDelta: ev.Text})
		case unified.UsageEvent:
			a.recordCompactionUsage(ev.Usage())
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
	a.emitCompactionEvent(CompactionEvent{Type: CompactionEventSummaryCompleted, Trigger: trigger, Reason: reason, Summary: result})
	return result, nil
}

func (a *Instance) CompactionPolicy() CompactionPolicy {
	if a == nil {
		return CompactionPolicy{Enabled: false, KeepWindow: defaultKeepWindow, ContextWindowRatio: defaultAutoCompactionContextWindowRatio, ContextWindow: defaultAutoCompactionFallbackContextWindow, ContextWindowSource: "fallback", ThresholdTokens: int(float64(defaultAutoCompactionFallbackContextWindow) * defaultAutoCompactionContextWindowRatio), Fallback: true}
	}
	keepWindow := a.autoCompaction.KeepWindow
	if keepWindow <= 0 {
		keepWindow = defaultKeepWindow
	}
	ratio := a.autoCompaction.ContextWindowRatio
	if ratio <= 0 {
		ratio = defaultAutoCompactionContextWindowRatio
	}
	if ratio > 1 {
		ratio = 1
	}
	contextWindow := a.route.contextWindow
	source := "modeldb"
	fallback := false
	if contextWindow <= 0 {
		contextWindow = defaultAutoCompactionFallbackContextWindow
		source = "fallback"
		fallback = true
	}
	return CompactionPolicy{Enabled: a.autoCompaction.Enabled, KeepWindow: keepWindow, ContextWindowRatio: ratio, ContextWindow: contextWindow, ContextWindowSource: source, ThresholdTokens: int(float64(contextWindow) * ratio), Fallback: fallback}
}

func (a *Instance) autoCompactionThreshold() int { return a.CompactionPolicy().ThresholdTokens }

func (a *Instance) maybeAutoCompact(ctx context.Context) {
	policy := a.CompactionPolicy()
	if !policy.Enabled {
		return
	}
	threshold := policy.ThresholdTokens

	tokens, err := a.estimateProjectedTokens()
	if err != nil {
		return
	}
	if tokens < threshold {
		a.emitCompactionEvent(CompactionEvent{Type: CompactionEventSkipped, Trigger: CompactionTriggerAuto, Reason: "below_threshold", EstimatedTokens: tokens, ThresholdTokens: threshold, ContextWindow: policy.ContextWindow, ContextWindowRatio: policy.ContextWindowRatio, ContextWindowSource: policy.ContextWindowSource, KeepWindow: policy.KeepWindow})
		return
	}

	result, err := a.CompactWithOptions(ctx, CompactOptions{KeepWindow: policy.KeepWindow, Trigger: CompactionTriggerAuto, Reason: "context_window_ratio", EstimatedTokens: tokens, ThresholdTokens: threshold})
	if err != nil {
		if errors.Is(err, ErrNothingToCompact) {
			return
		}
		return
	}

	a.emitAutoCompactionEvent(result, tokens, threshold)
}

func (a *Instance) emitAutoCompactionEvent(result CompactionResult, estimatedTokens, threshold int) {
	payload := map[string]any{
		"trigger":               "context_window_ratio",
		"estimated_tokens":      estimatedTokens,
		"threshold":             threshold,
		"threshold_ratio":       result.ContextWindowRatio,
		"context_window":        result.ContextWindow,
		"context_window_source": result.ContextWindowSource,
		"replaced_count":        result.ReplacedCount,
		"tokens_before":         result.TokensBefore,
		"tokens_after":          result.TokensAfter,
		"saved_tokens":          result.SavedTokens,
		"compaction_node_id":    result.CompactionNodeID,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	event := thread.Event{Kind: "conversation.auto_compaction", Payload: raw}
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

func compactionCommittedEvent(result CompactionResult) CompactionEvent {
	return CompactionEvent{Type: CompactionEventCommitted, Trigger: result.Trigger, Reason: result.Reason, Summary: result.Summary, EstimatedTokens: result.EstimatedTokens, ThresholdTokens: result.ThresholdTokens, ContextWindow: result.ContextWindow, ContextWindowRatio: result.ContextWindowRatio, ContextWindowSource: result.ContextWindowSource, KeepWindow: result.KeepWindow, ReplacedCount: result.ReplacedCount, TokensBefore: result.TokensBefore, TokensAfter: result.TokensAfter, SavedTokens: result.SavedTokens, CompactionNodeID: string(result.CompactionNodeID)}
}

func (a *Instance) recordCompactionUsage(u unified.Usage) {
	if a == nil || a.tracker == nil {
		return
	}
	record := usage.FromUnified(u, usage.Dims{Provider: a.route.resolvedProvider, Model: a.route.resolvedModel, SessionID: a.sessionID, Labels: map[string]string{"operation": "compaction"}})
	record.Source = "compaction"
	a.tracker.Record(record)
	a.persistUsageEvent(record)
}
