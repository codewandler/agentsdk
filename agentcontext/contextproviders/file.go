package contextproviders

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/llmadapter/unified"
)

// FileSpec describes one file whose content becomes a context fragment.
type FileSpec struct {
	Path      string
	Key       agentcontext.FragmentKey
	Role      unified.Role
	Authority agentcontext.FragmentAuthority
	Optional  bool
}

// FileProviderOption configures a FileProvider.
type FileProviderOption func(*FileProvider)

type fileSnapshot struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	ModTime int64  `json:"mod_time"`
}

// FileProvider reads files from disk and renders each non-empty file as a
// separate context fragment. Relative paths resolve against the configured
// working directory.
type FileProvider struct {
	key      agentcontext.ProviderKey
	cache    agentcontext.CachePolicy
	workDir  string
	files    []FileSpec
	readFile func(path string) ([]byte, fs.FileInfo, error)
}

// FileContext creates a provider that renders file content as context fragments.
func FileContext(key agentcontext.ProviderKey, files []FileSpec, opts ...FileProviderOption) *FileProvider {
	p := &FileProvider{
		key:   key,
		cache: agentcontext.CachePolicy{Stable: true, Scope: agentcontext.CacheThread},
		files: append([]FileSpec(nil), files...),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}
	return p
}

// WithFileWorkDir sets the working directory used to resolve relative paths.
func WithFileWorkDir(dir string) FileProviderOption {
	return func(p *FileProvider) { p.workDir = dir }
}

// WithFileCache sets the fragment cache policy applied to rendered files.
func WithFileCache(cache agentcontext.CachePolicy) FileProviderOption {
	return func(p *FileProvider) { p.cache = cache }
}

// WithFileReader overrides the file reader for testing.
func WithFileReader(read func(path string) ([]byte, fs.FileInfo, error)) FileProviderOption {
	return func(p *FileProvider) { p.readFile = read }
}

func (p *FileProvider) Key() agentcontext.ProviderKey {
	if p == nil || p.key == "" {
		return "files"
	}
	return p.key
}

func (p *FileProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	fragments, snapshots, err := p.render(ctx)
	if err != nil {
		return agentcontext.ProviderContext{}, err
	}
	providerContext := agentcontext.ProviderContext{
		Fragments:   fragments,
		Fingerprint: agentcontext.ProviderFingerprint(fragments),
	}
	if len(snapshots) > 0 {
		data, err := json.Marshal(snapshots)
		if err == nil {
			providerContext.Snapshot = &agentcontext.ProviderSnapshot{Data: data}
		}
	}
	return providerContext, nil
}

func (p *FileProvider) StateFingerprint(ctx context.Context, req agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if fp, ok := p.snapshotFingerprint(req.Previous); ok {
		return fp, true, nil
	}
	fragments, _, err := p.render(ctx)
	if err != nil {
		return "", false, err
	}
	return agentcontext.ProviderFingerprint(fragments), true, nil
}

func (p *FileProvider) snapshotFingerprint(previous *agentcontext.ProviderRenderRecord) (string, bool) {
	if previous == nil || previous.Fingerprint == "" || previous.Snapshot == nil || len(previous.Snapshot.Data) == 0 {
		return "", false
	}
	var snapshots []fileSnapshot
	if err := json.Unmarshal(previous.Snapshot.Data, &snapshots); err != nil || len(snapshots) == 0 {
		return "", false
	}
	current, err := p.statSnapshots(snapshots)
	if err != nil {
		return "", false
	}
	if !sameFileSnapshots(snapshots, current) {
		return "", false
	}
	return previous.Fingerprint, true
}

func (p *FileProvider) statSnapshots(previous []fileSnapshot) ([]fileSnapshot, error) {
	current := make([]fileSnapshot, 0, len(previous))
	for _, snapshot := range previous {
		path := p.resolvePath(snapshot.Path)
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		current = append(current, fileSnapshot{
			Path:    snapshot.Path,
			Size:    info.Size(),
			ModTime: info.ModTime().UnixNano(),
		})
	}
	return current, nil
}

func (p *FileProvider) render(ctx context.Context) ([]agentcontext.ContextFragment, []fileSnapshot, error) {
	if p == nil || len(p.files) == 0 {
		return nil, nil, nil
	}
	fragments := make([]agentcontext.ContextFragment, 0, len(p.files))
	snapshots := make([]fileSnapshot, 0, len(p.files))
	for i, spec := range p.files {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		path := strings.TrimSpace(spec.Path)
		if path == "" {
			if spec.Optional {
				continue
			}
			return nil, nil, fmt.Errorf("file provider %s: file %d: path is required", p.Key(), i+1)
		}
		resolved := p.resolvePath(path)
		body, info, err := p.read(resolved)
		if err != nil {
			if spec.Optional && errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, nil, fmt.Errorf("file provider %s: %s: %w", p.Key(), path, err)
		}
		content := strings.TrimSpace(string(body))
		if content == "" {
			continue
		}
		fragmentKey := spec.Key
		if fragmentKey == "" {
			fragmentKey = agentcontext.FragmentKey(fmt.Sprintf("%s/%d", p.Key(), len(fragments)+1))
		}
		role := spec.Role
		if role == "" {
			role = unified.RoleUser
		}
		authority := spec.Authority
		if authority == "" {
			authority = agentcontext.AuthorityUser
		}
		fragments = append(fragments, agentcontext.ContextFragment{
			Key:         fragmentKey,
			Role:        role,
			Content:     content,
			Authority:   authority,
			CachePolicy: p.cache,
		})
		snapshots = append(snapshots, fileSnapshot{
			Path:    path,
			Size:    info.Size(),
			ModTime: info.ModTime().UnixNano(),
		})
	}
	return fragments, snapshots, nil
}

func (p *FileProvider) read(path string) ([]byte, fs.FileInfo, error) {
	if p != nil && p.readFile != nil {
		return p.readFile(path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, err
	}
	return body, info, nil
}

func (p *FileProvider) resolvePath(path string) string {
	if filepath.IsAbs(path) || strings.TrimSpace(p.workDir) == "" {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(p.workDir, path))
}

func sameFileSnapshots(a, b []fileSnapshot) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if filepath.Clean(a[i].Path) != filepath.Clean(b[i].Path) {
			return false
		}
		if a[i].Size != b[i].Size || a[i].ModTime != b[i].ModTime {
			return false
		}
	}
	return true
}
