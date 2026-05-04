package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/skill"
	threadjsonlstore "github.com/codewandler/agentsdk/thread/jsonlstore"
	"github.com/codewandler/agentsdk/tools/shell"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

const (
	testProvider = "test"
	testModel    = "model"
)



func TestAgentRunTurnUsesDefaultCachePolicy(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("response", "resp1"))
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSystem("system"),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)

	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	require.Len(t, client.Requests(), 1)
	require.Equal(t, unified.CachePolicyOn, client.RequestAt(0).CachePolicy)
	requireMessageText(t, client.RequestAt(0).Messages[len(client.RequestAt(0).Messages)-1], "hello")
	requireRequestContainsText(t, client.RequestAt(0), "working_directory:")
}

func TestAgentMaterializesLoadedSkills(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md": {Data: []byte("---\nname: coder\ndescription: Coder skill\n---\nUse careful edits.")},
	}
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSpec(Spec{
			Name:         "coder",
			System:       "Base system.",
			Inference:    InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
			Skills:       []string{"coder"},
			SkillSources: []skill.Source{skill.FSSource("test", "test", fsys, "skills", skill.SourceEmbedded, 0)},
		}),
	)
	require.NoError(t, err)
	require.Contains(t, a.MaterializedSystem(), "Base system.")
	require.Contains(t, a.MaterializedSystem(), "Loaded skills:")
	require.Contains(t, a.MaterializedSystem(), "Use careful edits.")
	require.Equal(t, []string{"coder"}, a.skillRepo.LoadedNames())
}

