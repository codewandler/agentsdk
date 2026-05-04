package agent

// CompactionTrigger identifies why a compaction operation ran.
type CompactionTrigger string

const (
	CompactionTriggerManual CompactionTrigger = "manual"
	CompactionTriggerAuto   CompactionTrigger = "auto"
)

// CompactionEventType identifies compaction lifecycle events.
type CompactionEventType string

const (
	CompactionEventStarted          CompactionEventType = "compaction.started"
	CompactionEventSummaryDelta     CompactionEventType = "compaction.summary_delta"
	CompactionEventSummaryCompleted CompactionEventType = "compaction.summary_completed"
	CompactionEventCommitted        CompactionEventType = "compaction.committed"
	CompactionEventSkipped          CompactionEventType = "compaction.skipped"
	CompactionEventFailed           CompactionEventType = "compaction.failed"
)

// CompactionEvent is a presentation/lifecycle event for compaction. Summary
// deltas are intentionally transient; the final durable summary lives in the
// conversation.compaction event.
type CompactionEvent struct {
	Type                CompactionEventType
	Trigger             CompactionTrigger
	Reason              string
	Stage               string
	SummaryDelta        string
	Summary             string
	EstimatedTokens     int
	ThresholdTokens     int
	ContextWindow       int
	ContextWindowRatio  float64
	ContextWindowSource string
	KeepWindow          int
	ReplacedCount       int
	TokensBefore        int
	TokensAfter         int
	SavedTokens         int
	CompactionNodeID    string
	Err                 error
}

type CompactionEventHandler func(CompactionEvent)

func (a *Instance) AddCompactionEventHandler(handler CompactionEventHandler) func() {
	if a == nil || handler == nil {
		return func() {}
	}
	a.compactionEventsMu.Lock()
	if a.compactionEvents == nil {
		a.compactionEvents = map[int]CompactionEventHandler{}
	}
	id := a.nextCompactionEventSub
	a.nextCompactionEventSub++
	a.compactionEvents[id] = handler
	a.compactionEventsMu.Unlock()
	return func() {
		a.compactionEventsMu.Lock()
		delete(a.compactionEvents, id)
		a.compactionEventsMu.Unlock()
	}
}

func (a *Instance) emitCompactionEvent(event CompactionEvent) {
	if a == nil {
		return
	}
	a.renderCompactionEvent(event)
	a.compactionEventsMu.Lock()
	handlers := make([]CompactionEventHandler, 0, len(a.compactionEvents))
	for _, handler := range a.compactionEvents {
		handlers = append(handlers, handler)
	}
	a.compactionEventsMu.Unlock()
	for _, handler := range handlers {
		handler(event)
	}
}

func (a *Instance) initCompactionEventFields() {
	if a.compactionEvents == nil {
		a.compactionEvents = map[int]CompactionEventHandler{}
	}
}
