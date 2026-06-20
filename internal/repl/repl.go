// Package repl implements promptshell's interactive shell: start it with no
// task and type tasks directly at a prompt, like the mysql client.
package repl

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/chzyer/readline"

	"github.com/oluwatayo/promptshell/internal/config"
	"github.com/oluwatayo/promptshell/internal/runner"
)

const prompt = "promptshell> "

// Run starts the interactive session. Meta-commands (prefixed with ":") adjust
// session settings; any other line is treated as a task. Settings changed here
// apply to the session only and are not persisted to config.
func Run(cfg config.Config) error {
	rl, err := readline.New(prompt)
	if err != nil {
		return err
	}
	defer rl.Close()

	// Session-scoped options, seeded from the config default provider.
	opt := runner.Options{Provider: cfg.DefaultProvider}

	fmt.Println("promptshell interactive shell — type a task, or :help for commands (:quit to exit).")

	for {
		line, err := rl.Readline()
		if err == readline.ErrInterrupt { // Ctrl-C
			if line == "" {
				break
			}
			continue
		}
		if err != nil { // Ctrl-D / EOF
			break
		}

		line = strings.TrimSpace(line)
		switch {
		case line == "":
			continue
		case line == "exit" || line == "quit":
			break
		case strings.HasPrefix(line, ":"):
			if quit := handleMeta(line, &opt); quit {
				rl.Close()
				fmt.Println("bye")
				return nil
			}
			continue
		}

		// Route confirmation through readline so it doesn't contend for stdin.
		runOpt := opt
		runOpt.Confirm = func() (bool, error) { return confirmVia(rl) }
		if err := runner.Run(context.Background(), cfg, runOpt, line); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
		}
	}

	fmt.Println("bye")
	return nil
}

// confirmVia asks the run/abort question through the readline instance so the
// REPL and the prompt share one input source.
func confirmVia(rl *readline.Instance) (bool, error) {
	rl.SetPrompt("Run this script? [y/N] ")
	defer rl.SetPrompt(prompt)
	line, err := rl.Readline()
	if err != nil { // Ctrl-C / Ctrl-D during the prompt => treat as no
		return false, nil
	}
	return runner.YesNo(line), nil
}

// handleMeta runs a ":" meta-command. It returns true if the session should
// exit.
func handleMeta(line string, opt *runner.Options) bool {
	fields := strings.Fields(line)
	cmd := fields[0]
	arg := strings.TrimSpace(strings.TrimPrefix(line, cmd))

	switch cmd {
	case ":quit", ":q", ":exit":
		return true
	case ":help", ":h":
		printHelp()
	case ":provider":
		setOrShow(arg, &opt.Provider, "provider")
	case ":model":
		setOrShow(arg, &opt.Model, "model")
	case ":shell":
		setOrShow(arg, &opt.Shell, "shell")
	case ":yes":
		opt.AssumeYes = !opt.AssumeYes
		fmt.Printf("auto-run is now %v\n", opt.AssumeYes)
	case ":verbose":
		opt.Verbose = !opt.Verbose
		fmt.Printf("verbose is now %v\n", opt.Verbose)
	default:
		fmt.Printf("unknown command %q — try :help\n", cmd)
	}
	return false
}

func setOrShow(arg string, field *string, label string) {
	if arg == "" {
		cur := *field
		if cur == "" {
			cur = "(default)"
		}
		fmt.Printf("%s: %s\n", label, cur)
		return
	}
	*field = arg
	fmt.Printf("%s set to %q\n", label, arg)
}

func printHelp() {
	fmt.Println(`commands:
  <task>              generate a script for the task and (after confirm) run it
  :provider [name]    show or set the provider for this session
  :model [name]       show or set the model for this session
  :shell [name]       show or set the shell used to run scripts
  :yes                toggle auto-run (skip the confirmation prompt)
  :verbose            toggle verbose diagnostics
  :help               show this help
  :quit               exit (also: exit, quit, Ctrl-D)`)
}