func TestAgentReadsInstructionPathsFromSpecAndReflectsAgentsMarkdownUpdates(t *testing.T) {
	workspace := t.TempDir()
	agentsPath := filepath.Join(workspace, ".agents", "AGENTS.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(agentsPath), 0o755))
	require.NoError(t, os.WriteFile(agentsPath, []byte("first instructions"), 0o644))
	client := runnertest.NewClient(runnertest.TextStream("ok"), runnertest.TextStream("ok"))
	a, err := New(
		WithClient(client),
		WithWorkspace(workspace),
		WithSpec(Spec{
			Name:             "coder",
			System:           "Base system.",
			Inference:        InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
			InstructionPaths: []string{".agents/AGENTS.md"},
		}),
	)
	require.NoError(t, err)
	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	requireRequestContainsText(t, client.RequestAt(0), "first instructions")

	require.NoError(t, os.WriteFile(agentsPath, []byte("updated instructions"), 0o644))
	require.NoError(t, a.RunTurn(context.Background(), 2, "hello again"))
	requireRequestContainsText(t, client.RequestAt(1), "updated instructions")
}

func TestAgentRemovesMissingOptionalAgentsMarkdownFragmentOnNextTurn(t *testing.T) {
	workspace := t.TempDir()
	agentsPath := filepath.Join(workspace, "AGENTS.md")
	require.NoError(t, os.WriteFile(agentsPath, []byte("temporary instructions"), 0o644))
	client := runnertest.NewClient(runnertest.TextStream("ok"), runnertest.TextStream("ok"))
	a, err := New(
		WithClient(client),
		WithWorkspace(workspace),
		WithSpec(Spec{
			Name:             "coder",
			Inference:        InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
			InstructionPaths: []string{"AGENTS.md"},
		}),
	)
	require.NoError(t, err)
	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	requireRequestContainsText(t, client.RequestAt(0), "temporary instructions")

	require.NoError(t, os.Remove(agentsPath))
	require.NoError(t, a.RunTurn(context.Background(), 2, "hello again"))
	last := client.RequestAt(1).Messages[len(client.RequestAt(1).Messages)-1]
	requireMessageText(t, last, "hello again")
	require.NotContains(t, last.Content[0].(unified.TextPart).Text, "temporary instructions")
	require.Contains(t, last.Content[0].(unified.TextPart).Text, "<system-context>")
	require.Contains(t, last.Content[0].(unified.TextPart).Text, "removed")
}

func TestAgentPersistsAndResumesSession(t *testing.T) {
	dir := t.TempDir()
	firstClient := runnertest.NewClient(runnertest.TextStream("first response", "resp_text"))
	first, err := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	require.NoError(t, first.RunTurn(context.Background(), 1, "first task"))
	require.NotEmpty(t, first.SessionID())

	secondClient := runnertest.NewClient(runnertest.TextStream("second response", "resp_text2"))
	second, err := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithResumeSession(first.SessionID()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	require.Equal(t, first.SessionID(), second.SessionID())
	require.NoError(t, second.RunTurn(context.Background(), 1, "second task"))
	require.Len(t, secondClient.Requests(), 1)
	require.Len(t, secondClient.RequestAt(0).Messages, 3)
	requireRequestContainsText(t, secondClient.RequestAt(0), "working_directory:")
	requireRequestContainsText(t, secondClient.RequestAt(0), "current_time:")
	requireMessageText(t, secondClient.RequestAt(0).Messages[0], "first task")
	requireMessageText(t, secondClient.RequestAt(0).Messages[1], "first response")
	requireMessageText(t, secondClient.RequestAt(0).Messages[2], "second task")
}

func TestAgentResumesSessionByIDFromStoreDir(t *testing.T) {
	dir := t.TempDir()
	firstClient := runnertest.NewClient(runnertest.TextStream("first response", "resp_text"))
	first, err := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	require.NoError(t, first.RunTurn(context.Background(), 1, "first task"))

	secondClient := runnertest.NewClient(runnertest.TextStream("second response", "resp_text2"))
	second, err := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithResumeSession(first.SessionID()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	require.Equal(t, first.SessionID(), second.SessionID())
	require.NoError(t, second.RunTurn(context.Background(), 1, "second task"))
	require.Len(t, secondClient.RequestAt(0).Messages, 3)
	requireRequestContainsText(t, secondClient.RequestAt(0), "working_directory:")
	requireMessageText(t, secondClient.RequestAt(0).Messages[2], "second task")
}

func TestAgentUsesNativeContinuationWhenAvailable(t *testing.T) {
	dir := t.TempDir()
	providerIdentity := conversation.ProviderIdentity{
		ProviderName: testProvider,
		APIKind:      "openai.responses",
		NativeModel:  testModel,
	}
	firstClient := runnertest.NewClient([]unified.Event{
		unified.RouteEvent{
			ProviderName:         testProvider,
			TargetAPI:            "openai.responses",
			NativeModel:          testModel,
			ConsumerContinuation: unified.ContinuationPreviousResponseID,
			InternalContinuation: unified.ContinuationPreviousResponseID,
			Transport:            unified.TransportHTTPSSE,
		},
		unified.TextDeltaEvent{Text: "first response"},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "resp_text"},
	})
	first, err := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	first.providerIdentity = providerIdentity
	require.NoError(t, first.RunTurn(context.Background(), 1, "first task"))

	secondClient := runnertest.NewClient(runnertest.TextStream("second response", "resp_text2"))
	second, err := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithResumeSession(first.SessionID()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	second.providerIdentity = providerIdentity
	require.NoError(t, second.RunTurn(context.Background(), 1, "second task"))
	require.Len(t, secondClient.Requests(), 1)
	require.Len(t, secondClient.RequestAt(0).Messages, 1)
	requireRequestContainsText(t, secondClient.RequestAt(0), "working_directory:")
	requireRequestContainsText(t, secondClient.RequestAt(0), "current_time:")
	requireMessageText(t, secondClient.RequestAt(0).Messages[0], "second task")
	previousResponseID, ok, err := unified.GetExtension[string](secondClient.RequestAt(0).Extensions, unified.ExtOpenAIPreviousResponseID)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, "resp_text", previousResponseID)
}

func TestAgentReasoningConfigRequiresExplicitThinking(t *testing.T) {
	a := &Instance{inference: DefaultInferenceOptions()}
	_, ok := a.reasoningConfig()
	require.False(t, ok)

	a.inference.Thinking = ThinkingModeOn
	cfg, ok := a.reasoningConfig()
	require.True(t, ok)
	require.True(t, cfg.Expose)
	require.Equal(t, unified.ReasoningEffortMedium, cfg.Effort)
}

func TestAgentResetChangesSession(t *testing.T) {
	a, err := New(
		WithClient(runnertest.NewClient(runnertest.TextStream("ok"))),
		WithWorkspace(t.TempDir()),
		WithToolTimeout(5*time.Second),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	old := a.SessionID()
	a.Reset()
	require.NotEqual(t, old, a.SessionID())
}

func TestAgentSpecToolPatternsLimitActiveTools(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithTools(shell.Tools()),
		WithSpec(Spec{
			Name:      "coder",
			Inference: InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
			Tools:     []string{"bash"},
		}),
	)
	require.NoError(t, err)
	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	require.Len(t, client.Requests(), 1)
	require.Len(t, client.RequestAt(0).Tools, 1)
	require.Equal(t, "bash", client.RequestAt(0).Tools[0].Name)
}

func TestAgentSpecRoundTripIncludesResourceMetadata(t *testing.T) {
	a, err := New(
		WithClient(runnertest.NewClient(runnertest.TextStream("ok"))),
		WithWorkspace(t.TempDir()),
		WithSpec(Spec{
			Name:        "coder",
			Description: "Coder",
			System:      "You code.",
			Inference:   InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
			Tools:       []string{"bash"},
			Skills:      []string{"coder"},
			Commands:    []string{"review"},
		}),
	)
	require.NoError(t, err)
	spec := a.Spec()
	require.Equal(t, []string{"bash"}, spec.Tools)
	require.Equal(t, []string{"coder"}, spec.Skills)
	require.Equal(t, []string{"review"}, spec.Commands)
}

func requireMessageText(t *testing.T, msg unified.Message, want string) {
	t.Helper()
	require.NotEmpty(t, msg.Content)
	first, ok := msg.Content[0].(unified.TextPart)
	require.True(t, ok)
	if len(msg.Content) == 1 {
		require.Equal(t, want, first.Text)
		return
	}
	require.Contains(t, first.Text, "<system-context>")
	second, ok := msg.Content[1].(unified.TextPart)
	require.True(t, ok)
	require.Equal(t, want, second.Text)
}

func requireRequestContainsText(t *testing.T, req unified.Request, want string) {
	t.Helper()
	for _, msg := range req.Messages {
		for _, part := range msg.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				return
			}
		}
	}
	t.Fatalf("request does not contain text %q in %#v", want, req.Messages)
}

