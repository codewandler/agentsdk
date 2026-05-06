package appconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/resource"
)

// LoadOption configures the loading process.
type LoadOption func(*loadConfig)

type loadConfig struct {
	defaults Config
	roots    []string
	workDir  string
	noUser   bool
}

// WithDefaults sets the initial config that other sources merge onto.
func WithDefaults(cfg Config) LoadOption {
	return func(lc *loadConfig) { lc.defaults = cfg }
}

// WithConfigRoots adds directories to probe for config entry files.
func WithConfigRoots(roots ...string) LoadOption {
	return func(lc *loadConfig) { lc.roots = append(lc.roots, roots...) }
}

// WithWorkDir sets the working directory. It is probed for entry files
// and used as the base for relative include paths. Defaults to os.Getwd().
func WithWorkDir(dir string) LoadOption {
	return func(lc *loadConfig) { lc.workDir = dir }
}

// WithoutUserConfig disables loading ~/.agentsdk/default.config.yaml.
func WithoutUserConfig() LoadOption {
	return func(lc *loadConfig) { lc.noUser = true }
}

// LoadSource records where a config was loaded from.
type LoadSource struct {
	Path   string
	Config Config
}

// LoadResult holds the final merged config and all sources.
type LoadResult struct {
	Config      Config
	Agents      []AgentDoc
	Commands    []CommandDoc
	Workflows   []WorkflowDoc
	Actions     []ActionDoc
	Datasources []DatasourceDoc
	Triggers    []TriggerDoc
	Bundles     []resource.ContributionBundle // agentdir-loaded bundles
	EntryPath   string
	Sources     []LoadSource
}

// UserConfigDir is the directory probed for user-level default config.
const UserConfigDir = ".agentsdk"

// UserConfigNames are the filenames probed in the user config directory.
var UserConfigNames = []string{"default.config.yaml", "default.config.yml", "default.config.json"}

// Loader loads and merges appconfig documents from multiple roots with
// cycle detection and include expansion.
type Loader struct {
	visited map[string]bool
	result  LoadResult
}

// NewLoader creates a Loader.
func NewLoader() *Loader {
	return &Loader{visited: make(map[string]bool)}
}

// Load processes options and returns the merged LoadResult.
//
// Loading order:
//  1. Defaults (WithDefaults)
//  2. User config (~/.agentsdk/default.config.yaml) unless WithoutUserConfig
//  3. Explicit roots (WithConfigRoots) — probed for entry files
//  4. Working directory (WithWorkDir or $PWD) — probed for entry files
//
// Each root is probed for entry files (agentsdk.app.yaml, agentsdk.app.yml).
// Include directives are recursively expanded with cycle detection.
func (l *Loader) Load(opts ...LoadOption) (LoadResult, error) {
	cfg := &loadConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt(cfg)
		}
	}

	// Apply defaults.
	l.result.Config = cfg.defaults

	// Process includes from defaults.
	defaultIncludes := l.result.Config.Include
	l.result.Config.Include = nil

	// Resolve work dir.
	workDir := cfg.workDir
	if workDir == "" {
		wd, err := os.Getwd()
		if err != nil {
			return LoadResult{}, fmt.Errorf("appconfig: get working directory: %w", err)
		}
		workDir = wd
	}
	workDir, _ = filepath.Abs(workDir)

	// 0. Default agentdir includes — always probe .agents and .claude in workdir.
	for _, dir := range []string{
		filepath.Join(workDir, ".agents"),
		filepath.Join(workDir, ".claude"),
	} {
		if err := l.loadInclude(dir, dir); err != nil {
			return LoadResult{}, fmt.Errorf("appconfig: default include %s: %w", dir, err)
		}
	}

	// 1. User config.
	if !cfg.noUser {
		if home, err := os.UserHomeDir(); err == nil {
			userDir := filepath.Join(home, UserConfigDir)
			if path, ok := findConfigFile(userDir, UserConfigNames); ok {
				if err := l.loadRoot(path); err != nil {
					return LoadResult{}, fmt.Errorf("appconfig: user config: %w", err)
				}
			}
		}
	}

	// 2. Explicit roots.
	for _, root := range cfg.roots {
		root = expandVars(root, workDir)
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if path, ok := FindEntryFile(abs); ok {
				if err := l.loadRoot(path); err != nil {
					return LoadResult{}, fmt.Errorf("appconfig: root %s: %w", root, err)
				}
			}
		} else {
			if err := l.loadRoot(abs); err != nil {
				return LoadResult{}, fmt.Errorf("appconfig: root %s: %w", root, err)
			}
		}
	}

	// 3. Working directory.
	if path, ok := FindEntryFile(workDir); ok {
		if err := l.loadRoot(path); err != nil {
			return LoadResult{}, fmt.Errorf("appconfig: workdir: %w", err)
		}
	}

	// 4. Process includes from defaults (after workDir is known).
	for _, pattern := range defaultIncludes {
		expanded := expandVars(pattern, workDir)
		if err := l.loadInclude(expanded, pattern); err != nil {
			return LoadResult{}, fmt.Errorf("appconfig: default include %s: %w", pattern, err)
		}
	}

	// Set entry path from first source if not set.
	if l.result.EntryPath == "" && len(l.result.Sources) > 0 {
		l.result.EntryPath = l.result.Sources[0].Path
	}

	return l.result, nil
}

