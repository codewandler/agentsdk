package skills

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/codewandler/agentsdk/skill"
	"github.com/codewandler/agentsdk/tool"
)

const KeyActivationState = "agentsdk.skill_activation_state"

type actionParams struct {
	Actions []Action `json:"actions" jsonschema:"description=List of skill activation actions,required"`
}

type Action struct {
	Action     string   `json:"action" jsonschema:"description=Action name. Phase 1 supports activate,required"`
	Skill      string   `json:"skill,omitempty" jsonschema:"description=Skill name to activate"`
	References []string `json:"references,omitempty" jsonschema:"description=Exact relative reference paths under references/ to activate"`
}

type actionResult struct {
	Action            string   `json:"action"`
	Skill             string   `json:"skill,omitempty"`
	SkillStatus       string   `json:"skill_status,omitempty"`
	ActivatedRefs     []string `json:"activated_references,omitempty"`
	AlreadyActiveRefs []string `json:"already_active_references,omitempty"`
	Error             string   `json:"error,omitempty"`
}

type result struct {
	Results      []actionResult `json:"results"`
	ActiveSkills []string       `json:"active_skills,omitempty"`
}

func (r result) IsError() bool { return false }
func (r result) String() string {
	var lines []string
	for _, item := range r.Results {
		if item.Error != "" {
			lines = append(lines, fmt.Sprintf("%s %q: %s", item.Action, item.Skill, item.Error))
			continue
		}
		line := fmt.Sprintf("%s %q: %s", item.Action, item.Skill, item.SkillStatus)
		if len(item.ActivatedRefs) > 0 {
			line += " refs=" + strings.Join(item.ActivatedRefs, ", ")
		}
		if len(item.AlreadyActiveRefs) > 0 {
			line += " already_active_refs=" + strings.Join(item.AlreadyActiveRefs, ", ")
		}
		lines = append(lines, line)
	}
	if len(r.ActiveSkills) > 0 {
		lines = append(lines, "active skills: "+strings.Join(r.ActiveSkills, ", "))
	}
	return strings.Join(lines, "\n")
}

func (r result) MarshalJSON() ([]byte, error) {
	type wire struct {
		Type         string         `json:"type"`
		Results      []actionResult `json:"results"`
		ActiveSkills []string       `json:"active_skills,omitempty"`
	}
	return json.Marshal(wire{Type: "skill", Results: r.Results, ActiveSkills: r.ActiveSkills})
}

func Tools() []tool.Tool {
	return []tool.Tool{skillTool()}
}

func skillTool() tool.Tool {
	return tool.New("skill",
		"Activate skills and exact references under references/. Supports batched actions. Activate a skill first, then activate exact relative reference paths like references/tradeoffs.md. Example: {\"actions\":[{\"action\":\"activate\",\"skill\":\"architecture\"}]} then {\"actions\":[{\"action\":\"activate\",\"skill\":\"architecture\",\"references\":[\"references/tradeoffs.md\"]}]}",
		func(ctx tool.Ctx, p actionParams) (tool.Result, error) {
			if len(p.Actions) == 0 {
				return tool.Error("at least one action is required"), nil
			}
			state, err := activationState(ctx)
			if err != nil {
				return tool.Error(err.Error()), nil
			}
			out := result{Results: make([]actionResult, 0, len(p.Actions))}
			for _, action := range p.Actions {
				res := actionResult{Action: action.Action, Skill: strings.TrimSpace(action.Skill)}
				switch strings.TrimSpace(action.Action) {
				case "activate":
					if res.Skill == "" {
						res.Error = "skill is required"
						out.Results = append(out.Results, res)
						continue
					}
					beforeStatus := state.Status(res.Skill)
					if len(action.References) > 0 && beforeStatus == skill.StatusInactive {
						res.Error = fmt.Sprintf("references for %q require the skill to be active first", res.Skill)
						out.Results = append(out.Results, res)
						continue
					}
					status, err := state.ActivateSkill(res.Skill)
					if err != nil {
						res.Error = err.Error()
						out.Results = append(out.Results, res)
						continue
					}
					res.SkillStatus = string(status)
					if len(action.References) > 0 {
						before := activeRefSet(state.ActiveReferences(res.Skill))
						activated, err := state.ActivateReferences(res.Skill, action.References)
						if err != nil {
							res.Error = err.Error()
							out.Results = append(out.Results, res)
							continue
						}
						res.ActivatedRefs = activated
						res.AlreadyActiveRefs = alreadyActiveRefs(before, action.References, activated)
					}
					if beforeStatus == skill.StatusInactive && status == skill.StatusDynamic {
						res.SkillStatus = "activated"
					} else if beforeStatus != skill.StatusInactive {
						res.SkillStatus = "already_active"
					}
				default:
					res.Error = fmt.Sprintf("unsupported action %q", action.Action)
				}
				out.Results = append(out.Results, res)
			}
			out.ActiveSkills = state.ActiveSkillNames()
			return out, nil
		},
	)
}

func activationState(ctx tool.Ctx) (*skill.ActivationState, error) {
	v, ok := ctx.Extra()[KeyActivationState]
	if !ok {
		return nil, fmt.Errorf("skill tool requires %q in Extra(); check agent wiring", KeyActivationState)
	}
	state, ok := v.(*skill.ActivationState)
	if !ok {
		return nil, fmt.Errorf("%q has unexpected type %T", KeyActivationState, v)
	}
	return state, nil
}

func activeRefSet(refs []skill.Reference) map[string]bool {
	out := make(map[string]bool, len(refs))
	for _, ref := range refs {
		out[ref.Path] = true
	}
	return out
}

func alreadyActiveRefs(before map[string]bool, requested, activated []string) []string {
	activatedSet := map[string]bool{}
	for _, ref := range activated {
		activatedSet[ref] = true
	}
	seen := map[string]bool{}
	var out []string
	for _, ref := range requested {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		if before[ref] && !activatedSet[ref] {
			out = append(out, ref)
		}
	}
	sort.Strings(out)
	return out
}
