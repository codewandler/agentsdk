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
		command.Description("Activate a skill on the current agent"),
		command.WithPolicy(command.Policy{Internal: true}),
	).
		Handle(command.Typed(h.skillCommand),
			command.TypedInput[skillActivateCommandInput](),
			command.Arg("name").Required(),
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
	state := inst.ContextState()
	if state == "context: no render state" {
		state = fmt.Sprintf("context: no render state yet for agent %q\nrun a turn first to capture provider context", inst.Spec().Name)
	}
	return command.Notice(state), nil
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
