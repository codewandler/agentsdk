// Package tool — result types (this file: Result interface + concrete implementations).
package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Result is the value returned by Tool.Execute.
//
// String() is called by the loop to produce the text the LLM sees in the
// tool result message. It must be deterministic and human-readable.
//
// MarshalJSON enables persistence in JSONL session files so tool results
// survive process restarts and session resumption.
//
// IsError() signals whether this result represents a tool-level failure.
// The loop forwards IsError to the LLM as the tool_result "is_error" flag,
// letting the model know the tool encountered a problem.
type Result interface {
	fmt.Stringer   // → what the LLM sees
	json.Marshaler // → how it's persisted in JSONL

	// IsError returns true when this result represents a tool-level failure.
	IsError() bool
}

// ── Text Result ──────────────────────────────────────────────────────────────

// TextResult is the simplest Result: a plain string.
// Use Text() or Error() to construct one.
type TextResult struct {
	content string
	isError bool
}

func (r TextResult) String() string { return r.content }
func (r TextResult) IsError() bool  { return r.isError }
func (r TextResult) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    string `json:"type"`
		Content string `json:"content"`
		IsError bool   `json:"is_error,omitempty"`
	}{Type: "text", Content: r.content, IsError: r.isError})
}

// Text returns a successful TextResult.
func Text(s string) Result { return TextResult{content: s} }

// Textf returns a successful TextResult using fmt.Sprintf formatting.
func Textf(format string, args ...any) Result { return Text(fmt.Sprintf(format, args...)) }

// Error returns an error TextResult.
func Error(s string) Result { return TextResult{content: s, isError: true} }

// Errorf returns an error TextResult using fmt.Sprintf formatting.
func Errorf(format string, args ...any) Result { return Error(fmt.Sprintf(format, args...)) }

// ── Block types ──────────────────────────────────────────────────────────────

// Block is a typed content unit within a BlocksResult.
// String() renders it as LLM-readable plain text.
// MarshalJSON persists it with a type tag for round-trip decoding.
type Block interface {
	BlockType() string
	fmt.Stringer
	json.Marshaler
}

// TextBlock is a plain text paragraph.
type TextBlock struct{ Content string }

func (b TextBlock) BlockType() string { return "text" }
func (b TextBlock) String() string    { return b.Content }
func (b TextBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}{Type: "text", Content: b.Content})
}

// CodeBlock is a language-tagged code snippet.
type CodeBlock struct{ Lang, Content string }

func (b CodeBlock) BlockType() string { return "code" }
func (b CodeBlock) String() string {
	if b.Lang != "" {
		return "```" + b.Lang + "\n" + b.Content + "\n```"
	}
	return "```\n" + b.Content + "\n```"
}
func (b CodeBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    string `json:"type"`
		Lang    string `json:"lang,omitempty"`
		Content string `json:"content"`
	}{Type: "code", Lang: b.Lang, Content: b.Content})
}

// TableBlock is a tabular data block.
type TableBlock struct {
	Headers []string
	Rows    [][]string
}

func (b TableBlock) BlockType() string { return "table" }
func (b TableBlock) String() string {
	var sb strings.Builder
	sb.WriteString("| " + strings.Join(b.Headers, " | ") + " |\n")
	sep := make([]string, len(b.Headers))
	for i := range sep {
		sep[i] = "---"
	}
	sb.WriteString("| " + strings.Join(sep, " | ") + " |\n")
	for _, row := range b.Rows {
		sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
	}
	return sb.String()
}
func (b TableBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    string     `json:"type"`
		Headers []string   `json:"headers"`
		Rows    [][]string `json:"rows"`
	}{Type: "table", Headers: b.Headers, Rows: b.Rows})
}

// ListBlock is a bulleted list.
type ListBlock struct{ Items []string }

func (b ListBlock) BlockType() string { return "list" }
func (b ListBlock) String() string {
	lines := make([]string, len(b.Items))
	for i, item := range b.Items {
		lines[i] = "- " + item
	}
	return strings.Join(lines, "\n")
}
func (b ListBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type  string   `json:"type"`
		Items []string `json:"items"`
	}{Type: "list", Items: b.Items})
}

// SectionBlock is a titled section with nested content.
type SectionBlock struct {
	Title   string
	Content BlocksResult
}

func (b SectionBlock) BlockType() string { return "section" }
func (b SectionBlock) String() string {
	return "## " + b.Title + "\n" + b.Content.String()
}
func (b SectionBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type    string       `json:"type"`
		Title   string       `json:"title"`
		Content BlocksResult `json:"content"`
	}{Type: "section", Title: b.Title, Content: b.Content})
}

