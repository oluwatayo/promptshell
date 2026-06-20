package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/oluwatayo/promptshell/internal/config"
	"github.com/oluwatayo/promptshell/internal/llm"

	// Register the available providers.
	_ "github.com/oluwatayo/promptshell/internal/llm/gemini"
	_ "github.com/oluwatayo/promptshell/internal/llm/ollama"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(argv []string) error {
	fs := flag.NewFlagSet("promptshell", flag.ContinueOnError)
	providerFlag := fs.String("provider", "", "LLM provider to use (e.g. ollama, gemini)")
	modelFlag := fs.String("model", "", "model override for the selected provider")
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

	return generate(cfg, *providerFlag, *modelFlag, args[0])
}

func printUsage() {
	fmt.Println(`usage:
  promptshell [--provider P] [--model M] "<task>"   generate and run a script
  promptshell config                                show current configuration
  promptshell config provider <name>                set the default provider
  promptshell config key <provider> <api-key>       save an API key for a provider
  promptshell config model <provider> <model>       set the model for a provider

Provider selection precedence: --provider > PROMPTSHELL_PROVIDER > config default.
Ollama is the default and runs locally with no API key.`)
}

// generate is the core flow: pick a provider, ask it to produce a shell script,
// then write, chmod, and execute it.
func generate(cfg config.Config, providerFlag, modelFlag, prompt string) error {
	providerName := firstNonEmpty(providerFlag, os.Getenv("PROMPTSHELL_PROVIDER"), cfg.DefaultProvider)
	ps := cfg.Provider(providerName)

	apiKey := resolveKey(providerName, ps)
	if keyRequired(providerName) && apiKey == "" {
		return fmt.Errorf("no api key for %q. set one with: promptshell config key %s <api-key> (or set %s)",
			providerName, providerName, keyEnvVar(providerName))
	}

	model := firstNonEmpty(modelFlag, ps.Model)
	provider, err := llm.New(providerName, llm.Config{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: ps.BaseURL,
	})
	if err != nil {
		return err
	}

	fmt.Printf("generating with %s...\n", provider.Name())
	ctx := context.Background()
	resp, err := provider.Generate(ctx, llm.Request{
		Prompt: "generate a shell script for this task: " + prompt,
	})
	if err != nil {
		return err
	}

	script := resp.Text
	script = strings.Replace(script, "```sh\n", "", 1)
	script = strings.TrimSuffix(script, "```\n")
	if err := os.WriteFile("prompt.sh", []byte(script), 0o644); err != nil {
		return fmt.Errorf("writing prompt.sh: %w", err)
	}

	if _, err := exec.Command("bash", "-c", "chmod a+x prompt.sh").Output(); err != nil {
		return fmt.Errorf("granting execute permission to prompt.sh: %w", err)
	}
	out, err := exec.Command("bash", "-c", "./prompt.sh").Output()
	if err != nil {
		return fmt.Errorf("running prompt.sh: %w", err)
	}
	fmt.Print(string(out))
	return nil
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
