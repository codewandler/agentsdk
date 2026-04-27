package ui

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	md "github.com/codewandler/agentsdk/markdown"
)

type State int

const (
	StateIdle State = iota
	StateReasoning
	StateText
)

// StepDisplay manages streamed output for one model/tool step.
type StepDisplay struct {
	w              io.Writer
	state          State
	buffer         *md.Buffer
	render         Renderer
	codeRenderer   nativeMarkdownRenderer
	rendered       bool
	atLineStart    bool
	fenceCandidate string
	code           *streamingCodeBlock
	inlineBuf      string
}

type streamingCodeBlock struct {
	marker   byte
	length   int
	language string
	pending  string
}

func NewStepDisplay(w io.Writer) *StepDisplay {
	return NewStepDisplayWithRenderer(w, NewMarkdownRendererForWriter(w))
}

func NewStepDisplayWithRenderer(w io.Writer, renderer Renderer) *StepDisplay {
	if renderer == nil {
		renderer = func(s string) string { return s }
	}
	d := &StepDisplay{w: w, render: renderer, codeRenderer: newNativeMarkdownRenderer(), atLineStart: true}
	d.buffer = md.NewBuffer(func(blocks []md.Block) {
		for _, block := range blocks {
			d.writeRenderedMarkdown(block.Markdown)
		}
	})
	return d
}

func (d *StepDisplay) WriteReasoning(chunk string) {
	if d.state == StateIdle {
		fmt.Fprint(d.w, Dim)
		d.state = StateReasoning
	}
	fmt.Fprint(d.w, chunk)
}

func (d *StepDisplay) WriteText(chunk string) {
	if d.state == StateReasoning {
		fmt.Fprintf(d.w, "%s\n\n", Reset)
	}
	if d.state != StateText {
		d.state = StateText
	}
	d.writeTextChunk(chunk)
}

