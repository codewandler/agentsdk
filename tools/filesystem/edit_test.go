package filesystem

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codewandler/agentsdk/tool"
)

// ── Unit tests: original-based merge engine ───────────────────────────────────

func TestResolveOperationsAgainstOriginal_MultipleInsertsUseOriginalLineNumbers(t *testing.T) {
	edits, errs := resolveOperationsAgainstOriginal("1\n2\n3\n4\n", []Operation{
		{Insert: &InsertOp{Line: 2, Content: "A\n"}},
		{Insert: &InsertOp{Line: 4, Content: "B\n"}},
	})
	require.Empty(t, errs)
	require.Len(t, edits, 2)
	require.Equal(t, byteOffsetForLine("1\n2\n3\n4\n", 2), edits[0].startByte)
	require.Equal(t, byteOffsetForLine("1\n2\n3\n4\n", 4), edits[1].startByte)
}

func TestDetectConflicts_ReplaceOverlapsRemove(t *testing.T) {
	edits := []resolvedEdit{
		{opIndex: 0, kind: "replace", startByte: 2, endByte: 6, startLine: 1, endLine: 1, newText: "ABCD"},
		{opIndex: 1, kind: "remove", startByte: 4, endByte: 8, startLine: 1, endLine: 1},
	}
	err := detectConflicts(edits)
	require.Error(t, err)
	require.Contains(t, err.Error(), "conflicts")
}

func TestDetectConflicts_InsertInsideRemovedRange(t *testing.T) {
	edits := []resolvedEdit{
		{opIndex: 0, kind: "remove", startByte: 2, endByte: 8, startLine: 1, endLine: 2},
		{opIndex: 1, kind: "insert", startByte: 4, endByte: 4, startLine: 1, endLine: 1, newText: "X"},
	}
	err := detectConflicts(edits)
	require.Error(t, err)
	require.Contains(t, err.Error(), "insert target falls inside")
}

func TestApplyResolvedEdits_SamePositionInsertsPreserveOrder(t *testing.T) {
	content := "one\ntwo\n"
	edits := []resolvedEdit{
		{opIndex: 0, kind: "insert", startByte: 4, endByte: 4, newText: "A\n"},
		{opIndex: 1, kind: "insert", startByte: 4, endByte: 4, newText: "B\n"},
	}
	got, err := applyResolvedEdits(content, edits)
	require.NoError(t, err)
	require.Equal(t, "one\nA\nB\ntwo\n", got)
}

func TestApplyResolvedEdits_MultipleAppendsPreserveOrder(t *testing.T) {
	content := "one"
	edits := append(resolveAppend(content, &AppendOp{Content: "A"}, 0), resolveAppend(content, &AppendOp{Content: "B"}, 1)...)
	got, err := applyResolvedEdits(content, edits)
	require.NoError(t, err)
	require.Equal(t, "one\nA\nB", got)
}

func TestResolveOperationsAgainstOriginal_ReplaceAllProducesMultipleHunks(t *testing.T) {
	edits, errs := resolveOperationsAgainstOriginal("foo bar foo", []Operation{
		{Replace: &ReplaceOp{OldString: "foo", NewString: "baz", ReplaceAll: true}},
	})
	require.Empty(t, errs)
	require.Len(t, edits, 2)
}

func TestResolveOperationsAgainstOriginal_IfMissingBecomesEOFInsert(t *testing.T) {
	edits, errs := resolveOperationsAgainstOriginal("hello", []Operation{
		{Replace: &ReplaceOp{OldString: "missing", NewString: "x", IfMissing: " + found"}},
	})
	require.Empty(t, errs)
	require.Len(t, edits, 1)
	require.Equal(t, len("hello"), edits[0].startByte)
	require.Equal(t, " + found", edits[0].newText)
}

func TestResolveOperationsAgainstOriginal_RemoveOldString_MultipleMatchesRemovesFirstOnly(t *testing.T) {
	edits, errs := resolveOperationsAgainstOriginal("foo bar foo", []Operation{
		{Remove: &RemoveOp{RemoveByString: &RemoveByString{OldString: "foo"}}},
	})
	require.Empty(t, errs)
	require.Len(t, edits, 1)
	require.Equal(t, 0, edits[0].startByte)
	require.Equal(t, 3, edits[0].endByte)
}

