package filesystem

import (
	"context"

	"github.com/codewandler/agentsdk/action"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codewandler/agentsdk/tool"
)

// ── test helpers ──────────────────────────────────────────────────────────────

// testCtx is a minimal tool.Ctx for filesystem tests.
type testCtx struct {
	action.BaseCtx
	workDir string
}

func (c *testCtx) WorkDir() string       { return c.workDir }
func (c *testCtx) AgentID() string       { return "test-agent" }
func (c *testCtx) SessionID() string     { return "test-session" }
func (c *testCtx) Extra() map[string]any { return nil }

// ctx returns a testCtx rooted at dir.
func ctx(dir string) tool.Ctx {
	return &testCtx{BaseCtx: action.BaseCtx{Context: context.Background()}, workDir: dir}
}

// ── file_write ────────────────────────────────────────────────────────────────

func TestFilesystem_FileWrite_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	c := ctx(dir)
	tl := fileWrite()

	raw, _ := json.Marshal(FileWriteParams{Path: "hello.txt", Content: "hello world\n"})
	res, err := tl.Execute(c, raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())

	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	require.NoError(t, err)
	require.Equal(t, "hello world\n", string(data))
}

func TestFilesystem_FileWrite_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	c := ctx(dir)
	tl := fileWrite()

	raw, _ := json.Marshal(FileWriteParams{Path: "a/b/c.txt", Content: "nested"})
	res, err := tl.Execute(c, raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())

	data, err := os.ReadFile(filepath.Join(dir, "a/b/c.txt"))
	require.NoError(t, err)
	require.Equal(t, "nested", string(data))
}

// ── file_read ─────────────────────────────────────────────────────────────────

func TestFilesystem_FileRead_ReadsContent(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.txt"), []byte("line1\nline2\nline3\n"), 0644))

	c := ctx(dir)
	tl := fileRead()

	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{"test.txt"}})
	res, err := tl.Execute(c, raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "line1")
	require.Contains(t, res.String(), "line2")
	require.Contains(t, res.String(), "line3")
}

func TestFilesystem_FileRead_NotFound(t *testing.T) {
	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{"nonexistent.txt"}})
	res, err := tl.Execute(ctx(t.TempDir()), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
}

// ── file_read v2 tests ──────────────────────────────────────────────────────────

func TestFilesystem_FileRead_SinglePath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\ntwo\nthree\n"), 0644))

	tl := fileRead()

	// String path (singular)
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{"a.txt"}})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	require.Contains(t, output, "[File:")
	require.Contains(t, output, "one")
	require.Contains(t, output, "three")
	// Header format: [Lines: N total]
	require.Contains(t, output, "[Lines: 3 total]")
}

func TestFilesystem_FileRead_GlobPattern(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("file a\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("file b\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte("file c\n"), 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{filepath.Join(dir, "*.txt")}})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	// All three files should appear
	require.Contains(t, output, "file a")
	require.Contains(t, output, "file b")
	require.Contains(t, output, "file c")
	// Separator between files
	require.Contains(t, output, "──────────────────────────────────────────")
	// Three headers
	require.Equal(t, 3, strings.Count(output, "[File:"), "expected 3 file headers")
}

func TestFilesystem_FileRead_MultiplePaths(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x content\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "y.txt"), []byte("y content\n"), 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{filepath.Join(dir, "x.txt"), filepath.Join(dir, "y.txt")}})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	require.Contains(t, output, "x content")
	require.Contains(t, output, "y content")
}

func TestFilesystem_FileRead_Ranges(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 20)
	for i := 0; i < 20; i++ {
		lines[i] = fmt.Sprintf("line%02d", i+1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{
		Path:   tool.StringSliceParam{"file.txt"},
		Ranges: []LineRange{{Start: 3, End: 5}},
	})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	require.Contains(t, output, "line03")
	require.Contains(t, output, "line04")
	require.Contains(t, output, "line05")
	require.NotContains(t, output, "line01")
	require.NotContains(t, output, "line06")
	require.Contains(t, output, "[Ranges: 3-5]")
}

