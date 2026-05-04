package builderapp

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/action"
	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/app"
	"github.com/codewandler/agentsdk/resource"
	"github.com/codewandler/agentsdk/terminal/cli"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/tools/toolmgmt"
	"github.com/codewandler/agentsdk/tools/web"
	"github.com/codewandler/llmadapter/unified"
)

const ResourcesRoot = "resources"

//go:embed resources/agentsdk.app.json resources/.agents/agents/*.md resources/.agents/skills/*/SKILL.md resources/.agents/skills/*/references/*.md resources/.agents/workflows/*.yaml resources/.agents/commands/*.yaml
var embeddedResources embed.FS

func Resources() fs.FS { return embeddedResources }

func DefaultSessionsDir(projectDir string) string {
	return filepath.Join(projectDir, ".agentsdk", "builder", "sessions")
}

func DefaultTargetSessionsDir(projectDir string) string {
	return filepath.Join(projectDir, ".agentsdk", "builder", "target-sessions")
}

type Config struct {
	ProjectDir        string
	SessionsDir       string
	TargetSessionsDir string
}

func NormalizeConfig(cfg Config) (Config, error) {
	if strings.TrimSpace(cfg.ProjectDir) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Config{}, err
		}
		cfg.ProjectDir = wd
	}
	var err error
	cfg.ProjectDir, err = filepath.Abs(cfg.ProjectDir)
	if err != nil {
		return Config{}, err
	}
	if cfg.SessionsDir == "" {
		cfg.SessionsDir = DefaultSessionsDir(cfg.ProjectDir)
	}
	if cfg.TargetSessionsDir == "" {
		cfg.TargetSessionsDir = DefaultTargetSessionsDir(cfg.ProjectDir)
	}
	cfg.SessionsDir, err = filepath.Abs(cfg.SessionsDir)
	if err != nil {
		return Config{}, err
	}
	cfg.TargetSessionsDir, err = filepath.Abs(cfg.TargetSessionsDir)
	if err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func AppOptions(cfg Config) ([]app.Option, error) {
	cfg, err := NormalizeConfig(cfg)
	if err != nil {
		return nil, err
	}
	plugin := Plugin{cfg: cfg}
	return []app.Option{app.WithPlugin(plugin)}, nil
}

type Plugin struct{ cfg Config }

func (p Plugin) Name() string { return "builder" }

func (p Plugin) Actions() []action.Action { return Actions(p.cfg) }

func (p Plugin) CatalogTools() []tool.Tool { return p.DefaultTools() }

func (p Plugin) DefaultTools() []tool.Tool {
	actions := Actions(p.cfg)
	tools := make([]tool.Tool, 0, len(actions)+6)
	for _, a := range actions {
		tools = append(tools, tool.FromAction(a))
	}
	tools = append(tools, toolmgmt.Tools()...)
	tools = append(tools, web.Tools(nil)...)
	tools = append(tools, web.SearchTool(web.DefaultSearchProviderFromEnv()))
	return tools
}

func (p Plugin) ContextProviders() []agentcontext.Provider {
	return []agentcontext.Provider{ProjectContextProvider{Config: p.cfg}}
}

type ProjectContextProvider struct{ Config Config }

func (p ProjectContextProvider) Key() agentcontext.ProviderKey { return "builder.project" }

func (p ProjectContextProvider) GetContext(context.Context, agentcontext.Request) (agentcontext.ProviderContext, error) {
	content := fmt.Sprintf("## Builder project\n\n- Project directory: %s\n- Builder resources: embedded first-party builder app\n- Builder sessions: %s\n- Target test sessions: %s\n\nThe current working directory is the project under construction. Do not treat its .agents directory as the builder's own resources; inspect it as the target app when needed.\n", p.Config.ProjectDir, p.Config.SessionsDir, p.Config.TargetSessionsDir)
	return agentcontext.ProviderContext{Fragments: []agentcontext.ContextFragment{{Key: "builder.project", Role: unified.RoleSystem, Content: content, Authority: agentcontext.AuthorityDeveloper}}}, nil
}

