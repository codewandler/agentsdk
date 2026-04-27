package ui

import (
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/usage"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/assert"
)

func TestCompactCount(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{42, "42"},
		{999, "999"},
		{1000, "1.0k"},
		{1340, "1.3k"},
		{1500, "1.5k"},
		{9999, "10.0k"},
		{10000, "10.0k"},
		{10500, "10.5k"},
		{99999, "100.0k"},
		{100000, "100k"},
		{123456, "123k"},
		{999999, "1000k"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			assert.Equal(t, tt.want, CompactCount(tt.input))
		})
	}
}

func TestFormatCost(t *testing.T) {
	tests := []struct {
		name string
		cost float64
		want string
	}{
		{"zero", 0, ""},
		{"tiny", 0.00001, "$0.000010"},
		{"small", 0.0023, "$0.0023"},
		{"medium", 0.0412, "$0.0412"},
		{"dollar", 1.24, "$1.24"},
		{"large", 12.50, "$12.50"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, FormatCost(tt.cost))
		})
	}
}

func TestTruncate(t *testing.T) {
	assert.Equal(t, "hello", Truncate("hello", 300))
	assert.Equal(t, "", Truncate("", 0))

	long := strings.Repeat("x", 400)
	result := Truncate(long, 300)
	assert.Len(t, result, 300)
	assert.True(t, strings.HasSuffix(result, "..."))
}

func TestFormatUsageParts(t *testing.T) {
	t.Run("all fields with cache", func(t *testing.T) {
		rec := usage.Record{
			Usage: unified.Usage{
				Tokens: unified.TokenItems{
					{Kind: unified.TokenKindInputNew, Count: 1204},
					{Kind: unified.TokenKindInputCacheRead, Count: 8432},
					{Kind: unified.TokenKindOutput, Count: 87},
				},
				Costs: unified.CostItems{{Kind: unified.CostKindInput, Amount: 0.0023}},
			},
		}
		parts := FormatUsageParts(rec)
		assert.Contains(t, parts, "in: 9.6k")
		assert.Contains(t, parts, "cache_r: 8.4k 87.5%")
		assert.Contains(t, parts, "new: 1.2k")
		assert.Contains(t, parts, "out: 87")
		assert.Contains(t, parts, "cost: $0.0023")
	})

	t.Run("no cache plain input output", func(t *testing.T) {
		rec := usage.Record{Usage: unified.Usage{Tokens: unified.TokenItems{
			{Kind: unified.TokenKindInputNew, Count: 100},
			{Kind: unified.TokenKindOutput, Count: 50},
		}}}
		parts := FormatUsageParts(rec)
		assert.Contains(t, parts, "in: 100")
		assert.Contains(t, parts, "out: 50")
		assert.NotContains(t, parts, "cache")
		assert.NotContains(t, parts, "cost")
	})

	t.Run("cache read and write with non-cache input", func(t *testing.T) {
		rec := usage.Record{Usage: unified.Usage{Tokens: unified.TokenItems{
			{Kind: unified.TokenKindInputNew, Count: 200},
			{Kind: unified.TokenKindInputCacheRead, Count: 300},
			{Kind: unified.TokenKindInputCacheWrite, Count: 100},
			{Kind: unified.TokenKindOutput, Count: 50},
		}}}
		parts := FormatUsageParts(rec)
		assert.Contains(t, parts, "in: 600")
		assert.Contains(t, parts, "cache_r: 300 50.0%")
		assert.Contains(t, parts, "cache_w: 100")
		assert.Contains(t, parts, "new: 200")
	})

	t.Run("output and reasoning are displayed separately without overlap", func(t *testing.T) {
		rec := usage.Record{Usage: unified.Usage{Tokens: unified.TokenItems{
			{Kind: unified.TokenKindOutput, Count: 21},
			{Kind: unified.TokenKindOutputReasoning, Count: 9},
		}}}
		parts := FormatUsageParts(rec)
		assert.Contains(t, parts, "out: 21")
		assert.Contains(t, parts, "reason: 9")
		assert.NotContains(t, parts, "out: 30")
	})

	t.Run("empty record", func(t *testing.T) {
		assert.Equal(t, "", FormatUsageParts(usage.Record{}))
	})
}

