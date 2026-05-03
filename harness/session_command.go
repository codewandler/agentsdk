package harness

import (
	"context"

	"github.com/codewandler/agentsdk/command"
)

type SessionCommandHandler struct {
	Session *Session
}

type sessionInfoCommandInput struct{}

func newSessionCommand(session *Session) (*command.Tree, error) {
	h := SessionCommandHandler{Session: session}
	return command.NewTree("session", command.Description("Inspect the active session")).
		Sub("info", command.Typed(h.sessionInfoCommand),
			command.Description("Show session metadata"),
		).
		Build()
}

func (h SessionCommandHandler) sessionInfoCommand(context.Context, sessionInfoCommandInput) (command.Result, error) {
	return command.Display(SessionInfoPayload{Info: h.Session.Info()}), nil
}
