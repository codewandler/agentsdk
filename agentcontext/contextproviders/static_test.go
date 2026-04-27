package contextproviders

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/invopop/jsonschema"
)

func TestStaticProvidersRenderCoreContext(t *testing.T) {
	providers := []agentcontext.Provider{
		Model(ModelInfo{Name: "gpt-test", Provider: "openai", ContextWindow: 128000, Effort: "high"}),
		Permissions("workspace-write"),
		ProjectInstructions(ProjectInstruction{Path: "AGENTS.md", Content: "follow repo notes"}),
		AgentsMarkdown([]string{"AGENTS.md"}, AgentsMarkdownOption(WithFileReader(func(path string) ([]byte, fs.FileInfo, error) {
			return []byte("follow repo notes"), fakeFileInfo{name: path, size: int64(len("follow repo notes")), modTime: time.Unix(1714200000, 0)}, nil
		}))),
		Skills(skill.Skill{Name: "planner", Description: "planning", SourceLabel: "repo", Body: "plan carefully"}),
		Tools(tool.New("search", "search files", func(tool.Ctx, struct{}) (tool.Result, error) {
			return tool.Text("ok"), nil
		})),
	}

	for _, provider := range providers {
		providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
		if err != nil {
			t.Fatal(err)
		}
		if len(providerContext.Fragments) == 0 {
			t.Fatalf("%s returned no fragments", provider.Key())
		}
		if providerContext.Fingerprint == "" {
			t.Fatalf("%s returned no fingerprint", provider.Key())
		}
		fingerprint, ok, err := provider.(agentcontext.FingerprintingProvider).StateFingerprint(context.Background(), agentcontext.Request{})
		if err != nil {
			t.Fatal(err)
		}
		if !ok || fingerprint != providerContext.Fingerprint {
			t.Fatalf("%s fingerprint = %q ok=%v, want %q", provider.Key(), fingerprint, ok, providerContext.Fingerprint)
		}
	}
}

func TestStaticProvidersUseStableFragmentKeys(t *testing.T) {
	project := ProjectInstructions(ProjectInstruction{Path: "./nested/AGENTS.md", Content: "repo notes"})
	projectContext, err := project.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := projectContext.Fragments[0].Key, agentcontext.FragmentKey("project_instructions/nested_AGENTS.md"); got != want {
		t.Fatalf("project key = %q, want %q", got, want)
	}

	skills := Skills(skill.Skill{Name: "skill one", Body: "body"})
	skillContext, err := skills.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := skillContext.Fragments[0].Key, agentcontext.FragmentKey("skills/loaded/skill_one"); got != want {
		t.Fatalf("skill key = %q, want %q", got, want)
	}
}

func TestToolsProviderRendersSortedToolList(t *testing.T) {
	provider := Tools(
		namedTool{name: "zeta", description: "last"},
		namedTool{name: "alpha", description: "first"},
	)
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	content := providerContext.Fragments[0].Content
	if !strings.Contains(content, "- alpha: first\n- zeta: last") {
		t.Fatalf("tools content not sorted: %s", content)
	}
}

type namedTool struct {
	name        string
	description string
}

func (t namedTool) Name() string        { return t.name }
func (t namedTool) Description() string { return t.description }
func (t namedTool) Schema() *jsonschema.Schema {
	return nil
}
func (t namedTool) Execute(tool.Ctx, json.RawMessage) (tool.Result, error) {
	return tool.Text("ok"), nil
}
func (t namedTool) Guidance() string { return "" }
