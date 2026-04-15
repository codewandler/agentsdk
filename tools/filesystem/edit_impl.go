package filesystem

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/codewandler/core/tool"
	idiff "github.com/codewandler/core/internal/diff"
	"github.com/codewandler/core/internal/humanize"
)

var (
	readFileForEdit   = os.ReadFile
	writeFileForEdit  = os.WriteFile
	renameFileForEdit = os.Rename
)

// ── file_edit ─────────────────────────────────────────────────────────────────

type FileEditParams struct {
	Path         tool.StringSliceParam `json:"path"`
	DryRun       bool                  `json:"dry_run,omitempty"`
	AllowPartial bool                  `json:"allow_partial,omitempty"`
	Operations   []Op                  `json:"operations"`
}

// Op is a discriminated union of file edit operations.
// Implementations: ReplaceOp, InsertOp, RemoveOp, AppendOp, PatchOp.
//
// JSONSchemaAlias tells the jsonschema.Reflector to generate a oneOf schema
// from the variant structs below, instead of a generic object.
type Op = Operation

func (o Op) JSONSchemaAlias() any {
	return OpSchema{}
}

// OpSchema is used by the reflector to generate the oneOf schema.
// Each field corresponds to one variant; omitempty ensures the field is
// only present when set, giving the LLM a clear discriminator.
type OpSchema struct {
	Replace *ReplaceOp `json:"replace,omitempty" jsonschema:"description=Replace text."`
	Insert  *InsertOp  `json:"insert,omitempty" jsonschema:"description=Insert content before a line."`
	Remove  *RemoveOp  `json:"remove,omitempty" jsonschema:"description=Remove text or lines."`
	Append  *AppendOp  `json:"append,omitempty" jsonschema:"description=Append content to end of file."`
	Patch   *PatchOp   `json:"patch,omitempty" jsonschema:"description=Apply a unified diff patch."`
}

type Operation struct {
	Replace *ReplaceOp `json:"replace,omitempty"`
	Insert  *InsertOp  `json:"insert,omitempty"`
	Remove  *RemoveOp  `json:"remove,omitempty"`
	Append  *AppendOp  `json:"append,omitempty"`
	Patch   *PatchOp   `json:"patch,omitempty"`
}

func (o Operation) valid() bool {
	n := 0
	if o.Replace != nil {
		n++
	}
	if o.Insert != nil {
		n++
	}
	if o.Remove != nil {
		n++
	}
	if o.Append != nil {
		n++
	}
	if o.Patch != nil {
		n++
	}
	return n == 1
}

type ReplaceOp struct {
	OldString  string `json:"old_string" jsonschema:"description=Exact text to find and replace. Must match the file content exactly including whitespace.,required"`
	NewString  string `json:"new_string" jsonschema:"description=Replacement text. Use empty string to delete old_string."`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=Replace all occurrences (default false - fails if more than one match exists)."`
	IfMissing  string `json:"if_missing,omitempty" jsonschema:"description=If old_string is not found append this content to the file instead of failing. Useful for idempotent edits."`
}

type InsertOp struct {
	Line    int    `json:"line" jsonschema:"description=Line number to insert before (1-indexed). Content is inserted before this line.,required"`
	Content string `json:"content" jsonschema:"description=Text to insert.,required"`
	Indent  string `json:"indent,omitempty" jsonschema:"description=Indentation mode: 'auto' (default) copies the target line's leading whitespace; 'none' preserves content exactly as written."`
}

type RemoveOp struct {
	OldString string `json:"old_string,omitempty" jsonschema:"description=Exact text to find and remove. Mutually exclusive with lines."`
	Lines     []int  `json:"lines,omitempty" jsonschema:"description=Line numbers to remove (1-indexed). One element [n] removes line n. Two elements [start end] removes that inclusive range."`
}

type AppendOp struct {
	Content string `json:"content" jsonschema:"description=Text to append to the end of the file. A newline is added automatically if the file does not end with one.,required"`
}

type PatchOp struct {
	Patch string `json:"patch" jsonschema:"description=Unified diff patch to apply (@@ -L\\,S +L\\,S @@ format).,required"`
}

