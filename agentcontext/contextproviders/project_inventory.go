package contextproviders

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
	ignore "github.com/sabhiram/go-gitignore"
)

const (
	defaultProjectInventoryMaxFiles = 5000
	defaultProjectInventoryMaxDirs  = 12
	defaultProjectInventoryMaxBytes = 4000
)

type ProjectInventoryOption func(*ProjectInventoryProvider)

type ProjectInventoryProvider struct {
	key      agentcontext.ProviderKey
	workDir  string
	maxFiles int
	maxDirs  int
	maxBytes int
}

func ProjectInventory(opts ...ProjectInventoryOption) *ProjectInventoryProvider {
	p := &ProjectInventoryProvider{
		key:      "project_inventory",
		maxFiles: defaultProjectInventoryMaxFiles,
		maxDirs:  defaultProjectInventoryMaxDirs,
		maxBytes: defaultProjectInventoryMaxBytes,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

func WithProjectInventoryKey(key agentcontext.ProviderKey) ProjectInventoryOption {
	return func(p *ProjectInventoryProvider) { p.key = key }
}

func WithProjectInventoryWorkDir(workDir string) ProjectInventoryOption {
	return func(p *ProjectInventoryProvider) { p.workDir = workDir }
}

func WithProjectInventoryMaxFiles(max int) ProjectInventoryOption {
	return func(p *ProjectInventoryProvider) { p.maxFiles = max }
}

func WithProjectInventoryMaxDirs(max int) ProjectInventoryOption {
	return func(p *ProjectInventoryProvider) { p.maxDirs = max }
}

func WithProjectInventoryMaxBytes(max int) ProjectInventoryOption {
	return func(p *ProjectInventoryProvider) { p.maxBytes = max }
}

func (p *ProjectInventoryProvider) Key() agentcontext.ProviderKey {
	if p == nil || p.key == "" {
		return "project_inventory"
	}
	return p.key
}

func (p *ProjectInventoryProvider) GetContext(ctx context.Context, req agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	content, err := p.content(ctx)
	if err != nil {
		return agentcontext.ProviderContext{}, err
	}
	fp := contentFingerprint("project_inventory", content)
	if content == "" {
		return agentcontext.ProviderContext{Fingerprint: fp}, nil
	}
	return agentcontext.ProviderContext{
		Fragments: []agentcontext.ContextFragment{{
			Key:       "project/inventory",
			Role:      unified.RoleUser,
			Content:   content,
			Authority: agentcontext.AuthorityUser,
			CachePolicy: agentcontext.CachePolicy{
				Scope: agentcontext.CacheTurn,
			},
		}},
		Fingerprint: fp,
	}, nil
}

func (p *ProjectInventoryProvider) StateFingerprint(ctx context.Context, req agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	content, err := p.content(ctx)
	if err != nil {
		return "", false, err
	}
	return contentFingerprint("project_inventory", content), true, nil
}

func (p *ProjectInventoryProvider) content(ctx context.Context) (string, error) {
	root := p.workDir
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		root = wd
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if !info.IsDir() {
		return "", nil
	}

	inv := inventory{
		root:            absRoot,
		languages:       map[string]int{},
		packageManagers: map[string]bool{},
		testPatterns:    map[string]bool{},
		entrypoints:     map[string]bool{},
		keyDirs:         map[string]int{},
	}
	gi := loadRootGitignore(absRoot)
	maxFiles := p.maxFilesOrDefault()
	walkErr := filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		name := d.Name()
		if d.IsDir() {
			if shouldSkipInventoryDir(rel, name) || (gi != nil && gi.MatchesPath(rel)) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if gi != nil && gi.MatchesPath(rel) {
			return nil
		}
		inv.filesSeen++
		if inv.filesSeen > maxFiles {
			inv.truncated = true
			return filepath.SkipAll
		}
		if dir := topLevelDirForPath(rel); dir != "" {
			inv.keyDirs[dir]++
		}
		if lang := languageForPath(rel); lang != "" {
			inv.languages[lang]++
		}
		if isPackageManagerFile(rel) {
			inv.packageManagers[rel] = true
		}
		if isTestFile(rel) {
			inv.testPatterns[testPatternForPath(rel)] = true
		}
		if isEntrypoint(rel) {
			inv.entrypoints[rel] = true
		}
		return nil
	})
	if walkErr != nil && walkErr != context.Canceled && walkErr != context.DeadlineExceeded {
		return "", walkErr
	}
	return limitProjectInventoryContent(renderInventory(inv, p.maxDirsOrDefault()), p.maxBytesOrDefault()), nil
}

func (p *ProjectInventoryProvider) maxFilesOrDefault() int {
	if p == nil || p.maxFiles <= 0 {
		return defaultProjectInventoryMaxFiles
	}
	return p.maxFiles
}

func (p *ProjectInventoryProvider) maxDirsOrDefault() int {
	if p == nil || p.maxDirs <= 0 {
		return defaultProjectInventoryMaxDirs
	}
	return p.maxDirs
}

func (p *ProjectInventoryProvider) maxBytesOrDefault() int {
	if p == nil || p.maxBytes <= 0 {
		return defaultProjectInventoryMaxBytes
	}
	return p.maxBytes
}

type inventory struct {
	root            string
	filesSeen       int
	truncated       bool
	languages       map[string]int
	packageManagers map[string]bool
	testPatterns    map[string]bool
	entrypoints     map[string]bool
	keyDirs         map[string]int
}

func renderInventory(inv inventory, maxDirs int) string {
	var b strings.Builder
	b.WriteString("Project inventory:\n")
	writeLine(&b, "root", inv.root)
	writeLine(&b, "files_scanned", fmt.Sprintf("%d", inv.filesSeen))
	if inv.truncated {
		writeLine(&b, "truncated", "true")
	}
	writeInventoryList(&b, "languages", formatCounts(inv.languages))
	writeInventoryList(&b, "package_managers", sortedBoolKeys(inv.packageManagers))
	writeInventoryList(&b, "key_dirs", topDirs(inv.keyDirs, maxDirs))
	writeInventoryList(&b, "test_patterns", sortedBoolKeys(inv.testPatterns))
	writeInventoryList(&b, "entrypoints", sortedBoolKeys(inv.entrypoints))
	return strings.TrimSpace(b.String())
}

func writeInventoryList(b *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		return
	}
	writeLine(b, key, strings.Join(values, ", "))
}

