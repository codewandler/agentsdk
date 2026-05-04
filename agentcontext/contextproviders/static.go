package contextproviders

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/codewandler/agentsdk/agentcontext"
	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/llmadapter/unified"
)

type ModelInfo struct {
	Name          string
	Provider      string
	ContextWindow int
	Effort        string
}

func Model(info ModelInfo) agentcontext.Provider {
	return staticProvider{
		key: "model",
		fragment: agentcontext.ContextFragment{
			Key:       "model/current",
			Role:      unified.RoleUser,
			Content:   renderModelInfo(info),
			Authority: agentcontext.AuthorityDeveloper,
			CachePolicy: agentcontext.CachePolicy{
				Stable: true,
				Scope:  agentcontext.CacheTurn,
			},
		},
	}
}

func Permissions(content string) agentcontext.Provider {
	return staticProvider{
		key: "permissions",
		fragment: agentcontext.ContextFragment{
			Key:       "permissions/current",
			Role:      unified.RoleSystem,
			Content:   strings.TrimSpace(content),
			Authority: agentcontext.AuthorityDeveloper,
			CachePolicy: agentcontext.CachePolicy{
				Stable: true,
				Scope:  agentcontext.CacheTurn,
			},
		},
	}
}

type ProjectInstruction struct {
	Path    string
	Content string
}

func ProjectInstructions(instructions ...ProjectInstruction) agentcontext.Provider {
	fragments := make([]agentcontext.ContextFragment, 0, len(instructions))
	for _, instruction := range instructions {
		content := strings.TrimSpace(instruction.Content)
		if content == "" {
			continue
		}
		key := "project_instructions/" + sanitizeKey(instruction.Path)
		if instruction.Path == "" {
			key = fmt.Sprintf("project_instructions/%d", len(fragments)+1)
		}
		fragments = append(fragments, agentcontext.ContextFragment{
			Key:       agentcontext.FragmentKey(key),
			Role:      unified.RoleUser,
			Content:   renderNamedContent(instruction.Path, content),
			Authority: agentcontext.AuthorityUser,
			CachePolicy: agentcontext.CachePolicy{
				Stable: true,
				Scope:  agentcontext.CacheThread,
			},
		})
	}
	return staticSetProvider{key: "project_instructions", fragments: fragments}
}

type SkillInventory struct {
	Catalog *skill.Repository
	State   *skill.ActivationState
}

func Skills(skills ...skill.Skill) agentcontext.Provider {
	fragments := make([]agentcontext.ContextFragment, 0, len(skills))
	for _, loaded := range skills {
		if strings.TrimSpace(loaded.Body) == "" {
			continue
		}
		fragments = append(fragments, agentcontext.ContextFragment{
			Key:       agentcontext.FragmentKey("skills/loaded/" + sanitizeKey(loaded.Name)),
			Role:      unified.RoleUser,
			Content:   renderSkill(loaded),
			Authority: agentcontext.AuthorityUser,
			CachePolicy: agentcontext.CachePolicy{
				Stable: true,
				Scope:  agentcontext.CacheThread,
			},
		})
	}
	return staticSetProvider{key: "skills", fragments: fragments}
}

func SkillInventoryProvider(inventory SkillInventory) agentcontext.Provider {
	return skillInventoryProvider{inventory: inventory}
}

func Tools(tools ...tool.Tool) agentcontext.Provider {
	specs := make([]string, 0, len(tools))
	for _, t := range tools {
		if t == nil || t.Name() == "" {
			continue
		}
		line := "- " + t.Name()
		if desc := strings.TrimSpace(t.Description()); desc != "" {
			line += ": " + desc
		}
		if guidance := strings.TrimSpace(t.Guidance()); guidance != "" {
			line += "\n  guidance:\n"
			for _, segment := range strings.Split(guidance, "\n") {
				line += "    " + segment + "\n"
			}
			line = strings.TrimRight(line, "\n")
		}
		specs = append(specs, line)
	}
	sort.Strings(specs)
	return staticProvider{
		key: "tools",
		fragment: agentcontext.ContextFragment{
			Key:       "tools/active",
			Role:      unified.RoleUser,
			Content:   strings.Join(specs, "\n"),
			Authority: agentcontext.AuthorityDeveloper,
			CachePolicy: agentcontext.CachePolicy{
				Stable: true,
				Scope:  agentcontext.CacheTurn,
			},
		},
	}
}