func TestRenderedBlankLineTrimming(t *testing.T) {
	assert.True(t, IsVisuallyBlankRenderedLine(""))
	assert.True(t, IsVisuallyBlankRenderedLine("   \t"))
	assert.True(t, IsVisuallyBlankRenderedLine("\x1b[0m"))
	assert.True(t, IsVisuallyBlankRenderedLine("\x1b[38;5;252m\x1b[0m  "))
	assert.False(t, IsVisuallyBlankRenderedLine("\x1b[38;5;252mTitle\x1b[0m"))

	assert.Equal(t, "\x1b[0mTitle", TrimOuterRenderedBlankLines("\n\x1b[0mTitle\n\x1b[0m\n"))
	assert.Equal(t, "hello\nworld", TrimOuterRenderedBlankLines("\n  \nhello\nworld\n\n"))
	assert.Equal(t, "", TrimOuterRenderedBlankLines("\n \n\t\n"))
}

func TestStepDisplay(t *testing.T) {
	plain := func(s string) string { return s }

	t.Run("reasoning then text", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplayWithRenderer(&buf, plain)

		sd.WriteReasoning("thinking...")
		sd.WriteText("answer")
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "thinking...")
		assert.Contains(t, out, "answer")
		assert.Contains(t, out, Dim)
		assert.Contains(t, out, Reset)
	})

	t.Run("plain prose streams immediately", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplayWithRenderer(&buf, plain)

		sd.WriteText("hello ")
		assert.Contains(t, buf.String(), "hello ")
		sd.WriteText("world\n\n")
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "hello world")
		assert.NotContains(t, out, Dim)
	})

	t.Run("list markdown waits for stable boundary", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplayWithRenderer(&buf, plain)

		sd.WriteText("- one\n- two\n")
		assert.Empty(t, buf.String())
		sd.WriteText("\n")
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "- one")
		assert.Contains(t, out, "- two")
	})

	t.Run("rendered blocks use controlled separators", func(t *testing.T) {
		var buf strings.Builder
		renderer := func(s string) string {
			switch s {
			case "Paragraph one.\n":
				return TrimOuterRenderedBlankLines("\n\x1b[0mParagraph one.\n\n")
			case "- a\n- b\n":
				return TrimOuterRenderedBlankLines("\n\x1b[0m* a\n* b\n\n")
			default:
				return s
			}
		}
		sd := NewStepDisplayWithRenderer(&buf, renderer)
		sd.writeRenderedMarkdown("Paragraph one.\n")
		sd.writeRenderedMarkdown("- a\n- b\n")
		assert.Equal(t, "\x1b[0mParagraph one.\n\n\x1b[0m* a\n* b", buf.String())
	})

	t.Run("fenced code streams complete lines before close", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplay(&buf)

		sd.WriteText("Before\n\n```go\nfmt.Println(1)\n")
		out := buf.String()
		assert.Contains(t, out, "Before")
		assert.Contains(t, out, "fmt")
		assert.Contains(t, out, "Println")
		assert.NotContains(t, out, "```")

		sd.WriteText("```\nAfter")
		sd.End()
		out = buf.String()
		assert.Contains(t, out, "After")
		assert.NotContains(t, out, "```")
	})

	t.Run("fenced code waits for complete line", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplay(&buf)

		sd.WriteText("```go\nfmt.")
		assert.Empty(t, buf.String())

		sd.WriteText("Println(1)\n")
		out := buf.String()
		assert.Contains(t, out, "fmt")
		assert.Contains(t, out, "Println")
		assert.NotContains(t, out, "```")
	})

	t.Run("fenced code flushes partial line at end", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplay(&buf)

		sd.WriteText("```go\nfmt.Print")
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "fmt")
		assert.Contains(t, out, "Print")
		assert.NotContains(t, out, "```")
	})

	t.Run("line-start inline code does not wait for newline once it is not a fence", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplay(&buf)

		sd.WriteText("`not a fence")

		assert.Contains(t, buf.String(), "`not a fence")
	})

	t.Run("inline code on fast path is colored", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplayWithRenderer(&buf, plain)

		sd.WriteText("Use the `config.SetTimeout` method.")

		out := buf.String()
		assert.Contains(t, out, "Use the ")
		assert.Contains(t, out, CodePink+"config.SetTimeout"+Reset)
		assert.Contains(t, out, " method.")
	})

	t.Run("inline code across chunks is colored", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplayWithRenderer(&buf, plain)

		sd.WriteText("Call `foo")
		sd.WriteText("Bar` now")

		out := buf.String()
		assert.Contains(t, out, CodePink+"fooBar"+Reset)
	})

	t.Run("triple backtick inline code colored", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplayWithRenderer(&buf, plain)

		// Closing run must be exactly 3 backticks (same as opening)
		sd.WriteText("Use ```code `nested` ``` here")

		out := buf.String()
		assert.Contains(t, out, CodePink+"code `nested`"+Reset)
		assert.Contains(t, out, " here")
	})

	t.Run("inline emphasis on fast path is styled", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplayWithRenderer(&buf, plain)

		sd.WriteText("Use *italic* and **bold** and ***both***")

		out := buf.String()
		assert.Contains(t, out, Italic+"italic"+Reset)
		assert.Contains(t, out, Bold+"bold"+Reset)
		assert.Contains(t, out, Bold+Italic+"both"+Reset)
	})

	t.Run("tool call flushes pending markdown", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplayWithRenderer(&buf, plain)

		sd.WriteText("let me check")
		sd.PrintToolCall("bash", map[string]any{"command": "ls -la"})
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "let me check")
		assert.Contains(t, out, "bash")
		assert.Contains(t, out, `"command"`)
		assert.Contains(t, out, `"ls -la"`)
	})
}

