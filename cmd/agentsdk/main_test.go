package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codewandler/llmadapter/adapt"
	"github.com/codewandler/llmadapter/adapterconfig"
	"github.com/codewandler/llmadapter/compatibility"
	"github.com/stretchr/testify/require"
)

func TestRootCommandRegistersRun(t *testing.T) {
	cmd := rootCmd()
	run, _, err := cmd.Find([]string{"run"})
	require.NoError(t, err)
	require.NotNil(t, run)
	require.Equal(t, "run", run.Name())
	dev, _, err := cmd.Find([]string{"dev"})
	require.NoError(t, err)
	require.NotNil(t, dev)
	require.Equal(t, "dev", dev.Name())
	serve, _, err := cmd.Find([]string{"serve"})
	require.NoError(t, err)
	require.NotNil(t, serve)
	require.Equal(t, "serve", serve.Name())
	build, _, err := cmd.Find([]string{"build"})
	require.NoError(t, err)
	require.NotNil(t, build)
	require.Equal(t, "build", build.Name())
	discover, _, err := cmd.Find([]string{"discover"})
	require.NoError(t, err)
	require.NotNil(t, discover)
	require.Equal(t, "discover", discover.Name())
	models, _, err := cmd.Find([]string{"models"})
	require.NoError(t, err)
	require.NotNil(t, models)
	require.Equal(t, "models", models.Name())
}

func TestRunRejectsUnknownFlag(t *testing.T) {
	err := run([]string{"run", "--bad"})
	require.Error(t, err)
}

func TestServeStatusPrintsHarnessServiceSnapshot(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "coder.md"), "---\nname: coder\n---\nsystem")
	sessionsDir := filepath.Join(dir, "sessions")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"serve", dir, "--status", "--sessions-dir", sessionsDir, "--no-default-plugins"})

	require.NoError(t, cmd.Execute())
	text := out.String()
	require.Contains(t, text, "agentsdk service")
	require.Contains(t, text, "mode: harness.service")
	require.Contains(t, text, "health: ok")
	require.Contains(t, text, "sessions: "+sessionsDir)
	require.Contains(t, text, "active_sessions: 1")
	require.Contains(t, text, "agent=coder")
	require.Contains(t, text, "thread_backed=true")
}
func TestServeStatusRunsIntervalWorkflowTrigger(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "coder.md"), "---\nname: coder\n---\nsystem")
	writeTestFile(t, filepath.Join(dir, ".agents", "workflows", "echo.yaml"), "name: echo_flow\nsteps:\n  - id: echo\n    action: echo\n")
	sessionsDir := filepath.Join(dir, "sessions")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"serve", dir, "--status", "--sessions-dir", sessionsDir, "--no-default-plugins", "--trigger-interval", "1h", "--trigger-workflow", "echo_flow", "--trigger-input", "hello"})

	require.NoError(t, cmd.Execute())
	text := out.String()
	require.Contains(t, text, "jobs: 1")
	require.Contains(t, text, "job cli-interval")
	require.Contains(t, text, "target=workflow:echo_flow")
	require.Contains(t, text, "matched=1")
}

func TestBuildCommandStartsEmbeddedBuilderFromWorkingDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("/quit\n"))
	cmd.SetArgs([]string{"build"})

	require.NoError(t, cmd.Execute())
	text := out.String()
	require.Contains(t, text, "builder> ")
}