func requireRequestNotContainsText(t *testing.T, req unified.Request, want string) {
	t.Helper()
	for _, msg := range req.Messages {
		for _, part := range msg.Content {
			text, ok := part.(unified.TextPart)
			if ok && strings.Contains(text.Text, want) {
				t.Fatalf("request unexpectedly contains text %q in %#v", want, req.Messages)
			}
		}
	}
}

func TestAgentActivateSkillRefreshesMaterializedSystem(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/base/SKILL.md":  {Data: []byte("---\nname: base\ndescription: Base\n---\n# Base")},
		"skills/extra/SKILL.md": {Data: []byte("---\nname: extra\ndescription: Extra\n---\n# Extra")},
	}
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	a, err := New(
		WithSpec(Spec{
			Name:         "coder",
			System:       "Base system.",
			Skills:       []string{"base"},
			SkillSources: []skill.Source{skill.FSSource("skills", "skills", fsys, "skills", skill.SourceEmbedded, 0)},
		}),
		WithClient(client),
		WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)
	require.Contains(t, a.MaterializedSystem(), "# Base")
	require.NotContains(t, a.MaterializedSystem(), "# Extra")

	status, err := a.ActivateSkill("extra")
	require.NoError(t, err)
	require.Equal(t, skill.StatusDynamic, status)
	require.Contains(t, a.MaterializedSystem(), "# Extra")
}