func fileEdit() tool.Tool {
	const guidance = `All operations are resolved against the original file content, not sequentially against prior edits in the same call.
If operations affect overlapping original regions, the file edit fails with a conflict error.
Multiple inserts/appends at the same position are allowed and keep operation order.
Insert content is used exactly as provided; include a trailing newline yourself if you want to insert a full new line.
dry_run=true shows the diff without writing anything to disk.
allow_partial=true skips files that fail (symlinks, missing, too large, conflicts) instead of aborting all.
SHA-256 hash check: the file is re-read before writing; if it changed since the tool read it, the call fails — re-read and retry.
path accepts a single string, an array, or a glob pattern (e.g. ["src/**/*.go"]).`

	return tool.New("file_edit",
		"Edit files using structured operations: replace, insert, remove, append, and patch operations in a single call. All operations are resolved against the original file content and merged before writing.",
		func(ctx tool.Ctx, p FileEditParams) (tool.Result, error) {
			if len(p.Path) == 0 {
				return tool.Error("path is required"), nil
			}
			if len(p.Operations) == 0 {
				return tool.Error("operations is required"), nil
			}

			for i, op := range p.Operations {
				if !op.valid() {
					return tool.Errorf(
						"operation[%d]: must have exactly one type (replace|insert|remove|append|patch)",
						i,
					), nil
				}
			}

			files := expandPaths(p.Path, ctx.WorkDir())
			if len(files) == 0 {
				return tool.Errorf("no files matched: %v", []string(p.Path)), nil
			}

			type fileState struct {
				info     os.FileInfo
				orig     string
				mod      string
				origHash [32]byte
			}
			states := make(map[string]*fileState)
			var failed []string
			for _, path := range files {
				isLink, _, err := isSymlink(path)
				if err != nil {
					if p.AllowPartial {
						failed = append(failed, fmt.Sprintf("%s: inspect failed: %v", path, err))
						continue
					}
					return tool.Errorf("cannot inspect %s: %v", path, err), nil
				}
				if isLink {
					if p.AllowPartial {
						failed = append(failed, fmt.Sprintf("%s: cannot edit symlink", path))
						continue
					}
					return tool.Errorf("cannot edit symlink: %s", path), nil
				}
				info, err := os.Stat(path)
				if err != nil {
					if p.AllowPartial {
						failed = append(failed, fmt.Sprintf("%s: cannot stat file: %v", path, err))
						continue
					}
					return tool.Errorf("cannot stat file: %s", path), nil
				}
				if info.IsDir() {
					if p.AllowPartial {
						failed = append(failed, fmt.Sprintf("%s: cannot edit directory", path))
						continue
					}
					return tool.Errorf("cannot edit directory: %s", path), nil
				}
				if info.Size() > maxFileSize {
					msg := fmt.Sprintf("%s: too large (%s, max 10MB)", path, humanize.Size(info.Size()))
					if p.AllowPartial {
						failed = append(failed, msg)
						continue
					}
					return tool.Error(msg), nil
				}
				data, err := readFileForEdit(path)
				if err != nil {
					if p.AllowPartial {
						failed = append(failed, fmt.Sprintf("%s: read failed: %v", path, err))
						continue
					}
					return nil, err
				}
				states[path] = &fileState{
					info:     info,
					orig:     string(data),
					mod:      string(data),
					origHash: sha256.Sum256(data),
				}
			}

			for _, path := range files {
				s, ok := states[path]
				if !ok {
					continue
				}
				edits, errs := resolveOperationsAgainstOriginal(s.orig, p.Operations)
				if len(errs) > 0 {
					if !p.AllowPartial {
						var msgs []string
						for _, e := range errs {
							msgs = append(msgs, e.Error())
						}
						return tool.Error(path + ": " + strings.Join(msgs, "; ")), nil
					}
					for _, e := range errs {
						failed = append(failed, path+": "+e.Error())
					}
					continue
				}
				if err := detectConflicts(edits); err != nil {
					if !p.AllowPartial {
						return tool.Error(path + ": " + err.Error()), nil
					}
					failed = append(failed, path+": "+err.Error())
					continue
				}
				newMod, err := applyResolvedEdits(s.orig, edits)
				if err != nil {
					if !p.AllowPartial {
						return tool.Error(path + ": " + err.Error()), nil
					}
					failed = append(failed, path+": "+err.Error())
					continue
				}
				s.mod = newMod
			}

			res := tool.NewResult()
			changed := 0
			for _, path := range files {
				s, ok := states[path]
				if !ok {
					continue
				}
				if s.orig != s.mod {
					changed++
					unified := idiff.Unified(path, s.orig, s.mod)
					added, removed := idiff.Stats(unified)
					res.Display(tool.DiffBlock{Path: path, UnifiedDiff: unified, Added: added, Removed: removed})
				}
			}

			if p.DryRun {
				res.Text(fmt.Sprintf("Dry run: %d file(s) would be modified.", changed))
			} else {
				type pendingWrite struct {
					path string
					info os.FileInfo
					mod  string
					hash [32]byte
				}

				var pending []pendingWrite
				for _, path := range files {
					s, ok := states[path]
					if !ok || s.orig == s.mod {
						continue
					}
					pending = append(pending, pendingWrite{
						path: path,
						info: s.info,
						mod:  s.mod,
						hash: s.origHash,
					})
				}

				var verified []pendingWrite
				for _, pw := range pending {
					cur, err := readFileForEdit(pw.path)
					if err != nil {
						if !p.AllowPartial {
							return tool.Errorf("verify %s: %v", pw.path, err), nil
						}
						failed = append(failed, fmt.Sprintf("%s: verify failed: %v", pw.path, err))
						continue
					}
					if sha256.Sum256(cur) != pw.hash {
						if !p.AllowPartial {
							return tool.Errorf(
								"File changed since read (hash mismatch).\nOriginal: %x\nCurrent:  %x\nPlease re-read and retry.",
								pw.hash, sha256.Sum256(cur),
							), nil
						}
						failed = append(failed, fmt.Sprintf("%s: file changed since read", pw.path))
						continue
					}
					verified = append(verified, pw)
				}

				for _, pw := range verified {
					tmp := pw.path + ".tmp"
					if err := writeFileForEdit(tmp, []byte(pw.mod), pw.info.Mode().Perm()); err != nil {
						if !p.AllowPartial {
							return tool.Errorf("write %s: %v", pw.path, err), nil
						}
						failed = append(failed, fmt.Sprintf("%s: write failed: %v", pw.path, err))
						continue
					}
				}

				for _, pw := range verified {
					tmp := pw.path + ".tmp"
					if _, err := os.Stat(tmp); err != nil {
						continue
					}
					if err := renameFileForEdit(tmp, pw.path); err != nil {
						_ = os.Remove(tmp)
						if !p.AllowPartial {
							return tool.Errorf("rename %s: %v", pw.path, err), nil
						}
						failed = append(failed, fmt.Sprintf("%s: rename failed: %v", pw.path, err))
					}
				}
				if changed > 0 {
					res.Textf("Summary: %d file(s) edited.", changed)
				}
			}

			if len(failed) > 0 {
				res.Textf("\n%d failed (allow_partial=true): %s", len(failed), strings.Join(failed, "; "))
			}

			return res.Build(), nil
		},
		tool.WithGuidance[FileEditParams](guidance),
	)
}

