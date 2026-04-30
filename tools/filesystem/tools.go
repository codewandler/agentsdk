// Package filesystem provides the filesystem tools.
package filesystem

import (
	"bufio"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	ignore "github.com/sabhiram/go-gitignore"

	"github.com/codewandler/agentsdk/internal/humanize"
	"github.com/codewandler/agentsdk/tool"
)

// ── Constants ─────────────────────────────────────────────────────────────────

const (
	maxFileSize         = 10 * 1024 * 1024
	maxWriteSize        = 1 * 1024 * 1024
	defaultLimit        = 500
	maxLimit            = 2000
	binaryCheckSize     = 8 * 1024
	maxGlobResults      = 1000
	maxGrepFiles        = 100
	maxGrepMatches      = 1000
	defaultTreeDepth    = 3
	maxTreeDepth        = 20
	defaultMaxTreeNodes = 1000
	maxMaxTreeNodes     = 5000
)

// ── Parameter types ───────────────────────────────────────────────────────────

type DirCreateParams struct {
	Path    string `json:"path" jsonschema:"description=Directory path to create,required"`
	Parents bool   `json:"parents,omitempty" jsonschema:"description=Create parent directories as needed (like mkdir -p)"`
}

type DirListParams struct {
	Path       string `json:"path" jsonschema:"description=Directory path to list,required"`
	ShowHidden bool   `json:"show_hidden,omitempty" jsonschema:"description=Include hidden files (names starting with .)"`
	Pattern    string `json:"pattern,omitempty" jsonschema:"description=Glob pattern to filter entries (e.g. *.go)"`
	SortBy     string `json:"sort_by,omitempty" jsonschema:"description=Sort order: name (default)\\, size\\, mtime"`
}

type DirTreeParams struct {
	Path        string `json:"path" jsonschema:"description=Root directory to display,required"`
	Depth       int    `json:"depth,omitempty" jsonschema:"description=Maximum recursion depth (default 3 max 20)"`
	ShowHidden  bool   `json:"show_hidden,omitempty" jsonschema:"description=Include hidden files"`
	Pattern     string `json:"pattern,omitempty" jsonschema:"description=Glob pattern to filter entries"`
	ShowSize    bool   `json:"show_size,omitempty" jsonschema:"description=Show file sizes"`
	ShowLines   bool   `json:"show_lines,omitempty" jsonschema:"description=Show file line counts"`
	Flat        bool   `json:"flat,omitempty" jsonschema:"description=List files flat (like find -type f) instead of tree"`
	MaxEntries  int    `json:"max_entries,omitempty" jsonschema:"description=Maximum number of entries (default 1000 max 5000)"`
	NoGitignore bool   `json:"no_gitignore,omitempty" jsonschema:"description=Disable .gitignore filtering"`
}

type LineRange struct {
	Start int `json:"start" jsonschema:"description=Start line (1-indexed),required"`
	End   int `json:"end,omitempty" jsonschema:"description=End line (inclusive. Default: start+limit-1)"`
}

type FileReadParams struct {
	Path   tool.StringSliceParam `json:"path" jsonschema:"description=File path(s) to read (supports glob patterns and arrays),required"`
	Ranges []LineRange           `json:"ranges,omitempty" jsonschema:"description=Line ranges to read (start/end inclusive). Defaults to all lines (capped at 500)."`
}

type FileWriteParams struct {
	Path      string `json:"path" jsonschema:"description=File path to write,required"`
	Content   string `json:"content" jsonschema:"description=Complete file content,required"`
	Overwrite bool   `json:"overwrite,omitempty" jsonschema:"description=Overwrite file if it already exists (default false)"`
}

type FileCopyParams struct {
	Src       string `json:"src" jsonschema:"description=Source file or directory path,required"`
	Dst       string `json:"dst" jsonschema:"description=Destination path,required"`
	Recursive bool   `json:"recursive,omitempty" jsonschema:"description=Copy directories recursively. Required when src is a directory"`
	Overwrite bool   `json:"overwrite,omitempty" jsonschema:"description=Overwrite destination if it already exists"`
}

type FileMoveParams struct {
	Src       string `json:"src" jsonschema:"description=Source file or directory path,required"`
	Dst       string `json:"dst" jsonschema:"description=Destination path,required"`
	Overwrite bool   `json:"overwrite,omitempty" jsonschema:"description=Overwrite destination if it already exists"`
}

type FileStatParams struct {
	Path string `json:"path" jsonschema:"description=File or directory path to stat,required"`
}

type FileDeleteParams struct {
	Path string `json:"path" jsonschema:"description=File path to delete,required"`
}

type GlobParams struct {
	Pattern     string `json:"pattern" jsonschema:"description=Glob pattern to match files (e.g. **/*.go or src/**/*.ts),required"`
	Path        string `json:"path,omitempty" jsonschema:"description=Directory to search in (default: working directory)"`
	NoGitignore bool   `json:"no_gitignore,omitempty" jsonschema:"description=Disable .gitignore filtering to include all files (default false)"`
}

type GrepParams struct {
	Pattern      string                 `json:"pattern" jsonschema:"description=Regex pattern to search for in file contents,required"`
	Paths        *tool.StringSliceParam `json:"paths" jsonschema:"description=File paths or glob patterns to search (optional - defaults to current dir)"`
	ShowContent  bool                   `json:"show_content,omitempty" jsonschema:"description=Include matching line content in output (default: false)"`
	ContextLines int                    `json:"context_lines,omitempty" jsonschema:"description=Number of surrounding lines to include around each match (implies show_content)"`
	NoGitignore  bool                   `json:"no_gitignore,omitempty" jsonschema:"description=Disable .gitignore filtering to include all files (default false)"`
}

// ── Tools ─────────────────────────────────────────────────────────────────────

// Tools returns all filesystem tools.
func Tools() []tool.Tool {
	return []tool.Tool{
		dirList(),
		dirTree(),
		dirCreate(),
		fileRead(),
		fileWrite(),
		fileEdit(),
		fileStat(),
		fileCopy(),
		fileMove(),
		fileDelete(),
		glob(),
		grep(),
	}
}

// ── dir_create ────────────────────────────────────────────────────────────────