func TestAgentActivateSkillReferencesRequiresActiveSkill(t *testing.T) {
	fsys := fstest.MapFS{
		"skills/base/SKILL.md":             {Data: []byte("---\nname: base\ndescription: Base\n---\n# Base")},
		"skills/base/references/review.md": {Data: []byte("---\ntrigger: review\n---\nReview body")},
	}
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	a, err := New(
		WithSpec(Spec{
			Name:         "coder",
			SkillSources: []skill.Source{skill.FSSource("skills", "skills", fsys, "skills", skill.SourceEmbedded, 0)},
		}),
		WithClient(client),
		WithWorkspace(t.TempDir()),
	)
	require.NoError(t, err)

	_, err = a.ActivateSkillReferences("base", []string{"references/review.md"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "require the skill to be active first")
}

func TestAgentResumesActivatedSkillAcrossSession(t *testing.T) {
	dir := t.TempDir()
	fsys := fstest.MapFS{
		"skills/base/SKILL.md":  {Data: []byte("---\nname: base\ndescription: Base\n---\n# Base")},
		"skills/extra/SKILL.md": {Data: []byte("---\nname: extra\ndescription: Extra\n---\n# Extra")},
	}
	firstClient := runnertest.NewClient(runnertest.TextStream("ok"))
	first, err := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithSpec(Spec{
			Name:         "coder",
			Skills:       []string{"base"},
			SkillSources: []skill.Source{skill.FSSource("skills", "skills", fsys, "skills", skill.SourceEmbedded, 0)},
			Inference:    InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
		}),
	)
	require.NoError(t, err)
	_, err = first.ActivateSkill("extra")
	require.NoError(t, err)
	require.Contains(t, first.MaterializedSystem(), "# Extra")

	secondClient := runnertest.NewClient(runnertest.TextStream("ok"))
	second, err := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithResumeSession(first.SessionID()),
		WithSpec(Spec{
			Name:         "coder",
			Skills:       []string{"base"},
			SkillSources: []skill.Source{skill.FSSource("skills", "skills", fsys, "skills", skill.SourceEmbedded, 0)},
			Inference:    InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
		}),
	)
	require.NoError(t, err)
	require.Contains(t, second.MaterializedSystem(), "# Extra")
	require.Equal(t, skill.StatusDynamic, second.SkillActivationState().Status("extra"))
}

func TestAgentResumesActivatedSkillReferenceAcrossSession(t *testing.T) {
	dir := t.TempDir()
	fsys := fstest.MapFS{
		"skills/base/SKILL.md":             {Data: []byte("---\nname: base\ndescription: Base\n---\n# Base")},
		"skills/base/references/review.md": {Data: []byte("---\ntrigger: review\n---\nReview body")},
	}
	firstClient := runnertest.NewClient(runnertest.TextStream("ok"))
	first, err := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithSpec(Spec{
			Name:         "coder",
			Skills:       []string{"base"},
			SkillSources: []skill.Source{skill.FSSource("skills", "skills", fsys, "skills", skill.SourceEmbedded, 0)},
			Inference:    InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
		}),
	)
	require.NoError(t, err)
	_, err = first.ActivateSkillReferences("base", []string{"references/review.md"})
	require.NoError(t, err)
	require.Contains(t, first.MaterializedSystem(), "Review body")

	secondClient := runnertest.NewClient(runnertest.TextStream("ok"))
	second, err := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithResumeSession(first.SessionID()),
		WithSpec(Spec{
			Name:         "coder",
			Skills:       []string{"base"},
			SkillSources: []skill.Source{skill.FSSource("skills", "skills", fsys, "skills", skill.SourceEmbedded, 0)},
			Inference:    InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
		}),
	)
	require.NoError(t, err)
	require.Contains(t, second.MaterializedSystem(), "Review body")
	require.Len(t, second.SkillActivationState().ActiveReferences("base"), 1)
}

// ── WithContextProviders tests ──────────────────────────────────────────────

type stubProvider struct {
	key agentcontext.ProviderKey
}

func (p stubProvider) Key() agentcontext.ProviderKey { return p.key }
func (p stubProvider) GetContext(context.Context, agentcontext.Request) (agentcontext.ProviderContext, error) {
	return agentcontext.ProviderContext{}, nil
}

