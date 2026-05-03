package harness

import (
	"context"
	"strings"
	"testing"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/command"
	"github.com/stretchr/testify/require"
)

func TestFormatAgentCommandCatalogRendersToolContext(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)

	text := FormatAgentCommandCatalog(session.CommandCatalog(CommandCatalogAgentCallable()))

	require.Contains(t, text, "session_command")
	require.Contains(t, text, "workflow list: List workflows")
	require.Contains(t, text, "workflow show: Show workflow")
	require.Contains(t, text, "name: string required source=arg")
	require.NotContains(t, text, "workflow start")
}

func TestFormatAgentCommandCatalogRendersEmptyCatalog(t *testing.T) {
	text := FormatAgentCommandCatalog(nil)

	require.Equal(t, "No agent-callable session commands are available.", text)
	require.False(t, strings.HasSuffix(text, "\n"))
}

func TestFormatAgentCommandCatalogRendersEnums(t *testing.T) {
	text := FormatAgentCommandCatalog([]CommandCatalogEntry{{Descriptor: command.Descriptor{
		Path:        []string{"workflow", "runs"},
		Description: "List workflow runs",
		Input: command.InputDescriptor{Fields: []command.InputFieldDescriptor{{
			Name:       "status",
			Source:     command.InputSourceFlag,
			Type:       command.InputTypeString,
			EnumValues: []string{"running", "succeeded", "failed"},
		}}},
	}}})

	require.Contains(t, text, "workflow runs: List workflow runs")
	require.Contains(t, text, "status: string optional source=flag enum=running|succeeded|failed")
}

func TestSessionAgentCommandCatalogContextProvider(t *testing.T) {
	session := newCommandEnvelopeTestSession(t)
	provider := session.AgentCommandCatalogContextProvider()

	require.Equal(t, AgentCommandCatalogProviderKey, provider.Key())

	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Len(t, providerContext.Fragments, 1)
	fragment := providerContext.Fragments[0]
	require.Equal(t, AgentCommandCatalogFragmentKey, fragment.Key)
	require.Equal(t, agentcontext.AuthorityDeveloper, fragment.Authority)
	require.True(t, fragment.CachePolicy.Stable)
	require.Equal(t, agentcontext.CacheThread, fragment.CachePolicy.Scope)
	require.Contains(t, fragment.Content, "workflow show")
	require.NotEmpty(t, providerContext.Fingerprint)

	fingerprint, ok, err := provider.(agentcontext.FingerprintingProvider).StateFingerprint(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, providerContext.Fingerprint, fingerprint)
}
