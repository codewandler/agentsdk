package trigger

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"
)

type EmitFunc func(Event)

type Source interface {
	ID() SourceID
	Start(context.Context, EmitFunc) error
}

type IntervalSource struct {
	SourceID  SourceID
	Every     time.Duration
	Immediate bool
	Now       func() time.Time
}

func (s IntervalSource) ID() SourceID { return s.SourceID }

func (s IntervalSource) EventType() string { return EventTypeInterval }

func (s IntervalSource) Start(ctx context.Context, emit EmitFunc) error {
	if s.Every <= 0 {
		return fmt.Errorf("trigger: interval source %q requires positive duration", s.SourceID)
	}
	now := s.Now
	if now == nil {
		now = time.Now
	}
	emitOne := func() {
		at := now().UTC()
		emit(Event{ID: newEventID(), Type: EventTypeInterval, SourceID: s.SourceID, At: at, Data: map[string]any{"every": s.Every.String()}})
	}
	if s.Immediate {
		emitOne()
	}
	ticker := time.NewTicker(s.Every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			emitOne()
		}
	}
}

var eventSeq uint64

func newEventID() EventID {
	return EventID(fmt.Sprintf("evt_%d", atomic.AddUint64(&eventSeq, 1)))
}
