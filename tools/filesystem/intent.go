package filesystem

import (
	"strings"

	"github.com/codewandler/agentsdk/tool"
)

// Intent declarations for filesystem tools.
// Each function returns a TypedToolOption that adds DeclareIntent to the tool.

func dirCreateIntent() tool.TypedToolOption[DirCreateParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p DirCreateParams) (tool.Intent, error) {
		path := resolvePath(p.Path, ctx.WorkDir())
		return tool.Intent{
			Tool:       "dir_create",
			ToolClass:  "filesystem_write",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "directory", Value: path, Locality: classifyLocality(ctx, path)},
				Operation: "write",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_write"},
		}, nil
	})
}

func dirListIntent() tool.TypedToolOption[DirListParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p DirListParams) (tool.Intent, error) {
		path := resolvePath(p.Path, ctx.WorkDir())
		return tool.Intent{
			Tool:       "dir_list",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "directory", Value: path, Locality: classifyLocality(ctx, path)},
				Operation: "read",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read"},
		}, nil
	})
}

func dirTreeIntent() tool.TypedToolOption[DirTreeParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p DirTreeParams) (tool.Intent, error) {
		path := resolvePath(p.Path, ctx.WorkDir())
		return tool.Intent{
			Tool:       "dir_tree",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "directory", Value: path, Locality: classifyLocality(ctx, path)},
				Operation: "read",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read"},
		}, nil
	})
}

func fileReadIntent() tool.TypedToolOption[FileReadParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p FileReadParams) (tool.Intent, error) {
		ops := make([]tool.IntentOperation, 0, len(p.Path))
		for _, path := range p.Path {
			abs := resolvePath(path, ctx.WorkDir())
			ops = append(ops, tool.IntentOperation{
				Resource:  tool.IntentResource{Category: "file", Value: abs, Locality: classifyLocality(ctx, abs)},
				Operation: "read",
				Certain:   true,
			})
		}
		return tool.Intent{
			Tool:       "file_read",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: ops,
			Behaviors:  []string{"filesystem_read"},
		}, nil
	})
}

func fileWriteIntent() tool.TypedToolOption[FileWriteParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p FileWriteParams) (tool.Intent, error) {
		abs := resolvePath(p.Path, ctx.WorkDir())
		return tool.Intent{
			Tool:       "file_write",
			ToolClass:  "filesystem_write",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "file", Value: abs, Locality: classifyLocality(ctx, abs)},
				Operation: "write",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_write"},
		}, nil
	})
}

func fileEditIntent() tool.TypedToolOption[FileEditParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p FileEditParams) (tool.Intent, error) {
		ops := make([]tool.IntentOperation, 0, len(p.Path))
		for _, path := range p.Path {
			abs := resolvePath(path, ctx.WorkDir())
			ops = append(ops, tool.IntentOperation{
				Resource:  tool.IntentResource{Category: "file", Value: abs, Locality: classifyLocality(ctx, abs)},
				Operation: "write",
				Certain:   true,
			})
		}
		return tool.Intent{
			Tool:       "file_edit",
			ToolClass:  "filesystem_write",
			Confidence: "high",
			Operations: ops,
			Behaviors:  []string{"filesystem_write"},
		}, nil
	})
}

func fileStatIntent() tool.TypedToolOption[FileStatParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p FileStatParams) (tool.Intent, error) {
		abs := resolvePath(p.Path, ctx.WorkDir())
		return tool.Intent{
			Tool:       "file_stat",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "file", Value: abs, Locality: classifyLocality(ctx, abs)},
				Operation: "read",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read"},
		}, nil
	})
}