func TestDetectConflicts_InsertAtReplaceStartBoundary_Allowed(t *testing.T) {
	edits := []resolvedEdit{
		{opIndex: 0, kind: "replace", startByte: 4, endByte: 7, startLine: 2, endLine: 2, newText: "XYZ"},
		{opIndex: 1, kind: "insert", startByte: 4, endByte: 4, startLine: 2, endLine: 2, newText: "A\n"},
	}
	require.NoError(t, detectConflicts(edits))
}

func TestDetectConflicts_InsertAtReplaceEndBoundary_Allowed(t *testing.T) {
	edits := []resolvedEdit{
		{opIndex: 0, kind: "replace", startByte: 4, endByte: 7, startLine: 2, endLine: 2, newText: "XYZ"},
		{opIndex: 1, kind: "insert", startByte: 7, endByte: 7, startLine: 2, endLine: 2, newText: "A\n"},
	}
	require.NoError(t, detectConflicts(edits))
}

func TestResolveOperationsAgainstOriginal_InsertPastEnd_ResolvesToEOFBoundary(t *testing.T) {
	content := "line1"
	edits, errs := resolveOperationsAgainstOriginal(content, []Operation{{Insert: &InsertOp{Line: 99, Content: "line2"}}})
	require.Empty(t, errs)
	require.Len(t, edits, 1)
	require.Equal(t, len(content), edits[0].startByte)
}

func TestResolveOperationsAgainstOriginal_InsertAutoIndent_UsesOriginalTargetIndent(t *testing.T) {
	content := "	foo\n	bar"
	edits, errs := resolveOperationsAgainstOriginal(content, []Operation{{Insert: &InsertOp{Line: 2, Content: "inserted\n", Indent: "auto"}}})
	require.Empty(t, errs)
	require.Len(t, edits, 1)
	require.Equal(t, "\tinserted\n", edits[0].newText)
}

func TestResolveOperationsAgainstOriginal_InsertNone_PreservesExactContent(t *testing.T) {
	content := "	foo\n	bar"
	edits, errs := resolveOperationsAgainstOriginal(content, []Operation{{Insert: &InsertOp{Line: 2, Content: "inserted", Indent: "none"}}})
	require.Empty(t, errs)
	require.Len(t, edits, 1)
	require.Equal(t, "inserted", edits[0].newText)
}

func TestResolveOperationsAgainstOriginal_PatchProducesResolvedEdits(t *testing.T) {
	edits, errs := resolveOperationsAgainstOriginal("line1\nline2\nline3\n", []Operation{{Patch: &PatchOp{Patch: "@@ -1,3 +1,3 @@\n line1\n-line2\n+line2-mod\n line3\n"}}})
	require.Empty(t, errs)
	require.Len(t, edits, 1)
	require.Equal(t, "patch", edits[0].kind)
	require.Equal(t, 2, edits[0].startLine)
	require.Equal(t, 2, edits[0].endLine)
}

func TestDetectConflicts_PatchDerivedSpanOverlapsReplace(t *testing.T) {
	patchEdits, errs := resolveOperationsAgainstOriginal("line1\nline2\n", []Operation{{Patch: &PatchOp{Patch: "@@ -1,2 +1,2 @@\n line1\n-line2\n+line2-mod\n"}}})
	require.Empty(t, errs)
	require.Len(t, patchEdits, 1)
	err := detectConflicts([]resolvedEdit{
		patchEdits[0],
		{opIndex: 1, kind: "replace", startByte: 6, endByte: 11, startLine: 2, endLine: 2, newText: "X"},
	})
	require.Error(t, err)
}

// ── Basic tool tests ───────────────────────────────────────────────────────────

func TestExpandPaths(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.go")
	f2 := filepath.Join(tmp, "b.go")
	os.WriteFile(f1, []byte("a"), 0644)
	os.WriteFile(f2, []byte("b"), 0644)

	files := expandPaths(tool.StringSliceParam{f1}, tmp)
	require.Len(t, files, 1)

	files = expandPaths(tool.StringSliceParam{filepath.Join(tmp, "*.go")}, tmp)
	require.Len(t, files, 2)

	files = expandPaths(tool.StringSliceParam{f1, f1}, tmp)
	require.Len(t, files, 1)
}

// ── Integration tests ──────────────────────────────────────────────────────────

func TestFileEdit_Replace(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo bar\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "baz"}}}}
	result := fileEditCall(t, params, tmp)
	require.NotEmpty(t, result)
	require.Equal(t, "baz bar\n", readFileContent(t, f))
}

func TestFileEdit_DryRun(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, DryRun: true, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "Dry run")
	require.Equal(t, "foo\n", readFileContent(t, f))
}