func dirCreate() tool.Tool {
	return tool.New("dir_create",
		"Create a directory. Set parents=true to create parent directories as needed.",
		func(ctx tool.Ctx, p DirCreateParams) (tool.Result, error) {
			if p.Path == "" {
				return tool.Error("path cannot be empty"), nil
			}
			path := resolvePath(p.Path, ctx.WorkDir())
			if p.Parents {
				if err := os.MkdirAll(path, 0755); err != nil {
					return nil, fmt.Errorf("create directory %s: %w", path, err)
				}
				return tool.Textf("Created directory %s", path), nil
			}
			if err := os.Mkdir(path, 0755); err != nil {
				if os.IsExist(err) {
					return tool.Errorf("directory already exists: %s", path), nil
				}
				if os.IsNotExist(err) {
					return tool.Errorf("parent directory does not exist: %s (use parents=true to create parents)", filepath.Dir(path)), nil
				}
				return nil, fmt.Errorf("create directory %s: %w", path, err)
			}
			return tool.Textf("Created directory %s", path), nil
		},
		dirCreateIntent(),
	)
}

// ── dir_list ──────────────────────────────────────────────────────────────────

func dirList() tool.Tool {
	return tool.New("dir_list",
		"List directory contents with type, size, modification time, and permissions.",
		func(ctx tool.Ctx, p DirListParams) (tool.Result, error) {
			path := resolvePath(p.Path, ctx.WorkDir())
			entries, err := os.ReadDir(path)
			if err != nil {
				return tool.Errorf("read dir %s: %v", path, err), nil
			}

			// Filter
			var filtered []os.DirEntry
			for _, e := range entries {
				if !p.ShowHidden && strings.HasPrefix(e.Name(), ".") {
					continue
				}
				if p.Pattern != "" {
					matched, _ := filepath.Match(p.Pattern, e.Name())
					if !matched {
						continue
					}
				}
				filtered = append(filtered, e)
			}

			// Sort
			switch p.SortBy {
			case "size":
				sort.Slice(filtered, func(i, j int) bool {
					ii, _ := filtered[i].Info()
					ij, _ := filtered[j].Info()
					if ii == nil || ij == nil {
						return filtered[i].Name() < filtered[j].Name()
					}
					return ii.Size() > ij.Size()
				})
			case "mtime":
				sort.Slice(filtered, func(i, j int) bool {
					ii, _ := filtered[i].Info()
					ij, _ := filtered[j].Info()
					if ii == nil || ij == nil {
						return filtered[i].Name() < filtered[j].Name()
					}
					return ii.ModTime().After(ij.ModTime())
				})
			default: // name
				sort.Slice(filtered, func(i, j int) bool {
					di, dj := filtered[i].IsDir(), filtered[j].IsDir()
					if di != dj {
						return di
					}
					return filtered[i].Name() < filtered[j].Name()
				})
			}

			var lines []string
			for _, e := range filtered {
				info, err := e.Info()
				if err != nil {
					continue
				}
				var typeChar string
				switch {
				case e.IsDir():
					typeChar = "d"
				case info.Mode()&os.ModeSymlink != 0:
					typeChar = "l"
				default:
					typeChar = "-"
				}
				name := e.Name()
				if e.IsDir() {
					name += "/"
				}
				line := fmt.Sprintf("%s%s  %6s  %14s  %s",
					typeChar, formatPermissions(info.Mode()),
					humanize.Size(info.Size()),
					humanTime(info.ModTime()),
					name,
				)
				lines = append(lines, line)
			}

			return tool.Text(fmt.Sprintf("Directory: %s (%d entries)\n\n%s",
				path, len(lines), strings.Join(lines, "\n"))), nil
		},
		dirListIntent(),
	)
}

// ── dir_tree ──────────────────────────────────────────────────────────────────

func dirTree() tool.Tool {
	return tool.New("dir_tree",
		"Show a directory as a recursive ASCII tree, or list files flat (like find -type f).",
		func(ctx tool.Ctx, p DirTreeParams) (tool.Result, error) {
			path := resolvePath(p.Path, ctx.WorkDir())
			info, err := os.Stat(path)
			if err != nil {
				return tool.Errorf("stat %s: %v", path, err), nil
			}
			if !info.IsDir() {
				return tool.Errorf("%s is not a directory", path), nil
			}

			depth := p.Depth
			if depth < 1 {
				depth = defaultTreeDepth
			}
			if depth > maxTreeDepth {
				depth = maxTreeDepth
			}
			maxNodes := p.MaxEntries
			if maxNodes < 1 {
				maxNodes = defaultMaxTreeNodes
			}
			if maxNodes > maxMaxTreeNodes {
				maxNodes = maxMaxTreeNodes
			}

			var gi *ignore.GitIgnore
			if !p.NoGitignore {
				gi = loadGitignore(path)
			}

			var lines []string
			totalNodes := 0
			truncated := false

			if p.Flat {
				collectFiles(path, path, depth, 0, p.ShowHidden, p.Pattern, p.ShowSize, p.ShowLines, gi,
					&lines, &totalNodes, &truncated, maxNodes)
			} else {
				lines = append(lines, filepath.Base(path)+"/")
				buildDirTree(path, path, "", depth, 0, p.ShowHidden, p.Pattern, p.ShowSize, p.ShowLines, gi,
					&lines, &totalNodes, &truncated, maxNodes)
			}

			return tool.Text(strings.Join(lines, "\n")), nil
		},
		dirTreeIntent(),
	)
}

// ── file_read ─────────────────────────────────────────────────────────────────

// fileReadResult holds the result of reading a single file.
type fileReadResult struct {
	path       string
	header     string // formatted header line (no trailing newline)
	content    string // LLM-visible: line-numbered content
	rawContent string // TUI-visible: raw content without line-number prefix
	startLine  int    // first line of first merged range
	endLine    int    // last line of last merged range
	totalLines int
	truncated  bool
	err        error // non-nil means skip this file; errMsg() gives the display text
}

