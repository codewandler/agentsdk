package harness

import (
	"context"
	"strings"

	"github.com/codewandler/agentsdk/command"
)

// CommandEnvelope is the generic command execution envelope used by tools,
// actions, APIs, and other structured command callers. Exact per-command input
// schemas are provided through command catalogs and enforced by command tree execution.
type CommandEnvelope struct {
	Path  []string       `json:"path"`
	Input map[string]any `json:"input,omitempty"`
}

// CommandEnvelopeSchema returns the generic command execution envelope schema.
func CommandEnvelopeSchema() command.JSONSchema {
	return command.JSONSchema{
		Type:        "object",
		Description: "Execute a command from the provided command catalog.",
		Properties: map[string]command.JSONSchema{
			"path": {
				Type:        "array",
				Description: "Command path from the command catalog, for example [\"workflow\", \"show\"].",
				Items:       &command.JSONSchema{Type: "string"},
			},
			"input": {
				Type:        "object",
				Description: "Structured command input matching the selected command descriptor.",
			},
		},
		Required: []string{"path"},
	}
}

// AgentCommandCatalog returns the commands available through the command envelope.
func (s *Session) AgentCommandCatalog() []CommandCatalogEntry {
	return s.CommandCatalog(CommandCatalogAgentCallable())
}

// ExecuteCommandEnvelope executes one command through the generic command envelope.
// This is a trusted execution seam for SDK/API/action callers; agent-facing tool
// adapters should use ExecuteAgentCommandEnvelope instead.
func (s *Session) ExecuteCommandEnvelope(ctx context.Context, input CommandEnvelope) (command.Result, error) {
	path := envelopeCommandPath(input.Path)
	if len(path) == 0 {
		return command.Result{}, command.ValidationError{Code: command.ValidationInvalidSpec, Message: "harness: command envelope path is required"}
	}
	return s.ExecuteCommand(ctx, path, input.Input)
}

// ExecuteAgentCommandEnvelope executes one agent-callable command through the generic
// command envelope.
func (s *Session) ExecuteAgentCommandEnvelope(ctx context.Context, input CommandEnvelope) (command.Result, error) {
	path := envelopeCommandPath(input.Path)
	if len(path) == 0 {
		return command.Result{}, command.ValidationError{Code: command.ValidationInvalidSpec, Message: "harness: command envelope path is required"}
	}
	if !s.agentCallableCommandPath(path) {
		return command.Result{}, command.ErrNotCallable{Name: strings.Join(path, " "), Caller: "agent"}
	}
	return s.ExecuteCommandEnvelope(ctx, CommandEnvelope{Path: path, Input: input.Input})
}

func (s *Session) agentCallableCommandPath(path []string) bool {
	for _, entry := range s.AgentCommandCatalog() {
		if sameCommandPath(entry.Descriptor.Path, path) {
			return true
		}
	}
	return false
}

func sameCommandPath(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func envelopeCommandPath(path []string) []string {
	clean := make([]string, 0, len(path))
	for _, part := range path {
		part = strings.TrimPrefix(strings.TrimSpace(part), "/")
		if part != "" {
			clean = append(clean, part)
		}
	}
	return clean
}