type resolvedEdit struct {
	opIndex   int
	kind      string
	startByte int
	endByte   int
	startLine int
	endLine   int
	newText   string
}

func resolveOperationsAgainstOriginal(content string, ops []Operation) ([]resolvedEdit, []error) {
	var edits []resolvedEdit
	for i, op := range ops {
		resolved, err := resolveSingleOperation(content, op, i)
		if err != nil {
			return nil, []error{fmt.Errorf("op[%d]: %w", i, err)}
		}
		edits = append(edits, resolved...)
	}
	return edits, nil
}

func resolveSingleOperation(content string, op Operation, opIndex int) ([]resolvedEdit, error) {
	switch {
	case op.Replace != nil:
		return resolveReplace(content, op.Replace, opIndex)
	case op.Insert != nil:
		return resolveInsert(content, op.Insert, opIndex)
	case op.Remove != nil:
		return resolveRemove(content, op.Remove, opIndex)
	case op.Append != nil:
		return resolveAppend(content, op.Append, opIndex), nil
	case op.Patch != nil:
		return resolvePatch(content, op.Patch, opIndex)
	default:
		return nil, fmt.Errorf("unknown operation")
	}
}

func detectConflicts(edits []resolvedEdit) error {
	sorted := append([]resolvedEdit(nil), edits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].startByte != sorted[j].startByte {
			return sorted[i].startByte < sorted[j].startByte
		}
		if zeroWidth(sorted[i]) != zeroWidth(sorted[j]) {
			return zeroWidth(sorted[i])
		}
		if sorted[i].endByte != sorted[j].endByte {
			return sorted[i].endByte < sorted[j].endByte
		}
		return sorted[i].opIndex < sorted[j].opIndex
	})

	for i := 0; i < len(sorted); i++ {
		for j := i + 1; j < len(sorted); j++ {
			a := sorted[i]
			b := sorted[j]
			if a.endByte < b.startByte {
				break
			}
			if conflict, msg := editsConflict(a, b); conflict {
				return fmt.Errorf("operation[%d] conflicts with operation[%d]: %s", b.opIndex, a.opIndex, msg)
			}
		}
	}
	return nil
}