func fileRead() tool.Tool {
	return tool.New("file_read",
		"Read file content with line numbers.",
		func(ctx tool.Ctx, p FileReadParams) (tool.Result, error) {
			paths := expandPaths(p.Path, ctx.WorkDir())
			if len(paths) == 0 {
				return tool.Errorf("no files matched: %s", p.Path[0]), nil
			}

			// Process all files
			var results []fileReadResult
			var errors []string
			for _, path := range paths {
				res := readFile(path, p.Ranges, len(p.Ranges) > 0)
				res.path = path
				if res.err != nil {
					errors = append(errors, fmt.Sprintf("%s: %s", path, res.errMsg()))
				}
				results = append(results, res)
			}

			// Build output: LLM text + FileBlock display blocks.
			b := tool.NewResult()
			var sb strings.Builder
			lastIdx := len(results) - 1
			hasValidFile := false
			for i, res := range results {
				if res.err != nil {
					continue // skip files that errored
				}
				hasValidFile = true
				sb.WriteString(res.header)
				sb.WriteString("\n\n")
				if res.content != "" {
					sb.WriteString(res.content)
					sb.WriteString("\n")
				}
				if i < lastIdx {
					sb.WriteString("\n──────────────────────────────────────────\n\n")
				}
				// Display block: raw content for TUI rendering.
				b.Display(tool.FileBlock{
					Path:       res.path,
					Content:    res.rawContent,
					StartLine:  res.startLine,
					EndLine:    res.endLine,
					TotalLines: res.totalLines,
					Truncated:  res.truncated,
				})
			}

			if len(errors) > 0 {
				footer := fmt.Sprintf("%d file(s) skipped: %s", len(errors), strings.Join(errors, "; "))
				if hasValidFile {
					sb.WriteString(footer)
					sb.WriteByte('\n')
					b.Text(strings.TrimRight(sb.String(), "\n"))
					return b.Build(), nil
				}
				// All files failed → error result.
				return tool.Errorf("%s", footer), nil
			}

			b.Text(strings.TrimRight(sb.String(), "\n"))
			return b.Build(), nil
		},
		fileReadIntent(),
	)
}

// readFile reads a single file and returns its header + content.
func readFile(path string, ranges []LineRange, rangesSpecified bool) fileReadResult {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fileReadResult{err: fmt.Errorf("not found")}
		}
		return fileReadResult{err: fmt.Errorf("stat failed: %w", err)}
	}
	if info.IsDir() {
		return fileReadResult{err: fmt.Errorf("is a directory")}
	}
	if info.Size() > maxFileSize {
		return fileReadResult{err: fmt.Errorf("too large (%s, max 10MB)", humanize.Size(info.Size()))}
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return fileReadResult{err: fmt.Errorf("read failed: %w", err)}
	}
	allLines := strings.Split(string(content), "\n")
	if len(allLines) > 0 && allLines[len(allLines)-1] == "" {
		allLines = allLines[:len(allLines)-1]
	}
	totalLines := len(allLines)

	// Normalize ranges
	if !rangesSpecified {
		ranges = []LineRange{{Start: 1, End: 0}} // End=0 → capped at defaultLimit
	}

	rr := readRanges(allLines, ranges, defaultLimit, rangesSpecified)

	// Build header
	header := fmt.Sprintf("[File: %s] [Lines: %d total] [Size: %s]", path, totalLines, humanize.Size(info.Size()))
	if rangesSpecified && len(rr.merged) > 0 {
		header += " [Ranges: " + formatRanges(rr.merged) + "]"
	}

	startLine := 1
	if len(rr.merged) > 0 {
		startLine = rr.merged[0].start
	}
	endLine := rr.actualEnd
	if endLine == 0 {
		endLine = totalLines
	}

	return fileReadResult{
		header:     header,
		content:    rr.content,
		rawContent: rr.rawContent,
		totalLines: totalLines,
		truncated:  rr.truncated,
		startLine:  startLine,
		endLine:    endLine,
	}
}

// errMsg returns a human-readable error message for the result.
func (r fileReadResult) errMsg() string {
	if r.err == nil {
		return ""
	}
	return r.err.Error()
}

// formatRanges formats merged ranges for the header (e.g. "1-3, 10-12").
func formatRanges(merged []r) string {
	var parts []string
	for _, rng := range merged {
		parts = append(parts, fmt.Sprintf("%d-%d", rng.start, rng.end))
	}
	return strings.Join(parts, ", ")
}

// expandPaths expands StringSliceParam (files and/or glob patterns) into a sorted,
// deduplicated list of file paths. Relative paths are resolved against workdir.
func expandPaths(paths tool.StringSliceParam, workdir string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, pattern := range paths {
		abs := resolvePath(pattern, workdir)
		matches, _ := doublestar.FilepathGlob(abs)
		for _, m := range matches {
			if !seen[m] {
				seen[m] = true
				result = append(result, m)
			}
		}
	}
	sort.Strings(result)
	return result
}

// r is a line range with 1-indexed inclusive bounds.
type r struct{ start, end int }

// rangesResult holds the result of merging ranges for rendering.
type rangesResult struct {
	merged     []r    // merged sorted ranges (may be clamped to totalLines)
	content    string // LLM-visible: rendered lines with line-number prefix
	rawContent string // TUI-visible: raw lines without line-number prefix
	actualEnd  int    // last line number actually rendered (clamped to totalLines)
	truncated  bool   // true if we didn't show all lines in the file
}