// AttrsBlock is a key-value attribute set.
type AttrsBlock struct{ Attrs map[string]string }

func (b AttrsBlock) BlockType() string { return "attrs" }
func (b AttrsBlock) String() string {
	var lines []string
	for k, v := range b.Attrs {
		lines = append(lines, k+": "+v)
	}
	return strings.Join(lines, "\n")
}
func (b AttrsBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type  string            `json:"type"`
		Attrs map[string]string `json:"attrs"`
	}{Type: "attrs", Attrs: b.Attrs})
}

// ImageBlock is an image with optional alt text. String() gives a text fallback.
type ImageBlock struct {
	Alt       string
	Data      []byte
	MediaType string
}

func (b ImageBlock) BlockType() string { return "image" }
func (b ImageBlock) String() string {
	if b.Alt != "" {
		return "[image: " + b.Alt + "]"
	}
	return "[image]"
}
func (b ImageBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type      string `json:"type"`
		Alt       string `json:"alt,omitempty"`
		MediaType string `json:"media_type,omitempty"`
		// Data is intentionally omitted from JSON persistence (use file references instead)
	}{Type: "image", Alt: b.Alt, MediaType: b.MediaType})
}

// ── Display-only block types ──────────────────────────────────────────────────
//
// These block types live in BlocksResult.DisplayBlocks.
// They are never included in String() (the LLM never sees them), but they are
// serialized in MarshalJSON so UIs can decode them for rich rendering.

// DiffBlock represents a unified diff between two versions of a file.
// Tools that edit files should populate UnifiedDiff, Added, and Removed so
// the TUI can render a colored diff without parsing text.
type DiffBlock struct {
	Path        string `json:"path"`
	UnifiedDiff string `json:"unified_diff"`
	Added       int    `json:"added"`
	Removed     int    `json:"removed"`
}

func (b DiffBlock) BlockType() string { return "diff" }
func (b DiffBlock) String() string {
	return fmt.Sprintf("[diff: %s (+%d/-%d)]", b.Path, b.Added, b.Removed)
}
func (b DiffBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type        string `json:"type"`
		Path        string `json:"path"`
		UnifiedDiff string `json:"unified_diff"`
		Added       int    `json:"added"`
		Removed     int    `json:"removed"`
	}{Type: "diff", Path: b.Path, UnifiedDiff: b.UnifiedDiff, Added: b.Added, Removed: b.Removed})
}

// CommandBlock represents the result of executing a shell command.
// Tools that run shell commands (e.g. bash) populate this for TUI display.
type CommandBlock struct {
	Command  string        `json:"command"`
	Workdir  string        `json:"workdir,omitempty"`
	Stdout   string        `json:"stdout,omitempty"`
	Stderr   string        `json:"stderr,omitempty"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	TimedOut bool          `json:"timed_out,omitempty"`
}

func (b CommandBlock) BlockType() string { return "command" }
func (b CommandBlock) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "[exit: %d] [duration: %.1fs]", b.ExitCode, b.Duration.Seconds())
	if b.TimedOut {
		sb.WriteString(" [timed out]")
	}
	if b.Workdir != "" {
		fmt.Fprintf(&sb, " [dir: %s]", b.Workdir)
	}
	if b.Stdout != "" {
		sb.WriteString("\n=== STDOUT ===\n")
		sb.WriteString(b.Stdout)
	}
	if b.Stderr != "" {
		sb.WriteString("\n=== STDERR ===\n")
		sb.WriteString(b.Stderr)
	}
	return sb.String()
}
func (b CommandBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type     string        `json:"type"`
		Command  string        `json:"command"`
		Workdir  string        `json:"workdir,omitempty"`
		Stdout   string        `json:"stdout,omitempty"`
		Stderr   string        `json:"stderr,omitempty"`
		ExitCode int           `json:"exit_code"`
		Duration time.Duration `json:"duration"`
		TimedOut bool          `json:"timed_out,omitempty"`
	}{
		Type:     "command",
		Command:  b.Command,
		Workdir:  b.Workdir,
		Stdout:   b.Stdout,
		Stderr:   b.Stderr,
		ExitCode: b.ExitCode,
		Duration: b.Duration,
		TimedOut: b.TimedOut,
	})
}

// FileBlock represents the content of a single file read.
// Tools that read files (e.g. file_read) populate this for TUI display.
// Content holds the raw file text without line-number formatting.
type FileBlock struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated,omitempty"`
}

