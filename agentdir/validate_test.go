package agentdir

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func writeValidateFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func setupGlobalSkills(t *testing.T, homeDir string, names ...string) {
	t.Helper()
	for _, name := range names {
		require.NoError(t, os.MkdirAll(filepath.Join(homeDir, ".claude", "skills", name), 0o755))
		writeValidateFile(t, filepath.Join(homeDir, ".claude", "skills", name, "SKILL.md"),
			"---\nname: "+name+"\ndescription: global "+name+" skill\n---\nGlobal skill content.\n")
	}
}

func findCheck(result ValidationResult, category, subject string) *Check {
	for _, c := range result.Checks {
		if c.Category == category && c.Subject == subject {
			return &c
		}
	}
	return nil
}

func findCheckByMessage(result ValidationResult, substr string) *Check {
	for _, c := range result.Checks {
		if contains(c.Message, substr) {
			return &c
		}
	}
	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func checkStatuses(result ValidationResult) map[string]int {
	counts := map[string]int{}
	for _, c := range result.Checks {
		counts[c.Status]++
	}
	return counts
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestValidate_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)
	assert.False(t, result.OK())
	assert.False(t, result.Manifest.Found)
	assert.Empty(t, result.Agents)

	// Should warn about missing manifest and error about no agents.
	assert.NotNil(t, findCheckByMessage(result, "no manifest found"))
	assert.NotNil(t, findCheckByMessage(result, "no agents found"))
}

func TestValidate_ValidMinimalApp(t *testing.T) {
	dir := t.TempDir()
	homeDir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{
		"default_agent": "main",
		"sources": [".agents"]
	}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test agent
tools:
  - bash
  - file_*
capabilities: [planner]
---
You are a test agent.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: homeDir})
	require.NoError(t, err)
	assert.True(t, result.OK())

	// Manifest checks.
	assert.True(t, result.Manifest.Found)
	assert.Equal(t, "main", result.Manifest.DefaultAgent)
	assert.Equal(t, []string{".agents"}, result.Manifest.Sources)

	// Agent checks.
	require.Len(t, result.Agents, 1)
	assert.Equal(t, "main", result.Agents[0].Name)
	assert.True(t, result.Agents[0].HasFrontmatter)
	assert.True(t, result.Agents[0].HasSystem)
	assert.Equal(t, []string{"bash", "file_*"}, result.Agents[0].Tools)
	assert.Equal(t, []string{"planner"}, result.Agents[0].Capabilities)
}

func TestValidate_ManifestNoSources(t *testing.T) {
	dir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{
		"name": "broken-app",
		"description": "no sources"
	}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test
tools: [bash]
---
System prompt.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)
	assert.False(t, result.OK())

	check := findCheckByMessage(result, "no \"sources\" field")
	require.NotNil(t, check)
	assert.Equal(t, StatusError, check.Status)
}

func TestValidate_ManifestNoDefaultAgent(t *testing.T) {
	dir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{
		"sources": [".agents"]
	}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test
tools: [bash]
---
System prompt.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)
	// Missing default_agent is a warning, not an error.
	assert.True(t, result.OK())

	check := findCheckByMessage(result, "no \"default_agent\"")
	require.NotNil(t, check)
	assert.Equal(t, StatusWarning, check.Status)
}

func TestValidate_ManifestDefaultAgentMismatch(t *testing.T) {
	dir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{
		"default_agent": "nonexistent",
		"sources": [".agents"]
	}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test
tools: [bash]
---
System prompt.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)
	assert.False(t, result.OK())

	check := findCheckByMessage(result, "does not match any discovered agent")
	require.NotNil(t, check)
	assert.Equal(t, StatusError, check.Status)
}

