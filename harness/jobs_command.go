package harness

import (
	"context"
	"fmt"
	"strings"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/trigger"
)

type jobsCommandHandler struct {
	Session *Session
}

type jobStopCommandInput struct {
	ID trigger.RuleID `command:"arg=id"`
}

func newJobsCommand(session *Session) (*command.Tree, error) {
	h := jobsCommandHandler{Session: session}
	return command.NewTree("jobs", command.Description("List and stop trigger jobs"), command.WithPolicy(command.Policy{Internal: true})).
		Handle(h.jobsCommand).
		Sub("stop", command.Typed(h.jobStopCommand),
			command.Description("Stop a trigger job"),
			command.WithPolicy(command.Policy{Internal: true}),
			command.TypedInput[jobStopCommandInput](),
			command.Arg("id").Required(),
		).
		Build()
}

func (h jobsCommandHandler) jobsCommand(context.Context, command.Invocation) (command.Result, error) {
	registry := h.registry()
	if registry == nil {
		return command.Display(JobsPayload{Unavailable: "jobs: no trigger scheduler attached"}), nil
	}
	return command.Display(JobsPayload{Jobs: registry.Jobs()}), nil
}

func (h jobsCommandHandler) jobStopCommand(_ context.Context, input jobStopCommandInput) (command.Result, error) {
	registry := h.registry()
	if registry == nil {
		return command.Display(JobStopPayload{ID: input.ID, Error: "no trigger scheduler attached"}), nil
	}
	if err := registry.StopJob(input.ID); err != nil {
		return command.Display(JobStopPayload{ID: input.ID, Error: err.Error()}), nil
	}
	return command.Display(JobStopPayload{ID: input.ID, Stopped: true}), nil
}

func (h jobsCommandHandler) registry() trigger.RegistryView {
	if h.Session == nil {
		return nil
	}
	return h.Session.TriggerRegistry()
}

type JobsPayload struct {
	Jobs        []trigger.JobSummary
	Unavailable string
}

func (p JobsPayload) Display(command.DisplayMode) (string, error) {
	if p.Unavailable != "" {
		return p.Unavailable, nil
	}
	if len(p.Jobs) == 0 {
		return "No trigger jobs.", nil
	}
	var b strings.Builder
	b.WriteString("Trigger jobs:")
	for _, job := range p.Jobs {
		fmt.Fprintf(&b, "\n- %s status=%s target=%s:%s matched=%d skipped=%d", job.RuleID, job.Status, job.TargetKind, job.TargetName, job.Matched, job.Skipped)
		if job.LastError != "" {
			fmt.Fprintf(&b, " error=%q", job.LastError)
		}
		if job.LastWorkflowRun != "" {
			fmt.Fprintf(&b, " run=%s", job.LastWorkflowRun)
		}
	}
	return b.String(), nil
}

type JobStopPayload struct {
	ID      trigger.RuleID
	Stopped bool
	Error   string
}

func (p JobStopPayload) Display(command.DisplayMode) (string, error) {
	if p.Error != "" {
		return fmt.Sprintf("job %q: %s", p.ID, p.Error), nil
	}
	if p.Stopped {
		return fmt.Sprintf("job %q stopped", p.ID), nil
	}
	return fmt.Sprintf("job %q unchanged", p.ID), nil
}
