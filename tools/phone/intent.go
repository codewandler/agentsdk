package phone

import (
	"github.com/codewandler/agentsdk/tool"
)

func phoneIntent(sipAddr string) tool.TypedToolOption[PhoneParams] {
	return tool.WithDeclareIntent(func(_ tool.Ctx, p PhoneParams) (tool.Intent, error) {
		var ops []tool.IntentOperation
		var behaviors []string

		for _, op := range p.Operations {
			switch {
			case op.Dial != nil:
				ops = append(ops, tool.IntentOperation{
					Resource: tool.IntentResource{
						Category: "host",
						Value:    sipAddr,
						Locality: "network",
					},
					Operation: "network_write",
					Certain:   true,
				})
				behaviors = appendIfMissing(behaviors, "telephony_dial")

			case op.Hangup != nil:
				ops = append(ops, tool.IntentOperation{
					Resource: tool.IntentResource{
						Category: "host",
						Value:    sipAddr,
						Locality: "network",
					},
					Operation: "network_write",
					Certain:   true,
				})
				behaviors = appendIfMissing(behaviors, "telephony_hangup")

			case op.Status != nil:
				// Read-only — no operations.
				behaviors = appendIfMissing(behaviors, "telephony_status")
			}
		}

		return tool.Intent{
			Tool:       "phone",
			ToolClass:  "telephony",
			Confidence: "high",
			Operations: ops,
			Behaviors:  behaviors,
		}, nil
	})
}

func appendIfMissing(s []string, v string) []string {
	for _, existing := range s {
		if existing == v {
			return s
		}
	}
	return append(s, v)
}