func TestDiscoverPrintsResourcesAndDisabledSuggestions(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: Main agent\n---\nmain")
	writeTestFile(t, filepath.Join(dir, ".agents", "commands", "review.md"), "---\ndescription: Review command\n---\nreview")
	writeTestFile(t, filepath.Join(dir, ".agents", "skills", "go", "SKILL.md"), "---\nname: go\ndescription: Go skill\n---\n# Go")
	writeTestFile(t, filepath.Join(dir, ".agents", "skills", "go", "references", "testing.md"), "---\ntrigger: tests\n---\nTesting reference")
	writeTestFile(t, filepath.Join(dir, ".agents", "datasources", "docs.yaml"), "name: docs\ndescription: Documentation corpus\nkind: corpus\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "workflows", "sync-docs.yaml"), "name: sync_docs\ndescription: Sync documentation\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "actions", "echo.yaml"), "name: echo\ndescription: Echo action\nkind: builtin\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "triggers", "hourly.yaml"), "id: hourly\ndescription: Hourly trigger\nsource:\n  interval: 1h\ntarget:\n  workflow: sync_docs\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "commands", "deploy.yaml"), "name: deploy\ndescription: Deploy command\npath: [deploy]\ntarget:\n  workflow: sync_docs\n")
	writeTestFile(t, filepath.Join(dir, "Makefile"), "test:\n\tgo test ./...\n")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	text := out.String()
	require.Contains(t, text, "Agents:")
	require.Contains(t, text, "main")
	require.Contains(t, text, "Main agent")
	require.Contains(t, text, "Commands:")
	require.Contains(t, text, "/review")
	require.Contains(t, text, "Review command")
	require.Contains(t, text, "Skills:")
	require.Contains(t, text, "go")
	require.Contains(t, text, "Go skill")
	require.Contains(t, text, "References:")
	require.Contains(t, text, "go/references/testing.md")
	require.Contains(t, text, "triggers=tests")
	require.Contains(t, text, "Datasources:")
	require.Contains(t, text, "docs")
	require.Contains(t, text, "Documentation corpus")
	require.Contains(t, text, "Workflows:")
	require.Contains(t, text, "sync_docs")
	require.Contains(t, text, "Sync documentation")
	require.Contains(t, text, "Actions:")
	require.Contains(t, text, "echo")
	require.Contains(t, text, "Echo action")
	require.Contains(t, text, "Triggers:")
	require.Contains(t, text, "hourly")
	require.Contains(t, text, "Hourly trigger")
	require.Contains(t, text, "Structured commands:")
	require.Contains(t, text, "/deploy")
	require.Contains(t, text, "target=workflow:sync_docs")
	require.Contains(t, text, "Capabilities:")
	require.Contains(t, text, "none")
	require.Contains(t, text, "Disabled suggestions:")
	require.Contains(t, text, "Makefile")
}

func TestDiscoverJSONPrintsMachineReadableDescriptors(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: Main agent\ncapabilities:\n  - name: planner\n    instance-id: plans\n---\nmain")
	writeTestFile(t, filepath.Join(dir, ".agents", "commands", "review.md"), "---\ndescription: Review command\n---\nreview")
	writeTestFile(t, filepath.Join(dir, ".agents", "skills", "go", "SKILL.md"), "---\nname: go\ndescription: Go skill\n---\n# Go")
	writeTestFile(t, filepath.Join(dir, ".agents", "skills", "go", "references", "testing.md"), "---\ntrigger: tests\n---\nTesting reference")
	writeTestFile(t, filepath.Join(dir, ".agents", "datasources", "docs.yaml"), "name: docs\ndescription: Documentation corpus\nkind: corpus\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "workflows", "sync-docs.yaml"), "name: sync_docs\ndescription: Sync documentation\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "actions", "echo.yaml"), "name: echo\ndescription: Echo action\nkind: builtin\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "triggers", "hourly.yaml"), "id: hourly\ndescription: Hourly trigger\nsource:\n  interval: 1h\ntarget:\n  workflow: sync_docs\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "commands", "deploy.yaml"), "name: deploy\ndescription: Deploy command\npath: [deploy]\ntarget:\n  workflow: sync_docs\n")
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"sources":[".agents"],"plugins":[{"name":"local_cli","config":{"mode":"safe"}}]}`)

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", "--json", dir})
	require.NoError(t, cmd.Execute())

	var payload struct {
		Sources []string `json:"sources"`
		Agents  []struct {
			Name         string `json:"name"`
			Capabilities []struct {
				Name       string `json:"name"`
				InstanceID string `json:"instanceId"`
			} `json:"capabilities"`
		} `json:"agents"`
		Commands []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"commands"`
		SkillReferences    []struct{ Skill, Path string } `json:"skillReferences"`
		DataSources        []struct{ Name, Kind string }  `json:"datasources"`
		Workflows          []struct{ Name string }        `json:"workflows"`
		Actions            []struct{ Name string }        `json:"actions"`
		Triggers           []struct{ Name string }        `json:"triggers"`
		StructuredCommands []struct{ Name string }        `json:"structuredCommands"`
		Plugins            []struct {
			Name   string         `json:"name"`
			Config map[string]any `json:"config"`
		} `json:"plugins"`
		Capabilities []struct {
			Name       string `json:"name"`
			InstanceID string `json:"instanceId"`
			Agent      string `json:"agent"`
		} `json:"capabilities"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
	require.NotEmpty(t, payload.Sources)
	require.Equal(t, "main", payload.Agents[0].Name)
	require.Equal(t, "planner", payload.Agents[0].Capabilities[0].Name)
	require.Equal(t, "plans", payload.Capabilities[0].InstanceID)
	require.Equal(t, "review", payload.Commands[0].Name)
	require.Equal(t, "go", payload.SkillReferences[0].Skill)
	require.Equal(t, "docs", payload.DataSources[0].Name)
	require.Equal(t, "sync_docs", payload.Workflows[0].Name)
	require.Equal(t, "echo", payload.Actions[0].Name)
	require.Equal(t, "hourly", payload.Triggers[0].Name)
	require.Equal(t, "deploy", payload.StructuredCommands[0].Name)
	require.Equal(t, "local_cli", payload.Plugins[0].Name)
	require.Equal(t, "safe", payload.Plugins[0].Config["mode"])
}
func TestDiscoverPrintsFirstWinsDiagnostics(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "reviewer.md"), "---\nname: reviewer\n---\nfirst")

	writeTestFile(t, filepath.Join(dir, ".claude", "agents", "reviewer.md"), "---\nname: reviewer\n---\nsecond")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	text := out.String()
	require.Equal(t, 1, bytes.Count(out.Bytes(), []byte("  reviewer  ")))
	require.Contains(t, text, "warning")
	require.Contains(t, text, "already registered")
}
func TestDiscoverPrintsManifestPluginRefsWithConfig(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"sources":[".agents"],"plugins":[{"name":"local_cli","config":{"mode":"safe"}}]}`)
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "coder.md"), "---\nname: coder\n---\nsystem")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	require.Contains(t, out.String(), "Plugins:")
	require.Contains(t, out.String(), "local_cli  config=true")
}