func (b FileBlock) BlockType() string { return "file" }
func (b FileBlock) String() string {
	return fmt.Sprintf("[file: %s, lines %d-%d of %d]", b.Path, b.StartLine, b.EndLine, b.TotalLines)
}
func (b FileBlock) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Type       string `json:"type"`
		Path       string `json:"path"`
		Content    string `json:"content"`
		StartLine  int    `json:"start_line"`
		EndLine    int    `json:"end_line"`
		TotalLines int    `json:"total_lines"`
		Truncated  bool   `json:"truncated,omitempty"`
	}{
		Type:       "file",
		Path:       b.Path,
		Content:    b.Content,
		StartLine:  b.StartLine,
		EndLine:    b.EndLine,
		TotalLines: b.TotalLines,
		Truncated:  b.Truncated,
	})
}

// ── BlocksResult ─────────────────────────────────────────────────────────────

// BlocksResult is a composite Result built from typed Blocks.
//
// Blocks are LLM-visible: String() renders them as plain text for the model.
// DisplayBlocks are UI-only: excluded from String() so the LLM never sees them,
// but serialized in MarshalJSON so TUI renderers can decode and display them richly.
type BlocksResult struct {
	Blocks        []Block // LLM-visible
	DisplayBlocks []Block // UI-only; never sent to the LLM
	isError       bool
}

func (r BlocksResult) IsError() bool { return r.isError }

// String renders only Blocks as plain text for the LLM.
// DisplayBlocks are excluded.
func (r BlocksResult) String() string {
	parts := make([]string, len(r.Blocks))
	for i, b := range r.Blocks {
		parts[i] = b.String()
	}
	return strings.Join(parts, "\n\n")
}

func (r BlocksResult) MarshalJSON() ([]byte, error) {
	rawBlocks, err := marshalBlocks(r.Blocks)
	if err != nil {
		return nil, err
	}
	rawDisplay, err := marshalBlocks(r.DisplayBlocks)
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Type          string            `json:"type"`
		Blocks        []json.RawMessage `json:"blocks"`
		DisplayBlocks []json.RawMessage `json:"display_blocks,omitempty"`
		IsError       bool              `json:"is_error,omitempty"`
	}{
		Type:          "blocks",
		Blocks:        rawBlocks,
		DisplayBlocks: rawDisplay,
		IsError:       r.isError,
	})
}

func marshalBlocks(blocks []Block) ([]json.RawMessage, error) {
	if len(blocks) == 0 {
		return nil, nil
	}
	raw := make([]json.RawMessage, len(blocks))
	for i, b := range blocks {
		data, err := b.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("block[%d] marshal: %w", i, err)
		}
		raw[i] = data
	}
	return raw, nil
}

// ── Fluent Builder ────────────────────────────────────────────────────────────

// ResultBuilder builds a BlocksResult using a fluent API.
// Obtain one via NewResult(). Build() returns the final BlocksResult.
type ResultBuilder struct {
	blocks        []Block
	displayBlocks []Block
	isError       bool
}

// NewResult creates a new empty ResultBuilder.
func NewResult() *ResultBuilder { return &ResultBuilder{} }

// WithError marks this result as an error result.
func (b *ResultBuilder) WithError() *ResultBuilder { b.isError = true; return b }

// Text appends a TextBlock to Blocks (LLM-visible).
func (b *ResultBuilder) Text(s string) *ResultBuilder {
	b.blocks = append(b.blocks, TextBlock{Content: s})
	return b
}

// Textf appends a formatted TextBlock to Blocks (LLM-visible).
func (b *ResultBuilder) Textf(format string, args ...any) *ResultBuilder {
	return b.Text(fmt.Sprintf(format, args...))
}

// Code appends a CodeBlock to Blocks (LLM-visible).
func (b *ResultBuilder) Code(lang, content string) *ResultBuilder {
	b.blocks = append(b.blocks, CodeBlock{Lang: lang, Content: content})
	return b
}

// Table appends a TableBlock to Blocks (LLM-visible).
func (b *ResultBuilder) Table(headers []string, rows [][]string) *ResultBuilder {
	b.blocks = append(b.blocks, TableBlock{Headers: headers, Rows: rows})
	return b
}

// List appends a ListBlock to Blocks (LLM-visible).
func (b *ResultBuilder) List(items []string) *ResultBuilder {
	b.blocks = append(b.blocks, ListBlock{Items: items})
	return b
}