func readRanges(allLines []string, ranges []LineRange, defLimit int, rangesWereSpecified bool) rangesResult {
	totalLines := len(allLines)

	truncated := false
	var norm []r
	for _, rng := range ranges {
		s := rng.Start
		if s < 1 {
			s = 1
		}
		e := rng.End
		if e < s {
			e = s + defLimit - 1
		}
		if e-s+1 > maxLimit {
			e = s + maxLimit - 1
			truncated = true // hard-capped
		}
		norm = append(norm, r{s, e})
	}
	sort.Slice(norm, func(i, j int) bool { return norm[i].start < norm[j].start })

	// Merge overlapping
	var merged []r
	for _, rng := range norm {
		if len(merged) == 0 {
			merged = append(merged, rng)
			continue
		}
		last := &merged[len(merged)-1]
		if rng.start <= last.end+1 {
			if rng.end > last.end {
				last.end = rng.end
			}
		} else {
			merged = append(merged, rng)
		}
	}

	maxEnd := 0
	for _, rng := range merged {
		if rng.end > maxEnd {
			maxEnd = rng.end
		}
	}
	lineNumWidth := len(fmt.Sprintf("%d", maxEnd))
	fmtStr := fmt.Sprintf("%%%dd│ %%s", lineNumWidth)
	omitFmt := fmt.Sprintf("%%%ds│ [%%d lines omitted]", lineNumWidth)

	var sb strings.Builder
	var raw strings.Builder
	prevEnd := 0
	actualEnd := 0
	for _, rng := range merged {
		s := rng.start
		e := rng.end
		if s > totalLines {
			continue
		}
		if e > totalLines {
			e = totalLines
		}
		if prevEnd > 0 && s > prevEnd+1 {
			fmt.Fprintf(&sb, omitFmt+"\n", "~", s-prevEnd-1)
			fmt.Fprintf(&raw, "[%d lines omitted]\n", s-prevEnd-1)
		}
		for i, line := range allLines[s-1 : e] {
			fmt.Fprintf(&sb, fmtStr, s+i, line)
			sb.WriteByte('\n')
			raw.WriteString(line)
			raw.WriteByte('\n')
		}
		prevEnd = e
		actualEnd = e
	}

	// Also truncated if the merged ranges don't cover the full file.
	if actualEnd < totalLines {
		truncated = true
	}

	return rangesResult{
		merged:     merged,
		content:    strings.TrimRight(sb.String(), "\n"),
		rawContent: strings.TrimRight(raw.String(), "\n"),
		actualEnd:  actualEnd,
		truncated:  truncated,
	}
}

// ── file_write ────────────────────────────────────────────────────────────────

func fileWrite() tool.Tool {
	return tool.New("file_write",
		"Write content to a file, creating it if it doesn't exist. Fails if file exists unless overwrite=true. Creates parent directories as needed.",
		func(ctx tool.Ctx, p FileWriteParams) (tool.Result, error) {
			if p.Path == "" {
				return tool.Error("path cannot be empty"), nil
			}
			if len(p.Content) > maxWriteSize {
				return tool.Errorf("content too large (%s, max 1MB)", humanize.Size(int64(len(p.Content)))), nil
			}
			path := resolvePath(p.Path, ctx.WorkDir())
			// Check if file exists and overwrite is not set
			if _, err := os.Stat(path); err == nil && !p.Overwrite {
				return tool.Errorf("file already exists: %s (use overwrite=true to replace)", path), nil
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return nil, fmt.Errorf("create dirs: %w", err)
			}
			if err := os.WriteFile(path, []byte(p.Content), 0644); err != nil {
				return nil, fmt.Errorf("write %s: %w", path, err)
			}
			lines := strings.Count(p.Content, "\n")
			if !strings.HasSuffix(p.Content, "\n") && len(p.Content) > 0 {
				lines++
			}
			return tool.Textf("Wrote %s to %s (%d lines)", humanize.Size(int64(len(p.Content))), path, lines), nil
		},
		fileWriteIntent(),
	)
}

// ── file_stat ─────────────────────────────────────────────────────────────────

func fileStat() tool.Tool {
	return tool.New("file_stat",
		"Get detailed metadata about a file or directory: type, size, lines, permissions, modification time.",
		func(ctx tool.Ctx, p FileStatParams) (tool.Result, error) {
			path := resolvePath(p.Path, ctx.WorkDir())
			linfo, err := os.Lstat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return tool.Textf("path: %s\nexists: false", path), nil
				}
				return nil, fmt.Errorf("lstat %s: %w", path, err)
			}

			var sb strings.Builder
			fmt.Fprintf(&sb, "path: %s\nexists: true\n", path)

			if linfo.Mode()&os.ModeSymlink != 0 {
				target, _ := os.Readlink(path)
				fmt.Fprintf(&sb, "type: symlink\ntarget: %s\n", target)
				if info, err := os.Stat(path); err == nil {
					fmt.Fprintf(&sb, "size: %s\n", humanize.Size(info.Size()))
					fmt.Fprintf(&sb, "isDir: %v\n", info.IsDir())
				}
			} else if linfo.IsDir() {
				fmt.Fprintf(&sb, "type: directory\n")
				entries, _ := os.ReadDir(path)
				fmt.Fprintf(&sb, "entries: %d\n", len(entries))
			} else {
				fmt.Fprintf(&sb, "type: file\n")
				fmt.Fprintf(&sb, "size: %s (%d bytes)\n", humanize.Size(linfo.Size()), linfo.Size())
				if isBin, _ := isBinaryFile(path); isBin {
					fmt.Fprintf(&sb, "binary: true\n")
				} else {
					if lines, _ := countFileLines(path); lines > 0 {
						fmt.Fprintf(&sb, "lines: %d\n", lines)
					}
				}
			}
			fmt.Fprintf(&sb, "permissions: %s\n", formatPermissions(linfo.Mode()))
			fmt.Fprintf(&sb, "modified: %s (%s ago)\n", linfo.ModTime().Format(time.RFC3339), humanDuration(time.Since(linfo.ModTime())))
			return tool.Text(strings.TrimSpace(sb.String())), nil
		},
		fileStatIntent(),
	)
}

// ── file_copy ─────────────────────────────────────────────────────────────────

func fileCopy() tool.Tool {
	return tool.New("file_copy",
		"Copy a file or, with recursive=true, a directory. Refuses symlinks and existing destinations unless overwrite=true.",
		func(ctx tool.Ctx, p FileCopyParams) (tool.Result, error) {
			if p.Src == "" {
				return tool.Error("src cannot be empty"), nil
			}
			if p.Dst == "" {
				return tool.Error("dst cannot be empty"), nil
			}
			src := resolvePath(p.Src, ctx.WorkDir())
			dst := resolvePath(p.Dst, ctx.WorkDir())
			stats, err := prepareCopyMove(src, dst, p.Overwrite)
			if err != nil {
				return err, nil
			}
			if stats.srcInfo.IsDir() {
				if !p.Recursive {
					return tool.Errorf("source is a directory: %s (use recursive=true to copy directories)", src), nil
				}
				if isSubpath(src, dst) {
					return tool.Errorf("refusing to copy directory into itself: %s -> %s", src, dst), nil
				}
				if stats.dstExists {
					if err := removeExistingDestination(dst, stats.dstInfo); err != nil {
						return err, nil
					}
				}
				counts, err := copyDir(src, dst, stats.srcInfo.Mode().Perm())
				if err != nil {
					return nil, err
				}
				return tool.Textf("Copied directory %s to %s (%d file(s), %d directories)", src, dst, counts.files, counts.dirs), nil
			}
			if stats.dstExists {
				if err := removeExistingDestination(dst, stats.dstInfo); err != nil {
					return err, nil
				}
			}
			if err := copyFile(src, dst, stats.srcInfo.Mode().Perm()); err != nil {
				return nil, err
			}
			return tool.Textf("Copied file %s to %s (%s)", src, dst, humanize.Size(stats.srcInfo.Size())), nil
		},
		fileCopyIntent(),
	)
}

