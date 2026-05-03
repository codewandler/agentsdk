package harness

import "github.com/codewandler/agentsdk/command"

func outputDescriptor(kind string, description string) command.OutputDescriptor {
	return command.OutputDescriptor{
		Kind:        kind,
		Description: description,
		MediaTypes:  []string{"application/json", "text/plain"},
		Schema: command.JSONSchema{
			Type:        "object",
			Description: description,
		},
	}
}