func applyResolvedEdits(content string, edits []resolvedEdit) (string, error) {
	sorted := append([]resolvedEdit(nil), edits...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].startByte != sorted[j].startByte {
			return sorted[i].startByte < sorted[j].startByte
		}
		if zeroWidth(sorted[i]) != zeroWidth(sorted[j]) {
			return zeroWidth(sorted[i])
		}
		if sorted[i].endByte != sorted[j].endByte {
			return sorted[i].endByte < sorted[j].endByte
		}
		return sorted[i].opIndex < sorted[j].opIndex
	})

	var b strings.Builder
	cursor := 0
	for _, edit := range sorted {
		if edit.startByte < cursor {
			return "", fmt.Errorf("overlapping edits during apply")
		}
		if edit.startByte > len(content) || edit.endByte > len(content) || edit.startByte > edit.endByte {
			return "", fmt.Errorf("invalid edit range")
		}
		b.WriteString(content[cursor:edit.startByte])
		b.WriteString(edit.newText)
		cursor = edit.endByte
	}
	b.WriteString(content[cursor:])
	return b.String(), nil
}

func zeroWidth(edit resolvedEdit) bool {
	return edit.startByte == edit.endByte
}

func editsConflict(a, b resolvedEdit) (bool, string) {
	if zeroWidth(a) && zeroWidth(b) {
		if a.startByte == b.startByte {
			return false, ""
		}
		return false, ""
	}
	if zeroWidth(a) {
		if a.startByte > b.startByte && a.startByte < b.endByte {
			return true, fmt.Sprintf("insert target falls inside modified region lines %d-%d", b.startLine, b.endLine)
		}
		return false, ""
	}
	if zeroWidth(b) {
		if b.startByte > a.startByte && b.startByte < a.endByte {
			return true, fmt.Sprintf("insert target falls inside modified region lines %d-%d", a.startLine, a.endLine)
		}
		return false, ""
	}
	if a.startByte < b.endByte && b.startByte < a.endByte {
		return true, fmt.Sprintf("both modify original file region lines %d-%d", maxInt(a.startLine, b.startLine), minPositiveEnd(a.endLine, b.endLine))
	}
	return false, ""
}

func resolveReplace(content string, op *ReplaceOp, opIndex int) ([]resolvedEdit, error) {
	if op.OldString == "" {
		return nil, fmt.Errorf("old_string required")
	}
	matches := findAllStringMatches(content, op.OldString)
	if len(matches) == 0 {
		if op.IfMissing != "" {
			line := lineNumberForOffset(content, len(content))
			return []resolvedEdit{{
				opIndex:   opIndex,
				kind:      "insert",
				startByte: len(content),
				endByte:   len(content),
				startLine: line,
				endLine:   line,
				newText:   op.IfMissing,
			}}, nil
		}
		return nil, fmt.Errorf("old_string not found")
	}
	if len(matches) > 1 && !op.ReplaceAll {
		return nil, fmt.Errorf("multiple matches (%d); set replace_all=true", len(matches))
	}
	if !op.ReplaceAll {
		m := matches[0]
		return []resolvedEdit{{
			opIndex:   opIndex,
			kind:      "replace",
			startByte: m.start,
			endByte:   m.end,
			startLine: lineNumberForOffset(content, m.start),
			endLine:   lineNumberForOffset(content, maxInt(m.end-1, m.start)),
			newText:   op.NewString,
		}}, nil
	}
	edits := make([]resolvedEdit, 0, len(matches))
	for _, m := range matches {
		edits = append(edits, resolvedEdit{
			opIndex:   opIndex,
			kind:      "replace",
			startByte: m.start,
			endByte:   m.end,
			startLine: lineNumberForOffset(content, m.start),
			endLine:   lineNumberForOffset(content, maxInt(m.end-1, m.start)),
			newText:   op.NewString,
		})
	}
	return edits, nil
}

