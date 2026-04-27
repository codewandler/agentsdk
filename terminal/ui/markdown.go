package ui

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
)

var ansiOnlyLineRE = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Renderer renders Markdown for terminal output.
type Renderer func(string) string

// NewMarkdownRendererForWriter returns the default Markdown renderer.
//
// The renderer intentionally stays small: Goldmark parses standard Markdown,
// this package renders terminal styling directly, and Chroma highlights code.
func NewMarkdownRendererForWriter(io.Writer) Renderer {
	r := newNativeMarkdownRenderer()
	return func(s string) string {
		return r.Render(s)
	}
}

type nativeMarkdownRenderer struct {
	style *chroma.Style
	md    goldmark.Markdown
}

func newNativeMarkdownRenderer() nativeMarkdownRenderer {
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	for _, name := range []string{"catppuccin-mocha", "dracula", "monokai", "swapoff"} {
		if style := styles.Get(name); style != nil {
			return nativeMarkdownRenderer{style: style, md: md}
		}
	}
	return nativeMarkdownRenderer{style: styles.Fallback, md: md}
}

func (r nativeMarkdownRenderer) Render(s string) string {
	if r.md == nil || r.md.Parser() == nil {
		return strings.TrimRight(s, "\n")
	}
	source := []byte(s)
	doc := r.md.Parser().Parse(gmtext.NewReader(source))
	var out strings.Builder
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		rendered := r.renderBlock(n, source, 0)
		if rendered == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n\n")
		}
		out.WriteString(rendered)
	}
	return strings.TrimRight(out.String(), "\n")
}

func (r nativeMarkdownRenderer) renderBlock(n ast.Node, source []byte, indent int) string {
	switch node := n.(type) {
	case *ast.Paragraph:
		return r.renderInlineChildren(node, source)
	case *ast.Heading:
		text := strings.TrimSpace(r.renderInlineChildren(node, source))
		if text == "" {
			return ""
		}
		return renderHeading(node.Level, text)
	case *ast.List:
		return r.renderList(node, source, indent)
	case *ast.Blockquote:
		return renderBlockquote(r.renderContainerChildren(node, source, indent))
	case *extast.Table:
		return r.renderTable(node, source)
	case *ast.FencedCodeBlock:
		return r.highlightCode(string(node.Text(source)), string(node.Language(source)))
	case *ast.CodeBlock:
		return r.highlightCode(string(node.Text(source)), "")
	case *ast.ThematicBreak:
		return Dim + "------------------------" + Reset
	case *ast.HTMLBlock:
		return strings.TrimRight(rawBlockText(node, source), "\n")
	default:
		if n.Type() == ast.TypeBlock {
			return r.renderContainerChildren(n, source, indent)
		}
		return r.renderInline(n, source)
	}
}

func renderHeading(level int, text string) string {
	switch level {
	case 1:
		return Bold + BrightCyan + text + Reset
	case 2:
		return Bold + BrightGreen + text + Reset
	default:
		return Bold + text + Reset
	}
}

func (r nativeMarkdownRenderer) renderTable(table *extast.Table, source []byte) string {
	var rows [][]string
	headerRows := 0
	for row := table.FirstChild(); row != nil; row = row.NextSibling() {
		switch row.(type) {
		case *extast.TableHeader:
			rows = append(rows, r.renderTableRow(row, source))
			headerRows++
		case *extast.TableRow:
			rows = append(rows, r.renderTableRow(row, source))
		}
	}
	if len(rows) == 0 {
		return ""
	}
	widths := tableColumnWidths(rows)
	var out strings.Builder
	for i, row := range rows {
		if i > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(renderTableRow(row, widths, table.Alignments, i < headerRows))
		if i+1 == headerRows {
			out.WriteByte('\n')
			out.WriteString(renderTableSeparator(widths))
		}
	}
	return out.String()
}

func (r nativeMarkdownRenderer) renderTableRow(row ast.Node, source []byte) []string {
	var cells []string
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		cells = append(cells, strings.TrimSpace(r.renderInlineChildren(cell, source)))
	}
	return cells
}

