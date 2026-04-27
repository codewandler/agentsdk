package agentcontext

import (
	"strings"
	"testing"

	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestFormatRenderRecordsShowsProvidersAndFragments(t *testing.T) {
	out := FormatRenderRecords(map[ProviderKey]ProviderRenderRecord{
		"env": {
			ProviderKey: "env",
			Fingerprint: "provider-fp",
			Fragments: map[FragmentKey]RenderedFragmentRecord{
				"env/pwd": {
					Key:         "env/pwd",
					Fingerprint: "fragment-fp",
					Fragment: ContextFragment{
						Key:     "env/pwd",
						Role:    unified.RoleUser,
						Content: "working_directory: /repo",
					},
				},
			},
		},
	})

	require.Contains(t, out, "provider: env")
	require.Contains(t, out, "fingerprint: provider-fp")
	require.Contains(t, out, "env/pwd [active] fp=fragment-fp")
	require.Contains(t, out, "working_directory: /repo")
}

func TestFormatRenderRecordsEmpty(t *testing.T) {
	require.Equal(t, "context: no render state", FormatRenderRecords(nil))
	require.False(t, strings.HasSuffix(FormatRenderRecords(nil), "\n"))
}