func resolveInsert(content string, op *InsertOp, opIndex int) ([]resolvedEdit, error) {
	if op.Line < 1 {
		return nil, fmt.Errorf("line must be >= 1")
	}
	targetLine := op.Line
	lines := splitLinesPreserveShape(content)
	maxLine := len(lines) + 1
	if targetLine > maxLine {
		targetLine = maxLine
	}
	offset := byteOffsetForLine(content, targetLine)
	insertText := op.Content
	if op.Indent == "" || op.Indent == "auto" {
		if targetLine <= len(lines) {
			ws := getIndent(lines[targetLine-1])
			insertText = indentInsertedContent(insertText, ws)
		}
	}
	line := lineNumberForOffset(content, offset)
	return []resolvedEdit{{
		opIndex:   opIndex,
		kind:      "insert",
		startByte: offset,
		endByte:   offset,
		startLine: line,
		endLine:   line,
		newText:   insertText,
	}}, nil
}

func resolveRemove(content string, op *RemoveOp, opIndex int) ([]resolvedEdit, error) {
	if op.OldString != "" {
		idx := strings.Index(content, op.OldString)
		if idx < 0 {
			return nil, fmt.Errorf("old_string not found")
		}
		end := idx + len(op.OldString)
		return []resolvedEdit{{
			opIndex:   opIndex,
			kind:      "remove",
			startByte: idx,
			endByte:   end,
			startLine: lineNumberForOffset(content, idx),
			endLine:   lineNumberForOffset(content, maxInt(end-1, idx)),
		}}, nil
	}
	if len(op.Lines) == 0 {
		return nil, fmt.Errorf("must specify old_string or lines")
	}
	if len(op.Lines) != 1 && len(op.Lines) != 2 {
		return nil, fmt.Errorf("lines must contain 1 element [n] or 2 elements [start, end]")
	}
	lines := splitLinesPreserveShape(content)
	startLine := op.Lines[0]
	endLine := startLine
	if len(op.Lines) == 2 {
		endLine = op.Lines[1]
		if startLine < 1 || endLine < startLine || endLine > len(lines) {
			return nil, fmt.Errorf("invalid line range")
		}
	} else if startLine < 1 || startLine > len(lines) {
		return nil, fmt.Errorf("invalid line number")
	}
	startByte := byteOffsetForLine(content, startLine)
	endByte := byteOffsetForLine(content, endLine+1)
	return []resolvedEdit{{
		opIndex:   opIndex,
		kind:      "remove",
		startByte: startByte,
		endByte:   endByte,
		startLine: startLine,
		endLine:   endLine,
	}}, nil
}

func resolveAppend(content string, op *AppendOp, opIndex int) []resolvedEdit {
	if op.Content == "" {
		return nil
	}
	newText := op.Content
	if content != "" && content[len(content)-1] != '\n' {
		newText = "\n" + newText
	}
	line := lineNumberForOffset(content, len(content))
	return []resolvedEdit{{
		opIndex:   opIndex,
		kind:      "insert",
		startByte: len(content),
		endByte:   len(content),
		startLine: line,
		endLine:   line,
		newText:   newText,
	}}
}

func resolvePatch(content string, op *PatchOp, opIndex int) ([]resolvedEdit, error) {
	if op.Patch == "" {
		return nil, fmt.Errorf("patch required")
	}
	lines := strings.Split(content, "\n")
	hastrail := len(content) > 0 && content[len(content)-1] == '\n'
	if hastrail && len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	result, hunks, err := applyUnifiedPatchWithHunks(lines, op.Patch)
	if err != nil {
		return nil, err
	}
	if len(hunks) == 0 {
		return nil, nil
	}
	var edits []resolvedEdit
	for _, h := range hunks {
		hunkEdits, err := patchHunkToResolvedEdits(content, hastrail, h, result.addTrailingNewline, opIndex)
		if err != nil {
			return nil, err
		}
		edits = append(edits, hunkEdits...)
	}
	return edits, nil
}

type stringMatch struct {
	start int
	end   int
}

func findAllStringMatches(content, needle string) []stringMatch {
	if needle == "" {
		return nil
	}
	var matches []stringMatch
	search := 0
	for {
		idx := strings.Index(content[search:], needle)
		if idx < 0 {
			break
		}
		start := search + idx
		end := start + len(needle)
		matches = append(matches, stringMatch{start: start, end: end})
		search = end
	}
	return matches
}