func TestWithContextProvidersAddsExtraProviders(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	extra := stubProvider{key: "custom"}
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
		WithContextProviders(extra),
	)
	require.NoError(t, err)

	// The extra provider should appear in the context state after a turn.
	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	state := a.ContextState()
	require.Contains(t, state, "custom")
}

func TestWithContextProvidersDedupsBuiltinKeys(t *testing.T) {
	// A plugin provider with key "environment" should replace the built-in.
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	override := stubProvider{key: "environment"}
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
		WithContextProviders(override),
	)
	require.NoError(t, err)

	// Run a turn — should not fail with duplicate key error.
	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))

	// The context state should still mention environment (from the override).
	state := a.ContextState()
	require.Contains(t, state, "environment")
}

func TestWithContextProviderFactoriesRunsAfterSkillInit(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	var capturedInfo ContextProviderFactoryInfo
	factory := func(info ContextProviderFactoryInfo) []agentcontext.Provider {
		capturedInfo = info
		return []agentcontext.Provider{stubProvider{key: "factory_test"}}
	}
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
		WithContextProviderFactories(factory),
	)
	require.NoError(t, err)

	// Factory should have been called with populated info.
	require.NotEmpty(t, capturedInfo.Workspace)
	require.Equal(t, testProvider+"/"+testModel, capturedInfo.Model)

	// The factory-produced provider should appear in context.
	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	state := a.ContextState()
	require.Contains(t, state, "factory_test")
}

func TestWithContextProviderFactoriesSkillStateAvailable(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("ok"))
	fsys := fstest.MapFS{
		"skills/coder/SKILL.md": {Data: []byte("---\nname: coder\ndescription: Coder\n---\n# Coder")},
	}
	var capturedInfo ContextProviderFactoryInfo
	factory := func(info ContextProviderFactoryInfo) []agentcontext.Provider {
		capturedInfo = info
		return nil
	}
	_, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSpec(Spec{
			Name:         "coder",
			Inference:    InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000},
			Skills:       []string{"coder"},
			SkillSources: []skill.Source{skill.FSSource("test", "test", fsys, "skills", skill.SourceEmbedded, 0)},
		}),
		WithContextProviderFactories(factory),
	)
	require.NoError(t, err)

	// Factory should see the initialized skill repo and state.
	require.NotNil(t, capturedInfo.SkillRepository)
	require.NotNil(t, capturedInfo.SkillState)
	require.Equal(t, []string{"coder"}, capturedInfo.SkillRepository.LoadedNames())
}

func TestAgentCompactReplacesOlderMessages(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream("resp3"),
		runnertest.TextStream("Summary of old conversation."),
	)
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSystem("system"),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, a.RunTurn(ctx, 1, "old message 1"))
	require.NoError(t, a.RunTurn(ctx, 2, "old message 2"))
	require.NoError(t, a.RunTurn(ctx, 3, "recent message"))

	result, err := a.CompactWithOptions(ctx, CompactOptions{KeepWindow: 4})
	require.NoError(t, err)
	require.Equal(t, 2, result.ReplacedCount)
	require.Greater(t, result.TokensBefore, result.TokensAfter)
	require.NotEmpty(t, result.CompactionNodeID)

	// 3 turns + 1 summary request = 4 total requests.
	require.Len(t, client.Requests(), 4)
	summaryReq := client.RequestAt(3)
	require.Len(t, summaryReq.Messages, 1)
	require.Equal(t, unified.RoleUser, summaryReq.Messages[0].Role)
	require.True(t, summaryReq.Stream)
	require.Equal(t, "Summary of old conversation.", result.Summary)
}

func TestAgentCompactWithProvidedSummarySkipsLLMCall(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream("resp3"),
	)
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSystem("system"),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, a.RunTurn(ctx, 1, "old"))
	require.NoError(t, a.RunTurn(ctx, 2, "old2"))
	require.NoError(t, a.RunTurn(ctx, 3, "recent"))

	result, err := a.CompactWithOptions(ctx, CompactOptions{
		KeepWindow: 4,
		Summary:    "Manual summary.",
	})
	require.NoError(t, err)
	require.Equal(t, 2, result.ReplacedCount)
	// No extra LLM call — still only 3 requests.

	require.Len(t, client.Requests(), 3)
}