func TestFileEdit_Glob(t *testing.T) {
	tmp := t.TempDir()
	f1 := filepath.Join(tmp, "a.go")
	f2 := filepath.Join(tmp, "b.go")
	os.WriteFile(f1, []byte("foo\n"), 0644)
	os.WriteFile(f2, []byte("foo\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{filepath.Join(tmp, "*.go")}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "bar\n", readFileContent(t, f1))
	require.Equal(t, "bar\n", readFileContent(t, f2))
}

func TestFileEdit_NotFound(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "missing", NewString: "x"}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "old_string not found")
}

func TestFileEdit_EmptyOps(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "operations")
}

func TestFileEdit_MultipleInserts_UseOriginalLineNumbers(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("1\n2\n3\n4\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Insert: &InsertOp{Line: 2, Content: "A\n"}}, {Insert: &InsertOp{Line: 4, Content: "B\n"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "1\nA\n2\n3\nB\n4\n", readFileContent(t, f))
}

func TestFileEdit_ConflictingReplaceAndRemove_Fails(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("alpha beta gamma\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "beta", NewString: "BETA"}}, {Remove: &RemoveOp{RemoveByString: &RemoveByString{OldString: "beta gamma"}}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "conflicts")
	require.Equal(t, "alpha beta gamma\n", readFileContent(t, f))
}

func TestFileEdit_InsertInsideRemovedRange_Fails(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("line1\nline2\nline3\nline4\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Remove: &RemoveOp{RemoveByLines: &RemoveByLines{Lines: []int{2, 3}}}}, {Insert: &InsertOp{Line: 3, Content: "inserted\n"}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "conflicts")
	require.Equal(t, "line1\nline2\nline3\nline4\n", readFileContent(t, f))
}

func TestFileEdit_InsertAtReplaceStartBoundary_Allowed(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("one\ntwo\nthree\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Insert: &InsertOp{Line: 2, Content: "A\n"}}, {Replace: &ReplaceOp{OldString: "two", NewString: "TWO"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "one\nA\nTWO\nthree\n", readFileContent(t, f))
}

func TestFileEdit_InsertAtReplaceEndBoundary_Allowed(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("one\ntwo\nthree\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "two", NewString: "TWO"}}, {Insert: &InsertOp{Line: 3, Content: "A\n"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "one\nTWO\nA\nthree\n", readFileContent(t, f))
}

func TestFileEdit_SamePositionInserts_PreserveOrder(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("one\ntwo\n"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Insert: &InsertOp{Line: 2, Content: "A\n"}}, {Insert: &InsertOp{Line: 2, Content: "B\n"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "one\nA\nB\ntwo\n", readFileContent(t, f))
}

func TestFileEdit_Appends_PreserveOrder(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("one"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Append: &AppendOp{Content: "A"}}, {Append: &AppendOp{Content: "B"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "one\nA\nB", readFileContent(t, f))
}

func TestFileEdit_ReplaceIfMissing_OrdersWithAppendAtEOF(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("one"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "missing", NewString: "x", IfMissing: "A"}}, {Append: &AppendOp{Content: "B"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "oneA\nB", readFileContent(t, f))
}

func TestFileEdit_InsertAutoIndent_Preserved(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("\tfoo\n\tbar"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Insert: &InsertOp{Line: 2, Content: "inserted\n", Indent: "auto"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "\tfoo\n\tinserted\n\tbar", readFileContent(t, f))
}

func TestFileEdit_InsertNone_PreservesExactContent(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.txt")
	os.WriteFile(f, []byte("\tfoo\n\tbar"), 0644)

	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Insert: &InsertOp{Line: 2, Content: "inserted", Indent: "none"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "\tfoo\ninserted\tbar", readFileContent(t, f))
}

func TestFileEdit_Remove_ByOldString(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo XXX bar\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Remove: &RemoveOp{RemoveByString: &RemoveByString{OldString: "XXX "}}}}}
	result := fileEditCall(t, params, tmp)
	require.NotEmpty(t, result)
	require.Equal(t, "foo bar\n", readFileContent(t, f))
}

func TestFileEdit_Remove_ByLines(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Remove: &RemoveOp{RemoveByLines: &RemoveByLines{Lines: []int{2}}}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "line1\nline3\n", readFileContent(t, f))
}

func TestFileEdit_Remove_InvalidLinesLength(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\nline2\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Remove: &RemoveOp{RemoveByLines: &RemoveByLines{Lines: []int{1, 2, 3}}}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "lines must contain 1 element")
	require.Equal(t, "line1\nline2\n", readFileContent(t, f))
}

func TestFileEdit_Append(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Append: &AppendOp{Content: "bar"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "foo\nbar", readFileContent(t, f))
}

func TestFileEdit_Patch_AddLine(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Patch: &PatchOp{Patch: "@@ -1 +1,2 @@\n line1\n+line2\n"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "line1\nline2\n", readFileContent(t, f))
}

func TestFileEdit_Patch_RemoveLine(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Patch: &PatchOp{Patch: "@@ -1,3 +1,2 @@\n line1\n-line2\n line3\n"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "line1\nline3\n", readFileContent(t, f))
}

func TestFileEdit_Patch_ReplaceLine(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\nline2\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Patch: &PatchOp{Patch: "@@ -1,2 +1,2 @@\n line1\n-line2\n+line2-modified\n"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "line1\nline2-modified\n", readFileContent(t, f))
}

func TestFileEdit_Patch_InvalidPatch_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Patch: &PatchOp{Patch: ""}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "patch required")
}

func TestFileEdit_PatchAndReplace_NonOverlappingMerge(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Patch: &PatchOp{Patch: "@@ -1 +1,2 @@\n line1\n+line1_5\n"}}, {Replace: &ReplaceOp{OldString: "line3", NewString: "line3-mod"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "line1\nline1_5\nline2\nline3-mod\n", readFileContent(t, f))
}

func TestFileEdit_PatchAndReplace_OverlappingConflict(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\nline2\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Patch: &PatchOp{Patch: "@@ -1,2 +1,2 @@\n line1\n-line2\n+line2-modified\n"}}, {Replace: &ReplaceOp{OldString: "line2", NewString: "X"}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "conflicts")
	require.Equal(t, "line1\nline2\n", readFileContent(t, f))
}