func (d *StepDisplay) PrintToolCall(name string, args map[string]any) {
	switch d.state {
	case StateReasoning:
		fmt.Fprintf(d.w, "%s\n", Reset)
	case StateText:
		d.flushText()
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

func (d *StepDisplay) End() {
	switch d.state {
	case StateReasoning:
		fmt.Fprintf(d.w, "%s\n", Reset)
	case StateText:
		d.flushText()
		fmt.Fprint(d.w, "\n")
		d.rendered = false
	}
	d.state = StateIdle
	d.atLineStart = true
}

func (d *StepDisplay) writeRenderedMarkdown(markdown string) {
	rendered := d.render(markdown)
	if rendered == "" {
		return
	}
	if d.rendered {
		fmt.Fprint(d.w, "\n\n")
	}
	fmt.Fprint(d.w, rendered)
	d.rendered = true
}

func (d *StepDisplay) writeTextChunk(chunk string) {
	for chunk != "" {
		if d.code != nil {
			chunk = d.writeCodeChunk(chunk)
			continue
		}
		if d.fenceCandidate != "" || (d.atLineStart && maybeFenceOpeningPrefix(chunk)) {
			chunk = d.writeFenceCandidate(chunk)
			continue
		}
		if d.buffer.Pending() != "" || (d.atLineStart && shouldBufferMarkdownLineStart(chunk)) {
			_, _ = d.buffer.WriteString(chunk)
			if d.buffer.Pending() == "" {
				d.atLineStart = strings.HasSuffix(chunk, "\n")
			}
			return
		}
		n := len(chunk)
		if idx := strings.IndexByte(chunk, '\n'); idx >= 0 {
			n = idx + 1
		}
		part := chunk[:n]
		d.writeFastPathWithInline(part)
		d.atLineStart = strings.HasSuffix(part, "\n")
		chunk = chunk[n:]
	}
}

func (d *StepDisplay) writeFenceCandidate(chunk string) string {
	d.fenceCandidate += chunk
	if !couldBeFenceOpening(d.fenceCandidate) {
		fmt.Fprint(d.w, d.fenceCandidate)
		d.rendered = true
		d.atLineStart = strings.HasSuffix(d.fenceCandidate, "\n")
		d.fenceCandidate = ""
		return ""
	}
	idx := strings.IndexByte(d.fenceCandidate, '\n')
	if idx < 0 {
		return ""
	}
	line := d.fenceCandidate[:idx+1]
	rest := d.fenceCandidate[idx+1:]
	d.fenceCandidate = ""

	open, ok := parseOpeningFence(line)
	if !ok {
		fmt.Fprint(d.w, line)
		d.atLineStart = true
		return rest
	}

	_ = d.buffer.Flush()
	d.code = &streamingCodeBlock{
		marker:   open.marker,
		length:   open.length,
		language: open.language,
	}
	d.atLineStart = true
	return rest
}

func (d *StepDisplay) writeCodeChunk(chunk string) string {
	d.code.pending += chunk
	for {
		idx := strings.IndexByte(d.code.pending, '\n')
		if idx < 0 {
			return ""
		}
		line := d.code.pending[:idx+1]
		d.code.pending = d.code.pending[idx+1:]
		if isClosingFence(line, d.code.marker, d.code.length) {
			rest := d.code.pending
			d.code = nil
			d.atLineStart = true
			return rest
		}
		d.writeStreamedCode(line)
	}
}

func (d *StepDisplay) writeStreamedCode(code string) {
	rendered := d.codeRenderer.highlightCodePreserve(code, d.code.language)
	if rendered == "" {
		return
	}
	fmt.Fprint(d.w, rendered)
	d.rendered = true
	d.atLineStart = strings.HasSuffix(code, "\n")
}

func (d *StepDisplay) flushText() {
	d.flushFenceCandidate()
	if d.code != nil {
		d.flushCode()
	}
	if d.inlineBuf != "" {
		fmt.Fprint(d.w, d.inlineBuf)
		d.inlineBuf = ""
	}
	_ = d.buffer.Flush()
}

func (d *StepDisplay) flushFenceCandidate() {
	if d.fenceCandidate == "" {
		return
	}
	if open, ok := parseOpeningFence(d.fenceCandidate); ok {
		d.code = &streamingCodeBlock{
			marker:   open.marker,
			length:   open.length,
			language: open.language,
		}
		d.fenceCandidate = ""
		return
	}
	fmt.Fprint(d.w, d.fenceCandidate)
	d.rendered = true
	d.atLineStart = strings.HasSuffix(d.fenceCandidate, "\n")
	d.fenceCandidate = ""
}

func (d *StepDisplay) flushCode() {
	if d.code.pending != "" && !isClosingFence(d.code.pending, d.code.marker, d.code.length) {
		d.writeStreamedCode(d.code.pending)
	}
	d.code = nil
	d.atLineStart = true
}

func maybeFenceOpeningPrefix(s string) bool {
	if s == "" {
		return false
	}
	segment := s
	if idx := strings.IndexByte(segment, '\n'); idx >= 0 {
		segment = segment[:idx]
	}
	trimmed := strings.TrimLeft(segment, " ")
	indent := len(segment) - len(trimmed)
	if indent > 3 {
		return false
	}
	if trimmed == "" {
		return !strings.Contains(s, "\n")
	}
	return trimmed[0] == '`' || trimmed[0] == '~'
}

func couldBeFenceOpening(s string) bool {
	segment := s
	if idx := strings.IndexByte(segment, '\n'); idx >= 0 {
		segment = segment[:idx]
	}
	trimmed := strings.TrimLeft(segment, " ")
	indent := len(segment) - len(trimmed)
	if indent > 3 {
		return false
	}
	if trimmed == "" {
		return true
	}
	marker := trimmed[0]
	if marker != '`' && marker != '~' {
		return false
	}
	run := countLeadingByte(trimmed, marker)
	if run >= 3 {
		// Backtick fences cannot contain backticks in the info string.
		if marker == '`' && strings.Contains(trimmed[run:], "`") {
			return false
		}
		return true
	}
	return run == len(trimmed)
}

func shouldBufferMarkdownLineStart(s string) bool {
	if s == "" {
		return false
	}
	segment := s
	if idx := strings.IndexByte(segment, '\n'); idx >= 0 {
		segment = segment[:idx]
	}
	trimmed := strings.TrimLeft(segment, " ")
	indent := len(segment) - len(trimmed)
	if trimmed == "" {
		return indent > 0 && indent <= 3 && !strings.Contains(s, "\n")
	}
	if indent >= 4 {
		return true
	}
	switch trimmed[0] {
	case '#', '>', '|', '<', '`', '~':
		return true
	case '-', '*', '+':
		return len(trimmed) == 1 || trimmed[1] == ' ' || trimmed[1] == '\t' || trimmed[1] == trimmed[0]
	default:
		return startsOrderedList(trimmed)
	}
}

func startsOrderedList(s string) bool {
	if s == "" || !unicode.IsDigit(rune(s[0])) {
		return false
	}
	for i, r := range s {
		if !unicode.IsDigit(r) {
			return (r == '.' || r == ')') && i+1 < len(s) && (s[i+1] == ' ' || s[i+1] == '\t')
		}
	}
	return len(s) <= 9
}

// writeFastPathWithInline styles inline code spans on the fast path.
// It tracks backtick runs and emits ANSI-colored spans for matched pairs.
// Unmatched backticks at line/stream end are buffered and carried forward.
func (d *StepDisplay) writeFastPathWithInline(s string) {
	d.inlineBuf += s
	d.inlineBuf = flushInlineStyles(d.inlineBuf, d.w)
}

func flushInlineStyles(buf string, w io.Writer) string {
	keep := 0
	for keep < len(buf) {
		codeIdx := strings.IndexByte(buf[keep:], '`')
		emphIdx := -1
		for i := keep; i < len(buf); i++ {
			if buf[i] == '*' || buf[i] == '_' {
				emphIdx = i - keep
				break
			}
		}
		var start int
		var isCode bool
		if codeIdx >= 0 && (emphIdx < 0 || codeIdx < emphIdx) {
			start = keep + codeIdx
			isCode = true
		} else if emphIdx >= 0 {
			start = keep + emphIdx
			isCode = false
		} else {
			fmt.Fprint(w, buf[keep:])
			return ""
		}
		run := 1
		for start+run < len(buf) && buf[start+run] == buf[start] {
			run++
		}
		if isCode {
			closeIdx := findBacktickRun(buf[start+run:], run)
			if closeIdx < 0 {
				if start > keep {
					fmt.Fprint(w, buf[keep:start])
				}
				return buf[start:]
			}
			if start > keep {
				fmt.Fprint(w, buf[keep:start])
			}
			contentStart := start + run
			contentEnd := start + run + closeIdx
			fmt.Fprint(w, CodePink+strings.TrimSpace(buf[contentStart:contentEnd])+Reset)
			keep = contentEnd + run
		} else {
			closeIdx := findEmphasisRun(buf[start+run:], buf[start], run)
			if closeIdx < 0 {
				if start > keep {
					fmt.Fprint(w, buf[keep:start])
				}
				return buf[start:]
			}
			if start > keep {
				fmt.Fprint(w, buf[keep:start])
			}
			contentStart := start + run
			contentEnd := start + run + closeIdx
			text := strings.TrimSpace(buf[contentStart:contentEnd])
			var style string
			switch run {
			case 1:
				style = Italic + text + Reset
			case 2:
				style = Bold + text + Reset
			default:
				style = Bold + Italic + text + Reset
			}
			fmt.Fprint(w, style)
			keep = contentEnd + run
		}
	}
	return ""
}

func findBacktickRun(s string, want int) int {
	for i := 0; i <= len(s)-want; i++ {
		if s[i] != '`' {
			continue
		}
		actual := 1
		for i+actual < len(s) && s[i+actual] == '`' {
			actual++
		}
		if actual == want {
			return i
		}
		i += actual - 1
	}
	return -1
}

func findEmphasisRun(s string, marker byte, want int) int {
	for i := 0; i <= len(s)-want; i++ {
		if s[i] != marker {
			continue
		}
		actual := 1
		for i+actual < len(s) && s[i+actual] == marker {
			actual++
		}
		if actual == want {
			return i
		}
		i += actual - 1
	}
	return -1
}