func TestAgentCompactionLifecycleEventsStreamSummary(t *testing.T) {
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream("resp3"),
		[]unified.Event{
			unified.TextDeltaEvent{Text: "Summary "},
			unified.TextDeltaEvent{Text: "stream."},
			unified.UsageEvent{Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 5}, {Kind: unified.TokenKindOutput, Count: 2}}},
			unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "summary"},
		},
	)
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSystem("system"),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	var events []CompactionEvent
	cancel := a.AddCompactionEventHandler(func(event CompactionEvent) { events = append(events, event) })
	defer cancel()

	ctx := context.Background()
	require.NoError(t, a.RunTurn(ctx, 1, "old"))
	require.NoError(t, a.RunTurn(ctx, 2, "old2"))
	require.NoError(t, a.RunTurn(ctx, 3, "recent"))
	result, err := a.CompactWithOptions(ctx, CompactOptions{KeepWindow: 4})
	require.NoError(t, err)
	require.Equal(t, "Summary stream.", result.Summary)

	// Verify compaction events were emitted through the handler.
	var summaryText string
	for _, event := range events {
		if event.Type == CompactionEventSummaryDelta {
			summaryText += event.SummaryDelta
		}
	}
	require.Contains(t, summaryText, "Summary stream.")

	types := make([]CompactionEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	require.Contains(t, types, CompactionEventStarted)
	require.Contains(t, types, CompactionEventSummaryDelta)
	require.Contains(t, types, CompactionEventSummaryCompleted)
	require.Contains(t, types, CompactionEventCommitted)
	records := a.Tracker().Records()
	require.NotEmpty(t, records)
	require.Equal(t, "compaction", records[len(records)-1].Source)
	require.Equal(t, "compaction", records[len(records)-1].Dims.Labels["operation"])
}

func TestAgentCompactTooShortReturnsError(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("resp"))
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)

	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))

	_, err = a.Compact(context.Background())
	require.ErrorIs(t, err, ErrNothingToCompact)
}

func TestAgentAutoCompactionTriggersAboveThreshold(t *testing.T) {
	largeResponse := strings.Repeat("x", 400_000)
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream(largeResponse),
		runnertest.TextStream("Summary of conversation."),
	)
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
		WithAutoCompaction(AutoCompactionConfig{
			Enabled:            true,
			ContextWindowRatio: 0.01,
			KeepWindow:         2,
		}),
	)
	require.NoError(t, err)
	a.contextWindow = 100_000
	var events []CompactionEvent
	cancel := a.AddCompactionEventHandler(func(event CompactionEvent) { events = append(events, event) })
	defer cancel()

	ctx := context.Background()
	require.NoError(t, a.RunTurn(ctx, 1, "hello"))
	require.NoError(t, a.RunTurn(ctx, 2, "generate large"))
	var committed bool
	for _, event := range events {
		if event.Type == CompactionEventCommitted && event.Trigger == CompactionTriggerAuto {
			committed = true
		}
	}
	require.True(t, committed, "expected auto-compaction committed event")
}

func TestAgentAutoCompactionEnabledByDefault(t *testing.T) {
	largeResponse := strings.Repeat("x", 400_000)
	client := runnertest.NewClient(
		runnertest.TextStream("resp1"),
		runnertest.TextStream("resp2"),
		runnertest.TextStream(largeResponse),
		runnertest.TextStream("Summary of conversation."),
	)
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	a.contextWindow = 100_000
	var events []CompactionEvent
	cancel := a.AddCompactionEventHandler(func(event CompactionEvent) { events = append(events, event) })
	defer cancel()

	ctx := context.Background()
	require.NoError(t, a.RunTurn(ctx, 1, "hello"))
	require.NoError(t, a.RunTurn(ctx, 2, "continue"))
	require.NoError(t, a.RunTurn(ctx, 3, "generate large"))
	var committed bool
	for _, event := range events {
		if event.Type == CompactionEventCommitted && event.Trigger == CompactionTriggerAuto {
			committed = true
		}
	}
	require.True(t, committed, "expected auto-compaction committed event")
}