func tableColumnWidths(rows [][]string) []int {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			for len(widths) <= i {
				widths = append(widths, 0)
			}
			if width := visibleLen(cell); width > widths[i] {
				widths[i] = width
			}
		}
	}
	for i, width := range widths {
		if width == 0 {
			widths[i] = 1
		}
	}
	return widths
}

func renderTableRow(row []string, widths []int, alignments []extast.Alignment, header bool) string {
	var out strings.Builder
	out.WriteString("| ")
	for i, width := range widths {
		if i > 0 {
			out.WriteString(" | ")
		}
		cell := ""
		if i < len(row) {
			cell = row[i]
		}
		if header {
			cell = Bold + cell + Reset
		}
		out.WriteString(padTableCell(cell, width, tableAlignment(alignments, i)))
	}
	out.WriteString(" |")
	return out.String()
}

func renderTableSeparator(widths []int) string {
	var out strings.Builder
	out.WriteString("| ")
	for i, width := range widths {
		if i > 0 {
			out.WriteString(" | ")
		}
		out.WriteString(Dim)
		out.WriteString(strings.Repeat("-", width))
		out.WriteString(Reset)
	}
	out.WriteString(" |")
	return out.String()
}

func tableAlignment(alignments []extast.Alignment, i int) extast.Alignment {
	if i >= 0 && i < len(alignments) {
		return alignments[i]
	}
	return extast.AlignNone
}

func padTableCell(s string, width int, alignment extast.Alignment) string {
	padding := width - visibleLen(s)
	if padding <= 0 {
		return s
	}
	switch alignment {
	case extast.AlignRight:
		return strings.Repeat(" ", padding) + s
	case extast.AlignCenter:
		left := padding / 2
		right := padding - left
		return strings.Repeat(" ", left) + s + strings.Repeat(" ", right)
	default:
		return s + strings.Repeat(" ", padding)
	}
}

func visibleLen(s string) int {
	return len(visibleText(s))
}

func visibleText(s string) string {
	return ansiOnlyLineRE.ReplaceAllString(s, "")
}

func (r nativeMarkdownRenderer) renderContainerChildren(n ast.Node, source []byte, indent int) string {
	var out strings.Builder
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		rendered := r.renderBlock(child, source, indent)
		if rendered == "" {
			continue
		}
		if out.Len() > 0 {
			out.WriteString("\n")
		}
		out.WriteString(rendered)
	}
	return out.String()
}

func (r nativeMarkdownRenderer) renderList(list *ast.List, source []byte, indent int) string {
	var out strings.Builder
	index := list.Start
	if index == 0 {
		index = 1
	}
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		marker := "- "
		if list.IsOrdered() {
			marker = strconv.Itoa(index) + ". "
			index++
		}
		text := r.renderListItem(item, source, indent+len(marker))
		out.WriteString(strings.Repeat(" ", indent))
		out.WriteString(marker)
		out.WriteString(indentContinuation(text, indent+len(marker)))
	}
	return out.String()
}

func (r nativeMarkdownRenderer) renderListItem(item ast.Node, source []byte, indent int) string {
	var out strings.Builder
	var prefix strings.Builder
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		if _, ok := child.(*extast.TaskCheckBox); ok {
			prefix.WriteString(r.renderInline(child, source))
			continue
		}
		rendered := r.renderBlock(child, source, indent)
		if rendered == "" {
			continue
		}
		if isRenderedTaskMarker(rendered) {
			prefix.WriteString(rendered)
			if !strings.HasSuffix(rendered, " ") {
				prefix.WriteByte(' ')
			}
			continue
		}
		if out.Len() == 0 && prefix.Len() > 0 {
			rendered = prefix.String() + strings.TrimLeft(rendered, "\n\r\t ")
			prefix.Reset()
		}
		if out.Len() > 0 {
			out.WriteByte('\n')
		}
		out.WriteString(rendered)
	}
	if out.Len() == 0 && prefix.Len() > 0 {
		return strings.TrimRight(prefix.String(), " ")
	}
	return normalizeTaskMarkerSpacing(out.String())
}

func isRenderedTaskMarker(s string) bool {
	visible := strings.TrimSpace(visibleText(s))
	return visible == "[x]" || visible == "[ ]"
}