func Actions(cfg Config) []action.Action {
	return []action.Action{
		action.NewTyped(action.Spec{Name: "builder_inspect_project", Description: "Inspect the current project directory for agentsdk app files."}, func(ctx action.Ctx, input InspectProjectInput) (InspectProjectOutput, error) {
			return InspectProject(ctx, cfg, input)
		}),
		action.NewTyped(action.Spec{Name: "builder_discover_target", Description: "Discover the current project as an isolated target agentsdk app."}, func(ctx action.Ctx, input DiscoverTargetInput) (DiscoverTargetOutput, error) {
			return DiscoverTarget(ctx, cfg, input)
		}),
		action.NewTyped(action.Spec{Name: "builder_run_target_smoke", Description: "Load the current project as an isolated target app and run non-destructive smoke checks."}, func(ctx action.Ctx, input RunTargetSmokeInput) (RunTargetSmokeOutput, error) {
			return RunTargetSmoke(ctx, cfg, input)
		}),
		action.NewTyped(action.Spec{Name: "builder_scaffold_resource_app", Description: "Scaffold a minimal resource-only agentsdk app in the current project directory."}, func(ctx action.Ctx, input ScaffoldResourceAppInput) (ScaffoldResourceAppOutput, error) {
			return ScaffoldResourceApp(ctx, cfg, input)
		}),
		action.NewTyped(action.Spec{Name: "builder_write_project_file", Description: "Write one file under the current project directory with path-safety checks."}, func(ctx action.Ctx, input WriteProjectFileInput) (WriteProjectFileOutput, error) {
			return WriteProjectFile(ctx, cfg, input)
		}),
	}
}

type InspectProjectInput struct{}

type InspectProjectOutput struct {
	ProjectDir      string   `json:"projectDir"`
	Exists          bool     `json:"exists"`
	HasManifest     bool     `json:"hasManifest"`
	HasAgentsDir    bool     `json:"hasAgentsDir"`
	HasGoMod        bool     `json:"hasGoMod"`
	HasReadme       bool     `json:"hasReadme"`
	TopLevelEntries []string `json:"topLevelEntries"`
}

func InspectProject(ctx context.Context, cfg Config, _ InspectProjectInput) (InspectProjectOutput, error) {
	cfg, err := NormalizeConfig(cfg)
	if err != nil {
		return InspectProjectOutput{}, err
	}
	out := InspectProjectOutput{ProjectDir: cfg.ProjectDir}
	info, err := os.Stat(cfg.ProjectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return out, err
	}
	out.Exists = info.IsDir()
	out.HasManifest = fileExists(filepath.Join(cfg.ProjectDir, "agentsdk.app.json"))
	out.HasAgentsDir = dirExists(filepath.Join(cfg.ProjectDir, ".agents"))
	out.HasGoMod = fileExists(filepath.Join(cfg.ProjectDir, "go.mod"))
	out.HasReadme = fileExists(filepath.Join(cfg.ProjectDir, "README.md"))
	entries, err := os.ReadDir(cfg.ProjectDir)
	if err != nil {
		return out, err
	}
	for _, entry := range entries {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		default:
		}
		out.TopLevelEntries = append(out.TopLevelEntries, entry.Name())
	}
	sort.Strings(out.TopLevelEntries)
	if len(out.TopLevelEntries) > 40 {
		out.TopLevelEntries = out.TopLevelEntries[:40]
	}
	return out, nil
}

type DiscoverTargetInput struct{}

type DiscoverTargetOutput struct {
	ProjectDir  string   `json:"projectDir"`
	Sources     []string `json:"sources"`
	Agents      []string `json:"agents"`
	Commands    []string `json:"commands"`
	Workflows   []string `json:"workflows"`
	Actions     []string `json:"actions"`
	Triggers    []string `json:"triggers"`
	DataSources []string `json:"datasources"`
	Diagnostics []string `json:"diagnostics"`
}

