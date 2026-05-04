package engineerapp

import (
	"embed"
	"io/fs"
)

const ResourcesRoot = "resources"

//go:embed resources/agentsdk.app.json resources/.agents/agents/*.md resources/.agents/commands/*.md resources/.agents/skills/*/SKILL.md
var embeddedResources embed.FS

func Resources() fs.FS { return embeddedResources }