// ── file_move ─────────────────────────────────────────────────────────────────

func fileMove() tool.Tool {
	return tool.New("file_move",
		"Move or rename a file or directory. Refuses symlinks and existing destinations unless overwrite=true.",
		func(ctx tool.Ctx, p FileMoveParams) (tool.Result, error) {
			if p.Src == "" {
				return tool.Error("src cannot be empty"), nil
			}
			if p.Dst == "" {
				return tool.Error("dst cannot be empty"), nil
			}
			src := resolvePath(p.Src, ctx.WorkDir())
			dst := resolvePath(p.Dst, ctx.WorkDir())
			stats, err := prepareCopyMove(src, dst, p.Overwrite)
			if err != nil {
				return err, nil
			}
			if stats.srcInfo.IsDir() && isSubpath(src, dst) {
				return tool.Errorf("refusing to move directory into itself: %s -> %s", src, dst), nil
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
				return nil, fmt.Errorf("create destination parent directory: %w", err)
			}
			if stats.dstExists {
				if err := removeExistingDestination(dst, stats.dstInfo); err != nil {
					return err, nil
				}
			}
			if err := os.Rename(src, dst); err != nil {
				return nil, fmt.Errorf("move %s to %s: %w", src, dst, err)
			}
			kind := "file"
			if stats.srcInfo.IsDir() {
				kind = "directory"
			}
			return tool.Textf("Moved %s %s to %s", kind, src, dst), nil
		},
		fileMoveIntent(),
	)
}

// ── file_delete ───────────────────────────────────────────────────────────────

func fileDelete() tool.Tool {
	return tool.New("file_delete",
		"Delete a regular file. Refuses to delete directories or symlinks.",
		func(ctx tool.Ctx, p FileDeleteParams) (tool.Result, error) {
			if p.Path == "" {
				return tool.Error("path cannot be empty"), nil
			}
			path := resolvePath(p.Path, ctx.WorkDir())
			linfo, err := os.Lstat(path)
			if err != nil {
				if os.IsNotExist(err) {
					return tool.Errorf("file not found: %s", path), nil
				}
				return nil, fmt.Errorf("lstat %s: %w", path, err)
			}
			if linfo.IsDir() {
				return tool.Errorf("refusing to delete directory: %s (use bash rm -rf if intentional)", path), nil
			}
			if linfo.Mode()&os.ModeSymlink != 0 {
				return tool.Errorf("refusing to delete symlink: %s", path), nil
			}
			if err := os.Remove(path); err != nil {
				return nil, fmt.Errorf("delete %s: %w", path, err)
			}
			return tool.Textf("Deleted %s", path), nil
		},
		fileDeleteIntent(),
	)
}

// ── glob ──────────────────────────────────────────────────────────────────────

func glob() tool.Tool {
	return tool.New("glob",
		"Find files matching a glob pattern (supports ** for recursive matching).",
		func(ctx tool.Ctx, p GlobParams) (tool.Result, error) {
			if p.Pattern == "" {
				return tool.Error("pattern cannot be empty"), nil
			}
			searchRoot := ctx.WorkDir()
			if p.Path != "" {
				searchRoot = resolvePath(p.Path, ctx.WorkDir())
			}

			var gi *ignore.GitIgnore
			if !p.NoGitignore {
				gi = loadGitignore(searchRoot)
			}

			fsys := os.DirFS(searchRoot)
			matches, err := doublestar.Glob(fsys, p.Pattern)
			if err != nil {
				return tool.Errorf("glob %q: %v", p.Pattern, err), nil
			}

			var filtered []string
			for _, m := range matches {
				if gi != nil && gi.MatchesPath(m) {
					continue
				}
				// Skip directories
				if info, err := os.Stat(filepath.Join(searchRoot, m)); err == nil && info.IsDir() {
					continue
				}
				filtered = append(filtered, m)
				if len(filtered) >= maxGlobResults {
					break
				}
			}

			if len(filtered) == 0 {
				return tool.Textf("No files matched pattern %q in %s", p.Pattern, searchRoot), nil
			}
			sort.Strings(filtered)
			return tool.Textf("Found %d file(s):\n%s", len(filtered), strings.Join(filtered, "\n")), nil
		},
		globIntent(),
	)
}

// ── grep ──────────────────────────────────────────────────────────────────────

