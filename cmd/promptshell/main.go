package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/oluwatayo/promptshell/internal/config"
	"github.com/oluwatayo/promptshell/internal/repl"
	"github.com/oluwatayo/promptshell/internal/runner"

	// Register the available providers.
	_ "github.com/oluwatayo/promptshell/internal/llm/anthropic"
	_ "github.com/oluwatayo/promptshell/internal/llm/gemini"
	_ "github.com/oluwatayo/promptshell/internal/llm/ollama"
	_ "github.com/oluwatayo/promptshell/internal/llm/openai"
)

// version is the build version, overridden at release time via -ldflags.
var version = "dev"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("promptshell", flag.ContinueOnError)
	fs.Usage = printUsage
	opt := runner.Options{}
	showVersion := fs.Bool("version", false, "print the promptshell version and exit")
	fs.StringVar(&opt.Provider, "provider", "", "LLM provider to use (ollama, gemini, openai, anthropic)")
	fs.StringVar(&opt.Model, "model", "", "model override for the selected provider")
	fs.StringVar(&opt.Shell, "shell", "", "shell used to run the generated script (default: $PROMPTSHELL_SHELL or bash)")
	fs.BoolVar(&opt.DryRun, "dry-run", false, "print the generated script without running it")
	fs.BoolVar(&opt.AssumeYes, "yes", false, "run the generated script without asking for confirmation")
	fs.BoolVar(&opt.Verbose, "verbose", false, "print extra diagnostic output to stderr")
	if err := fs.Parse(argv); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *showVersion {
		fmt.Printf("promptshell %s\n", version)
		return nil
	}
	args := fs.Args()

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	switch {
	case len(args) == 0:
		// No task given: start the interactive shell.
		return repl.Run(cfg)
	case args[0] == "config":
		return runConfig(cfg, args[1:])
	default:
		return runner.Run(context.Background(), cfg, opt, args[0])
	}
}

func printUsage() {
	fmt.Println(`usage:
  promptshell                             start the interactive shell
  promptshell [flags] "<task>"            generate a script for a task, then run it
  promptshell config                      show current configuration
  promptshell config provider <name>      set the default provider
  promptshell config key <provider> <key> save an API key for a provider
  promptshell config model <provider> <m> set the model for a provider

flags:
  --provider P   LLM provider (ollama, gemini, openai, anthropic)
  --model M      model override for the selected provider
  --shell S      shell used to run the script (default: $PROMPTSHELL_SHELL or bash)
  --dry-run      print the generated script without running it
  --yes          run without asking for confirmation
  --verbose      print extra diagnostic output

Provider selection precedence: --provider > PROMPTSHELL_PROVIDER > config default.
Ollama is the default and runs locally with no API key.

The generated script is always shown before it runs, and promptshell asks for
confirmation unless --yes is given.`)
}

// runConfig handles the `config` subcommands.
func runConfig(cfg config.Config, args []string) error {
	if len(args) == 0 {
		fmt.Printf("default provider: %s\n", cfg.DefaultProvider)
		if len(cfg.Providers) == 0 {
			fmt.Println("no per-provider settings saved")
			return nil
		}
		for name, p := range cfg.Providers {
			fmt.Printf("  %s: model=%q baseURL=%q key=%s\n", name, p.Model, p.BaseURL, maskKey(p.APIKey))
		}
		return nil
	}

	switch args[0] {
	case "provider":
		if len(args) < 2 {
			return errors.New("usage: promptshell config provider <name>")
		}
		cfg.SetDefaultProvider(args[1])
	case "key":
		if len(args) < 3 {
			return errors.New("usage: promptshell config key <provider> <api-key>")
		}
		cfg.SetKey(args[1], args[2])
	case "model":
		if len(args) < 3 {
			return errors.New("usage: promptshell config model <provider> <model>")
		}
		cfg.SetModel(args[1], args[2])
	default:
		return fmt.Errorf("unknown config command %q (try: provider, key, model)", args[0])
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Println("saved")
	return nil
}

func maskKey(key string) string {
	if key == "" {
		return "(unset)"
	}
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}