func splitLinesPreserveShape(content string) []string {
	lines := strings.Split(content, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func byteOffsetForLine(content string, line int) int {
	if line <= 1 {
		return 0
	}
	currentLine := 1
	for i := 0; i < len(content); i++ {
		if currentLine == line {
			return i
		}
		if content[i] == '\n' {
			currentLine++
			if currentLine == line {
				return i + 1
			}
		}
	}
	return len(content)
}

func lineNumberForOffset(content string, offset int) int {
	if offset <= 0 {
		return 1
	}
	if offset > len(content) {
		offset = len(content)
	}
	line := 1
	for i := 0; i < offset; i++ {
		if content[i] == '\n' {
			line++
		}
	}
	return line
}

func indentInsertedContent(content, indent string) string {
	parts := strings.Split(content, "\n")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
		for i := range parts {
			parts[i] = indent + parts[i]
		}
		return strings.Join(parts, "\n") + "\n"
	}
	for i := range parts {
		parts[i] = indent + parts[i]
	}
	return strings.Join(parts, "\n")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minPositiveEnd(a, b int) int {
	if a == 0 {
		return b
	}
	if b == 0 {
		return a
	}
	if a < b {
		return a
	}
	return b
}

func getIndent(line string) string {
	i := 0
	for i < len(line) && (line[i] == ' ' || line[i] == '\t') {
		i++
	}
	return line[:i]
}

// patchHunkToResolvedEdits groups contiguous +/- lines into one changed block and
// flushes whenever a context line appears, so patch context does not widen the
// conflict region beyond the bytes actually changed.
func patchHunkToResolvedEdits(content string, hasTrailingNewline bool, h patchHunk, addTrailingNewline bool, opIndex int) ([]resolvedEdit, error) {
	currentLine := h.origStart
	if h.origStart == 0 && h.origCount == 0 {
		currentLine = 1
	}
	currentByte := byteOffsetForLine(content, currentLine)
	lineCount := len(splitLinesPreserveShape(content))

	var edits []resolvedEdit
	var inBlock bool
	blockStartByte := 0
	blockEndByte := 0
	blockStartLine := 0
	blockEndLine := 0
	var blockNewText strings.Builder

	flush := func() {
		if !inBlock {
			return
		}
		newText := blockNewText.String()
		if !hasTrailingNewline && !addTrailingNewline && blockEndByte == len(content) && strings.HasSuffix(newText, "\n") {
			newText = strings.TrimSuffix(newText, "\n")
		}
		edits = append(edits, resolvedEdit{
			opIndex:   opIndex,
			kind:      "patch",
			startByte: blockStartByte,
			endByte:   blockEndByte,
			startLine: blockStartLine,
			endLine:   blockEndLine,
			newText:   newText,
		})
		inBlock = false
		blockNewText.Reset()
	}

	startBlock := func() {
		if inBlock {
			return
		}
		inBlock = true
		blockStartByte = currentByte
		blockEndByte = currentByte
		blockStartLine = currentLine
		blockEndLine = currentLine
	}

	for _, dl := range h.lines {
		if dl == "" {
			continue
		}
		prefix, text := dl[0], dl[1:]
		switch prefix {
		case ' ':
			flush()
			currentByte = advanceOffsetByLine(content, currentByte)
			currentLine++
		case '-':
			startBlock()
			lineEnd := advanceOffsetByLine(content, currentByte)
			blockEndByte = lineEnd
			blockEndLine = currentLine
			currentByte = lineEnd
			currentLine++
		case '+':
			startBlock()
			blockNewText.WriteString(text)
			if addTrailingNewline || currentByte < len(content) || hasTrailingNewline || currentLine <= lineCount {
				blockNewText.WriteString("\n")
			}
		default:
			return nil, fmt.Errorf("unsupported patch line prefix %q", string(prefix))
		}
	}
	flush()
	return edits, nil
}

func advanceOffsetByLine(content string, start int) int {
	for i := start; i < len(content); i++ {
		if content[i] == '\n' {
			return i + 1
		}
	}
	return len(content)
}

func applyUnifiedPatchWithHunks(fileLines []string, patch string) (*patchResult, []patchHunk, error) {
	hunks, addTrailing, err := parseUnifiedDiff(patch)
	if err != nil {
		return nil, nil, fmt.Errorf("parse patch: %w", err)
	}
	if len(hunks) == 0 {
		return nil, nil, fmt.Errorf("patch contains no hunks")
	}
	result, err := applyUnifiedPatch(fileLines, patch)
	if err != nil {
		return nil, nil, err
	}
	result.addTrailingNewline = addTrailing
	return result, hunks, nil
}
