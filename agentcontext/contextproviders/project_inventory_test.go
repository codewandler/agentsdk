package contextproviders

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
)

func TestProjectInventoryProviderRendersSummary(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "go.mod", "module example.com/test\n")
	writeProjectFile(t, dir, "cmd/app/main.go", "package main\nfunc main() {}\n")
	writeProjectFile(t, dir, "tools/example/example.go", "package example\n")
	writeProjectFile(t, dir, "tools/example/example_test.go", "package example\n")
	writeProjectFile(t, dir, "README.md", "# Test\n")
	writeProjectFile(t, dir, "config/app.yaml", "name: test\n")

	provider := ProjectInventory(WithProjectInventoryWorkDir(dir))
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if provider.Key() != "project_inventory" {
		t.Fatalf("key = %q", provider.Key())
	}
	if got, want := len(providerContext.Fragments), 1; got != want {
		t.Fatalf("fragments = %d, want %d", got, want)
	}
	fragment := providerContext.Fragments[0]
	if fragment.Key != "project/inventory" {
		t.Fatalf("fragment key = %q", fragment.Key)
	}
	for _, want := range []string{
		"Project inventory:",
		"root: " + dir,
		"files_scanned: 6",
		"Go(3 files)",
		"Markdown(1 files)",
		"YAML(1 files)",
		"package_managers: go.mod",
		"key_dirs:",
		"cmd/",
		"tools/",
		"test_patterns: tools/example/*_test.go",
		"entrypoints: cmd/app/main.go",
	} {
		if !strings.Contains(fragment.Content, want) {
			t.Fatalf("content missing %q:\n%s", want, fragment.Content)
		}
	}
	if providerContext.Fingerprint == "" {
		t.Fatal("missing provider fingerprint")
	}
	fingerprint, ok, err := provider.StateFingerprint(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if !ok || fingerprint != providerContext.Fingerprint {
		t.Fatalf("fingerprint = %q ok=%v, want %q", fingerprint, ok, providerContext.Fingerprint)
	}
}

func TestProjectInventoryProviderRespectsGitignore(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, ".gitignore", "ignored/\n")
	writeProjectFile(t, dir, "a.go", "package a\n")
	writeProjectFile(t, dir, "b.go", "package b\n")
	writeProjectFile(t, dir, "ignored/c.go", "package ignored\n")

	provider := ProjectInventory(WithProjectInventoryWorkDir(dir))
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	content := providerContext.Fragments[0].Content
	for _, want := range []string{"files_scanned: 3", "Go(2 files)"} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "ignored/") {
		t.Fatalf("ignored directory leaked into inventory: %s", content)
	}
}

func TestProjectInventoryProviderCapsFiles(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "a.go", "package a\n")
	writeProjectFile(t, dir, "b.go", "package b\n")

	provider := ProjectInventory(WithProjectInventoryWorkDir(dir), WithProjectInventoryMaxFiles(1))
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	content := providerContext.Fragments[0].Content
	for _, want := range []string{"files_scanned: 2", "truncated: true", "Go(1 files)"} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %q:\n%s", want, content)
		}
	}
}

func TestProjectInventoryProviderMaxBytes(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, dir, "go.mod", "module example.com/test\n")
	writeProjectFile(t, dir, "cmd/app/main.go", "package main\n")

	provider := ProjectInventory(WithProjectInventoryWorkDir(dir), WithProjectInventoryMaxBytes(80))
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	content := providerContext.Fragments[0].Content
	if len(content) > 80 {
		t.Fatalf("content length = %d, want <= 80: %s", len(content), content)
	}
	if !strings.Contains(content, "truncated_bytes: true") {
		t.Fatalf("content missing truncation marker: %s", content)
	}
}

func writeProjectFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
