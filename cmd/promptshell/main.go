package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oluwatayo/promptshell/internal/config"
	"github.com/oluwatayo/promptshell/internal/llm"
	"github.com/oluwatayo/promptshell/internal/shell"

	// Register the available providers.
	_ "github.com/oluwatayo/promptshell/internal/llm/anthropic"
	_ "github.com/oluwatayo/promptshell/internal/llm/gemini"
	_ "github.com/oluwatayo/promptshell/internal/llm/ollama"
	_ "github.com/oluwatayo/promptshell/internal/llm/openai"
)

const scriptPath = "prompt.sh"

// genOptions holds the resolved flags for a generation run.
type genOptions struct {
	provider  string
	model     string
	shell     string
	dryRun    bool
	assumeYes bool
	verbose   bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("promptshell", flag.ContinueOnError)
	opt := genOptions{}
	fs.StringVar(&opt.provider, "provider", "", "LLM provider to use (e.g. ollama, gemini, openai, anthropic)")
	fs.StringVar(&opt.model, "model", "", "model override for the selected provider")
	fs.StringVar(&opt.shell, "shell", "", "shell used to run the generated script (default: $PROMPTSHELL_SHELL or bash)")
	fs.BoolVar(&opt.dryRun, "dry-run", false, "print the generated script without running it")
	fs.BoolVar(&opt.assumeYes, "yes", false, "run the generated script without asking for confirmation")
	fs.BoolVar(&opt.verbose, "verbose", false, "print extra diagnostic output to stderr")
	if err := fs.Parse(argv); err != nil {
		return err
	}
	args := fs.Args()

	if len(args) == 0 {
		printUsage()
		return nil
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if args[0] == "config" {
		return runConfig(cfg, args[1:])
	}

	return generate(cfg, opt, args[0])
}

func printUsage() {
	fmt.Println(`usage:
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

// generate is the core flow: pick a provider, ask it to produce a shell script,
// preview it, and — after confirmation — run it.
func generate(cfg config.Config, opt genOptions, prompt string) error {
	providerName := firstNonEmpty(opt.provider, os.Getenv("PROMPTSHELL_PROVIDER"), cfg.DefaultProvider)
	ps := cfg.Provider(providerName)

	apiKey := resolveKey(providerName, ps)
	if keyRequired(providerName) && apiKey == "" {
		return fmt.Errorf("no api key for %q. set one with: promptshell config key %s <api-key> (or set %s)",
			providerName, providerName, keyEnvVar(providerName))
	}

	model := firstNonEmpty(opt.model, ps.Model)
	provider, err := llm.New(providerName, llm.Config{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: ps.BaseURL,
	})
	if err != nil {
		return err
	}

	if opt.verbose {
		fmt.Fprintf(os.Stderr, "provider=%s model=%s\n", providerName, firstNonEmpty(model, "(default)"))
	}

	fmt.Printf("generating with %s...\n", provider.Name())
	resp, err := provider.Generate(context.Background(), llm.Request{
		Prompt: "generate a shell script for this task: " + prompt,
	})
	if err != nil {
		return err
	}

	script := shell.Extract(resp.Text)
	fmt.Println("\n--- generated script ---")
	fmt.Println(script)
	fmt.Println("------------------------")

	if opt.dryRun {
		fmt.Println("(dry run — not executing)")
		return nil
	}

	if !opt.assumeYes {
		ok, err := confirm("Run this script?")
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("aborted")
			return nil
		}
	}

	if err := shell.Write(scriptPath, script); err != nil {
		return fmt.Errorf("writing %s: %w", scriptPath, err)
	}

	sh := firstNonEmpty(opt.shell, os.Getenv("PROMPTSHELL_SHELL"), shell.DefaultShell)
	if opt.verbose {
		fmt.Fprintf(os.Stderr, "running: %s %s\n", sh, scriptPath)
	}
	out, runErr := shell.Execute(context.Background(), sh, scriptPath)
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	if runErr != nil {
		return fmt.Errorf("running %s: %w", scriptPath, runErr)
	}
	return nil
}

// confirm prompts the user for a yes/no answer, defaulting to no.
func confirm(prompt string) (bool, error) {
	fmt.Printf("%s [y/N] ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
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

// keyRequired reports whether a provider needs an API key. Local providers do
// not.
func keyRequired(provider string) bool {
	return provider != "ollama"
}

// resolveKey returns the API key for a provider, preferring the provider-
// specific environment variable, then the legacy global one, then config.
func resolveKey(provider string, ps config.ProviderSettings) string {
	if key := os.Getenv(keyEnvVar(provider)); key != "" {
		return key
	}
	if key := os.Getenv("PROMPTSHELL_API_KEY"); key != "" {
		return key
	}
	return ps.APIKey
}

func keyEnvVar(provider string) string {
	return "PROMPTSHELL_" + strings.ToUpper(provider) + "_API_KEY"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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