func normalizeTaskMarkerSpacing(s string) string {
	replacements := map[string]string{
		BrightGreen + "[x]" + Reset + " \n": BrightGreen + "[x]" + Reset + " ",
		Dim + "[ ]" + Reset + " \n":         Dim + "[ ]" + Reset + " ",
		"[x] \n":                            "[x] ",
		"[ ] \n":                            "[ ] ",
	}
	for old, next := range replacements {
		s = strings.ReplaceAll(s, old, next)
	}
	return s
}

func indentContinuation(s string, indent int) string {
	lines := strings.Split(s, "\n")
	for i := 1; i < len(lines); i++ {
		if lines[i] != "" {
			lines[i] = strings.Repeat(" ", indent) + lines[i]
		}
	}
	return strings.Join(lines, "\n")
}

func renderBlockquote(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = Dim + ">" + Reset
			continue
		}
		lines[i] = Dim + "> " + Reset + line
	}
	return strings.Join(lines, "\n")
}

func rawBlockText(n ast.Node, source []byte) string {
	lines := n.Lines()
	if lines == nil || lines.Len() == 0 {
		return string(n.Text(source))
	}
	var out strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		out.Write(seg.Value(source))
	}
	return out.String()
}

func (r nativeMarkdownRenderer) renderInlineChildren(n ast.Node, source []byte) string {
	var out strings.Builder
	previousTaskCheckBox := false
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		rendered := r.renderInline(child, source)
		if previousTaskCheckBox {
			rendered = strings.TrimLeft(rendered, "\n\r\t ")
		}
		out.WriteString(rendered)
		_, previousTaskCheckBox = child.(*extast.TaskCheckBox)
	}
	return out.String()
}

func (r nativeMarkdownRenderer) renderInline(n ast.Node, source []byte) string {
	switch node := n.(type) {
	case *ast.Text:
		text := string(node.Value(source))
		if node.HardLineBreak() {
			return text + "\n"
		}
		if node.SoftLineBreak() {
			return text + " "
		}
		return text
	case *ast.String:
		return string(node.Value)
	case *ast.CodeSpan:
		return "\x1b[38;2;203;166;247m" + strings.TrimSpace(string(node.Text(source))) + Reset
	case *ast.Emphasis:
		text := r.renderInlineChildren(node, source)
		if node.Level >= 2 {
			return Bold + text + Reset
		}
		return "\x1b[3m" + text + Reset
	case *ast.Link:
		text := r.renderInlineChildren(node, source)
		dest := string(node.Destination)
		if dest == "" || dest == text {
			return text
		}
		return text + Dim + " (" + dest + ")" + Reset
	case *ast.AutoLink:
		return string(node.URL(source))
	case *ast.RawHTML:
		return rawBlockText(node, source)
	case *extast.Strikethrough:
		return "\x1b[9m" + r.renderInlineChildren(node, source) + Reset
	case *extast.TaskCheckBox:
		if node.IsChecked {
			return BrightGreen + "[x]" + Reset + " "
		}
		return Dim + "[ ]" + Reset + " "
	default:
		return r.renderInlineChildren(n, source)
	}
}

func (r nativeMarkdownRenderer) highlightCode(code, language string) string {
	return strings.TrimRight(r.highlightCodePreserve(code, language), "\n")
}

func (r nativeMarkdownRenderer) highlightCodePreserve(code, language string) string {
	lexer := lexers.Get(language)
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	iterator, err := chroma.Coalesce(lexer).Tokenise(nil, code)
	if err != nil {
		return code
	}
	var out bytes.Buffer
	if err := writeChromaANSI(&out, r.style, iterator); err != nil {
		return code
	}
	return out.String()
}

func writeChromaANSI(w io.Writer, style *chroma.Style, iterator chroma.Iterator) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("format highlighted code: %v", r)
		}
	}()
	sequences := map[chroma.TokenType]string{}
	for token := iterator(); token != chroma.EOF; token = iterator() {
		seq, ok := sequences[token.Type]
		if !ok {
			entry := style.Get(token.Type)
			entry.Background = 0
			seq = ansiStyle(entry)
			sequences[token.Type] = seq
		}
		fmt.Fprint(w, seq)
		fmt.Fprint(w, token.Value)
		if seq != "" {
			fmt.Fprint(w, Reset)
		}
	}
	return nil
}

