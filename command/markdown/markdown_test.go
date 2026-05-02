package markdown

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/codewandler/agentsdk/command"
	"github.com/stretchr/testify/require"
)

func TestMarkdownCommandRendersAgentTurn(t *testing.T) {
	cmd, err := New("review.md", []byte(`---
description: Review changes
aliases: [rv]
argument-hint: focus
---
Review this repo.

Focus: {{.Query}}
Raw: {{.Raw}}
`))
	require.NoError(t, err)
	require.Equal(t, "review", cmd.Spec().Name)
	require.Equal(t, []string{"rv"}, cmd.Spec().Aliases)
	require.Equal(t, "focus", cmd.Spec().ArgumentHint)

	result, err := cmd.Execute(context.Background(), command.Params{
		Raw:   "security",
		Args:  []string{"security"},
		Flags: map[string]string{},
	})
	require.NoError(t, err)
	require.Equal(t, command.ResultAgentTurn, result.Kind)
	input, ok := command.AgentTurnInput(result)
	require.True(t, ok)
	require.Contains(t, input, "Focus: security")
	require.Contains(t, input, "Raw: security")
}

func TestLoadFSLoadsMarkdownCommands(t *testing.T) {
	fsys := fstest.MapFS{
		".agents/commands/commit.md": {Data: []byte("---\ndescription: Commit changes\n---\nCommit now.")},
		".agents/commands/skip.txt":  {Data: []byte("ignore")},
	}
	cmds, err := LoadFS(fsys, ".agents/commands")
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	require.Equal(t, "commit", cmds[0].Spec().Name)
}
