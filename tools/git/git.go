// Package git provides git_status and git_diff tools.
package git

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/codewandler/core/tool"
	idiff "github.com/codewandler/core/internal/diff"
)

const gitTimeout = 30 * time.Second

// Tools returns the git tools: git_status, git_diff.
func Tools() []tool.Tool {
	return []tool.Tool{
		gitStatus(),
		gitDiff(),
	}
}

// ── git_status ──────────────────────────────────────────────────────────────

// GitStatusParams are the parameters for the git_status tool.
type GitStatusParams struct {
	Short bool `json:"short,omitempty" jsonschema:"description=Use short format (default: true)"`
}

func gitStatus() tool.Tool {
	return tool.New("git_status",
		"Show the working tree status. Returns staged, unstaged, and untracked file lists.",
		func(ctx tool.Ctx, p GitStatusParams) (tool.Result, error) {
			out, err := runGit(ctx, ctx.WorkDir(), "status", "--short", "--branch")
			if err != nil {
				return tool.Errorf("git status: %s", err), nil
			}
			return tool.NewResult().Text(out).Build(), nil
		},
	)
}

// ── git_diff ────────────────────────────────────────────────────────────────

// GitDiffParams are the parameters for the git_diff tool.
type GitDiffParams struct {
	Staged bool     `json:"staged,omitempty" jsonschema:"description=Show staged (cached) changes instead of unstaged"`
	Ref    string   `json:"ref,omitempty" jsonschema:"description=Ref or ref range (e.g. HEAD~3\\, main..feature\\, abc123..def456)"`
	Paths  []string `json:"paths,omitempty" jsonschema:"description=Limit diff to specific file paths"`
}

func gitDiff() tool.Tool {
	return tool.New("git_diff",
		"Show changes between commits, staging area, and working tree. Returns colored diffs in the TUI.",
		func(ctx tool.Ctx, p GitDiffParams) (tool.Result, error) {
			args := []string{"diff"}
			if p.Staged {
				args = append(args, "--staged")
			}
			if p.Ref != "" {
				args = append(args, p.Ref)
			}
			if len(p.Paths) > 0 {
				args = append(args, "--")
				args = append(args, p.Paths...)
			}

			out, err := runGit(ctx, ctx.WorkDir(), args...)
			if err != nil {
				return tool.Errorf("git diff: %s", err), nil
			}

			if strings.TrimSpace(out) == "" {
				return tool.NewResult().Text("No changes.").Build(), nil
			}

			// Parse the unified diff into per-file chunks.
			chunks := splitDiffByFile(out)

			res := tool.NewResult()
			totalAdded, totalRemoved := 0, 0
			var fileSummaries []string

			for _, chunk := range chunks {
				added, removed := idiff.Stats(chunk.diff)
				totalAdded += added
				totalRemoved += removed

				res.Display(tool.DiffBlock{
					Path:        chunk.path,
					UnifiedDiff: chunk.diff,
					Added:       added,
					Removed:     removed,
				})
				fileSummaries = append(fileSummaries, fmt.Sprintf("  %s (+%d/-%d)", chunk.path, added, removed))
			}

			// LLM-visible summary.
			summary := fmt.Sprintf("%d file(s) changed, +%d/-%d lines", len(chunks), totalAdded, totalRemoved)
			if len(fileSummaries) > 0 {
				summary += "\n" + strings.Join(fileSummaries, "\n")
			}
			res.Text(summary)

			return res.Build(), nil
		},
	)
}

// ── helpers ──────────────────────────────────────────────────────────────────

// diffChunk holds a single file's diff text and its path.
type diffChunk struct {
	path string
	diff string
}

// splitDiffByFile splits a combined git diff output into per-file chunks.
// Each chunk starts at "diff --git a/... b/...".
func splitDiffByFile(raw string) []diffChunk {
	lines := strings.Split(raw, "\n")
	var chunks []diffChunk
	var current *diffChunk

	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") {
			if current != nil {
				chunks = append(chunks, *current)
			}
			path := extractPathFromDiffHeader(line)
			current = &diffChunk{path: path, diff: line + "\n"}
			continue
		}
		if current != nil {
			current.diff += line + "\n"
		}
	}
	if current != nil {
		chunks = append(chunks, *current)
	}
	return chunks
}

// extractPathFromDiffHeader extracts the file path from "diff --git a/foo b/foo".
func extractPathFromDiffHeader(line string) string {
	// Format: "diff --git a/<path> b/<path>"
	parts := strings.SplitN(line, " b/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	// Fallback: strip "diff --git a/"
	s := strings.TrimPrefix(line, "diff --git a/")
	if idx := strings.Index(s, " "); idx >= 0 {
		return s[:idx]
	}
	return s
}

// runGit executes a git command and returns its stdout.
// Stderr is included in the error message on failure.
func runGit(ctx context.Context, workDir string, args ...string) (string, error) {
	tCtx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()

	cmd := exec.CommandContext(tCtx, "git", args...)
	cmd.Dir = workDir

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%w: %s", err, errMsg)
		}
		return "", err
	}
	return stdout.String(), nil
}
