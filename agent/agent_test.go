package agent

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/codewandler/agentsdk/conversation"
	"github.com/codewandler/agentsdk/runnertest"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

const (
	testProvider = "test"
	testModel    = "model"
)

func TestAgentRunTurnUsesCacheKeyAndRecordsRequest(t *testing.T) {
	client := runnertest.NewClient(runnertest.TextStream("response", "resp1"))
	a, err := New(
		WithClient(client),
		WithWorkspace(t.TempDir()),
		WithSystem("system"),
		WithCacheKeyPrefix("test:"),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)

	require.NoError(t, a.RunTurn(context.Background(), 1, "hello"))
	require.Len(t, client.Requests(), 1)
	require.Equal(t, unified.CachePolicyOn, client.RequestAt(0).CachePolicy)
	require.Equal(t, "test:"+a.SessionID(), client.RequestAt(0).CacheKey)
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
	require.Equal(t, []string{"coder"}, a.SkillRepository().LoadedNames())
}

func TestAgentPersistsAndResumesSession(t *testing.T) {
	dir := t.TempDir()
	firstClient := runnertest.NewClient(runnertest.TextStream("first response", "resp_text"))
	first, err := New(
		WithClient(firstClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	require.NoError(t, first.RunTurn(context.Background(), 1, "first task"))
	require.NotEmpty(t, first.SessionStorePath())

	secondClient := runnertest.NewClient(runnertest.TextStream("second response", "resp_text2"))
	second, err := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithResumeSession(first.SessionStorePath()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	require.Equal(t, first.SessionID(), second.SessionID())
	require.NoError(t, second.RunTurn(context.Background(), 1, "second task"))
	require.Len(t, secondClient.Requests(), 1)
	require.Len(t, secondClient.RequestAt(0).Messages, 5)
	requireMessageText(t, secondClient.RequestAt(0).Messages[0], "first task")
	requireMessageText(t, secondClient.RequestAt(0).Messages[1], "first response")
	requireRequestContainsText(t, secondClient.RequestAt(0), "working_directory:")
	requireRequestContainsText(t, secondClient.RequestAt(0), "current_time:")
	requireMessageText(t, secondClient.RequestAt(0).Messages[4], "second task")
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
		WithSessionStoreDir(dir),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	first.providerIdentity = providerIdentity
	require.NoError(t, first.RunTurn(context.Background(), 1, "first task"))

	secondClient := runnertest.NewClient(runnertest.TextStream("second response", "resp_text2"))
	second, err := New(
		WithClient(secondClient),
		WithWorkspace(t.TempDir()),
		WithSessionStoreDir(dir),
		WithResumeSession(first.SessionStorePath()),
		WithInferenceOptions(InferenceOptions{Model: testProvider + "/" + testModel, MaxTokens: 1000}),
	)
	require.NoError(t, err)
	second.providerIdentity = providerIdentity
	require.NoError(t, second.RunTurn(context.Background(), 1, "second task"))
	require.Len(t, secondClient.Requests(), 1)
	require.Len(t, secondClient.RequestAt(0).Messages, 3)
	requireRequestContainsText(t, secondClient.RequestAt(0), "working_directory:")
	requireRequestContainsText(t, secondClient.RequestAt(0), "current_time:")
	requireMessageText(t, secondClient.RequestAt(0).Messages[2], "second task")
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
	require.Len(t, msg.Content, 1)
	text, ok := msg.Content[0].(unified.TextPart)
	require.True(t, ok)
	require.Equal(t, want, text.Text)
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
