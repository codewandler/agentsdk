package tool_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/codewandler/core/tool"
)

// ── BlocksResult.String() only renders Blocks ─────────────────────────────────

func TestBlocksResult_String_ExcludesDisplayBlocks(t *testing.T) {
	r := tool.NewResult().
		Text("LLM sees this").
		Display(tool.DiffBlock{Path: "x.go", UnifiedDiff: "diff content", Added: 1}).
		Build()

	s := r.String()
	require.Equal(t, "LLM sees this", s)
	require.NotContains(t, s, "diff content")
}

func TestBlocksResult_String_EmptyDisplayBlocks(t *testing.T) {
	r := tool.NewResult().Text("only LLM").Build()
	require.Equal(t, "only LLM", r.String())
}

// ── Display blocks round-trip through JSON ────────────────────────────────────

func TestBlocksResult_MarshalJSON_IncludesDisplayBlocks(t *testing.T) {
	r := tool.NewResult().
		Text("hello").
		Display(tool.DiffBlock{Path: "a.go", UnifiedDiff: "@@ -1 +1 @@\n-old\n+new\n", Added: 1, Removed: 1}).
		Build()

	data, err := r.(tool.BlocksResult).MarshalJSON()
	require.NoError(t, err)

	var raw map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &raw))
	require.Contains(t, string(raw["display_blocks"]), "diff")
}

func TestBlocksResult_Decode_PreservesDisplayBlocks(t *testing.T) {
	original := tool.NewResult().
		Text("summary for LLM").
		Display(tool.DiffBlock{Path: "foo.go", UnifiedDiff: "some diff", Added: 3, Removed: 2}).
		Display(tool.FileBlock{Path: "bar.go", Content: "hello", StartLine: 1, EndLine: 5, TotalLines: 10}).
		Build()

	data, err := original.(tool.BlocksResult).MarshalJSON()
	require.NoError(t, err)

	decoded, err := tool.Decode(data)
	require.NoError(t, err)

	br, ok := decoded.(tool.BlocksResult)
	require.True(t, ok)
	require.Len(t, br.Blocks, 1)
	require.Len(t, br.DisplayBlocks, 2)

	// Verify LLM text unchanged
	require.Equal(t, "summary for LLM", decoded.String())

	// Verify DiffBlock fields
	diff, ok := br.DisplayBlocks[0].(tool.DiffBlock)
	require.True(t, ok)
	require.Equal(t, "foo.go", diff.Path)
	require.Equal(t, "some diff", diff.UnifiedDiff)
	require.Equal(t, 3, diff.Added)
	require.Equal(t, 2, diff.Removed)

	// Verify FileBlock fields
	fb, ok := br.DisplayBlocks[1].(tool.FileBlock)
	require.True(t, ok)
	require.Equal(t, "bar.go", fb.Path)
	require.Equal(t, "hello", fb.Content)
	require.Equal(t, 1, fb.StartLine)
	require.Equal(t, 10, fb.TotalLines)
}

// ── DiffBlock ────────────────────────────────────────────────────────────────

func TestDiffBlock_BlockType(t *testing.T) {
	require.Equal(t, "diff", tool.DiffBlock{}.BlockType())
}

func TestDiffBlock_String_Fallback(t *testing.T) {
	b := tool.DiffBlock{Path: "x.go", Added: 2, Removed: 1}
	require.Equal(t, "[diff: x.go (+2/-1)]", b.String())
}

func TestDiffBlock_RoundTrip(t *testing.T) {
	b := tool.DiffBlock{Path: "a/b.go", UnifiedDiff: "@@ diff @@", Added: 5, Removed: 3}
	data, err := b.MarshalJSON()
	require.NoError(t, err)

	// decodeBlock is not exported, but we can round-trip through a full BlocksResult
	result := tool.NewResult().Display(b).Build()
	raw, err := result.(tool.BlocksResult).MarshalJSON()
	require.NoError(t, err)

	decoded, err := tool.Decode(raw)
	require.NoError(t, err)
	br := decoded.(tool.BlocksResult)
	got := br.DisplayBlocks[0].(tool.DiffBlock)
	require.Equal(t, b, got)
	_ = data
}

// ── CommandBlock ─────────────────────────────────────────────────────────────

