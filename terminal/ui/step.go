package ui

import (
	"encoding/json"
	"fmt"
	"io"
)

type State int

const (
	StateIdle State = iota
	StateReasoning
	StateText
)

// StepDisplay manages streamed output for one model/tool step.
type StepDisplay struct {
	w        io.Writer
	state    State
	sr       liveMarkdownRenderer
	rendered bool
}

func NewStepDisplay(w io.Writer) *StepDisplay {
	return &StepDisplay{
		w:  w,
		sr: newLiveMarkdownRenderer(w),
	}
}

func (d *StepDisplay) WriteReasoning(chunk string) {
	if d.state == StateIdle {
		fmt.Fprint(d.w, Dim)
		d.state = StateReasoning
	}
	_, _ = d.sr.Write([]byte(chunk))
	d.rendered = true
}

func (d *StepDisplay) WriteText(chunk string) {
	if d.state == StateReasoning {
		_ = d.sr.Flush()
		d.sr = newLiveMarkdownRenderer(d.w)
		fmt.Fprintf(d.w, "%s\n\n", Reset)
	}
	if d.state != StateText {
		d.state = StateText
	}
	_, _ = d.sr.Write([]byte(chunk))
	d.rendered = true
}

func (d *StepDisplay) PrintToolCall(name string, args map[string]any) {
	switch d.state {
	case StateReasoning:
		_ = d.sr.Flush()
		d.sr = newLiveMarkdownRenderer(d.w)
		fmt.Fprintf(d.w, "%s\n", Reset)
	case StateText:
		_ = d.sr.Flush()
		d.sr = newLiveMarkdownRenderer(d.w)
		fmt.Fprint(d.w, "\n")
	}
	d.state = StateIdle
	d.rendered = false
	fmt.Fprintf(d.w, "\n%s> tool: %s%s\n", BrightYellow, name, Reset)
	if len(args) == 0 {
		fmt.Fprintf(d.w, "  %s(no args)%s\n", Dim, Reset)
		return
	}
	data, _ := json.MarshalIndent(args, "  ", "  ")
	fmt.Fprintf(d.w, "  %s%s%s\n", Dim, data, Reset)
}

// PrintToolCallCompact prints just the tool name without arguments.
func (d *StepDisplay) PrintToolCallCompact(name string) {
	switch d.state {
	case StateReasoning:
		_ = d.sr.Flush()
		d.sr = newLiveMarkdownRenderer(d.w)
		fmt.Fprintf(d.w, "%s\n", Reset)
	case StateText:
		_ = d.sr.Flush()
		d.sr = newLiveMarkdownRenderer(d.w)
		fmt.Fprint(d.w, "\n")
	}
	d.state = StateIdle
	d.rendered = false
	fmt.Fprintf(d.w, "\n%s> %s%s ", BrightYellow, name, Reset)
}

func (d *StepDisplay) End() {
	switch d.state {
	case StateReasoning:
		_ = d.sr.Flush()
		d.sr = newLiveMarkdownRenderer(d.w)
		fmt.Fprintf(d.w, "%s\n", Reset)
		d.rendered = false
	case StateText:
		_ = d.sr.Flush()
		d.sr = newLiveMarkdownRenderer(d.w)
		fmt.Fprint(d.w, "\n")
		d.rendered = false
	}
	d.state = StateIdle
}