func DiscoverTarget(ctx context.Context, cfg Config, _ DiscoverTargetInput) (DiscoverTargetOutput, error) {
	cfg, err := NormalizeConfig(cfg)
	if err != nil {
		return DiscoverTargetOutput{}, err
	}
	resolved, err := agentdir.ResolveDirWithOptions(cfg.ProjectDir, agentdir.ResolveOptions{Policy: resource.DiscoveryPolicy{}, LocalOnly: true})
	if err != nil {
		return DiscoverTargetOutput{}, err
	}
	_ = ctx
	out := DiscoverTargetOutput{ProjectDir: cfg.ProjectDir, Sources: append([]string(nil), resolved.Sources...)}
	for _, spec := range resolved.Bundle.AgentSpecs {
		out.Agents = append(out.Agents, spec.Name)
	}
	for _, cmd := range resolved.Bundle.Commands {
		out.Commands = append(out.Commands, cmd.Descriptor().Name)
	}
	for _, cmd := range resolved.Bundle.CommandResources {
		out.Commands = append(out.Commands, strings.Join(cmd.CommandPath, " "))
	}
	for _, workflow := range resolved.Bundle.Workflows {
		out.Workflows = append(out.Workflows, workflow.Name)
	}
	for _, action := range resolved.Bundle.Actions {
		out.Actions = append(out.Actions, action.Name)
	}
	for _, trigger := range resolved.Bundle.Triggers {
		out.Triggers = append(out.Triggers, trigger.Name)
	}
	for _, ds := range resolved.Bundle.DataSources {
		out.DataSources = append(out.DataSources, ds.Name)
	}
	for _, diag := range resolved.Bundle.Diagnostics {
		out.Diagnostics = append(out.Diagnostics, fmt.Sprintf("%s: %s", diag.Severity, diag.Message))
	}
	sort.Strings(out.Agents)
	sort.Strings(out.Commands)
	sort.Strings(out.Workflows)
	sort.Strings(out.Actions)
	sort.Strings(out.Triggers)
	sort.Strings(out.DataSources)
	return out, nil
}

type RunTargetSmokeInput struct{}

type SmokeCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Details string `json:"details,omitempty"`
}

type RunTargetSmokeOutput struct {
	ProjectDir        string               `json:"projectDir"`
	TargetSessionsDir string               `json:"targetSessionsDir"`
	TargetSessionID   string               `json:"targetSessionId,omitempty"`
	Checks            []SmokeCheck         `json:"checks"`
	Discovery         DiscoverTargetOutput `json:"discovery"`
}

func RunTargetSmoke(ctx context.Context, cfg Config, _ RunTargetSmokeInput) (RunTargetSmokeOutput, error) {
	cfg, err := NormalizeConfig(cfg)
	if err != nil {
		return RunTargetSmokeOutput{}, err
	}
	discovery, err := DiscoverTarget(ctx, cfg, DiscoverTargetInput{})
	out := RunTargetSmokeOutput{ProjectDir: cfg.ProjectDir, TargetSessionsDir: cfg.TargetSessionsDir, Discovery: discovery}
	if err != nil {
		out.Checks = append(out.Checks, SmokeCheck{Name: "discover target app", Status: "failed", Details: err.Error()})
		return out, nil
	}
	out.Checks = append(out.Checks, SmokeCheck{Name: "discover target app", Status: "passed"})
	loaded, err := cli.Load(ctx, cli.Config{Resources: cli.DirResources(cfg.ProjectDir), Workspace: cfg.ProjectDir, SessionsDir: cfg.TargetSessionsDir, NoDefaultPlugins: false})
	if err != nil {
		out.Checks = append(out.Checks, SmokeCheck{Name: "load target harness", Status: "failed", Details: err.Error()})
		return out, nil
	}
	out.TargetSessionID = loaded.Session.SessionID()
	out.Checks = append(out.Checks, SmokeCheck{Name: "load target harness", Status: "passed", Details: loaded.AgentName})
	if _, err := loaded.Session.ExecuteCommand(ctx, []string{"session", "info"}, nil); err != nil {
		out.Checks = append(out.Checks, SmokeCheck{Name: "target /session info", Status: "failed", Details: err.Error()})
	} else {
		out.Checks = append(out.Checks, SmokeCheck{Name: "target /session info", Status: "passed"})
	}
	if _, err := loaded.Session.ExecuteCommand(ctx, []string{"workflow", "list"}, nil); err != nil {
		out.Checks = append(out.Checks, SmokeCheck{Name: "target /workflow list", Status: "failed", Details: err.Error()})
	} else {
		out.Checks = append(out.Checks, SmokeCheck{Name: "target /workflow list", Status: "passed"})
	}
	return out, nil
}