func TestFilesystem_FileRead_DefaultRange_AllLines(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 10)
	for i := 0; i < 10; i++ {
		lines[i] = fmt.Sprintf("L%02d", i+1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "f.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{"f.txt"}})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	// All 10 lines present
	require.Contains(t, output, "L01")
	require.Contains(t, output, "L10")
	// No [Ranges:] in header when none specified
	require.NotContains(t, output, "[Ranges:")
	// Header shows total
	require.Contains(t, output, "[Lines: 10 total]")
}

func TestFilesystem_FileRead_RangesWithOmission(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 20)
	for i := 0; i < 20; i++ {
		lines[i] = fmt.Sprintf("L%02d", i+1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gap.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{
		Path:   tool.StringSliceParam{"gap.txt"},
		Ranges: []LineRange{{Start: 1, End: 3}, {Start: 10, End: 12}},
	})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	require.Contains(t, output, "L01")
	require.Contains(t, output, "L03")
	require.Contains(t, output, "L10")
	require.Contains(t, output, "L12")
	// Omission marker for gap
	require.Contains(t, output, "~")
	require.Contains(t, output, "[Ranges: 1-3, 10-12]")
}

func TestFilesystem_FileRead_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "big.txt")
	// Write a file > 10MB
	data := make([]byte, 11*1024*1024)
	for i := range data {
		data[i] = 'x'
	}
	require.NoError(t, os.WriteFile(f, data, 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{"big.txt"}})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	// Single file → error result
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "too large")
}

func TestFilesystem_FileRead_DirectoryInPaths(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.MkdirAll(subdir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{filepath.Join(dir, "a.txt"), subdir}})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	require.Contains(t, output, "a")
	require.Contains(t, output, "1 file(s) skipped")
}

func TestFilesystem_FileRead_NoMatchingGlob(t *testing.T) {
	dir := t.TempDir()
	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{"*.nonexistent"}})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "no files matched")
}

func TestFilesystem_FileRead_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "empty.txt"), []byte{}, 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{Path: tool.StringSliceParam{"empty.txt"}})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	require.Contains(t, output, "[Lines: 0 total]")
}

func TestFilesystem_FileRead_LineNumberWidth(t *testing.T) {
	dir := t.TempDir()
	lines := make([]string, 9999)
	for i := 0; i < 9999; i++ {
		lines[i] = fmt.Sprintf("L%04d", i+1)
	}
	require.NoError(t, os.WriteFile(filepath.Join(dir, "wide.txt"), []byte(strings.Join(lines, "\n")+"\n"), 0644))

	tl := fileRead()
	raw, _ := json.Marshal(FileReadParams{
		Path:   tool.StringSliceParam{"wide.txt"},
		Ranges: []LineRange{{Start: 9990}},
	})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	output := res.String()
	// Line 9999 should be 4-digit padded
	require.Contains(t, output, "9999│")
	// Line 9990 should be 4-digit padded
	require.Contains(t, output, "9990│")
}

// ── file_delete ───────────────────────────────────────────────────────────────

func TestFilesystem_FileDelete_DeletesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tmp.txt")
	require.NoError(t, os.WriteFile(path, []byte("bye"), 0644))

	tl := fileDelete()
	raw, _ := json.Marshal(FileDeleteParams{Path: "tmp.txt"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())

	_, err = os.Stat(path)
	require.True(t, os.IsNotExist(err), "file should be deleted")
}

func TestFilesystem_FileDelete_RefusesDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir")
	require.NoError(t, os.Mkdir(subdir, 0755))

	tl := fileDelete()
	raw, _ := json.Marshal(FileDeleteParams{Path: "subdir"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError(), "should refuse to delete directory")
}

// ── file_stat ─────────────────────────────────────────────────────────────────

func TestFilesystem_FileStat_ReturnsFileInfo(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "info.txt"), []byte("contents"), 0644))

	tl := fileStat()
	raw, _ := json.Marshal(FileStatParams{Path: "info.txt"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "exists: true")
	require.Contains(t, res.String(), "type: file")
}

func TestFilesystem_FileStat_NotExistReportsExists(t *testing.T) {
	tl := fileStat()
	raw, _ := json.Marshal(FileStatParams{Path: "ghost.txt"})
	res, err := tl.Execute(ctx(t.TempDir()), raw)
	require.NoError(t, err)
	require.Contains(t, res.String(), "exists: false")
}

// ── glob ──────────────────────────────────────────────────────────────────────

