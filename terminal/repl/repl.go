// Package repl provides a small terminal REPL for agentsdk agents.
package repl

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/codewandler/agentsdk/command"
	"github.com/codewandler/agentsdk/terminal/ui"
	"github.com/codewandler/agentsdk/usage"
)

// Target is the small app/frontend surface needed by the terminal REPL.
type Target interface {
	Send(context.Context, string) (command.Result, error)
	ParamsSummary() string
	SessionID() string
	Tracker() *usage.Tracker
	Out() io.Writer
}

type Options struct {
	Prompt             string
	ResetMessage       string
	PrintParamsSummary bool
}

type Option func(*Options)

func WithPrompt(prompt string) Option {
	return func(o *Options) { o.Prompt = prompt }
}

func WithResetMessage(message string) Option {
	return func(o *Options) { o.ResetMessage = message }
}

func WithParamsSummary(enabled bool) Option {
	return func(o *Options) { o.PrintParamsSummary = enabled }
}

// Run starts an interactive prompt loop.
func Run(ctx context.Context, target Target, input io.Reader, opts ...Option) error {
	if target == nil {
		return fmt.Errorf("repl: target is required")
	}
	if input == nil {
		input = os.Stdin
	}
	cfg := Options{
		Prompt:             "> ",
		ResetMessage:       "Session reset.",
		PrintParamsSummary: true,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}

	out := target.Out()
	if out == nil {
		out = os.Stdout
	}

	if cfg.PrintParamsSummary {
		if summary := target.ParamsSummary(); summary != "" {
			fmt.Fprintf(out, "%s[%s]%s\n", ui.Dim, summary, ui.Reset)
		}
	}

	scanner := bufio.NewScanner(input)
	var (
		mu         sync.Mutex
		turnCancel context.CancelFunc
	)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer func() {
		signal.Stop(sigCh)
		close(sigCh)
	}()
	go func() {
		for range sigCh {
			mu.Lock()
			cancel := turnCancel
			mu.Unlock()
			if cancel != nil {
				cancel()
			}
		}
	}()

	for {
		fmt.Fprint(out, cfg.Prompt)
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		turnCtx, cancel := context.WithCancel(ctx)
		mu.Lock()
		turnCancel = cancel
		mu.Unlock()

		exit, err := handleLine(turnCtx, target, out, line, cfg)

		mu.Lock()
		turnCancel = nil
		mu.Unlock()
		cancel()

		if err != nil && !errors.Is(err, context.Canceled) {
			ui.PrintError(out, err)
		}
		if exit {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	fmt.Fprintln(out)
	ui.PrintSessionUsage(out, target.SessionID(), aggregate(target))
	return nil
}

func handleLine(ctx context.Context, target Target, out io.Writer, line string, cfg Options) (bool, error) {
	result, err := target.Send(ctx, line)
	if err != nil {
		return false, err
	}
	return applyResult(out, result, cfg), nil
}

func applyResult(out io.Writer, result command.Result, cfg Options) bool {
	switch result.Kind {
	case command.ResultHandled:
		return false
	case command.ResultDisplay:
		text, err := command.Render(result, command.DisplayTerminal)
		if err != nil {
			ui.PrintError(out, err)
			return false
		}
		if text != "" {
			fmt.Fprintln(out, text)
		}
		return false
	case command.ResultAgentTurn:
		return false
	case command.ResultReset:
		if cfg.ResetMessage != "" {
			fmt.Fprintln(out, cfg.ResetMessage)
		}
		return false
	case command.ResultExit:
		return true
	default:
		return false
	}
}

func aggregate(target Target) usage.Record {
	if target == nil || target.Tracker() == nil {
		return usage.Record{}
	}
	return target.Tracker().Aggregate()
}