func fileCopyIntent() tool.TypedToolOption[FileCopyParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p FileCopyParams) (tool.Intent, error) {
		src := resolvePath(p.Src, ctx.WorkDir())
		dst := resolvePath(p.Dst, ctx.WorkDir())
		return tool.Intent{
			Tool:       "file_copy",
			ToolClass:  "filesystem_write",
			Confidence: "high",
			Operations: []tool.IntentOperation{
				{
					Resource:  tool.IntentResource{Category: "file", Value: src, Locality: classifyLocality(ctx, src)},
					Operation: "read",
					Certain:   true,
				},
				{
					Resource:  tool.IntentResource{Category: "file", Value: dst, Locality: classifyLocality(ctx, dst)},
					Operation: "write",
					Certain:   true,
				},
			},
			Behaviors: []string{"filesystem_read", "filesystem_write"},
		}, nil
	})
}

func fileMoveIntent() tool.TypedToolOption[FileMoveParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p FileMoveParams) (tool.Intent, error) {
		src := resolvePath(p.Src, ctx.WorkDir())
		dst := resolvePath(p.Dst, ctx.WorkDir())
		return tool.Intent{
			Tool:       "file_move",
			ToolClass:  "filesystem_write",
			Confidence: "high",
			Operations: []tool.IntentOperation{
				{
					Resource:  tool.IntentResource{Category: "file", Value: src, Locality: classifyLocality(ctx, src)},
					Operation: "delete",
					Certain:   true,
				},
				{
					Resource:  tool.IntentResource{Category: "file", Value: dst, Locality: classifyLocality(ctx, dst)},
					Operation: "write",
					Certain:   true,
				},
			},
			Behaviors: []string{"filesystem_write", "filesystem_delete"},
		}, nil
	})
}

func fileDeleteIntent() tool.TypedToolOption[FileDeleteParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p FileDeleteParams) (tool.Intent, error) {
		abs := resolvePath(p.Path, ctx.WorkDir())
		return tool.Intent{
			Tool:       "file_delete",
			ToolClass:  "filesystem_delete",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "file", Value: abs, Locality: classifyLocality(ctx, abs)},
				Operation: "delete",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_delete"},
		}, nil
	})
}

func globIntent() tool.TypedToolOption[GlobParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p GlobParams) (tool.Intent, error) {
		dir := ctx.WorkDir()
		if p.Path != "" {
			dir = resolvePath(p.Path, ctx.WorkDir())
		}
		return tool.Intent{
			Tool:       "glob",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "directory", Value: dir, Locality: classifyLocality(ctx, dir)},
				Operation: "read",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read"},
		}, nil
	})
}

func grepIntent() tool.TypedToolOption[GrepParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p GrepParams) (tool.Intent, error) {
		ops := make([]tool.IntentOperation, 0)
		if p.Paths != nil {
			for _, path := range *p.Paths {
				abs := resolvePath(path, ctx.WorkDir())
				ops = append(ops, tool.IntentOperation{
					Resource:  tool.IntentResource{Category: "file", Value: abs, Locality: classifyLocality(ctx, abs)},
					Operation: "read",
					Certain:   true,
				})
			}
		} else {
			// Default: search current directory.
			ops = append(ops, tool.IntentOperation{
				Resource:  tool.IntentResource{Category: "directory", Value: ctx.WorkDir(), Locality: "workspace"},
				Operation: "read",
				Certain:   true,
			})
		}
		return tool.Intent{
			Tool:       "grep",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: ops,
			Behaviors:  []string{"filesystem_read"},
		}, nil
	})
}

// classifyLocality maps a resolved path to a sensitivity zone.
// For now, a simple heuristic: if the path is inside the workspace dir,
// it's "workspace". Otherwise "unknown". Phase 3 will add full locality
// classification with configurable prefixes.
func classifyLocality(ctx tool.Ctx, absPath string) string {
	workDir := ctx.WorkDir()
	if workDir == "" {
		return "unknown"
	}
	// Ensure workDir ends with separator for proper prefix matching.
	// Without this, workDir="/tmp/project" would match "/tmp/project-evil".
	prefix := workDir
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	if strings.HasPrefix(absPath, prefix) || absPath == workDir {
		return "workspace"
	}
	return "unknown"
}