// Section appends a SectionBlock to Blocks (LLM-visible).
func (b *ResultBuilder) Section(title string, content BlocksResult) *ResultBuilder {
	b.blocks = append(b.blocks, SectionBlock{Title: title, Content: content})
	return b
}

// Attrs appends an AttrsBlock to Blocks (LLM-visible).
func (b *ResultBuilder) Attrs(attrs map[string]string) *ResultBuilder {
	b.blocks = append(b.blocks, AttrsBlock{Attrs: attrs})
	return b
}

// Image appends an ImageBlock to Blocks (LLM-visible).
func (b *ResultBuilder) Image(alt string, data []byte, mediaType string) *ResultBuilder {
	b.blocks = append(b.blocks, ImageBlock{Alt: alt, Data: data, MediaType: mediaType})
	return b
}

// Display appends a Block to DisplayBlocks (UI-only; excluded from LLM text).
func (b *ResultBuilder) Display(block Block) *ResultBuilder {
	b.displayBlocks = append(b.displayBlocks, block)
	return b
}

// Build finalises and returns the BlocksResult.
func (b *ResultBuilder) Build() Result {
	return BlocksResult{
		Blocks:        b.blocks,
		DisplayBlocks: b.displayBlocks,
		isError:       b.isError,
	}
}

// ── Decode Registry ───────────────────────────────────────────────────────────

// decoders maps JSON type tag → decoder function.
var decoders = map[string]func(json.RawMessage) (Result, error){
	"text":   decodeTextResult,
	"blocks": decodeBlocksResult,
}

// Decode reconstructs a Result from its persisted JSON form.
// Returns an error if the type tag is unknown or JSON is malformed.
func Decode(data []byte) (Result, error) {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typed); err != nil {
		return nil, fmt.Errorf("result decode: %w", err)
	}
	fn, ok := decoders[typed.Type]
	if !ok {
		return nil, fmt.Errorf("result decode: unknown type %q", typed.Type)
	}
	return fn(data)
}

func decodeTextResult(data json.RawMessage) (Result, error) {
	var v struct {
		Content string `json:"content"`
		IsError bool   `json:"is_error"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return TextResult{content: v.Content, isError: v.IsError}, nil
}

func decodeBlocksResult(data json.RawMessage) (Result, error) {
	var v struct {
		Blocks        []json.RawMessage `json:"blocks"`
		DisplayBlocks []json.RawMessage `json:"display_blocks"`
		IsError       bool              `json:"is_error"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	blocks, err := decodeBlocks(v.Blocks)
	if err != nil {
		return nil, err
	}
	displayBlocks, err := decodeBlocks(v.DisplayBlocks)
	if err != nil {
		return nil, err
	}
	return BlocksResult{
		Blocks:        blocks,
		DisplayBlocks: displayBlocks,
		isError:       v.IsError,
	}, nil
}

func decodeBlocks(raws []json.RawMessage) ([]Block, error) {
	blocks := make([]Block, 0, len(raws))
	for _, raw := range raws {
		b, err := decodeBlock(raw)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, b)
	}
	return blocks, nil
}

func decodeBlock(data json.RawMessage) (Block, error) {
	var typed struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &typed); err != nil {
		return nil, err
	}
	switch typed.Type {
	case "text":
		var v TextBlock
		return v, json.Unmarshal(data, &v)
	case "code":
		var v CodeBlock
		return v, json.Unmarshal(data, &v)
	case "table":
		var v TableBlock
		return v, json.Unmarshal(data, &v)
	case "list":
		var v ListBlock
		return v, json.Unmarshal(data, &v)
	case "attrs":
		var v AttrsBlock
		return v, json.Unmarshal(data, &v)
	case "image":
		var v ImageBlock
		return v, json.Unmarshal(data, &v)
	case "section":
		var raw struct {
			Title   string          `json:"title"`
			Content json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, err
		}
		content, err := decodeBlocksResult(raw.Content)
		if err != nil {
			return nil, fmt.Errorf("section content: %w", err)
		}
		return SectionBlock{Title: raw.Title, Content: content.(BlocksResult)}, nil
	case "diff":
		var v DiffBlock
		return v, json.Unmarshal(data, &v)
	case "command":
		var v CommandBlock
		return v, json.Unmarshal(data, &v)
	case "file":
		var v FileBlock
		return v, json.Unmarshal(data, &v)
	default:
		return nil, fmt.Errorf("unknown block type %q", typed.Type)
	}
}
