package contextproviders

import (
	"context"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
	"github.com/stretchr/testify/require"
)

func TestFileProviderRendersFilesAsFragments(t *testing.T) {
	provider := FileContext("files", []FileSpec{{Path: "AGENTS.md", Key: "agents_md/AGENTS.md"}},
		WithFileReader(mapFileReader(t, fstest.MapFS{"AGENTS.md": {Data: []byte("follow repo notes\n")}})),
	)
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Len(t, providerContext.Fragments, 1)
	require.Equal(t, agentcontext.FragmentKey("agents_md/AGENTS.md"), providerContext.Fragments[0].Key)
	require.Equal(t, unified.RoleUser, providerContext.Fragments[0].Role)
	require.Equal(t, agentcontext.AuthorityUser, providerContext.Fragments[0].Authority)
	require.Equal(t, "follow repo notes", providerContext.Fragments[0].Content)
	require.NotEmpty(t, providerContext.Fingerprint)
	require.NotNil(t, providerContext.Snapshot)
}

func TestFileProviderSkipsOptionalMissingAndEmptyFiles(t *testing.T) {
	provider := FileContext("files", []FileSpec{{Path: "missing.md", Optional: true}, {Path: "empty.md", Optional: true}},
		WithFileReader(mapFileReader(t, fstest.MapFS{"empty.md": {Data: []byte("  \n\t")}})),
	)
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Empty(t, providerContext.Fragments)
	require.NotEmpty(t, providerContext.Fingerprint)
}

func TestFileProviderStateFingerprintReusesPreviousSnapshotWhenStatUnchanged(t *testing.T) {
	modTime := time.Unix(1714200000, 0)
	provider := FileContext("files", []FileSpec{{Path: "AGENTS.md", Key: "agents_md/AGENTS.md"}},
		WithFileReader(func(path string) ([]byte, fs.FileInfo, error) {
			require.Equal(t, "AGENTS.md", path)
			return []byte("follow repo notes"), fakeFileInfo{name: path, size: int64(len("follow repo notes")), modTime: modTime}, nil
		}),
	)
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	previous := recordFromProviderContext(t, provider.Key(), providerContext)

	fp, ok, err := provider.StateFingerprint(context.Background(), agentcontext.Request{Previous: &previous})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, providerContext.Fingerprint, fp)
}

func TestFileProviderStateFingerprintFallsBackWhenSnapshotChanged(t *testing.T) {
	modTime := time.Unix(1714200000, 0)
	nextModTime := modTime.Add(time.Minute)
	statReads := 0
	provider := FileContext("files", []FileSpec{{Path: "AGENTS.md", Key: "agents_md/AGENTS.md"}},
		WithFileReader(func(path string) ([]byte, fs.FileInfo, error) {
			statReads++
			infoTime := modTime
			if statReads > 1 {
				infoTime = nextModTime
			}
			return []byte("follow repo notes"), fakeFileInfo{name: path, size: int64(len("follow repo notes")), modTime: infoTime}, nil
		}),
	)
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	previous := recordFromProviderContext(t, provider.Key(), providerContext)

	fp, ok, err := provider.StateFingerprint(context.Background(), agentcontext.Request{Previous: &previous})
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, providerContext.Fingerprint, fp)
	require.GreaterOrEqual(t, statReads, 2)
}

func TestAgentsMarkdownUsesStableKeysAndThreadCache(t *testing.T) {
	provider := AgentsMarkdown([]string{"./nested/AGENTS.md"}, AgentsMarkdownOption(WithFileReader(mapFileReader(t, fstest.MapFS{
		"nested/AGENTS.md": {Data: []byte("repo notes")},
	}))))
	providerContext, err := provider.GetContext(context.Background(), agentcontext.Request{})
	require.NoError(t, err)
	require.Len(t, providerContext.Fragments, 1)
	fragment := providerContext.Fragments[0]
	require.Equal(t, agentcontext.FragmentKey("agents_md/nested_AGENTS.md"), fragment.Key)
	require.Equal(t, agentcontext.CacheThread, fragment.CachePolicy.Scope)
	require.True(t, fragment.CachePolicy.Stable)
}

type fakeFileInfo struct {
	name    string
	size    int64
	modTime time.Time
}

func (f fakeFileInfo) Name() string       { return f.name }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() fs.FileMode  { return 0o644 }
func (f fakeFileInfo) ModTime() time.Time { return f.modTime }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func mapFileReader(t *testing.T, fsys fstest.MapFS) func(path string) ([]byte, fs.FileInfo, error) {
	t.Helper()
	return func(path string) ([]byte, fs.FileInfo, error) {
		clean := path
		clean = strings.TrimPrefix(clean, "./")
		data, err := fs.ReadFile(fsys, clean)
		if err != nil {
			return nil, nil, err
		}
		entry, err := fs.Stat(fsys, clean)
		if err != nil {
			return nil, nil, err
		}
		return data, entry, nil
	}
}

func recordFromProviderContext(t *testing.T, key agentcontext.ProviderKey, providerContext agentcontext.ProviderContext) agentcontext.ProviderRenderRecord {
	t.Helper()
	fragments := providerContext.Fragments
	if providerContext.Fingerprint == "" {
		providerContext.Fingerprint = agentcontext.ProviderFingerprint(fragments)
	}
	record := agentcontext.ProviderRenderRecord{
		ProviderKey: key,
		Fingerprint: providerContext.Fingerprint,
		Snapshot:    providerContext.Snapshot,
		Fragments:   make(map[agentcontext.FragmentKey]agentcontext.RenderedFragmentRecord, len(fragments)),
	}
	for _, fragment := range fragments {
		fragment.Fingerprint = agentcontext.FragmentFingerprint(fragment)
		record.Fragments[fragment.Key] = agentcontext.RenderedFragmentRecord{
			Key:         fragment.Key,
			Fingerprint: fragment.Fingerprint,
			Fragment:    fragment,
		}
	}
	return record
}
