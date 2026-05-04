package agent

import "fmt"

func (a *Instance) renderCompactionEvent(event CompactionEvent) {
	if a == nil || a.Out() == nil {
		return
	}
	label := "compaction"
	if event.Trigger == CompactionTriggerAuto {
		label = "auto-compaction"
	}
	switch event.Type {
	case CompactionEventStarted:
		fmt.Fprintf(a.Out(), "[%s started: reason=%s estimated=%d threshold=%d (%.0f%% of %d, source=%s); replacing %d messages]\n",
			label,
			event.Reason,
			event.EstimatedTokens,
			event.ThresholdTokens,
			event.ContextWindowRatio*100,
			event.ContextWindow,
			event.ContextWindowSource,
			event.ReplacedCount,
		)
		fmt.Fprintln(a.Out(), "Compaction summary:")
	case CompactionEventSummaryDelta:
		fmt.Fprint(a.Out(), event.SummaryDelta)
	case CompactionEventSummaryCompleted:
		fmt.Fprintln(a.Out())
	case CompactionEventCommitted:
		fmt.Fprintf(a.Out(), "[%s committed: replaced=%d before=%d after=%d saved~%d node=%s]\n",
			label,
			event.ReplacedCount,
			event.TokensBefore,
			event.TokensAfter,
			event.SavedTokens,
			event.CompactionNodeID,
		)
	case CompactionEventFailed:
		fmt.Fprintf(a.Out(), "[%s failed: stage=%s error=%v]\n", label, event.Stage, event.Err)
	}
}