func TestFileEdit_PatchAndReplace_ContextLine_NonOverlappingMerge(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\nline2\nline3\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Patch: &PatchOp{Patch: "@@ -1,3 +1,3 @@\n line1\n-line2\n+line2-modified\n line3\n"}}, {Replace: &ReplaceOp{OldString: "line1", NewString: "LINE1"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "LINE1\nline2-modified\nline3\n", readFileContent(t, f))
}

func TestFileEdit_PatchNormalization_PreservesTrailingNewline(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("line1\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Patch: &PatchOp{Patch: "@@ -1 +1,2 @@\n line1\n+line2\n"}}}}
	fileEditCall(t, params, tmp)
	require.True(t, strings.HasSuffix(readFileContent(t, f), "\n"))
}

func TestFileEdit_DiffInDisplayBlocks(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	tl := fileEdit()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := tl.Execute(ctx(tmp), raw)
	require.NoError(t, err)
	require.NotContains(t, res.String(), "---")
	require.NotContains(t, res.String(), "+++")
	require.Contains(t, res.String(), "edited")
	br, ok := res.(tool.BlocksResult)
	require.True(t, ok)
	require.NotEmpty(t, br.DisplayBlocks)
	diff, ok := br.DisplayBlocks[0].(tool.DiffBlock)
	require.True(t, ok)
	require.Equal(t, f, diff.Path)
	require.Contains(t, diff.UnifiedDiff, "-foo")
	require.Contains(t, diff.UnifiedDiff, "+bar")
}

func TestFileEdit_NoChangeNoDisplayBlock(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "foo"}}}}
	tl := fileEdit()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := tl.Execute(ctx(tmp), raw)
	require.NoError(t, err)
	br, ok := res.(tool.BlocksResult)
	require.True(t, ok)
	require.Empty(t, br.DisplayBlocks)
}

func TestFileEdit_AllowPartial_Success(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo bar\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, AllowPartial: true, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "baz"}}}}
	fileEditCall(t, params, tmp)
	require.Equal(t, "baz bar\n", readFileContent(t, f))
}

func TestFileEdit_AllowPartial_ReportsDirectorySkip(t *testing.T) {
	tmp := t.TempDir()
	okFile := filepath.Join(tmp, "ok.txt")
	skipDir := filepath.Join(tmp, "subdir")
	os.WriteFile(okFile, []byte("foo\n"), 0644)
	os.Mkdir(skipDir, 0755)
	params := FileEditParams{Path: tool.StringSliceParam{okFile, skipDir}, AllowPartial: true, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "failed")
	require.Contains(t, result, skipDir)
	require.Contains(t, result, "directory")
	require.Equal(t, "bar\n", readFileContent(t, okFile))
}

