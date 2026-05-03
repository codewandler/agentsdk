package runtime_test

import (
	"context"
	"fmt"
	"time"

	"github.com/codewandler/agentsdk/agentcontext/contextproviders"
	"github.com/codewandler/agentsdk/capabilities/planner"
	"github.com/codewandler/agentsdk/capability"
	"github.com/codewandler/agentsdk/plugins/localcli"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/runtime"
	"github.com/codewandler/agentsdk/thread"
	"github.com/codewandler/agentsdk/tool"
	"github.com/codewandler/agentsdk/toolactivation"
	"github.com/codewandler/llmadapter/unified"
)

type exampleClient struct{}

func (exampleClient) Request(_ context.Context, _ unified.Request) (<-chan unified.Event, error) {
	out := make(chan unified.Event, 2)
	out <- unified.TextDeltaEvent{Text: "ok"}
	out <- unified.CompletedEvent{FinishReason: unified.FinishReasonStop}
	close(out)
	return out, nil
}

func ExampleNew() {
	tools := toolactivation.New(localcli.New().DefaultTools()...)
	var text string
	agent, err := runtime.New(exampleClient{},
		runtime.WithModel("default"),
		runtime.WithSystem("You are a concise coding assistant."),
		runtime.WithTools(tools.ActiveTools()),
		runtime.WithToolChoice(unified.ToolChoice{Mode: unified.ToolChoiceAuto}),
		runtime.WithCachePolicy(unified.CachePolicyOn),
		runtime.WithMaxSteps(8),
		runtime.WithToolContextFactory(func(ctx context.Context) tool.Ctx {
			return runtime.NewToolContext(ctx,
				runtime.WithToolWorkDir("."),
				runtime.WithToolSessionID("example-session"),
				runtime.WithToolActivation(tools),
			)
		}),
		runtime.WithEventHandler(func(event runner.Event) {
			if ev, ok := event.(runner.TextDeltaEvent); ok {
				text += ev.Text
			}
		}),
	)
	if err != nil {
		panic(err)
	}
	if _, err := agent.RunTurn(context.Background(), "say ok"); err != nil {
		panic(err)
	}
	fmt.Println(text)
	// Output: ok
}

func ExampleOpenThreadEngine() {
	ctx := context.Background()
	tools := toolactivation.New(localcli.New().DefaultTools()...)
	store := thread.NewMemoryStore()
	registry, err := capability.NewRegistry(planner.Factory{})
	if err != nil {
		panic(err)
	}

	agent, stored, err := runtime.OpenThreadEngine(ctx,
		store,
		thread.CreateParams{
			ID:       "example-thread",
			Metadata: map[string]string{"title": "example"},
			Source:   thread.EventSource{Type: "example", SessionID: "example-session"},
		},
		exampleClient{},
		registry,
		runtime.WithModel("default"),
		runtime.WithTools(tools.ActiveTools()),
		runtime.WithCapabilities(capability.AttachSpec{
			CapabilityName: planner.CapabilityName,
			InstanceID:     "planner_1",
		}),
		runtime.WithContextProviders(
			contextproviders.Environment(contextproviders.WithWorkDir(".")),
			contextproviders.Time(time.Minute),
		),
	)
	if err != nil {
		panic(err)
	}
	if _, err := agent.RunTurn(ctx, "say ok"); err != nil {
		panic(err)
	}
	fmt.Println(stored.ID)
	// Output: example-thread
}
