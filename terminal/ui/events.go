package ui

import (
	"fmt"
	"io"

	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/unified"
)

// DebugCategories controls verbose output for specific event categories.
// When a category is true, detailed output is shown; otherwise a compact
// summary is rendered.
type DebugCategories map[string]bool

const (
	// DebugTools enables verbose tool output: streaming chunks, full result
	// text, and detailed tool call arguments.
	DebugTools = "tools"
	// DebugUsage enables detailed usage/cost breakdown per step.
	DebugUsage = "usage"
)

// EventDisplay renders runner events for one agent turn.
type EventDisplay struct {
	out            io.Writer
	tracker        *usage.Tracker
	turnID         string
	sessionID      string
	fallbackModel  string
	route          usage.RouteState
	debug          DebugCategories
	stepDisplay    *StepDisplay
	stepUsage      usage.Record
	stepsCompleted int
	printedCall    map[string]bool
	lastToolName   string
}

type EventDisplayOption func(*EventDisplay)

func WithTracker(t *usage.Tracker) EventDisplayOption {
	return func(d *EventDisplay) { d.tracker = t }
}

func WithTurnID(turnID string) EventDisplayOption {
	return func(d *EventDisplay) { d.turnID = turnID }
}

func WithSessionID(sessionID string) EventDisplayOption {
	return func(d *EventDisplay) { d.sessionID = sessionID }
}

func WithFallbackModel(model string) EventDisplayOption {
	return func(d *EventDisplay) { d.fallbackModel = model }
}

func WithRouteState(route usage.RouteState) EventDisplayOption {
	return func(d *EventDisplay) { d.route = route }
}

// WithDebugCategories sets which event categories show verbose output.
func WithDebugCategories(categories DebugCategories) EventDisplayOption {
	return func(d *EventDisplay) { d.debug = categories }
}

func NewEventDisplay(out io.Writer, opts ...EventDisplayOption) *EventDisplay {
	d := &EventDisplay{out: out, printedCall: map[string]bool{}}
	for _, opt := range opts {
		if opt != nil {
			opt(d)
		}
	}
	return d
}

func (d *EventDisplay) Handler() runner.EventHandler {
	return d.Handle
}

func (d *EventDisplay) StepsCompleted() int { return d.stepsCompleted }

func (d *EventDisplay) TurnUsage() usage.Record {
	if d.tracker == nil || d.turnID == "" {
		return usage.Record{}
	}
	return d.tracker.AggregateTurn(d.turnID)
}

func (d *EventDisplay) Handle(event runner.Event) {
	if d == nil || d.out == nil {
		return
	}
	switch ev := event.(type) {
	case runner.StepStartEvent:
		PrintStepHeader(d.out, ev.Step, ev.MaxSteps)
		d.stepDisplay = NewStepDisplay(d.out)
		d.stepUsage = usage.Record{}
	case runner.RouteEvent:
		d.route.Apply(ev.ProviderIdentity)
	case runner.TextDeltaEvent:
		if d.stepDisplay != nil {
			d.stepDisplay.WriteText(ev.Text)
		}
	case runner.ReasoningDeltaEvent:
		if d.stepDisplay != nil {
			d.stepDisplay.WriteReasoning(ev.Text)
		}
	case runner.ToolCallEvent:
		d.printToolCall(ev.Call)
	case runner.ToolOutputDeltaEvent:
		PrintToolOutputDelta(d.out, ev.Chunk)
	case runner.ToolStatusEvent:
		PrintToolStatus(d.out, ev.Message)
	case runner.ToolResultEvent:
		if d.debug[DebugTools] {
			PrintToolResult(d.out, ev.Output, ev.IsError)
		} else {
			PrintToolResultCompact(d.out, ev.Name, ev.IsError)
		}
	case runner.UsageEvent:
		rec := usage.FromRunnerEvent(ev, usage.RunnerEventOptions{
			TurnID:        d.turnID,
			SessionID:     d.sessionID,
			FallbackModel: d.fallbackModel,
			RouteState:    d.route,
		})
		if d.tracker != nil {
			d.tracker.Record(rec)
		}
		d.stepUsage = usage.Merge(d.stepUsage, rec)
	case runner.StepDoneEvent:
		if d.stepDisplay != nil {
			d.stepDisplay.End()
			d.stepDisplay = nil
		}
		if d.debug[DebugUsage] {
			PrintStepUsageDebug(d.out, ev.Step, d.stepUsage, ev.Model)
		} else {
			PrintStepUsage(d.out, ev.Step, d.stepUsage, ev.Model)
		}
		d.stepsCompleted++
		if ev.FinishReason == unified.FinishReasonLength {
			fmt.Fprintf(d.out, "\n%s! model hit output token limit%s\n", BrightYellow, Reset)
		}
	case runner.ThreadEvent:
		PrintThreadEvent(d.out, ev.Event)
	case runner.ErrorEvent:
		if d.stepDisplay != nil {
			d.stepDisplay.End()
			d.stepDisplay = nil
		}
	}
}

func (d *EventDisplay) printToolCall(call unified.ToolCall) {
	if d.stepDisplay == nil {
		return
	}
	key := call.ID
	if key == "" {
		key = fmt.Sprintf("%s:%d", call.Name, call.Index)
	}
	if d.printedCall[key] {
		return
	}
	args, _ := runner.ToolCallArgsMap(call)
	if len(args) == 0 {
		return
	}
	d.printedCall[key] = true
	d.lastToolName = call.Name
	if d.debug[DebugTools] {
		d.stepDisplay.PrintToolCall(call.Name, args)
	} else {
		d.stepDisplay.PrintToolCallCompact(call.Name)
	}
}