func formatCounts(counts map[string]int) []string {
	items := make([]string, 0, len(counts))
	for k, v := range counts {
		items = append(items, fmt.Sprintf("%s(%d files)", k, v))
	}
	sort.Slice(items, func(i, j int) bool {
		ci := countInFormattedItem(items[i])
		cj := countInFormattedItem(items[j])
		if ci != cj {
			return ci > cj
		}
		return items[i] < items[j]
	})
	return items
}

func countInFormattedItem(item string) int {
	start := strings.LastIndex(item, "(")
	end := strings.Index(item[start+1:], " ")
	if start < 0 || end < 0 {
		return 0
	}
	var n int
	_, _ = fmt.Sscanf(item[start+1:start+1+end], "%d", &n)
	return n
}

func sortedBoolKeys(values map[string]bool) []string {
	out := make([]string, 0, len(values))
	for k := range values {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func topDirs(counts map[string]int, max int) []string {
	type item struct {
		dir   string
		count int
	}
	items := make([]item, 0, len(counts))
	for dir, count := range counts {
		items = append(items, item{dir: dir, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].dir < items[j].dir
	})
	if len(items) > max {
		items = items[:max]
	}
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.dir
	}
	return out
}

func shouldSkipInventoryDir(rel, name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "dist", "build", "target", ".next", ".cache", "__pycache__":
		return true
	}
	return strings.HasPrefix(rel, ".git/")
}

func topLevelDirForPath(rel string) string {
	idx := strings.IndexByte(rel, '/')
	if idx <= 0 {
		return ""
	}
	return rel[:idx+1]
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "Go"
	case ".md", ".markdown":
		return "Markdown"
	case ".yaml", ".yml":
		return "YAML"
	case ".json":
		return "JSON"
	case ".js", ".mjs", ".cjs":
		return "JavaScript"
	case ".ts", ".tsx":
		return "TypeScript"
	case ".py":
		return "Python"
	case ".rs":
		return "Rust"
	case ".java":
		return "Java"
	case ".kt", ".kts":
		return "Kotlin"
	case ".rb":
		return "Ruby"
	case ".php":
		return "PHP"
	case ".sh", ".bash", ".zsh":
		return "Shell"
	case ".toml":
		return "TOML"
	}
	return ""
}

func isPackageManagerFile(path string) bool {
	switch filepath.Base(path) {
	case "go.mod", "package.json", "pnpm-lock.yaml", "yarn.lock", "package-lock.json", "pyproject.toml", "requirements.txt", "Cargo.toml", "Gemfile", "composer.json", "Makefile", "Taskfile.yml":
		return true
	}
	return false
}

func isTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.go") || strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".test.tsx") || strings.HasSuffix(base, ".spec.ts") || strings.HasSuffix(base, ".spec.tsx") || strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")
}

func testPatternForPath(path string) string {
	dir := filepath.ToSlash(filepath.Dir(path))
	base := filepath.Base(path)
	if strings.HasSuffix(base, "_test.go") {
		return joinPatternDir(dir, "*_test.go")
	}
	if strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py") {
		return joinPatternDir(dir, "test_*.py")
	}
	if strings.HasSuffix(base, "_test.py") {
		return joinPatternDir(dir, "*_test.py")
	}
	if strings.Contains(base, ".test.") {
		return joinPatternDir(dir, "*.test.*")
	}
	if strings.Contains(base, ".spec.") {
		return joinPatternDir(dir, "*.spec.*")
	}
	return path
}

func joinPatternDir(dir, pattern string) string {
	if dir == "." || dir == "" {
		return pattern
	}
	return dir + "/" + pattern
}

func isEntrypoint(path string) bool {
	base := filepath.Base(path)
	if path == "main.go" || strings.HasSuffix(path, "/main.go") || path == "cmd/agentsdk/main.go" {
		return true
	}
	switch base {
	case "main.py", "app.py", "server.js", "index.js", "index.ts", "main.ts":
		return true
	}
	return false
}

func loadRootGitignore(root string) *ignore.GitIgnore {
	path := filepath.Join(root, ".gitignore")
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	gi, err := ignore.CompileIgnoreFile(path)
	if err != nil {
		return nil
	}
	return gi
}

func limitProjectInventoryContent(content string, max int) string {
	if max <= 0 || len(content) <= max {
		return content
	}
	marker := "\ntruncated_bytes: true"
	if max <= len(marker) {
		return content[:max]
	}
	return strings.TrimRight(content[:max-len(marker)], "\n ") + marker
}
