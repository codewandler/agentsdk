package main

import (
	"bytes"
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
	serve, _, err := cmd.Find([]string{"serve"})
	require.NoError(t, err)
	require.NotNil(t, serve)
	require.Equal(t, "serve", serve.Name())
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
	err := run([]string{"run", "./agent", "--bad"})
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

func TestDiscoverPrintsResourcesAndDisabledSuggestions(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), "---\nname: main\ndescription: Main agent\n---\nmain")
	writeTestFile(t, filepath.Join(dir, ".agents", "commands", "review.md"), "---\ndescription: Review command\n---\nreview")
	writeTestFile(t, filepath.Join(dir, ".agents", "skills", "go", "SKILL.md"), "---\nname: go\ndescription: Go skill\n---\n# Go")
	writeTestFile(t, filepath.Join(dir, ".agents", "datasources", "docs.yaml"), "name: docs\ndescription: Documentation corpus\nkind: corpus\n")
	writeTestFile(t, filepath.Join(dir, ".agents", "workflows", "sync-docs.yaml"), "name: sync_docs\ndescription: Sync documentation\n")
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
	require.Contains(t, text, "Datasources:")
	require.Contains(t, text, "docs")
	require.Contains(t, text, "Documentation corpus")
	require.Contains(t, text, "Workflows:")
	require.Contains(t, text, "sync_docs")
	require.Contains(t, text, "Sync documentation")
	require.Contains(t, text, "Disabled suggestions:")
	require.Contains(t, text, "Makefile")
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