func grep() tool.Tool {
	return tool.New("grep",
		"Search file contents using regex. "+
			"Prefer this over bash grep for all searches. "+
			"Only fall back to bash when piping grep output into another command (e.g. | wc -l, | head).",
		func(ctx tool.Ctx, p GrepParams) (tool.Result, error) {
			if p.Pattern == "" {
				return tool.Error("pattern cannot be empty"), nil
			}
			if p.Paths == nil || len(*p.Paths) == 0 {
				defaultPaths := tool.StringSliceParam{"."}
				p.Paths = &defaultPaths
			}
			re, err := regexp.Compile(p.Pattern)
			if err != nil {
				return tool.Errorf("invalid regex %q: %v", p.Pattern, err), nil
			}

			workdir := ctx.WorkDir()
			var gi *ignore.GitIgnore
			if !p.NoGitignore {
				gi = loadGitignore(workdir)
			}

			showContent := p.ShowContent || p.ContextLines > 0

			// Collect candidate files
			var candidates []string
			seen := make(map[string]bool)
			for _, pathOrGlob := range *p.Paths {
				abs := resolvePath(pathOrGlob, workdir)
				// Check if it's a direct file path first
				if info, err := os.Stat(abs); err == nil && !info.IsDir() {
					if !seen[abs] {
						seen[abs] = true
						candidates = append(candidates, abs)
					}
					continue
				}
				// Directory: expand to recursive glob
				if info, err := os.Stat(abs); err == nil && info.IsDir() {
					globPattern := expandDirToGlob(pathOrGlob)
					fsys := os.DirFS(workdir)
					matches, _ := doublestar.Glob(fsys, globPattern)
					for _, m := range matches {
						full := filepath.Join(workdir, m)
						if gi != nil && gi.MatchesPath(m) {
							continue
						}
						if info, err := os.Stat(full); err == nil && !info.IsDir() && !seen[full] {
							seen[full] = true
							candidates = append(candidates, full)
						}
					}
					continue
				}
				// Try as glob pattern
				fsys := os.DirFS(workdir)
				matches, _ := doublestar.Glob(fsys, pathOrGlob)
				for _, m := range matches {
					full := filepath.Join(workdir, m)
					if gi != nil && gi.MatchesPath(m) {
						continue
					}
					if info, err := os.Stat(full); err == nil && !info.IsDir() && !seen[full] {
						seen[full] = true
						candidates = append(candidates, full)
					}
				}
			}

			type matchLine struct {
				num  int
				text string
			}
			type fileMatch struct {
				path    string
				matches []matchLine
			}

			var results []fileMatch
			totalMatches := 0

			for _, file := range candidates {
				if len(results) >= maxGrepFiles || totalMatches >= maxGrepMatches {
					break
				}
				data, err := os.ReadFile(file)
				if err != nil {
					continue
				}
				if isBin, _ := isBinaryFile(file); isBin {
					continue
				}

				fileLines := strings.Split(string(data), "\n")
				var fileMatches []matchLine
				for i, line := range fileLines {
					if re.MatchString(line) {
						if showContent {
							ctx := p.ContextLines
							start := i - ctx
							if start < 0 {
								start = 0
							}
							end := i + ctx
							if end >= len(fileLines) {
								end = len(fileLines) - 1
							}
							for j := start; j <= end; j++ {
								fileMatches = append(fileMatches, matchLine{num: j + 1, text: fileLines[j]})
							}
						} else {
							fileMatches = append(fileMatches, matchLine{num: i + 1})
						}
						totalMatches++
					}
				}
				if len(fileMatches) > 0 {
					rel, _ := filepath.Rel(workdir, file)
					results = append(results, fileMatch{path: rel, matches: fileMatches})
				}
			}

			if len(results) == 0 {
				var sb strings.Builder
				sb.WriteString("No matches found.\n\nChecked files:\n")
				for _, f := range candidates {
					rel, _ := filepath.Rel(workdir, f)
					fmt.Fprintf(&sb, "  %s\n", rel)
				}
				if len(candidates) == 0 {
					sb.WriteString("  (no files matched the path/pattern)\n")
				}
				return tool.Text(strings.TrimSpace(sb.String())), nil
			}

			var sb strings.Builder
			for _, fm := range results {
				if showContent {
					fmt.Fprintf(&sb, "%s:\n", fm.path)
					for _, m := range fm.matches {
						fmt.Fprintf(&sb, "  %d: %s\n", m.num, m.text)
					}
				} else {
					lines := make([]string, len(fm.matches))
					for i, m := range fm.matches {
						lines[i] = fmt.Sprintf("%d", m.num)
					}
					fmt.Fprintf(&sb, "%s: lines %s\n", fm.path, strings.Join(lines, ","))
				}
			}
			return tool.Text(strings.TrimSpace(sb.String())), nil
		},
		grepIntent(),
	)
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func resolvePath(path, workdir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workdir, path)
}

type copyMoveStats struct {
	srcInfo   os.FileInfo
	dstInfo   os.FileInfo
	dstExists bool
}

type copyCounts struct {
	files int
	dirs  int
}

func prepareCopyMove(src, dst string, overwrite bool) (copyMoveStats, tool.Result) {
	var stats copyMoveStats
	if src == dst {
		return stats, tool.Error("source and destination are the same path")
	}

	srcInfo, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return stats, tool.Errorf("source not found: %s", src)
		}
		return stats, tool.Errorf("lstat source %s: %v", src, err)
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 {
		return stats, tool.Errorf("refusing to copy or move symlink source: %s", src)
	}
	stats.srcInfo = srcInfo

	dstInfo, err := os.Lstat(dst)
	if err == nil {
		if dstInfo.Mode()&os.ModeSymlink != 0 {
			return stats, tool.Errorf("refusing to replace symlink destination: %s", dst)
		}
		if !overwrite {
			return stats, tool.Errorf("destination already exists: %s (use overwrite=true to replace)", dst)
		}
		stats.dstInfo = dstInfo
		stats.dstExists = true
		return stats, nil
	}
	if !os.IsNotExist(err) {
		return stats, tool.Errorf("lstat destination %s: %v", dst, err)
	}
	return stats, nil
}

func removeExistingDestination(path string, info os.FileInfo) tool.Result {
	if info.Mode()&os.ModeSymlink != 0 {
		return tool.Errorf("refusing to replace symlink destination: %s", path)
	}
	if info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return tool.Errorf("remove existing destination directory %s: %v", path, err)
		}
		return nil
	}
	if err := os.Remove(path); err != nil {
		return tool.Errorf("remove existing destination file %s: %v", path, err)
	}
	return nil
}

func copyFile(src, dst string, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return fmt.Errorf("create destination parent directory: %w", err)
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, perm)
	if err != nil {
		return fmt.Errorf("create destination %s: %w", dst, err)
	}
	defer func() { _ = out.Close() }()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s to %s: %w", src, dst, err)
	}
	return nil
}

func copyDir(src, dst string, perm os.FileMode) (copyCounts, error) {
	counts := copyCounts{dirs: 1}
	if err := os.MkdirAll(dst, perm); err != nil {
		return counts, fmt.Errorf("create destination directory %s: %w", dst, err)
	}
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == src {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing to copy symlink: %s", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return fmt.Errorf("create directory %s: %w", target, err)
			}
			counts.dirs++
			return nil
		}
		if err := copyFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}
		counts.files++
		return nil
	})
	return counts, err
}

