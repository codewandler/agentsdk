package contextproviders

import (
	"context"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
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
		namedTool{name: "alpha", description: "first", guidance: "Use exact inputs"},
	)
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	content := providerContext.Fragments[0].Content
	if !strings.Contains(content, "- alpha: first\n  guidance:\n    Use exact inputs\n- zeta: last") {
		t.Fatalf("tools content not sorted/guided: %s", content)
	}
}

func TestSkillInventoryProviderRendersAvailableAndActivatedState(t *testing.T) {
	repo, state := testSkillInventory(t)
	_, err := state.ActivateSkill("architecture")
	if err != nil {
		t.Fatal(err)
	}
	_, err = state.ActivateReferences("architecture", []string{"references/tradeoffs.md"})
	if err != nil {
		t.Fatal(err)
	}

	provider := SkillInventoryProvider(SkillInventory{Catalog: repo, State: state})
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(providerContext.Fragments) != 3 {
		t.Fatalf("fragment count = %d, want 3", len(providerContext.Fragments))
	}
	if !strings.Contains(providerContext.Fragments[0].Content, "status: dynamic") {
		t.Fatalf("missing active status in %q", providerContext.Fragments[0].Content)
	}
	if !strings.Contains(providerContext.Fragments[0].Content, "Decide carefully") {
		t.Fatalf("missing active body in %q", providerContext.Fragments[0].Content)
	}
	if !strings.Contains(providerContext.Fragments[1].Content, "path: references/tradeoffs.md") {
		t.Fatalf("missing active ref in %q", providerContext.Fragments[1].Content)
	}
	if !strings.Contains(providerContext.Fragments[2].Content, "status: inactive") {
		t.Fatalf("missing inactive status in %q", providerContext.Fragments[2].Content)
	}
	if strings.Contains(providerContext.Fragments[2].Content, "Review deeply") {
		t.Fatalf("inactive skill unexpectedly rendered body: %q", providerContext.Fragments[2].Content)
	}
}

func TestSkillInventoryProviderOmitsInactiveReferencesFromPromptContext(t *testing.T) {
	repo, state := testSkillInventory(t)
	_, err := state.ActivateSkill("architecture")
	if err != nil {
		t.Fatal(err)
	}

	provider := SkillInventoryProvider(SkillInventory{Catalog: repo, State: state})
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range providerContext.Fragments {
		if strings.Contains(fragment.Content, "tradeoffs reference") {
			t.Fatalf("inactive reference unexpectedly rendered: %q", fragment.Content)
		}
	}
}

func testSkillInventory(t *testing.T) (*skill.Repository, *skill.ActivationState) {
	t.Helper()
	fsys := fstest.MapFS{
		"skills/architecture/SKILL.md":                {Data: []byte("---\nname: architecture\ndescription: Architecture help\n---\nDecide carefully")},
		"skills/architecture/references/tradeoffs.md": {Data: []byte("---\ntrigger: tradeoffs\n---\ntradeoffs reference")},
		"skills/review/SKILL.md":                      {Data: []byte("---\nname: review\ndescription: Review help\n---\nReview deeply")},
	}
	repo, err := skill.NewRepository([]skill.Source{skill.FSSource("skills", "skills", fsys, "skills", skill.SourceEmbedded, 0)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := skill.NewActivationState(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	return repo, state
}

func TestSkillInventoryProviderStableReferenceFragmentKey(t *testing.T) {
	repo, state := testSkillInventory(t)
	_, err := state.ActivateSkill("architecture")
	if err != nil {
		t.Fatal(err)
	}
	_, err = state.ActivateReferences("architecture", []string{"references/tradeoffs.md"})
	if err != nil {
		t.Fatal(err)
	}
	provider := SkillInventoryProvider(SkillInventory{Catalog: repo, State: state})
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, fragment := range providerContext.Fragments {
		if fragment.Key == agentcontext.FragmentKey("skills/references/architecture/references_tradeoffs.md") {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing stable reference fragment key in %#v", providerContext.Fragments)
	}
}

type namedTool struct {
	name        string
	description string
	guidance    string
}

func (t namedTool) Name() string        { return t.name }
func (t namedTool) Description() string { return t.description }
func (t namedTool) Schema() *jsonschema.Schema {
	return nil
}
func (t namedTool) Execute(tool.Ctx, json.RawMessage) (tool.Result, error) {
	return tool.Text("ok"), nil
}
func (t namedTool) Guidance() string { return t.guidance }