// loadRoot loads a single file with cycle detection, then processes includes.
func (l *Loader) loadRoot(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", path, err)
	}
	if l.visited[abs] {
		return nil
	}
	l.visited[abs] = true

	// Snapshot config before loading to capture what this file contributes.
	before := l.result.Config
	if err := loadFileInto(&l.result, abs); err != nil {
		return err
	}

	// Record source.
	l.result.Sources = append(l.result.Sources, LoadSource{
		Path:   abs,
		Config: l.result.Config,
	})

	// Process includes.
	includes := l.result.Config.Include
	l.result.Config.Include = nil
	// Restore includes that were there before this file.
	_ = before

	baseDir := filepath.Dir(abs)
	for _, pattern := range includes {
		expanded := expandVars(pattern, baseDir)
		if !isGlob(expanded) {
			if err := l.loadInclude(expanded, pattern); err != nil {
				return fmt.Errorf("include %s: %w", pattern, err)
			}
			continue
		}
		matches, err := filepath.Glob(expanded)
		if err != nil {
			return fmt.Errorf("glob %q (expanded: %q): %w", pattern, expanded, err)
		}
		sort.Strings(matches)
		for _, match := range matches {
			if err := l.loadInclude(match, pattern); err != nil {
				return fmt.Errorf("include %s: %w", match, err)
			}
		}
	}

	return nil
}

// loadInclude loads a single include path. Directories are loaded as agentdir
// roots. Files are loaded as appconfig documents.
func (l *Loader) loadInclude(path string, pattern string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", path, err)
	}
	if l.visited[abs] {
		return nil
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // skip missing includes silently
		}
		return err
	}
	if info.IsDir() {
		return l.loadAgentDir(abs)
	}
	return l.loadRoot(abs)
}

// loadAgentDir loads a directory as an agentdir layout and appends the
// resulting ContributionBundle to the LoadResult.
func (l *Loader) loadAgentDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	if l.visited[abs] {
		return nil
	}
	l.visited[abs] = true

	bundle, err := agentdir.LoadDir(abs)
	if err != nil {
		return fmt.Errorf("load agentdir %s: %w", abs, err)
	}
	l.result.Bundles = append(l.result.Bundles, bundle)
	l.result.Sources = append(l.result.Sources, LoadSource{Path: abs})
	return nil
}

func findConfigFile(dir string, names []string) (string, bool) {
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
	}
	return "", false
}

func isGlob(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

func dedup(paths []string) []string {
	seen := make(map[string]bool, len(paths))
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			abs = p
		}
		if seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, p)
	}
	return out
}