func TestCommandBlock_BlockType(t *testing.T) {
	require.Equal(t, "command", tool.CommandBlock{}.BlockType())
}

func TestCommandBlock_String_IncludesExitAndStreams(t *testing.T) {
	b := tool.CommandBlock{
		Command:  "go test ./...",
		ExitCode: 1,
		Duration: 500 * time.Millisecond,
		Stdout:   "ok pkg",
		Stderr:   "FAIL",
	}
	s := b.String()
	require.Contains(t, s, "[exit: 1]")
	require.Contains(t, s, "=== STDOUT ===")
	require.Contains(t, s, "ok pkg")
	require.Contains(t, s, "=== STDERR ===")
	require.Contains(t, s, "FAIL")
}

func TestCommandBlock_RoundTrip(t *testing.T) {
	b := tool.CommandBlock{
		Command:  "ls -la",
		Workdir:  "/tmp",
		Stdout:   "file1\nfile2",
		ExitCode: 0,
		Duration: 100 * time.Millisecond,
	}
	result := tool.NewResult().Display(b).Build()
	raw, err := result.(tool.BlocksResult).MarshalJSON()
	require.NoError(t, err)

	decoded, err := tool.Decode(raw)
	require.NoError(t, err)
	got := decoded.(tool.BlocksResult).DisplayBlocks[0].(tool.CommandBlock)
	require.Equal(t, b.Command, got.Command)
	require.Equal(t, b.Workdir, got.Workdir)
	require.Equal(t, b.Stdout, got.Stdout)
	require.Equal(t, b.ExitCode, got.ExitCode)
	require.Equal(t, b.Duration, got.Duration)
}

// ── FileBlock ────────────────────────────────────────────────────────────────

func TestFileBlock_BlockType(t *testing.T) {
	require.Equal(t, "file", tool.FileBlock{}.BlockType())
}

func TestFileBlock_String_Fallback(t *testing.T) {
	b := tool.FileBlock{Path: "main.go", StartLine: 1, EndLine: 50, TotalLines: 200}
	require.Equal(t, "[file: main.go, lines 1-50 of 200]", b.String())
}

func TestFileBlock_RoundTrip(t *testing.T) {
	b := tool.FileBlock{
		Path:       "/a/b.go",
		Content:    "package main\n",
		StartLine:  1,
		EndLine:    10,
		TotalLines: 100,
		Truncated:  true,
	}
	result := tool.NewResult().Display(b).Build()
	raw, err := result.(tool.BlocksResult).MarshalJSON()
	require.NoError(t, err)

	decoded, err := tool.Decode(raw)
	require.NoError(t, err)
	got := decoded.(tool.BlocksResult).DisplayBlocks[0].(tool.FileBlock)
	require.Equal(t, b, got)
}

// ── IsError propagation ───────────────────────────────────────────────────────

func TestBlocksResult_IsError_WithDisplay(t *testing.T) {
	r := tool.NewResult().WithError().Text("fail").Display(tool.DiffBlock{Path: "x"}).Build()
	require.True(t, r.IsError())
}

// ── SectionBlock decode ──────────────────────────────────────────────────────

func TestSectionBlock_RoundTrip(t *testing.T) {
	inner := tool.NewResult().
		Text("inner text").
		Display(tool.DiffBlock{Path: "x.go", UnifiedDiff: "diff", Added: 1}).
		Build()

	sb := tool.SectionBlock{Title: "Changes", Content: inner.(tool.BlocksResult)}
	result := tool.NewResult().Section(sb.Title, sb.Content).Build()

	data, err := result.(tool.BlocksResult).MarshalJSON()
	require.NoError(t, err)

	decoded, err := tool.Decode(data)
	require.NoError(t, err)

	br := decoded.(tool.BlocksResult)
	require.Len(t, br.Blocks, 1)
	got, ok := br.Blocks[0].(tool.SectionBlock)
	require.True(t, ok)
	require.Equal(t, "Changes", got.Title)
	require.Len(t, got.Content.Blocks, 1)
	require.Len(t, got.Content.DisplayBlocks, 1)

	// Verify nested DisplayBlock survived.
	diff, ok := got.Content.DisplayBlocks[0].(tool.DiffBlock)
	require.True(t, ok)
	require.Equal(t, "x.go", diff.Path)
}
