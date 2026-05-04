package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/runner"
	"github.com/codewandler/agentsdk/terminal/repl"
	"github.com/codewandler/agentsdk/terminal/ui"
)

func Run(ctx context.Context, cfg Config) error {
	if ctx == nil {
		ctx = context.Background()
	}
	loaded, err := Load(ctx, cfg)
	if err != nil {
		return err
	}
	in := loaded.In
	out := loaded.Out
	errOut := loaded.Err

	if strings.TrimSpace(cfg.Task) != "" {
		runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt)
		defer stopSignals()
		cancel := func() {}
		if cfg.TotalTimeout > 0 {
			runCtx, cancel = context.WithTimeout(runCtx, cfg.TotalTimeout)
		}
		defer cancel()
		if cfg.Verbose {
			if summary := loaded.Session.ParamsSummary(); summary != "" {
				fmt.Fprintf(out, "%s[%s]%s\n", ui.Dim, summary, ui.Reset)
			}
		}
		result, err := loaded.Session.Send(runCtx, cfg.Task)
		if renderErr := renderOneShotResult(out, result); renderErr != nil && err == nil {
			err = renderErr
		}
		fmt.Fprintln(out)
		ui.PrintSessionUsage(out, loaded.Session.SessionID(), loaded.Session.Tracker().Aggregate())
		if errors.Is(err, runner.ErrMaxStepsReached) {
			fmt.Fprintf(errOut, "Warning: %v\n", err)
			return nil
		}
		return err
	}

	prompt := cfg.Prompt
	if prompt == "" || prompt == "agentsdk> " {
		prompt = fmt.Sprintf("agent(%s)> ", loaded.AgentName)
	}
	return repl.Run(ctx, loaded.Session, in, repl.WithPrompt(prompt))
}

func renderOneShotResult(out io.Writer, result command.Result) error {
	text, err := command.Render(result, command.DisplayTerminal)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}
	_, err = fmt.Fprintln(out, text)
	return err
}