type ScaffoldResourceAppInput struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Force       bool   `json:"force,omitempty"`
}

type ScaffoldResourceAppOutput struct {
	ProjectDir string   `json:"projectDir"`
	Files      []string `json:"files"`
}

func ScaffoldResourceApp(ctx context.Context, cfg Config, input ScaffoldResourceAppInput) (ScaffoldResourceAppOutput, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = "agentsdk app"
	}
	description := strings.TrimSpace(input.Description)
	if description == "" {
		description = "Generated agentsdk resource app."
	}
	files := map[string]string{
		"agentsdk.app.json":             "{\n  \"sources\": [\".agents\"]\n}\n",
		"README.md":                     "# " + name + "\n\n" + description + "\n\n## Run\n\n```bash\nagentsdk discover --local .\nagentsdk run .\n```\n",
		".agents/agents/main.md":        "---\nname: main\ndescription: " + description + "\n---\nYou are the main assistant for " + name + ".\n",
		".agents/workflows/verify.yaml": "name: verify_app\ndescription: Verify the generated app.\nsteps:\n  - id: echo\n    action: echo\n",
		".agents/actions/echo.yaml":     "name: echo\ndescription: Placeholder generated action.\nkind: placeholder\n",
	}
	out := ScaffoldResourceAppOutput{}
	for rel, content := range files {
		written, err := writeUnderProject(ctx, cfg, rel, content, input.Force)
		if err != nil {
			return out, err
		}
		out.Files = append(out.Files, written)
	}
	cfg, _ = NormalizeConfig(cfg)
	out.ProjectDir = cfg.ProjectDir
	sort.Strings(out.Files)
	return out, nil
}

type WriteProjectFileInput struct {
	Path      string `json:"path" jsonschema:"required"`
	Content   string `json:"content"`
	Overwrite bool   `json:"overwrite,omitempty"`
}

type WriteProjectFileOutput struct {
	ProjectDir string `json:"projectDir"`
	Path       string `json:"path"`
	Bytes      int    `json:"bytes"`
}

func WriteProjectFile(ctx context.Context, cfg Config, input WriteProjectFileInput) (WriteProjectFileOutput, error) {
	rel, err := writeUnderProject(ctx, cfg, input.Path, input.Content, input.Overwrite)
	if err != nil {
		return WriteProjectFileOutput{}, err
	}
	cfg, _ = NormalizeConfig(cfg)
	return WriteProjectFileOutput{ProjectDir: cfg.ProjectDir, Path: rel, Bytes: len([]byte(input.Content))}, nil
}

func writeUnderProject(ctx context.Context, cfg Config, rel string, content string, overwrite bool) (string, error) {
	cfg, err := NormalizeConfig(cfg)
	if err != nil {
		return "", err
	}
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "." || rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return "", fmt.Errorf("builder: path %q must be relative and stay under project directory", rel)
	}
	abs := filepath.Join(cfg.ProjectDir, rel)
	cleanProject := cfg.ProjectDir + string(os.PathSeparator)
	if abs != cfg.ProjectDir && !strings.HasPrefix(abs, cleanProject) {
		return "", fmt.Errorf("builder: path %q escapes project directory", rel)
	}
	if !overwrite {
		if _, err := os.Stat(abs); err == nil {
			return "", fmt.Errorf("builder: %s already exists", rel)
		} else if !os.IsNotExist(err) {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, []byte(content), 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(rel), nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func PrettyJSON(v any) string {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(raw)
}