type staticProvider struct {
	key      agentcontext.ProviderKey
	fragment agentcontext.ContextFragment
}

func (p staticProvider) Key() agentcontext.ProviderKey { return p.key }

func (p staticProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	if strings.TrimSpace(p.fragment.Content) == "" {
		return agentcontext.ProviderContext{}, nil
	}
	return agentcontext.ProviderContext{
		Fragments:   []agentcontext.ContextFragment{p.fragment},
		Fingerprint: contentFingerprint(string(p.key), p.fragment.Content),
	}, nil
}

func (p staticProvider) StateFingerprint(ctx context.Context, _ agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return contentFingerprint(string(p.key), p.fragment.Content), true, nil
}

type staticSetProvider struct {
	key       agentcontext.ProviderKey
	fragments []agentcontext.ContextFragment
}

func (p staticSetProvider) Key() agentcontext.ProviderKey { return p.key }

func (p staticSetProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	fragments := append([]agentcontext.ContextFragment(nil), p.fragments...)
	return agentcontext.ProviderContext{
		Fragments:   fragments,
		Fingerprint: agentcontext.ProviderFingerprint(fragments),
	}, nil
}

func (p staticSetProvider) StateFingerprint(ctx context.Context, _ agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	return agentcontext.ProviderFingerprint(p.fragments), true, nil
}

type skillInventoryProvider struct {
	inventory SkillInventory
}

func (p skillInventoryProvider) Key() agentcontext.ProviderKey { return "skills" }

func (p skillInventoryProvider) GetContext(ctx context.Context, _ agentcontext.Request) (agentcontext.ProviderContext, error) {
	if err := ctx.Err(); err != nil {
		return agentcontext.ProviderContext{}, err
	}
	fragments := p.fragments()
	return agentcontext.ProviderContext{
		Fragments:   fragments,
		Fingerprint: agentcontext.ProviderFingerprint(fragments),
	}, nil
}

func (p skillInventoryProvider) StateFingerprint(ctx context.Context, _ agentcontext.Request) (string, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	fragments := p.fragments()
	return agentcontext.ProviderFingerprint(fragments), true, nil
}

func (p skillInventoryProvider) fragments() []agentcontext.ContextFragment {
	catalog := p.inventory.Catalog
	if catalog == nil && p.inventory.State != nil {
		catalog = p.inventory.State.Repository()
	}
	if catalog == nil {
		return nil
	}
	list := catalog.List()
	fragments := make([]agentcontext.ContextFragment, 0, len(list))
	for _, item := range list {
		status := skill.StatusInactive
		if p.inventory.State != nil {
			status = p.inventory.State.Status(item.Name)
		}
		content := renderSkillMetadata(item, status)
		if status != skill.StatusInactive && strings.TrimSpace(item.Body) != "" {
			content += "\n\n" + strings.TrimSpace(item.Body)
		}
		fragments = append(fragments, agentcontext.ContextFragment{
			Key:       agentcontext.FragmentKey("skills/catalog/" + sanitizeKey(item.Name)),
			Role:      unified.RoleUser,
			Content:   content,
			Authority: agentcontext.AuthorityUser,
			CachePolicy: agentcontext.CachePolicy{
				Stable: true,
				Scope:  agentcontext.CacheThread,
			},
		})
		if p.inventory.State != nil {
			for _, ref := range p.inventory.State.ActiveReferences(item.Name) {
				fragments = append(fragments, agentcontext.ContextFragment{
					Key:       agentcontext.FragmentKey("skills/references/" + sanitizeKey(item.Name) + "/" + sanitizeKey(ref.Path)),
					Role:      unified.RoleUser,
					Content:   renderReference(ref),
					Authority: agentcontext.AuthorityUser,
					CachePolicy: agentcontext.CachePolicy{
						Stable: true,
						Scope:  agentcontext.CacheThread,
					},
				})
			}
		}
	}
	return fragments
}