func TestManifestRejectsEmptyPluginRefs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"plugins":[{"name":""}]}`)

	cmd := rootCmd()
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.ErrorContains(t, cmd.Execute(), "plugin name is required")
}

func TestDiscoverFormatsMultilineDescriptions(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: \"Line one\\nLine two\"\n---\nmain")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	require.Contains(t, out.String(), "Line one Line two")
	require.NotContains(t, out.String(), `Line one\nLine two`)
}

func TestDiscoverTruncatesLongDescriptions(t *testing.T) {
	dir := t.TempDir()
	long := strings.Repeat("word ", 80)
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: "+long+"\n---\nmain")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"discover", "--local", dir})
	require.NoError(t, cmd.Execute())

	require.Contains(t, out.String(), "...")
	require.NotContains(t, out.String(), long)
}

func TestDiscoverBundledExamplesAndDogfoodApps(t *testing.T) {
	repoRoot := filepath.Clean(filepath.Join("..", ".."))
	cases := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "local quickstart",
			path: filepath.Join(repoRoot, "examples", "local-quickstart"),
			want: []string{"Agents:", "quickstart", "Commands:", "/hello"},
		},
		{
			name: "workflow app",
			path: filepath.Join(repoRoot, "examples", "workflow-app"),
			want: []string{"Workflows:", "summarize_topic", "Structured commands:", "/summarize"},
		},
		{
			name: "command tree",
			path: filepath.Join(repoRoot, "examples", "command-tree"),
			want: []string{"Commands:", "/explain", "Structured commands:", "/project plan"},
		},
		{
			name: "resource only",
			path: filepath.Join(repoRoot, "examples", "resource-only-app"),
			want: []string{"Workflows:", "session_summary", "Triggers:", "hourly-summary"},
		},
		{
			name: "hybrid app",
			path: filepath.Join(repoRoot, "examples", "hybrid-app"),
			want: []string{"Plugins:", "local_cli", "Workflows:", "operator_check"},
		},
		{
			name: "engineer app",
			path: filepath.Join(repoRoot, "apps", "engineer"),
			want: []string{"Agents:", "main", "Commands:", "/review"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := rootCmd()
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			cmd.SetArgs([]string{"discover", "--local", tc.path})
			require.NoError(t, cmd.Execute())
			for _, want := range tc.want {
				require.Contains(t, out.String(), want)
			}
		})
	}
}

func TestModelsPrintIncludesModelForCatalogRows(t *testing.T) {
	var out bytes.Buffer
	err := printApprovedModelSelections(&out, "", "test-evidence", []adapterconfig.UseCaseModelSelection{{
		Resolution: adapterconfig.ModelResolutionCandidate{
			PublicModel: "haiku",
			SourceAPI:   adapt.ApiAnthropicMessages,
			Provider:    "claude",
			ProviderAPI: adapt.ApiAnthropicMessages,
			NativeModel: "claude-haiku",
		},
		Evaluation: compatibility.Evaluation{Status: compatibility.StatusApproved},
	}})

	require.NoError(t, err)
	text := out.String()
	require.Contains(t, text, "Models: discovered from compatibility evidence")
	require.Contains(t, text, "Evidence: test-evidence")
	require.Contains(t, text, "approved  model=haiku")
	require.Contains(t, text, "source_api=anthropic.messages")
}

func TestEvidenceModelsDeduplicatesAndSorts(t *testing.T) {
	got := evidenceModels(adapterconfig.CompatibilityEvidence{Rows: []adapterconfig.CompatibilityRowEvidence{
		{PublicModel: "sonnet"},
		{PublicModel: "haiku"},
		{PublicModel: "sonnet"},
		{NativeModel: "gpt-5.4"},
	}}, false)

	require.Equal(t, []string{"gpt-5.4", "haiku", "sonnet"}, got)
}

func TestEvidenceModelsCanFilterThinkingRows(t *testing.T) {
	got := evidenceModels(adapterconfig.CompatibilityEvidence{Rows: []adapterconfig.CompatibilityRowEvidence{
		{PublicModel: "qwen3-coder", Reasoning: string(compatibility.EvidenceUnsupported)},
		{PublicModel: "sonnet", Reasoning: string(compatibility.EvidenceLive)},
		{PublicModel: "haiku", Reasoning: string(compatibility.EvidenceLive)},
	}}, true)

	require.Equal(t, []string{"haiku", "sonnet"}, got)
}

func TestThinkingModelSelectionsFiltersReasoningEvidence(t *testing.T) {
	selections := thinkingModelSelections([]adapterconfig.UseCaseModelSelection{
		{
			Resolution: adapterconfig.ModelResolutionCandidate{PublicModel: "qwen3-coder"},
			Evidence:   adapterconfig.CompatibilityRowEvidence{Reasoning: string(compatibility.EvidenceUnsupported)},
		},
		{
			Resolution: adapterconfig.ModelResolutionCandidate{PublicModel: "sonnet"},
			Evidence:   adapterconfig.CompatibilityRowEvidence{Reasoning: string(compatibility.EvidenceLive)},
		},
	})

	require.Len(t, selections, 1)
	require.Equal(t, "sonnet", selections[0].Resolution.PublicModel)
}

func TestModelsHelpUsesModelCompatibilityFlags(t *testing.T) {
	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"models", "--help"})

	require.NoError(t, cmd.Execute())
	text := out.String()
	require.Contains(t, text, "Model Compatibility:")
	require.Contains(t, text, "--model-use-case")
	require.Contains(t, text, "--model-approved-only")
	require.Contains(t, text, "--thinking")
	require.NotContains(t, text, "--use-case")
	require.NotContains(t, text, "--approved-only")
}

func TestModelsRejectsOldModelCompatibilityFlagNames(t *testing.T) {
	cmd := rootCmd()
	cmd.SetArgs([]string{"models", "--use-case", "agentic_coding"})

	require.Error(t, cmd.Execute())
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func TestValidatePassesValidApp(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"default_agent":"main","sources":[".agents"]}`)
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: Test\ntools: [bash]\n---\nSystem prompt.")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"validate", dir})
	require.NoError(t, cmd.Execute())
	require.Contains(t, out.String(), "0 errors")
}

func TestValidateFailsBrokenApp(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"name":"broken"}`)
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "# No frontmatter")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"validate", dir})
	require.ErrorContains(t, cmd.Execute(), "validation failed")
	require.Contains(t, out.String(), "no \"sources\" field")
	require.Contains(t, out.String(), "no YAML frontmatter")
}

func TestValidateJSONFailsBrokenApp(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"name":"broken"}`)
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "# No frontmatter")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"validate", "--json", dir})
	require.ErrorContains(t, cmd.Execute(), "validation failed")
	require.Contains(t, out.String(), "\"found\": true")
}