func TestFileEdit_DefaultMode_NoPartialWriteOnSecondFileFailure(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.txt")
	bdir := filepath.Join(tmp, "nested")
	b := filepath.Join(bdir, "b.txt")
	os.MkdirAll(bdir, 0755)
	os.WriteFile(a, []byte("foo\n"), 0644)
	os.WriteFile(b, []byte("foo\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{a, b}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	require.NoError(t, os.Chmod(bdir, 0555))
	defer os.Chmod(bdir, 0755)
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := fileEdit().Execute(ctx(tmp), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Equal(t, "foo\n", readFileContent(t, a))
	require.Equal(t, "foo\n", readFileContent(t, b))
}

func TestFileEdit_AllowPartial_OneFails(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, AllowPartial: true, Operations: []Operation{{Replace: &ReplaceOp{OldString: "missing", NewString: "x"}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "failed")
	require.Equal(t, "foo\n", readFileContent(t, f))
}

func TestFileEdit_AllowPartial_OneFileConflictsOtherSucceeds(t *testing.T) {
	tmp := t.TempDir()
	ok := filepath.Join(tmp, "ok.txt")
	bad := filepath.Join(tmp, "bad.txt")
	os.WriteFile(ok, []byte("line1\nline2\nline3\n"), 0644)
	os.WriteFile(bad, []byte("alpha beta gamma\n"), 0644)
	params := FileEditParams{
		Path:         tool.StringSliceParam{ok, bad},
		AllowPartial: true,
		Operations: []Operation{
			{Insert: &InsertOp{Line: 2, Content: "A\n"}},
			{Replace: &ReplaceOp{OldString: "line2", NewString: "LINE2"}},
		},
	}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "failed")
	require.Equal(t, "line1\nA\nLINE2\nline3\n", readFileContent(t, ok))
	require.Equal(t, "alpha beta gamma\n", readFileContent(t, bad))
}

func TestFileEdit_LargeFile_ReturnsError(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "large.txt")
	content := strings.Repeat("a", maxFileSize+1)
	os.WriteFile(f, []byte(content), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "a", NewString: "b", ReplaceAll: true}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "too large")
	require.Equal(t, content, readFileContent(t, f))
}

func TestFileEdit_AllowPartial_ReportsLargeFileSkip(t *testing.T) {
	tmp := t.TempDir()
	okFile := filepath.Join(tmp, "ok.txt")
	largeFile := filepath.Join(tmp, "large.txt")
	largeContent := strings.Repeat("a", maxFileSize+1)
	os.WriteFile(okFile, []byte("foo\n"), 0644)
	os.WriteFile(largeFile, []byte(largeContent), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{okFile, largeFile}, AllowPartial: true, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	result := fileEditCall(t, params, tmp)
	require.Contains(t, result, "failed")
	require.Contains(t, result, largeFile)
	require.Contains(t, result, "too large")
	require.Equal(t, "bar\n", readFileContent(t, okFile))
	require.Equal(t, largeContent, readFileContent(t, largeFile))
}

func TestFileEdit_SequentialEditsWithoutConcurrentChange(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)
	params1 := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	fileEditCall(t, params1, tmp)
	require.Equal(t, "bar\n", readFileContent(t, f))
	params2 := FileEditParams{Path: tool.StringSliceParam{f}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "bar", NewString: "baz"}}}}
	result := fileEditCall(t, params2, tmp)
	require.NotContains(t, result, "hash mismatch")
	require.Equal(t, "baz\n", readFileContent(t, f))
}

