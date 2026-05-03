package harness

import (
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/skill"
)

type CommandHelpPayload struct {
	Descriptors []command.Descriptor
	AppCommands []command.Descriptor
}

func (p CommandHelpPayload) Display(command.DisplayMode) (string, error) {
	var b strings.Builder
	if len(p.Descriptors) == 0 && len(p.AppCommands) == 0 {
		return "No commands registered.", nil
	}
	b.WriteString("Commands:")
	for _, d := range p.Descriptors {
		writeCommandDescriptor(&b, d)
	}
	for _, spec := range p.AppCommands {
		fmt.Fprintf(&b, "\n/%s", spec.Name)
		if spec.ArgumentHint != "" {
			fmt.Fprintf(&b, " %s", spec.ArgumentHint)
		}
		if spec.Description != "" {
			fmt.Fprintf(&b, " - %s", spec.Description)
		}
	}
	return b.String(), nil
}

func writeCommandDescriptor(b *strings.Builder, d command.Descriptor) {
	fmt.Fprintf(b, "\n/%s", strings.Join(d.Path, " "))
	if d.ArgumentHint != "" {
		fmt.Fprintf(b, " %s", d.ArgumentHint)
	}
	if d.Description != "" {
		fmt.Fprintf(b, " - %s", d.Description)
	}
	for _, sub := range d.Subcommands {
		writeCommandDescriptor(b, sub)
	}
}

type AgentsPayload struct {
	Agents       []agent.Spec
	DefaultAgent string
}

func (p AgentsPayload) Display(command.DisplayMode) (string, error) {
	if len(p.Agents) == 0 {
		return "No agents registered.", nil
	}
	var b strings.Builder
	b.WriteString("Agents:")
	for _, spec := range p.Agents {
		mark := " "
		if spec.Name == p.DefaultAgent {
			mark = "*"
		}
		fmt.Fprintf(&b, "\n%s %s", mark, spec.Name)
		if spec.Description != "" {
			fmt.Fprintf(&b, " - %s", spec.Description)
		}
	}
	return b.String(), nil
}

type SkillsPayload struct {
	State       *skill.ActivationState
	Unavailable string
}

func (p SkillsPayload) Display(command.DisplayMode) (string, error) {
	if p.Unavailable != "" {
		return p.Unavailable, nil
	}
	if p.State == nil {
		return "skills: unavailable", nil
	}
	active := p.State.ActiveSkills()
	if len(active) == 0 {
		return "No active skills.", nil
	}
	var b strings.Builder
	b.WriteString("Skills:")
	for _, item := range active {
		fmt.Fprintf(&b, "\n- %s (%s)", item.Name, p.State.Status(item.Name))
		if item.Description != "" {
			fmt.Fprintf(&b, ": %s", item.Description)
		}
	}
	return b.String(), nil
}

type SkillActivationPayload struct {
	Skill   string
	Before  skill.Status
	Status  skill.Status
	Message string
	Error   string
}

func (p SkillActivationPayload) Display(command.DisplayMode) (string, error) {
	if p.Message != "" {
		return p.Message, nil
	}
	if p.Error != "" {
		return "skill: " + p.Error, nil
	}
	if p.Skill == "" {
		return "skill: no skill", nil
	}
	if p.Before == skill.StatusBase || p.Status == skill.StatusBase {
		return fmt.Sprintf("skill: %q already active (base)", p.Skill), nil
	}
	if p.Before == skill.StatusDynamic {
		return fmt.Sprintf("skill: %q already active (dynamic)", p.Skill), nil
	}
	return fmt.Sprintf("skill: activated %q", p.Skill), nil
}

type CompactPayload struct {
	Result      agent.CompactionResult
	Unavailable string
	Error       string
}

func (p CompactPayload) Display(command.DisplayMode) (string, error) {
	if p.Unavailable != "" {
		return p.Unavailable, nil
	}
	if p.Error != "" {
		return "compact: " + p.Error, nil
	}
	saved := p.Result.TokensBefore - p.Result.TokensAfter
	return fmt.Sprintf(
		"Compacted: replaced %d messages with summary\nEstimated tokens: before=%d after=%d (saved ~%d)",
		p.Result.ReplacedCount,
		p.Result.TokensBefore,
		p.Result.TokensAfter,
		saved,
	), nil
}