func isSubpath(parent, child string) bool {
	parentClean, err := filepath.Abs(parent)
	if err != nil {
		parentClean = filepath.Clean(parent)
	}
	childClean, err := filepath.Abs(child)
	if err != nil {
		childClean = filepath.Clean(child)
	}
	rel, err := filepath.Rel(parentClean, childClean)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

// expandDirToGlob converts a directory path to a recursive glob pattern.
// "." → "**/*"
// "tools" → "tools/**/*"
// "tools/**/*" → "tools/**/*" (already a glob, no change)
func expandDirToGlob(path string) string {
	// Already a glob pattern?
	if strings.Contains(path, "*") {
		return path
	}
	// Append recursive match
	if path == "." {
		return "**/*"
	}
	return path + "/**/*"
}

func isSymlink(path string) (bool, string, error) {
	linfo, err := os.Lstat(path)
	if err != nil {
		return false, "", err
	}
	if linfo.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(path)
		return true, target, nil
	}
	return false, "", nil
}

func isBinaryFile(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()

	buf := make([]byte, binaryCheckSize)
	n, err := f.Read(buf)
	if err != nil && n == 0 {
		return false, nil
	}
	buf = buf[:n]
	for _, b := range buf {
		if b == 0 {
			return true, nil
		}
	}
	return false, nil
}

func countFileLines(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	n := 0
	for scanner.Scan() {
		n++
	}
	return n, scanner.Err()
}

func humanTime(t time.Time) string {
	return humanDuration(time.Since(t))
}

func humanDuration(d time.Duration) string {
	d = d.Abs()
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		m := int(d.Minutes())
		if m == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", m)
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", h)
	case d < 30*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	case d < 365*24*time.Hour:
		months := int(d.Hours() / (24 * 30))
		if months == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", months)
	default:
		years := int(d.Hours() / (24 * 365))
		if years == 1 {
			return "1 year"
		}
		return fmt.Sprintf("%d years", years)
	}
}

func formatPermissions(mode fs.FileMode) string {
	perm := mode.Perm()
	var buf [9]byte
	const rwx = "rwxrwxrwx"
	for i := 0; i < 9; i++ {
		if perm&(1<<uint(8-i)) != 0 {
			buf[i] = rwx[i]
		} else {
			buf[i] = '-'
		}
	}
	return string(buf[:])
}

func loadGitignore(workdir string) *ignore.GitIgnore {
	path := filepath.Join(workdir, ".gitignore")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	gi, err := ignore.CompileIgnoreFile(path)
	if err != nil {
		return nil
	}
	return gi
}

func formatFileTreeEntry(path, name string, showSize, showLines bool) string {
	var parts []string
	if showSize {
		if info, err := os.Stat(path); err == nil {
			parts = append(parts, humanize.Size(info.Size()))
		}
	}
	if showLines {
		if binary, err := isBinaryFile(path); err == nil && !binary {
			if lines, err := countFileLines(path); err == nil {
				parts = append(parts, fmt.Sprintf("%dL", lines))
			}
		}
	}
	if len(parts) == 0 {
		return name
	}
	return fmt.Sprintf("%s (%s)", name, strings.Join(parts, ", "))
}

func buildDirTree(
	rootPath, absPath, prefix string,
	maxDepth, curDepth int,
	showHidden bool,
	pattern string,
	showSize bool,
	showLines bool,
	gi *ignore.GitIgnore,
	lines *[]string,
	totalNodes *int,
	truncated *bool,
	maxNodes int,
) {
	if *truncated {
		return
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool {
		di, dj := entries[i].IsDir(), entries[j].IsDir()
		if di != dj {
			return di
		}
		return entries[i].Name() < entries[j].Name()
	})

	var filtered []os.DirEntry
	for _, e := range entries {
		if !showHidden && e.Name() != "" && e.Name()[0] == '.' {
			continue
		}
		if gi != nil {
			rel, err := filepath.Rel(rootPath, filepath.Join(absPath, e.Name()))
			if err == nil {
				check := rel
				if e.IsDir() {
					check = rel + "/"
				}
				if gi.MatchesPath(check) {
					continue
				}
			}
		}
		if pattern != "" && !e.IsDir() {
			matched, err := filepath.Match(pattern, e.Name())
			if err != nil || !matched {
				continue
			}
		}
		filtered = append(filtered, e)
	}

	for i, entry := range filtered {
		if *truncated {
			return
		}
		isLast := i == len(filtered)-1
		connector, childPrefix := "├── ", prefix+"│   "
		if isLast {
			connector, childPrefix = "└── ", prefix+"    "
		}
		displayName := entry.Name()
		if entry.IsDir() {
			displayName += "/"
		} else if showSize || showLines {
			displayName = formatFileTreeEntry(filepath.Join(absPath, entry.Name()), displayName, showSize, showLines)
		}
		*lines = append(*lines, prefix+connector+displayName)
		*totalNodes++
		if *totalNodes >= maxNodes {
			*lines = append(*lines, childPrefix+fmt.Sprintf("... (truncated at %d entries)", maxNodes))
			*truncated = true
			return
		}
		if entry.IsDir() && curDepth+1 < maxDepth {
			buildDirTree(rootPath, filepath.Join(absPath, entry.Name()), childPrefix,
				maxDepth, curDepth+1, showHidden, pattern, showSize, showLines, gi,
				lines, totalNodes, truncated, maxNodes)
		}
	}
}