func TestFileEdit_DryRun_DiffInDisplayBlocks(t *testing.T) {
	tmp := t.TempDir()
	f := filepath.Join(tmp, "test.go")
	os.WriteFile(f, []byte("foo\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{f}, DryRun: true, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	tl := fileEdit()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := tl.Execute(ctx(tmp), raw)
	require.NoError(t, err)
	require.NotContains(t, res.String(), "---")
	require.Contains(t, res.String(), "Dry run")
	br, ok := res.(tool.BlocksResult)
	require.True(t, ok)
	require.NotEmpty(t, br.DisplayBlocks)
	diff, ok := br.DisplayBlocks[0].(tool.DiffBlock)
	require.True(t, ok)
	require.Contains(t, diff.UnifiedDiff, "-foo")
	require.Contains(t, diff.UnifiedDiff, "+bar")
	require.Equal(t, "foo\n", readFileContent(t, f))
}

func TestFileEdit_VerificationReadFailure_ReturnsErrorAndWritesNothing(t *testing.T) {
	tmp := t.TempDir()
	a := filepath.Join(tmp, "a.txt")
	b := filepath.Join(tmp, "b.txt")
	os.WriteFile(a, []byte("foo\n"), 0644)
	os.WriteFile(b, []byte("foo\n"), 0644)
	params := FileEditParams{Path: tool.StringSliceParam{a, b}, Operations: []Operation{{Replace: &ReplaceOp{OldString: "foo", NewString: "bar"}}}}
	originalReadFile := readFileForEdit
	readCalls := 0
	readFileForEdit = func(path string) ([]byte, error) {
		if path == b {
			readCalls++
			if readCalls == 2 {
				return nil, os.ErrPermission
			}
		}
		return os.ReadFile(path)
	}
	defer func() { readFileForEdit = originalReadFile }()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := fileEdit().Execute(ctx(tmp), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "verify")
	require.Equal(t, "foo\n", readFileContent(t, a))
	require.Equal(t, "foo\n", readFileContent(t, b))
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func readFileContent(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

func fileEditCall(t *testing.T, params FileEditParams, workDir string) string {
	t.Helper()
	tl := fileEdit()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := tl.Execute(ctx(workDir), raw)
	require.NoError(t, err, "tool error: %s", res)
	return res.String()
}

// ── Schema shape tests ────────────────────────────────────────────────────────

func TestFileEditSchema_TopLevelRequired(t *testing.T) {
	s := tool.SchemaFor[FileEditParams]()
	require.Contains(t, s.Required, "path", "path must be required")
	require.Contains(t, s.Required, "operations", "operations must be required")
}

func TestFileEditSchema_OperationsItemsIsOneOf(t *testing.T) {
	s := tool.SchemaFor[FileEditParams]()
	require.NotNil(t, s.Properties)
	opsProp, ok := s.Properties.Get("operations")
	require.True(t, ok, "operations property must exist")
	require.NotNil(t, opsProp.Items, "operations must have items")
	require.Len(t, opsProp.Items.OneOf, 5, "operations.items must be a oneOf with 5 variants")

	wantKeys := []string{"replace", "insert", "remove", "append", "patch"}
	for i, variant := range opsProp.Items.OneOf {
		require.Len(t, variant.Required, 1, "variant %d must have exactly one required key", i)
		require.Equal(t, wantKeys[i], variant.Required[0], "variant %d required key", i)
		require.NotNil(t, variant.AdditionalProperties, "variant %d must have additionalProperties", i)
	}
}

func TestFileEditSchema_RemoveVariantIsInnerOneOf(t *testing.T) {
	s := tool.SchemaFor[FileEditParams]()
	opsProp, _ := s.Properties.Get("operations")
	// remove is the 3rd variant (index 2)
	removeVariant := opsProp.Items.OneOf[2]
	removeProp, ok := removeVariant.Properties.Get("remove")
	require.True(t, ok, "remove variant must have a remove property")
	require.Len(t, removeProp.OneOf, 2, "remove value must be a oneOf with 2 branches")

	var keys []string
	for _, branch := range removeProp.OneOf {
		require.Len(t, branch.Required, 1, "each remove branch must have exactly one required key")
		keys = append(keys, branch.Required[0])
	}
	require.ElementsMatch(t, []string{"old_string", "lines"}, keys)
}

func TestFileEditSchema_NoSchemaOrDefsWrapper(t *testing.T) {
	raw, err := json.Marshal(tool.SchemaFor[FileEditParams]())
	require.NoError(t, err)
	s := string(raw)
	require.NotContains(t, s, `"$schema"`)
	require.NotContains(t, s, `"$defs"`)
	require.NotContains(t, s, `"$id"`)
}

func TestFileEditSchema_DescriptionsPresent(t *testing.T) {
	raw, err := json.Marshal(tool.SchemaFor[FileEditParams]())
	require.NoError(t, err)
	s := string(raw)
	// spot-check a description from each op type
	require.Contains(t, s, "Exact text to find and replace")  // replace
	require.Contains(t, s, "Line number to insert before")    // insert
	require.Contains(t, s, "Mutually exclusive with lines")   // remove/byString
	require.Contains(t, s, "Text to append to the end")       // append
	require.Contains(t, s, "Unified diff patch")              // patch
}