func renderModelInfo(info ModelInfo) string {
	var b strings.Builder
	writeLine(&b, "model", info.Name)
	writeLine(&b, "provider", info.Provider)
	if info.ContextWindow > 0 {
		writeLine(&b, "context_window", fmt.Sprintf("%d", info.ContextWindow))
	}
	writeLine(&b, "effort", info.Effort)
	return b.String()
}

func renderSkill(s skill.Skill) string {
	var b strings.Builder
	writeLine(&b, "skill", s.Name)
	writeLine(&b, "description", s.Description)
	writeLine(&b, "source", s.SourceLabel)
	if s.Body != "" {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		b.WriteString(strings.TrimSpace(s.Body))
	}
	return b.String()
}

func renderSkillMetadata(s skill.Skill, status skill.Status) string {
	var b strings.Builder
	writeLine(&b, "skill", s.Name)
	writeLine(&b, "description", s.Description)
	writeLine(&b, "source", s.SourceLabel)
	writeLine(&b, "source_id", s.SourceID)
	writeLine(&b, "directory", s.Dir)
	writeLine(&b, "status", string(status))
	writeLine(&b, "domain", s.Metadata.Domain)
	writeLine(&b, "role", s.Metadata.Role)
	writeLine(&b, "risk", s.Metadata.Risk)
	writeLine(&b, "compatibility", s.Metadata.Compatibility)
	if len(s.Metadata.AllowedTools) > 0 {
		writeLine(&b, "allowed_tools", strings.Join(s.Metadata.AllowedTools, ", "))
	}
	if len(s.References) > 0 {
		writeLine(&b, "references", fmt.Sprintf("%d discovered", len(s.References)))
		paths := make([]string, 0, len(s.References))
		for _, ref := range s.References {
			paths = append(paths, ref.Path)
		}
		sort.Strings(paths)
		writeLine(&b, "reference_paths", strings.Join(paths, ", "))
	}
	return b.String()
}

func renderReference(ref skill.Reference) string {
	var b strings.Builder
	writeLine(&b, "path", ref.Path)
	writeLine(&b, "skill", ref.SkillName)
	writeLine(&b, "source", ref.SourceLabel)
	if !ref.ModifiedAt.IsZero() {
		writeLine(&b, "modified", ref.ModifiedAt.UTC().Format(time.RFC3339))
	}
	triggers := ref.Metadata.AllTriggers()
	if len(triggers) > 0 {
		writeLine(&b, "triggers", strings.Join(triggers, ", "))
	}
	if strings.TrimSpace(ref.Body) != "" {
		b.WriteString("\n")
		b.WriteString(strings.TrimSpace(ref.Body))
	}
	return b.String()
}

func renderNamedContent(name string, content string) string {
	if strings.TrimSpace(name) == "" {
		return strings.TrimSpace(content)
	}
	return strings.TrimSpace(name) + ":\n" + strings.TrimSpace(content)
}

func sanitizeKey(value string) string {
	value = strings.TrimSpace(value)
	for strings.HasPrefix(value, "./") {
		value = strings.TrimPrefix(value, "./")
	}
	value = strings.Trim(value, "/")
	value = strings.ReplaceAll(value, "\\", "/")
	value = strings.ReplaceAll(value, " ", "_")
	value = strings.ReplaceAll(value, "/", "_")
	if value == "" {
		return "unknown"
	}
	return value
}