func collectFiles(
	rootPath, absPath string,
	maxDepth, curDepth int,
	showHidden bool,
	pattern string,
	showSize bool,
	showLines bool,
	gi *ignore.GitIgnore,
	lines *[]string,
	totalNodes *int,
	truncated *bool,
	maxNodes int,
) {
	if *truncated {
		return
	}
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if *truncated {
			return
		}
		name := entry.Name()
		if !showHidden && name != "" && name[0] == '.' {
			continue
		}
		fullPath := filepath.Join(absPath, name)
		relPath, err := filepath.Rel(rootPath, fullPath)
		if err != nil {
			continue
		}
		if gi != nil {
			check := relPath
			if entry.IsDir() {
				check = relPath + "/"
			}
			if gi.MatchesPath(check) {
				continue
			}
		}
		if entry.IsDir() {
			if curDepth+1 < maxDepth {
				collectFiles(rootPath, fullPath, maxDepth, curDepth+1,
					showHidden, pattern, showSize, showLines, gi, lines, totalNodes, truncated, maxNodes)
			}
		} else {
			if pattern != "" {
				if matched, err := filepath.Match(pattern, name); err != nil || !matched {
					continue
				}
			}
			displayName := relPath
			if showSize || showLines {
				displayName = formatFileTreeEntry(fullPath, relPath, showSize, showLines)
			}
			*lines = append(*lines, displayName)
			*totalNodes++
			if *totalNodes >= maxNodes {
				*lines = append(*lines, fmt.Sprintf("... (truncated at %d files)", maxNodes))
				*truncated = true
				return
			}
		}
	}
}

// ── Patch helpers (ported from agentsdk) ─────────────────────────────────────

type patchResult struct {
	lines              []string
	hunksApplied       int
	linesAdded         int
	linesRemoved       int
	addTrailingNewline bool
}

type patchHunk struct {
	origStart int
	origCount int
	newStart  int
	newCount  int
	lines     []string
}

func applyUnifiedPatch(fileLines []string, patch string) (*patchResult, error) {
	hunks, addTrailing, err := parseUnifiedDiff(patch)
	if err != nil {
		return nil, fmt.Errorf("parse patch: %w", err)
	}
	if len(hunks) == 0 {
		return nil, fmt.Errorf("patch contains no hunks")
	}

	out := make([]string, 0, len(fileLines))
	fileIdx := 0
	totalAdded, totalRemoved := 0, 0

	for hunkIdx, h := range hunks {
		hunkStart := h.origStart - 1
		if h.origStart == 0 && h.origCount == 0 {
			hunkStart = 0
		}
		if hunkStart < fileIdx {
			return nil, fmt.Errorf("hunk %d: overlaps with previous hunk", hunkIdx+1)
		}
		if hunkStart > len(fileLines) {
			return nil, fmt.Errorf("hunk %d: starts at line %d but file has %d lines", hunkIdx+1, h.origStart, len(fileLines))
		}

		out = append(out, fileLines[fileIdx:hunkStart]...)
		fileIdx = hunkStart

		// Verify context/removed lines
		checkIdx := fileIdx
		for _, dl := range h.lines {
			if dl == "" {
				continue
			}
			prefix, text := dl[0], dl[1:]
			switch prefix {
			case ' ', '-':
				if checkIdx >= len(fileLines) {
					return nil, fmt.Errorf("hunk %d: line %q not found (past end of file)", hunkIdx+1, text)
				}
				if fileLines[checkIdx] != text {
					return nil, fmt.Errorf("hunk %d: mismatch at line %d: expected %q, got %q",
						hunkIdx+1, checkIdx+1, text, fileLines[checkIdx])
				}
				checkIdx++
			}
		}

		// Apply
		for _, dl := range h.lines {
			if dl == "" {
				continue
			}
			prefix, text := dl[0], dl[1:]
			switch prefix {
			case ' ':
				out = append(out, fileLines[fileIdx])
				fileIdx++
			case '-':
				fileIdx++
				totalRemoved++
			case '+':
				out = append(out, text)
				totalAdded++
			}
		}
	}

	out = append(out, fileLines[fileIdx:]...)
	return &patchResult{
		lines:              out,
		hunksApplied:       len(hunks),
		linesAdded:         totalAdded,
		linesRemoved:       totalRemoved,
		addTrailingNewline: addTrailing,
	}, nil
}

func parseUnifiedDiff(patch string) ([]patchHunk, bool, error) {
	lines := strings.Split(patch, "\n")
	var hunks []patchHunk
	addTrailing := false
	i := 0
	for i < len(lines) {
		line := lines[i]
		if strings.HasPrefix(line, "--- ") || strings.HasPrefix(line, "+++ ") {
			i++
			continue
		}
		if strings.HasPrefix(line, "@@ ") {
			h, err := parseHunkHeader(line)
			if err != nil {
				return nil, false, fmt.Errorf("line %d: %w", i+1, err)
			}
			i++
			for i < len(lines) {
				dl := lines[i]
				if strings.HasPrefix(dl, "@@ ") || strings.HasPrefix(dl, "--- ") || strings.HasPrefix(dl, "+++ ") {
					break
				}
				if strings.HasPrefix(dl, `\ `) {
					addTrailing = false
					i++
					continue
				}
				if dl == "" && i == len(lines)-1 {
					i++
					break
				}
				h.lines = append(h.lines, dl)
				i++
			}
			hunks = append(hunks, h)
			continue
		}
		i++
	}
	return hunks, addTrailing, nil
}

func parseHunkHeader(line string) (patchHunk, error) {
	end := strings.Index(line[3:], " @@")
	if end < 0 {
		end = len(line) - 3
	}
	inner := strings.TrimSpace(line[3 : 3+end])
	parts := strings.Fields(inner)
	if len(parts) < 2 {
		return patchHunk{}, fmt.Errorf("invalid hunk header: %q", line)
	}
	os2, oc, err := parseRange(parts[0])
	if err != nil {
		return patchHunk{}, fmt.Errorf("invalid orig range: %w", err)
	}
	ns, nc, err := parseRange(parts[1])
	if err != nil {
		return patchHunk{}, fmt.Errorf("invalid new range: %w", err)
	}
	return patchHunk{origStart: os2, origCount: oc, newStart: ns, newCount: nc}, nil
}

func parseRange(s string) (int, int, error) {
	if s == "" {
		return 0, 0, fmt.Errorf("empty range")
	}
	s = s[1:]
	comma := strings.Index(s, ",")
	if comma < 0 {
		n, err := parseIntStr(s)
		return n, 1, err
	}
	start, err := parseIntStr(s[:comma])
	if err != nil {
		return 0, 0, err
	}
	count, err := parseIntStr(s[comma+1:])
	return start, count, err
}

func parseIntStr(s string) (int, error) {
	n := 0
	if s == "" {
		return 0, fmt.Errorf("empty integer")
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid integer %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
