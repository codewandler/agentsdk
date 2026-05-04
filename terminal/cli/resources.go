package cli

import (
	"fmt"
	"io/fs"

	"github.com/codewandler/agentsdk/agentdir"
	"github.com/codewandler/agentsdk/resource"
)

type Resources interface {
	Resolve(resource.DiscoveryPolicy) (agentdir.Resolution, error)
}

type ResourceFunc func(resource.DiscoveryPolicy) (agentdir.Resolution, error)

func (f ResourceFunc) Resolve(policy resource.DiscoveryPolicy) (agentdir.Resolution, error) {
	if f == nil {
		return agentdir.Resolution{}, fmt.Errorf("cli: resources are required")
	}
	return f(policy)
}

func DirResources(path string) Resources {
	return ResourceFunc(func(policy resource.DiscoveryPolicy) (agentdir.Resolution, error) {
		if path == "" {
			return agentdir.Resolution{}, fmt.Errorf("cli: resource path is required")
		}
		return agentdir.ResolveDirWithOptions(path, agentdir.ResolveOptions{Policy: policy})
	})
}

// MultiDirResources resolves multiple directory roots and merges their
// contributions. The first path is treated as the primary source (its manifest
// and default agent win). Additional paths are appended as extra discovery
// roots. If paths is empty, "." is used as the sole root.
func MultiDirResources(paths []string) Resources {
	if len(paths) == 0 {
		paths = []string{"."}
	}
	return ResourceFunc(func(policy resource.DiscoveryPolicy) (agentdir.Resolution, error) {
		primary, err := agentdir.ResolveDirWithOptions(paths[0], agentdir.ResolveOptions{Policy: policy})
		if err != nil {
			return agentdir.Resolution{}, err
		}
		for _, extra := range paths[1:] {
			resolved, err := agentdir.ResolveDirWithOptions(extra, agentdir.ResolveOptions{Policy: policy})
			if err != nil {
				return agentdir.Resolution{}, fmt.Errorf("resolve discovery root %q: %w", extra, err)
			}
			primary.Bundle.Append(resolved.Bundle)
			primary.Sources = append(primary.Sources, resolved.Sources...)
		}
		return primary, nil
	})
}

// EmbeddedWithDirResources resolves an embedded filesystem as the primary
// source and merges additional directory roots on top. This is used by
// first-party apps (dev, build) that ship embedded resources but also discover
// from the working directory or explicit paths.
func EmbeddedWithDirResources(fsys fs.FS, root string, dirs []string) Resources {
	return ResourceFunc(func(policy resource.DiscoveryPolicy) (agentdir.Resolution, error) {
		primary, err := agentdir.ResolveFS(fsys, root)
		if err != nil {
			return agentdir.Resolution{}, err
		}
		for _, dir := range dirs {
			resolved, err := agentdir.ResolveDirWithOptions(dir, agentdir.ResolveOptions{Policy: policy})
			if err != nil {
				return agentdir.Resolution{}, fmt.Errorf("resolve discovery root %q: %w", dir, err)
			}
			primary.Bundle.Append(resolved.Bundle)
			primary.Sources = append(primary.Sources, resolved.Sources...)
		}
		return primary, nil
	})
}

func EmbeddedResources(fsys fs.FS, root string) Resources {
	return ResourceFunc(func(resource.DiscoveryPolicy) (agentdir.Resolution, error) {
		return agentdir.ResolveFS(fsys, root)
	})
}

func ResolvedResources(resolution agentdir.Resolution) Resources {
	return ResourceFunc(func(resource.DiscoveryPolicy) (agentdir.Resolution, error) {
		return resolution, nil
	})
}