func TestFilesystem_Glob_FindsFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0644))

	tl := glob()
	raw, _ := json.Marshal(GlobParams{Pattern: "*.go"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.Contains(t, res.String(), "a.go")
	require.Contains(t, res.String(), "b.go")
	require.NotContains(t, res.String(), "c.txt")
}

func TestFilesystem_Glob_NoMatch(t *testing.T) {
	tl := glob()
	raw, _ := json.Marshal(GlobParams{Pattern: "*.go"})
	res, err := tl.Execute(ctx(t.TempDir()), raw)
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(res.String()), "no files")
}

// ── StringSliceParam tests ─────────────────────────────────────────────────────

func TestStringSliceParam_Unmarshal_SingularString(t *testing.T) {
	var p struct {
		Paths tool.StringSliceParam `json:"paths"`
	}
	input := `{"paths": "file1.go"}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{"file1.go"}, []string(p.Paths))
}

func TestStringSliceParam_Unmarshal_ArrayOfStrings(t *testing.T) {
	var p struct {
		Paths tool.StringSliceParam `json:"paths"`
	}
	input := `{"paths": ["file1.go", "file2.go"]}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{"file1.go", "file2.go"}, []string(p.Paths))
}

func TestStringSliceParam_Unmarshal_EmptyArray(t *testing.T) {
	var p struct {
		Paths tool.StringSliceParam `json:"paths"`
	}
	input := `{"paths": []}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string{}, []string(p.Paths))
}

func TestStringSliceParam_Unmarshal_Nil(t *testing.T) {
	var p struct {
		Paths tool.StringSliceParam `json:"paths"`
	}
	input := `{}`
	err := json.Unmarshal([]byte(input), &p)
	require.NoError(t, err)
	require.Equal(t, []string(nil), []string(p.Paths))
}

// ── grep ──────────────────────────────────────────────────────────────────────

func TestFilesystem_Grep_FindsMatches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644))

	tl := grep()
	raw, _ := json.Marshal(GrepParams{Pattern: "func main", Paths: (func() *tool.StringSliceParam { p := tool.StringSliceParam{"*.go"}; return &p }()), ShowContent: true})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.Contains(t, res.String(), "func main")
}

func TestFilesystem_Grep_NoMatches(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello world\n"), 0644))

	tl := grep()
	raw, _ := json.Marshal(GrepParams{Pattern: "xyz123", Paths: (func() *tool.StringSliceParam { p := tool.StringSliceParam{"*.txt"}; return &p }())})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.Contains(t, strings.ToLower(res.String()), "no matches")
}

// ── dir_list ──────────────────────────────────────────────────────────────────

func TestFilesystem_DirList_ListsEntries(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "foo.txt"), []byte(""), 0644))
	require.NoError(t, os.Mkdir(filepath.Join(dir, "subdir"), 0755))

	tl := dirList()
	raw, _ := json.Marshal(DirListParams{Path: "."})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.Contains(t, res.String(), "foo.txt")
	require.Contains(t, res.String(), "subdir/")
}

// ── dir_tree ──────────────────────────────────────────────────────────────────

func TestFilesystem_DirTree_ShowsTree(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg/sub"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg/sub/file.go"), []byte(""), 0644))

	tl := dirTree()
	raw, _ := json.Marshal(DirTreeParams{Path: ".", Depth: 5})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	output := res.String()
	require.Contains(t, output, "pkg")
	require.Contains(t, output, "file.go")
}

func TestFilesystem_DirTree_ShowLines(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644))

	tl := dirTree()
	raw, _ := json.Marshal(DirTreeParams{Path: ".", Depth: 2, ShowSize: true, ShowLines: true})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	output := res.String()
	require.Contains(t, output, "main.go")
	require.Contains(t, output, "3L")
	require.Contains(t, output, "B")
}

func TestFilesystem_DirTree_FlatShowLines(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "pkg"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "pkg", "main.go"), []byte("a\nb\n"), 0644))

	tl := dirTree()
	raw, _ := json.Marshal(DirTreeParams{Path: ".", Depth: 3, Flat: true, ShowLines: true})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.Contains(t, res.String(), "pkg/main.go (2L)")
}

func TestFilesystem_Grep_EmptyPaths_DefaultsToCurrentDir(t *testing.T) {
	tl := grep()
	// When Paths is not provided, it defaults to searching the current directory
	// Use raw JSON without paths field (don't use json.Marshal with nil pointer)
	raw := []byte(`{"pattern": "anything"}`)
	res, err := tl.Execute(ctx(t.TempDir()), raw)
	require.NoError(t, err)
	// Should search current dir, not error
	require.NotContains(t, res.String(), "Paths cannot be empty")
}

func TestFilesystem_Grep_SingularPath(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0644))

	tl := grep()
	// Test with a singular path (string instead of array)
	raw := []byte(`{"pattern": "func main", "paths": "*.go"}`)
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.Contains(t, res.String(), "main.go")
	require.Contains(t, res.String(), "3")
}

// ── Phase 2: Fuzzy Matching (AutoCorrect) ─────────────────────────────────────

// ── dir_create / file_copy / file_move ───────────────────────────────────────

func TestFilesystem_DirCreate_CreatesParentsWhenRequested(t *testing.T) {
	dir := t.TempDir()
	tl := dirCreate()

	raw, _ := json.Marshal(DirCreateParams{Path: "a/b/c", Parents: true})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	require.DirExists(t, filepath.Join(dir, "a", "b", "c"))
}

func TestFilesystem_DirCreate_RequiresParentsForNestedPath(t *testing.T) {
	dir := t.TempDir()
	tl := dirCreate()

	raw, _ := json.Marshal(DirCreateParams{Path: "a/b"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "parents=true")
}

func TestFilesystem_FileCopy_CopiesFileAndRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("source"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dst.txt"), []byte("existing"), 0644))
	tl := fileCopy()

	raw, _ := json.Marshal(FileCopyParams{Src: "src.txt", Dst: "dst.txt"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "overwrite=true")

	raw, _ = json.Marshal(FileCopyParams{Src: "src.txt", Dst: "dst.txt", Overwrite: true})
	res, err = tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	data, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
	require.NoError(t, err)
	require.Equal(t, "source", string(data))
}

func TestFilesystem_FileCopy_RecursiveDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src", "nested"), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "nested", "file.txt"), []byte("content"), 0644))
	tl := fileCopy()

	raw, _ := json.Marshal(FileCopyParams{Src: "src", Dst: "dst"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "recursive=true")

	raw, _ = json.Marshal(FileCopyParams{Src: "src", Dst: "dst", Recursive: true})
	res, err = tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	data, err := os.ReadFile(filepath.Join(dir, "dst", "nested", "file.txt"))
	require.NoError(t, err)
	require.Equal(t, "content", string(data))
	require.Contains(t, res.String(), "1 file(s)")
}

func TestFilesystem_FileCopy_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "target.txt"), []byte("target"), 0644))
	if err := os.Symlink("target.txt", filepath.Join(dir, "link.txt")); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	tl := fileCopy()

	raw, _ := json.Marshal(FileCopyParams{Src: "link.txt", Dst: "copy.txt"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "symlink")
}

func TestFilesystem_FileMove_MovesFileAndOverwritesWhenRequested(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src.txt"), []byte("source"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dst.txt"), []byte("existing"), 0644))
	tl := fileMove()

	raw, _ := json.Marshal(FileMoveParams{Src: "src.txt", Dst: "dst.txt"})
	res, err := tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.True(t, res.IsError())
	require.Contains(t, res.String(), "overwrite=true")

	raw, _ = json.Marshal(FileMoveParams{Src: "src.txt", Dst: "dst.txt", Overwrite: true})
	res, err = tl.Execute(ctx(dir), raw)
	require.NoError(t, err)
	require.False(t, res.IsError(), res.String())
	require.NoFileExists(t, filepath.Join(dir, "src.txt"))
	data, err := os.ReadFile(filepath.Join(dir, "dst.txt"))
	require.NoError(t, err)
	require.Equal(t, "source", string(data))
}

func TestFilesystem_ToolsIncludesCopyMoveCreate(t *testing.T) {
	names := map[string]bool{}
	for _, tl := range Tools() {
		names[tl.Name()] = true
	}
	require.True(t, names["dir_create"])
	require.True(t, names["file_copy"])
	require.True(t, names["file_move"])
}