func TestValidate_AgentNoFrontmatter(t *testing.T) {
	dir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"sources": [".agents"]}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "broken.md"), `# Broken agent

This agent has no YAML frontmatter at all.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)
	assert.False(t, result.OK())

	check := findCheckByMessage(result, "no YAML frontmatter")
	require.NotNil(t, check)
	assert.Equal(t, StatusError, check.Status)
}

func TestValidate_AgentNoTools(t *testing.T) {
	dir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"sources": [".agents"]}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Agent with no tools
---
System prompt.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)

	check := findCheckByMessage(result, "no tools: field")
	require.NotNil(t, check)
	assert.Equal(t, StatusWarning, check.Status)
}

func TestValidate_GlobalSkillsAvailableButNotIncluded(t *testing.T) {
	dir := t.TempDir()
	homeDir := t.TempDir()
	setupGlobalSkills(t, homeDir, "dex", "babelforce")

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"sources": [".agents"]}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test
tools: [bash]
skills: [dex, babelforce]
---
System prompt.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: homeDir})
	require.NoError(t, err)
	assert.False(t, result.OK())

	// Global skills exist but aren't included.
	assert.False(t, result.Skills.GlobalIncluded)
	assert.Contains(t, result.Skills.GlobalAvailable, "dex")
	assert.Contains(t, result.Skills.GlobalAvailable, "babelforce")

	// Skills referenced by agent are unresolvable.
	assert.Contains(t, result.Skills.Unresolvable, "dex")
	assert.Contains(t, result.Skills.Unresolvable, "babelforce")

	// Should have specific error about global skills existing but not enabled.
	check := findCheckByMessage(result, "exists globally")
	require.NotNil(t, check)
	assert.Equal(t, StatusError, check.Status)
}

func TestValidate_GlobalSkillsIncluded(t *testing.T) {
	dir := t.TempDir()
	homeDir := t.TempDir()
	setupGlobalSkills(t, homeDir, "dex", "babelforce")

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{
		"sources": [".agents"],
		"discovery": {"include_global_user_resources": true}
	}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test
tools: [bash]
skills: [dex, babelforce]
---
System prompt.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: homeDir})
	require.NoError(t, err)
	assert.True(t, result.OK())

	assert.True(t, result.Skills.GlobalIncluded)
	assert.Empty(t, result.Skills.Unresolvable)
}

func TestValidate_SkillReferencedButNotFound(t *testing.T) {
	dir := t.TempDir()
	homeDir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{
		"sources": [".agents"],
		"discovery": {"include_global_user_resources": true}
	}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test
tools: [bash]
skills: [nonexistent]
---
System prompt.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: homeDir})
	require.NoError(t, err)
	assert.False(t, result.OK())

	assert.Contains(t, result.Skills.Unresolvable, "nonexistent")
	check := findCheckByMessage(result, "not discoverable")
	require.NotNil(t, check)
	assert.Equal(t, StatusError, check.Status)
}

func TestValidate_WorkflowReferencesUndeclaredAction(t *testing.T) {
	dir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"sources": [".agents"]}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test
tools: [bash]
---
System prompt.
`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "workflows", "test.yaml"), `
name: test_workflow
description: Test workflow
steps:
  - id: step1
    action: missing_action
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)

	assert.Contains(t, result.Workflows, "test_workflow")
	check := findCheckByMessage(result, "missing_action")
	require.NotNil(t, check)
	assert.Equal(t, StatusWarning, check.Status)
}

func TestValidate_CommandTargetsUndeclaredWorkflow(t *testing.T) {
	dir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{"sources": [".agents"]}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Test
tools: [bash]
---
System prompt.
`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "commands", "test.yaml"), `
name: test-cmd
description: Test command
path: [test]
target:
  workflow: nonexistent_workflow
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)

	check := findCheckByMessage(result, "nonexistent_workflow")
	require.NotNil(t, check)
	assert.Equal(t, StatusWarning, check.Status)
}

func TestValidate_MultipleAgents(t *testing.T) {
	dir := t.TempDir()

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{
		"default_agent": "main",
		"sources": [".agents"]
	}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Main agent
tools: [bash, file_*]
capabilities: [planner]
---
Main system prompt.
`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "helper.md"), `---
name: helper
description: Helper agent
tools: [bash]
---
Helper system prompt.
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: t.TempDir()})
	require.NoError(t, err)
	assert.True(t, result.OK())
	assert.Len(t, result.Agents, 2)

	var names []string
	for _, a := range result.Agents {
		names = append(names, a.Name)
	}
	assert.Contains(t, names, "main")
	assert.Contains(t, names, "helper")
}

func TestValidate_FullApp(t *testing.T) {
	dir := t.TempDir()
	homeDir := t.TempDir()
	setupGlobalSkills(t, homeDir, "dex", "babelforce")

	writeValidateFile(t, filepath.Join(dir, "agentsdk.app.json"), `{
		"default_agent": "main",
		"discovery": {"include_global_user_resources": true},
		"sources": [".agents"]
	}`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "agents", "main.md"), `---
name: main
description: Full test agent
tools:
  - bash
  - file_*
  - web_fetch
  - skill
  - tools_*
skills: [dex, babelforce]
capabilities: [planner]
max-steps: 100
commands: [my-cmd]
---
Full system prompt.
`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "actions", "fetch.yaml"), `
name: fetch_data
description: Fetch data action
kind: host
`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "workflows", "daily.yaml"), `
name: daily_briefing
description: Daily briefing workflow
steps:
  - id: fetch
    action: fetch_data
`)
	writeValidateFile(t, filepath.Join(dir, ".agents", "commands", "my-cmd.yaml"), `
name: my-cmd
description: My command
path: [my-cmd]
target:
  workflow: daily_briefing
`)

	result, err := Validate(dir, ValidateOptions{HomeDir: homeDir})
	require.NoError(t, err)
	assert.True(t, result.OK(), "checks: %+v", result.Checks)

	// Everything discovered.
	assert.Equal(t, "main", result.Manifest.DefaultAgent)
	assert.Len(t, result.Agents, 1)
	assert.Contains(t, result.Workflows, "daily_briefing")
	assert.Contains(t, result.Actions, "fetch_data")
	assert.True(t, result.Skills.GlobalIncluded)
	assert.Empty(t, result.Skills.Unresolvable)

	// No errors.
	statuses := checkStatuses(result)
	assert.Zero(t, statuses[StatusError])
}

func TestValidate_OK(t *testing.T) {
	r := ValidationResult{
		Checks: []Check{
			{Status: StatusPassed, Message: "ok"},
			{Status: StatusWarning, Message: "warn"},
		},
	}
	assert.True(t, r.OK())

	r.Checks = append(r.Checks, Check{Status: StatusError, Message: "bad"})
	assert.False(t, r.OK())
}