func TestAgentAutoCompactionCanBeDisabled(t *testing.T) {
	largeResponse := strings.Repeat("x", 400_000)
	client := runnertest.NewClient(runnertest.TextStream(largeResponse))
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
		WithAutoCompaction(AutoCompactionConfig{Enabled: false}),
	)
	require.NoError(t, err)
	var events []CompactionEvent
	cancel := a.AddCompactionEventHandler(func(event CompactionEvent) { events = append(events, event) })
	defer cancel()

	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	for _, event := range events {
		require.NotEqual(t, CompactionEventCommitted, event.Type, "compaction should not trigger when disabled")
	}
}

func TestAgentAutoCompactionThresholdUsesContextWindow(t *testing.T) {
	a := &Instance{
		contextWindow:  200_000,
		autoCompaction: AutoCompactionConfig{Enabled: true},
	}
	require.Equal(t, 170_000, a.autoCompactionThreshold())
}

func TestAgentAutoCompactionThresholdUsesConfiguredContextWindowRatio(t *testing.T) {
	a := &Instance{
		contextWindow:  200_000,
		autoCompaction: AutoCompactionConfig{Enabled: true, ContextWindowRatio: 0.5},
	}
	require.Equal(t, 100_000, a.autoCompactionThreshold())
}

func TestAgentAutoCompactionThresholdClampsContextWindowRatio(t *testing.T) {
	a := &Instance{
		contextWindow:  200_000,
		autoCompaction: AutoCompactionConfig{Enabled: true, ContextWindowRatio: 1.2},
	}
	require.Equal(t, 200_000, a.autoCompactionThreshold())
}
func TestAgentAutoCompactionIgnoresDeprecatedAbsoluteThreshold(t *testing.T) {
	a := &Instance{
		contextWindow:  200_000,
		autoCompaction: AutoCompactionConfig{Enabled: true, TokenThreshold: 50_000, ContextWindowRatio: 0.5},
	}
	require.Equal(t, 100_000, a.autoCompactionThreshold())
}

func TestAgentAutoCompactionThresholdFallback(t *testing.T) {
	a := &Instance{
		autoCompaction: AutoCompactionConfig{Enabled: true},
	}
	require.Equal(t, 85_000, a.autoCompactionThreshold())
	policy := a.CompactionPolicy()
	require.True(t, policy.Fallback)
	require.Equal(t, "fallback", policy.ContextWindowSource)
}

func TestAgentReplaysUsageEventsAcrossSession(t *testing.T) {
	dir := t.TempDir()
	client := runnertest.NewClient([]unified.Event{
		unified.TextDeltaEvent{Text: "ok"},
		unified.UsageEvent{Tokens: unified.TokenItems{{Kind: unified.TokenKindInputNew, Count: 7}, {Kind: unified.TokenKindOutput, Count: 3}}},
		unified.CompletedEvent{FinishReason: unified.FinishReasonStop, MessageID: "msg_usage"},
	})
	first, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithSpec(Spec{Name: "coder", Inference: InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	require.NoError(t, first.RunTurn(context.Background(), 1, "hello"))
	require.Len(t, first.Tracker().Records(), 1)

	second, err := New(
		WithClient(runnertest.NewClient(runnertest.TextStream("ok"))),
		WithWorkspace(t.TempDir()),
		WithThreadStore(threadjsonlstore.Open(dir)),
		WithResumeSession(first.SessionID()),
		WithSpec(Spec{Name: "coder", Inference: InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}}),
	)
	require.NoError(t, err)
	records := second.Tracker().Records()
	require.Len(t, records, 1)
	require.Equal(t, 7, records[0].Usage.Tokens.Count(unified.TokenKindInputNew))
	require.Equal(t, 3, records[0].Usage.Tokens.Count(unified.TokenKindOutput))
	require.Equal(t, second.SessionID(), records[0].Dims.SessionID)
}
