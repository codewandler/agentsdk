package skill

import "github.com/codewandler/agentsdk/thread"

const (
	EventSkillActivated          thread.EventKind = "harness.skill_activated"
	EventSkillReferenceActivated thread.EventKind = "harness.skill_reference_activated"
)

type SkillActivatedEvent struct {
	Skill string `json:"skill"`
}

type SkillReferenceActivatedEvent struct {
	Skill string `json:"skill"`
	Path  string `json:"path"`
}

// ActivatorContextKey is the tool context Extra key used to pass a session-aware
// skill activator. Tools should prefer this over mutating ActivationState
// directly so dynamic skill/reference activation is persisted on thread-backed
// sessions.
const ActivatorContextKey = "agentsdk.skill_activator"

// Activator is implemented by session-aware skill activation owners, such as
// agent.Instance. It keeps the skill package independent from the agent package
// while allowing tools to request persisted activation.
type Activator interface {
	ActivateSkill(name string) (Status, error)
	ActivateSkillReferences(name string, refs []string) ([]string, error)
}
