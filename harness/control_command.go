package harness

import (
	"context"
	"errors"
	"fmt"
	"github.com/codewandler/agentsdk/agent"
	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/skill"
)

type controlCommandHandler struct {
	Session *Session
}

type turnCommandInput struct {
	Text string `command:"arg=text"`
}

type skillActivateCommandInput struct {
	Name string `command:"arg=name"`
}

type skillReferenceListCommandInput struct {
	Name string `command:"arg=name"`
}

type skillReferenceActivateCommandInput struct {
	Name string `command:"arg=name"`
	Path string `command:"arg=path"`
}

func newHelpCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("help",
		command.Description("Show available commands"),
		command.Alias("?"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(h.helpCommand).
		Build()
}

func newAgentsCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("agents",
		command.Description("Show available agents"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(h.agentsCommand).
		Build()
}

func newNewCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("new",
		command.Description("Start a new session"),
		command.Alias("reset"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(h.newCommand).
		Build()
}

func newQuitCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("quit",
		command.Description("Exit the app"),
		command.Alias("q", "exit"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(h.quitCommand).
		Build()
}

func newTurnCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("turn",
		command.Description("Run a prompt as an agent turn"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(command.Typed(h.turnCommand),
			command.TypedInput[turnCommandInput](),
			command.Arg("text").Required().Variadic(),
		).
		Build()
}

func newContextCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("context",
		command.Description("Show last context render state"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(h.contextCommand).
		Build()
}

func newCapabilitiesCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("capabilities",
		command.Description("List attached capabilities and projections"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(h.capabilitiesCommand).
		Build()
}

func newSkillsCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("skills",
		command.Description("List discovered skills and activation status"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(h.skillsCommand).
		Build()
}

func newSkillCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("skill",
		command.Description("Inspect and activate skills on the current agent"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(command.Typed(h.skillCommand),
			command.Description("Activate a skill by name"),
			command.TypedInput[skillActivateCommandInput](),
			command.Arg("name").Required(),
			command.Output(outputDescriptor("harness.skill.activate", "Skill activation result")),
		).
		Sub("activate", command.Typed(h.skillCommand),
			command.Description("Activate a skill by name"),
			command.TypedInput[skillActivateCommandInput](),
			command.Arg("name").Required(),
			command.Output(outputDescriptor("harness.skill.activate", "Skill activation result")),
		).
		Sub("refs", command.Typed(h.skillReferenceListCommand),
			command.Description("List references for a skill"),
			command.TypedInput[skillReferenceListCommandInput](),
			command.Arg("name").Required(),
			command.Output(outputDescriptor("harness.skill.references", "Skill reference list")),
		).
		Sub("ref", command.Typed(h.skillReferenceActivateCommand),
			command.Description("Activate one exact skill reference"),
			command.TypedInput[skillReferenceActivateCommandInput](),
			command.Arg("name").Required(),
			command.Arg("path").Required(),
			command.Output(outputDescriptor("harness.skill.reference.activate", "Skill reference activation result")),
		).
		Build()
}

func newCompactCommand(session *Session) (*command.Tree, error) {
	h := controlCommandHandler{Session: session}
	return command.NewTree("compact",
		command.Description("Summarize and compact conversation history"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(h.compactCommand).
		Build()
}

func (h controlCommandHandler) helpCommand(context.Context, command.Invocation) (command.Result, error) {
	s := h.Session
	if s == nil {
		return command.Display(CommandHelpPayload{}), nil
	}
	commands, err := s.Commands()
	if err != nil {
		return command.Result{}, err
	}
	payload := CommandHelpPayload{Descriptors: commands.Descriptors()}
	if s.App != nil && s.App.Commands() != nil {
		for _, cmd := range s.App.Commands().UserCommands() {
			if cmd != nil {
				payload.AppCommands = append(payload.AppCommands, cmd.Descriptor())
			}
		}
	}
	return command.Display(payload), nil
}

func (h controlCommandHandler) agentsCommand(context.Context, command.Invocation) (command.Result, error) {
	if h.Session == nil || h.Session.App == nil {
		return command.Display(AgentsPayload{}), nil
	}
	defaultAgent := ""
	if h.Session.Agent != nil {
		defaultAgent = h.Session.Agent.Spec().Name
	}
	return command.Display(AgentsPayload{Agents: h.Session.App.AgentSpecs(), DefaultAgent: defaultAgent}), nil
}

func (h controlCommandHandler) newCommand(context.Context, command.Invocation) (command.Result, error) {
	if h.Session != nil && h.Session.Agent != nil {
		h.Session.Agent.Reset()
	}
	return command.Reset(), nil
}

func (h controlCommandHandler) quitCommand(context.Context, command.Invocation) (command.Result, error) {
	return command.Exit(), nil
}

func (h controlCommandHandler) turnCommand(ctx context.Context, input turnCommandInput) (command.Result, error) {
	if h.Session == nil {
		return command.Result{}, fmt.Errorf("harness: session is required")
	}
	return h.Session.runAgentTurn(ctx, input.Text, 0)
}

func (h controlCommandHandler) contextCommand(context.Context, command.Invocation) (command.Result, error) {
	inst, ok := h.currentAgent()
	if !ok {
		return command.Notice("context: no current agent"), nil
	}
	state := h.Session.ContextState()
	text := state.Text
	if text == "context: no render state" {
		text = fmt.Sprintf("context: no render state yet for agent %q\nrun a turn first to capture provider context", inst.Spec().Name)
	}
	return command.Display(ContextStatePayload{State: state, Text: text}), nil
}

func (h controlCommandHandler) capabilitiesCommand(context.Context, command.Invocation) (command.Result, error) {
	if h.Session == nil {
		return command.Display(CapabilityStatePayload{}), nil
	}
	return command.Display(CapabilityStatePayload{State: h.Session.CapabilityState()}), nil
}
func (h controlCommandHandler) skillsCommand(context.Context, command.Invocation) (command.Result, error) {
	inst, ok := h.currentAgent()
	if !ok {
		return command.Display(SkillsPayload{Unavailable: "skills: no current agent"}), nil
	}
	state := inst.SkillActivationState()
	if state == nil {
		return command.Display(SkillsPayload{Unavailable: "skills: unavailable"}), nil
	}
	return command.Display(SkillsPayload{State: state}), nil
}

func (h controlCommandHandler) skillCommand(_ context.Context, input skillActivateCommandInput) (command.Result, error) {
	inst, ok := h.currentAgent()
	if !ok {
		return command.Display(SkillActivationPayload{Message: "skill: no current agent"}), nil
	}
	before := skill.StatusInactive
	if state := inst.SkillActivationState(); state != nil {
		before = state.Status(input.Name)
	}
	status, err := inst.ActivateSkill(input.Name)
	if err != nil {
		return command.Display(SkillActivationPayload{Skill: input.Name, Error: err.Error()}), nil
	}
	payload := SkillActivationPayload{Skill: input.Name, Before: before, Status: status}
	return command.Display(payload), nil
}

func (h controlCommandHandler) skillReferenceListCommand(_ context.Context, input skillReferenceListCommandInput) (command.Result, error) {
	inst, ok := h.currentAgent()
	if !ok {
		return command.Display(SkillReferencesPayload{Message: "skill refs: no current agent"}), nil
	}
	state := inst.SkillActivationState()
	if state == nil || state.Repository() == nil {
		return command.Display(SkillReferencesPayload{Message: "skill refs: unavailable"}), nil
	}
	refs := state.Repository().ListReferences(input.Name)
	return command.Display(SkillReferencesPayload{Skill: input.Name, Status: state.Status(input.Name), References: refs, ActiveReferences: state.ActiveReferences(input.Name)}), nil
}

func (h controlCommandHandler) skillReferenceActivateCommand(_ context.Context, input skillReferenceActivateCommandInput) (command.Result, error) {
	inst, ok := h.currentAgent()
	if !ok {
		return command.Display(SkillReferenceActivationPayload{Message: "skill ref: no current agent"}), nil
	}
	before := map[string]bool{}
	if state := inst.SkillActivationState(); state != nil {
		before = skillReferencePathSet(state.ActiveReferences(input.Name))
	}
	activated, err := inst.ActivateSkillReferences(input.Name, []string{input.Path})
	if err != nil {
		return command.Display(SkillReferenceActivationPayload{Skill: input.Name, Path: input.Path, Error: err.Error()}), nil
	}
	alreadyActive := before[input.Path] && len(activated) == 0
	return command.Display(SkillReferenceActivationPayload{Skill: input.Name, Path: input.Path, Activated: activated, AlreadyActive: alreadyActive}), nil
}

func (h controlCommandHandler) compactCommand(ctx context.Context, _ command.Invocation) (command.Result, error) {
	inst, ok := h.currentAgent()
	if !ok {
		return command.Display(CompactPayload{Unavailable: "compact: no current agent"}), nil
	}
	result, err := inst.Compact(ctx)
	if err != nil {
		if errors.Is(err, agent.ErrNothingToCompact) {
			return command.Display(CompactPayload{Unavailable: "compact: conversation too short to compact"}), nil
		}
		return command.Display(CompactPayload{Error: err.Error()}), nil
	}
	return command.Display(CompactPayload{Result: result}), nil
}

func (h controlCommandHandler) currentAgent() (*agent.Instance, bool) {
	if h.Session == nil || h.Session.Agent == nil {
		return nil, false
	}
	return h.Session.Agent, true
}