func TestMarkdownRenderer(t *testing.T) {
	t.Run("standard markdown is formatted", func(t *testing.T) {
		var buf strings.Builder
		render := NewMarkdownRendererForWriter(&buf)

		out := render("# Heading\n\n- item with **bold** and `code`\n")

		assert.Contains(t, out, "Heading")
		assert.Contains(t, out, "- item with ")
		assert.Contains(t, out, "bold")
		assert.Contains(t, out, "code")
		assert.NotContains(t, out, "# Heading")
		assert.NotContains(t, out, "**bold**")
		assert.NotContains(t, out, "`code`")
		assert.Contains(t, out, "\x1b[")
	})

	t.Run("fenced code is highlighted without markdown layout rewriting", func(t *testing.T) {
		var buf strings.Builder
		render := NewMarkdownRendererForWriter(&buf)

		out := render("```go\nfmt.Println(\"hi\")\n```\n")

		assert.Contains(t, out, "fmt")
		assert.Contains(t, out, "Println")
		assert.NotContains(t, out, "```")
		assert.Contains(t, out, "\x1b[")
	})

	t.Run("tables are formatted", func(t *testing.T) {
		var buf strings.Builder
		render := NewMarkdownRendererForWriter(&buf)

		out := render("| Name | Count |\n| --- | ---: |\n| Alpha | 2 |\n| Beta | 100 |\n")
		plainOut := ansiOnlyLineRE.ReplaceAllString(out, "")

		assert.Contains(t, plainOut, "| Name  | Count |")
		assert.Contains(t, plainOut, "| Alpha |     2 |")
		assert.Contains(t, plainOut, "| Beta  |   100 |")
		assert.NotContains(t, plainOut, "---:")
		assert.Contains(t, out, "\x1b[")
	})

	t.Run("gfm task lists and strikethrough are formatted", func(t *testing.T) {
		var buf strings.Builder
		render := NewMarkdownRendererForWriter(&buf)

		out := render("- [x] done\n- [ ] todo\n\n~~removed~~ and https://example.com\n")
		plainOut := ansiOnlyLineRE.ReplaceAllString(out, "")

		assert.Contains(t, plainOut, "- [x] done")
		assert.Contains(t, plainOut, "- [ ] todo")
		assert.Contains(t, plainOut, "removed")
		assert.Contains(t, plainOut, "https://example.com")
		assert.NotContains(t, plainOut, "~~removed~~")
		assert.Contains(t, out, "\x1b[9m")
	})

	t.Run("fenced code keeps following markdown separated", func(t *testing.T) {
		var buf strings.Builder
		render := NewMarkdownRendererForWriter(&buf)

		out := render("```go\nfmt.Println(\"hi\")\n```\nAfter\n")

		assert.Contains(t, out, "fmt")
		assert.Contains(t, out, "\nAfter")
	})

	t.Run("fenced code preserves intentional blank content lines", func(t *testing.T) {
		var buf strings.Builder
		render := NewMarkdownRendererForWriter(&buf)

		out := render("```text\n\nvalue\n\n```\n")
		plainOut := ansiOnlyLineRE.ReplaceAllString(out, "")

		assert.True(t, strings.HasPrefix(plainOut, "\n"), out)
		assert.Contains(t, plainOut, "\nvalue\n")
	})

	t.Run("streamed fenced code highlights before close", func(t *testing.T) {
		var buf strings.Builder
		sd := NewStepDisplay(&buf)

		sd.WriteText("```go\nfmt.Println(\"hi\")\n")
		assert.Contains(t, buf.String(), "fmt")

		sd.WriteText("```\n")
		sd.End()

		out := buf.String()
		assert.Contains(t, out, "fmt")
		assert.NotContains(t, out, "```")
		assert.Contains(t, out, "\x1b[")
	})
}
