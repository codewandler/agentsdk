package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/plugins/configplugin"
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
	require.Contains(t, text, "build> ")
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
	// Tree output groups by origin.
	require.Contains(t, text, "local:")
	// Resource names appear under kind sub-trees.
	require.Contains(t, text, "agents")
	require.Contains(t, text, "main")
	require.Contains(t, text, "commands")
	require.Contains(t, text, "review")
	require.Contains(t, text, "skills")
	require.Contains(t, text, "go")
	require.Contains(t, text, "workflows")
	require.Contains(t, text, "sync_docs")
	require.Contains(t, text, "actions")
	require.Contains(t, text, "echo")
	require.Contains(t, text, "triggers")
	require.Contains(t, text, "hourly")
	require.Contains(t, text, "deploy")
	// Resolution summary.
	require.Contains(t, text, "Resolution:")
	require.Contains(t, text, "\u2192")
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
	cmd.SetArgs([]string{"discover", "--local", "-o", "json", dir})
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
	cmd.SetArgs([]string{"discover", "--local", "-o", "json", dir})
	require.NoError(t, cmd.Execute())

	text := out.String()
	require.Contains(t, text, "reviewer")
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
	// Use JSON output to verify plugin config is preserved.
	cmd.SetArgs([]string{"discover", "--local", "-o", "json", dir})
	require.NoError(t, cmd.Execute())

	require.Contains(t, out.String(), "local_cli")
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

	// Tree output shows the agent name; description formatting is in JSON/YAML output.
	require.Contains(t, out.String(), "main")
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

	// Tree output shows names, not descriptions.
	require.Contains(t, out.String(), "main")
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
			want: []string{"quickstart", "hello"},
		},
		{
			name: "workflow app",
			path: filepath.Join(repoRoot, "examples", "workflow-app"),
			want: []string{"summarize_topic", "summarize-topic"},
		},
		{
			name: "command tree",
			path: filepath.Join(repoRoot, "examples", "command-tree"),
			want: []string{"commander", "plan-change", "plan_change"},
		},
		{
			name: "resource only",
			path: filepath.Join(repoRoot, "examples", "resource-only-app"),
			want: []string{"session_summary", "hourly-summary"},
		},
		{
			name: "hybrid app",
			path: filepath.Join(repoRoot, "examples", "hybrid-app"),
			want: []string{"operator", "operator_check"},
		},
		{
			name: "engineer app",
			path: filepath.Join(repoRoot, "apps", "engineer"),
			want: []string{"main", "review"},
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

func TestConfigDiscoverRendersResourceTree(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), "name: test-app\n---\nkind: agent\nname: coder\nsystem: test\n")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "discover", dir})
	require.NoError(t, cmd.Execute())

	text := out.String()
	require.Contains(t, text, "Sources:")
	require.Contains(t, text, "agents")
	require.Contains(t, text, "coder")
	require.Contains(t, text, "Resolution:")
}

func TestConfigDiscoverJSONUsesSharedDiscoveryPayload(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), "name: test-app\n---\nkind: agent\nname: coder\ndescription: Test coder\nsystem: test\n")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "discover", "-o", "json", dir})
	require.NoError(t, cmd.Execute())

	var payload struct {
		Sources []string `json:"sources"`
		Agents  []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
		} `json:"agents"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &payload))
	require.NotEmpty(t, payload.Sources)
	require.Len(t, payload.Agents, 1)
	require.Equal(t, "coder", payload.Agents[0].Name)
	require.Equal(t, "Test coder", payload.Agents[0].Description)
}

func TestConfigPrintRendersMaterializedSources(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), "kind: config\nname: test-app\nsources:\n  - .agents\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "coder.md"), "---\nname: coder\n---\nsystem")

	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "print", dir})
	require.NoError(t, cmd.Execute())

	text := out.String()
	require.Contains(t, text, "sources:")
	require.Contains(t, text, filepath.Join(dir, "agentsdk.app.yaml"))
	require.Contains(t, text, filepath.Join(dir, ".agents"))
}

func TestConfigDiscoverUsesSameSourcesAsRootDiscover(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), "kind: config\nname: test-app\nsources:\n  - .agents\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "coder.md"), "---\nname: coder\n---\nsystem")

	var rootOut bytes.Buffer
	root := rootCmd()
	root.SetOut(&rootOut)
	root.SetErr(&rootOut)
	root.SetArgs([]string{"discover", dir})
	require.NoError(t, root.Execute())

	var configOut bytes.Buffer
	config := rootCmd()
	config.SetOut(&configOut)
	config.SetErr(&configOut)
	config.SetArgs([]string{"config", "discover", dir})
	require.NoError(t, config.Execute())

	require.Equal(t, rootOut.String(), configOut.String())
}

func TestConfigDiscoverAppliesAppConfigModelPolicy(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "agentsdk.app.yaml"), `kind: config
name: test-app
model_policy:
  use_case: agentic_coding
  approved_only: true
---
kind: agent
name: coder
system: test
`)

	result, err := loadConfig([]string{dir}, nil)
	require.NoError(t, err)
	resolved, err := configplugin.ResolutionFromAppConfig(result)
	require.NoError(t, err)
	require.True(t, resolved.HasModelPolicy)
	require.Equal(t, "agentic_coding", string(resolved.ModelPolicy.UseCase))
	require.True(t, resolved.ModelPolicy.ApprovedOnly)
}

func TestConfigSchemaWritesToStdout(t *testing.T) {
	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "schema"})
	require.NoError(t, cmd.Execute())

	text := out.String()
	require.Contains(t, text, `"$schema":`)
	require.Contains(t, text, `"AppDocument"`)
	// Must be valid JSON.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal([]byte(text), &parsed))
}

func TestConfigSchemaOutDirWritesFile(t *testing.T) {
	dir := t.TempDir()
	cmd := rootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"config", "schema", "--out-dir", dir})
	require.NoError(t, cmd.Execute())

	require.Contains(t, out.String(), "agentsdk.schema.json")

	data, err := os.ReadFile(filepath.Join(dir, "agentsdk.schema.json"))
	require.NoError(t, err)
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(data, &parsed))
	require.Contains(t, parsed, "$schema")
}
