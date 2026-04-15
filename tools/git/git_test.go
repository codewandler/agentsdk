package git

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/codewandler/core/tool"
)

// ── test helpers ─────────────────────────────────────────────────────────────

type testCtx struct {
	context.Context
	workDir string
}

func (c *testCtx) WorkDir() string       { return c.workDir }
func (c *testCtx) AgentID() string       { return "test-agent" }
func (c *testCtx) SessionID() string     { return "test-session" }
func (c *testCtx) Extra() map[string]any { return nil }

func ctx(dir string) tool.Ctx {
	return &testCtx{Context: context.Background(), workDir: dir}
}

// callTool marshals params, executes the tool, and returns the result.
func callTool(t *testing.T, tl tool.Tool, params any, workDir string) tool.Result {
	t.Helper()
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	res, err := tl.Execute(ctx(workDir), raw)
	require.NoError(t, err)
	return res
}

// initGitRepo creates a temp dir with git init, user config, and an initial commit.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# init\n"), 0644))
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "init")
	return dir
}

func run(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "command %q failed: %s", name+" "+strings.Join(args, " "), string(out))
	return string(out)
}

// ── git_status tests ─────────────────────────────────────────────────────────

func TestGitStatus_CleanRepo(t *testing.T) {
	dir := initGitRepo(t)

	tl := gitStatus()
	res := callTool(t, tl, GitStatusParams{}, dir)
	require.NotNil(t, res)
	require.False(t, res.IsError())
	// Short format on a clean repo: only the branch header line, no file entries.
	// The output should not mention any modified/untracked files.
	out := res.String()
	require.NotEmpty(t, out)
}

func TestGitStatus_UntrackedFile(t *testing.T) {
	dir := initGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello\n"), 0644))

	tl := gitStatus()
	res := callTool(t, tl, GitStatusParams{}, dir)
	require.NotNil(t, res)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "new.txt")
}

func TestGitStatus_StagedFile(t *testing.T) {
	dir := initGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# updated\n"), 0644))
	run(t, dir, "git", "add", "README.md")

	tl := gitStatus()
	res := callTool(t, tl, GitStatusParams{}, dir)
	require.NotNil(t, res)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "README.md")
}

func TestGitStatus_NotAGitRepo(t *testing.T) {
	dir := t.TempDir() // no git init
	tl := gitStatus()
	res := callTool(t, tl, GitStatusParams{}, dir)
	require.NotNil(t, res)
	require.True(t, res.IsError())
}

// ── git_diff tests ──────────────────────────────────────────────────────────

func TestGitDiff_UnstagedChanges(t *testing.T) {
	dir := initGitRepo(t)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\n"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "add file")
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("line1\nline2\n"), 0644)

	tl := gitDiff()
	res := callTool(t, tl, GitDiffParams{}, dir)
	require.NotNil(t, res)
	require.False(t, res.IsError())

	br, ok := res.(tool.BlocksResult)
	require.True(t, ok, "expected BlocksResult, got %T", res)
	require.NotEmpty(t, br.DisplayBlocks)

	db, ok := br.DisplayBlocks[0].(tool.DiffBlock)
	require.True(t, ok, "expected DiffBlock, got %T", br.DisplayBlocks[0])
	require.Contains(t, db.Path, "file.txt")
	require.Contains(t, db.UnifiedDiff, "+line2")
	require.Greater(t, db.Added, 0)
}

func TestGitDiff_Staged(t *testing.T) {
	dir := initGitRepo(t)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("original\n"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "add file")
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("modified\n"), 0644)
	run(t, dir, "git", "add", "file.txt")

	tl := gitDiff()
	res := callTool(t, tl, GitDiffParams{Staged: true}, dir)
	require.NotNil(t, res)
	require.False(t, res.IsError())

	br, ok := res.(tool.BlocksResult)
	require.True(t, ok, "expected BlocksResult")
	require.NotEmpty(t, br.DisplayBlocks)
}

func TestGitDiff_BetweenRefs(t *testing.T) {
	dir := initGitRepo(t)
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v1\n"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "first")
	os.WriteFile(filepath.Join(dir, "file.txt"), []byte("v2\n"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "second")

	tl := gitDiff()
	res := callTool(t, tl, GitDiffParams{Ref: "HEAD~1..HEAD"}, dir)
	require.NotNil(t, res)
	require.False(t, res.IsError())

	br, ok := res.(tool.BlocksResult)
	require.True(t, ok, "expected BlocksResult")
	require.NotEmpty(t, br.DisplayBlocks)
}

func TestGitDiff_SpecificPaths(t *testing.T) {
	dir := initGitRepo(t)
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b\n"), 0644)
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-m", "add files")
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a modified\n"), 0644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b modified\n"), 0644)

	tl := gitDiff()
	res := callTool(t, tl, GitDiffParams{Paths: []string{"a.txt"}}, dir)
	require.NotNil(t, res)
	require.False(t, res.IsError())

	br, ok := res.(tool.BlocksResult)
	require.True(t, ok, "expected BlocksResult")
	// Should only contain diff for a.txt, not b.txt.
	for _, blk := range br.DisplayBlocks {
		if d, ok := blk.(tool.DiffBlock); ok {
			require.NotContains(t, d.Path, "b.txt")
		}
	}
}

func TestGitDiff_NoChanges(t *testing.T) {
	dir := initGitRepo(t)

	tl := gitDiff()
	res := callTool(t, tl, GitDiffParams{}, dir)
	require.NotNil(t, res)
	require.False(t, res.IsError())
	require.Contains(t, res.String(), "No changes")
}

func TestGitDiff_NotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	tl := gitDiff()
	res := callTool(t, tl, GitDiffParams{}, dir)
	require.NotNil(t, res)
	require.True(t, res.IsError())
}
