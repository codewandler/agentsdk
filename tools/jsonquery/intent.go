package jsonquery

import "github.com/codewandler/agentsdk/tool"

func queryIntent() tool.TypedToolOption[QueryParams] {
	return tool.WithDeclareIntent(func(ctx tool.Ctx, p QueryParams) (tool.Intent, error) {
		path := resolvePath(p.Path, ctx.WorkDir())
		return tool.Intent{
			Tool:       "json_query",
			ToolClass:  "filesystem_read",
			Confidence: "high",
			Operations: []tool.IntentOperation{{
				Resource:  tool.IntentResource{Category: "file", Value: path, Locality: classifyLocality(ctx, path)},
				Operation: "read",
				Certain:   true,
			}},
			Behaviors: []string{"filesystem_read"},
		}, nil
	})
}

func classifyLocality(ctx tool.Ctx, absPath string) string {
	workDir := ctx.WorkDir()
	if workDir == "" {
		return "unknown"
	}
	prefix := workDir
	if prefix[len(prefix)-1] != '/' {
		prefix += "/"
	}
	if absPath == workDir || len(absPath) >= len(prefix) && absPath[:len(prefix)] == prefix {
		return "workspace"
	}
	return "unknown"
}