func ansiStyle(entry chroma.StyleEntry) string {
	var b strings.Builder
	if entry.Bold == chroma.Yes {
		b.WriteString("\x1b[1m")
	}
	if entry.Underline == chroma.Yes {
		b.WriteString("\x1b[4m")
	}
	if entry.Italic == chroma.Yes {
		b.WriteString("\x1b[3m")
	}
	if entry.Colour.IsSet() {
		fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm", entry.Colour.Red(), entry.Colour.Green(), entry.Colour.Blue())
	}
	return b.String()
}

type fencedCodeBlock struct {
	openStart    int
	contentStart int
	contentEnd   int
	closeEnd     int
	language     string
}

func findFencedCodeBlocks(s string) []fencedCodeBlock {
	lines := splitLinesWithOffsets(s)
	var blocks []fencedCodeBlock
	for i := 0; i < len(lines); i++ {
		open, ok := parseOpeningFence(s[lines[i].start:lines[i].end])
		if !ok {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if isClosingFence(s[lines[j].start:lines[j].end], open.marker, open.length) {
				blocks = append(blocks, fencedCodeBlock{
					openStart:    lines[i].start,
					contentStart: lines[i].end,
					contentEnd:   lines[j].start,
					closeEnd:     lines[j].end,
					language:     open.language,
				})
				i = j
				break
			}
		}
	}
	return blocks
}

type lineRange struct {
	start int
	end   int
}

func splitLinesWithOffsets(s string) []lineRange {
	var lines []lineRange
	start := 0
	for start < len(s) {
		end := start
		if idx := strings.IndexByte(s[start:], '\n'); idx >= 0 {
			end = start + idx + 1
		} else {
			end = len(s)
		}
		lines = append(lines, lineRange{start: start, end: end})
		start = end
	}
	return lines
}

type openingFence struct {
	marker   byte
	length   int
	language string
}

func parseOpeningFence(line string) (openingFence, bool) {
	content := strings.TrimRight(line, "\r\n")
	trimmedLeft := strings.TrimLeft(content, " ")
	indent := len(content) - len(trimmedLeft)
	if indent > 3 || len(trimmedLeft) < 3 {
		return openingFence{}, false
	}
	marker := trimmedLeft[0]
	if marker != '`' && marker != '~' {
		return openingFence{}, false
	}
	length := countLeadingByte(trimmedLeft, marker)
	if length < 3 {
		return openingFence{}, false
	}
	info := strings.TrimSpace(trimmedLeft[length:])
	if marker == '`' && strings.Contains(info, "`") {
		return openingFence{}, false
	}
	return openingFence{
		marker:   marker,
		length:   length,
		language: firstField(info),
	}, true
}

func isClosingFence(line string, marker byte, length int) bool {
	content := strings.TrimRight(line, "\r\n")
	trimmedLeft := strings.TrimLeft(content, " ")
	indent := len(content) - len(trimmedLeft)
	if indent > 3 || len(trimmedLeft) < length {
		return false
	}
	if trimmedLeft[0] != marker {
		return false
	}
	run := countLeadingByte(trimmedLeft, marker)
	if run < length {
		return false
	}
	return strings.TrimSpace(trimmedLeft[run:]) == ""
}

func countLeadingByte(s string, b byte) int {
	count := 0
	for count < len(s) && s[count] == b {
		count++
	}
	return count
}

func firstField(s string) string {
	if s == "" {
		return ""
	}
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}

// TrimOuterRenderedBlankLines removes visually blank leading and trailing lines.
func TrimOuterRenderedBlankLines(s string) string {
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	start := 0
	for start < len(lines) && IsVisuallyBlankRenderedLine(lines[start]) {
		start++
	}
	end := len(lines)
	for end > start && IsVisuallyBlankRenderedLine(lines[end-1]) {
		end--
	}
	if start >= end {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

// IsVisuallyBlankRenderedLine reports whether a rendered line is blank after
// stripping ANSI SGR codes.
func IsVisuallyBlankRenderedLine(s string) bool {
	s = ansiOnlyLineRE.ReplaceAllString(s, "")
	return strings.TrimSpace(s) == ""
}
