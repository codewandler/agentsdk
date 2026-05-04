package skills

import (
	"context"
	"encoding/json"
	"testing"
	"testing/fstest"

	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/stretchr/testify/require"
)

type testCtx struct {
	context.Context
	extra map[string]any
}

func (c testCtx) WorkDir() string       { return "." }
func (c testCtx) AgentID() string       { return "agent" }
func (c testCtx) SessionID() string     { return "session" }
func (c testCtx) Extra() map[string]any { return c.extra }

func TestSkillToolRequiresActions(t *testing.T) {
	ctx := testCtx{Context: context.Background(), extra: map[string]any{skill.ContextKey: &skill.ActivationState{}}}
	result, err := Tools()[0].Execute(ctx, json.RawMessage(`{"actions":[]}`))
	require.NoError(t, err)
	require.True(t, result.IsError())
}

func TestSkillToolActivatesSkillAndReferences(t *testing.T) {
	state := testState(t)
	_, err := state.ActivateSkill("architecture")
	require.NoError(t, err)

	ctx := testCtx{Context: context.Background(), extra: map[string]any{skill.ContextKey: state}}
	result, err := Tools()[0].Execute(ctx, json.RawMessage(`{"actions":[{"action":"activate","skill":"architecture","references":["references/tradeoffs.md"]}]}`))
	require.NoError(t, err)
	require.False(t, result.IsError())
	require.Contains(t, result.String(), `activate "architecture": already_active refs=references/tradeoffs.md`)
}

func TestSkillToolRejectsUnknownAction(t *testing.T) {
	ctx := testCtx{Context: context.Background(), extra: map[string]any{skill.ContextKey: testState(t)}}
	result, err := Tools()[0].Execute(ctx, json.RawMessage(`{"actions":[{"action":"noop","skill":"architecture"}]}`))
	require.NoError(t, err)
	require.Contains(t, result.String(), `unsupported action "noop"`)
}

func TestSkillToolRejectsInactiveSkillReferences(t *testing.T) {
	ctx := testCtx{Context: context.Background(), extra: map[string]any{skill.ContextKey: testState(t)}}
	result, err := Tools()[0].Execute(ctx, json.RawMessage(`{"actions":[{"action":"activate","skill":"architecture","references":["references/tradeoffs.md"]}]}`))
	require.NoError(t, err)
	require.Contains(t, result.String(), `references for "architecture" require the skill to be active first`)
}

func TestSkillToolUsesSessionAwareActivator(t *testing.T) {
	state := testState(t)
	activator := &recordingActivator{state: state}
	ctx := testCtx{Context: context.Background(), extra: map[string]any{
		skill.ContextKey:          state,
		skill.ActivatorContextKey: activator,
	}}
	result, err := Tools()[0].Execute(ctx, json.RawMessage(`{"actions":[{"action":"activate","skill":"architecture"}]}`))
	require.NoError(t, err)
	require.False(t, result.IsError())
	require.Equal(t, []string{"architecture"}, activator.skills)
}

type recordingActivator struct {
	state  *skill.ActivationState
	skills []string
	refs   []string
}

func (a *recordingActivator) ActivateSkill(name string) (skill.Status, error) {
	a.skills = append(a.skills, name)
	return a.state.ActivateSkill(name)
}

func (a *recordingActivator) ActivateSkillReferences(name string, refs []string) ([]string, error) {
	a.refs = append(a.refs, refs...)
	return a.state.ActivateReferences(name, refs)
}

func testState(t *testing.T) *skill.ActivationState {
	t.Helper()
	fsys := fstest.MapFS{
		"skills/architecture/SKILL.md":                {Data: []byte("---\nname: architecture\ndescription: Architecture help\n---\nDecide carefully")},
		"skills/architecture/references/tradeoffs.md": {Data: []byte("---\ntrigger: tradeoffs\n---\ntradeoffs reference")},
	}
	repo, err := skill.NewRepository([]skill.Source{skill.FSSource("skills", "skills", fsys, "skills", skill.SourceEmbedded, 0)}, nil)
	require.NoError(t, err)
	state, err := skill.NewActivationState(repo, nil)
	require.NoError(t, err)
	return state
}

var _ tool.Tool = Tools()[0]
