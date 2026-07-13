// Package runner contains the core flow shared by the one-shot CLI and the
// interactive shell: pick a provider, generate a script, preview it, and —
// after confirmation — execute it.
package runner

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/oluwatayo/promptshell/internal/config"
	"github.com/oluwatayo/promptshell/internal/llm"
	"github.com/oluwatayo/promptshell/internal/llm/ollama"
	"github.com/oluwatayo/promptshell/internal/shell"
)

const scriptPath = "prompt.sh"

// Options controls a single task run.
type Options struct {
	Provider  string
	Model     string
	Shell     string
	DryRun    bool
	AssumeYes bool
	Verbose   bool

	// Confirm asks whether to run the previewed script. If nil, a default
	// stdin prompt is used. Ignored when AssumeYes or DryRun is set.
	Confirm func() (bool, error)
}

// Run generates a script for prompt and, after confirmation, executes it.
func Run(ctx context.Context, cfg config.Config, opt Options, prompt string) error {
	providerName := FirstNonEmpty(opt.Provider, os.Getenv("PROMPTSHELL_PROVIDER"), cfg.DefaultProvider)
	ps := cfg.Provider(providerName)

	apiKey := ResolveKey(providerName, ps)
	if KeyRequired(providerName) && apiKey == "" {
		return fmt.Errorf("no api key for %q. set one with: promptshell config key %s <api-key> (or set %s)",
			providerName, providerName, KeyEnvVar(providerName))
	}

	model := FirstNonEmpty(opt.Model, ps.Model)
	provider, err := llm.New(providerName, llm.Config{
		APIKey:  apiKey,
		Model:   model,
		BaseURL: ps.BaseURL,
	})
	if err != nil {
		return err
	}

	if opt.Verbose {
		fmt.Fprintf(os.Stderr, "provider=%s model=%s\n", providerName, FirstNonEmpty(model, "(default)"))
	}

	// On a fresh install with nothing configured, a failed Ollama call means
	// the user hasn't set anything up yet — guide them (and offer to set Ollama
	// up) instead of printing a progress line and a raw connection error.
	firstRun := looksLikeFirstRun(cfg, opt, providerName)
	if !firstRun {
		fmt.Printf("generating with %s...\n", provider.Name())
	}
	req := llm.Request{
		// Chatty models pad answers with prose and usage examples;
		// shell.Extract defends against that, but ask for raw output anyway.
		System: "You write shell scripts. Respond with only the script, in a single ```sh code block. " +
			"No explanations, no usage instructions, no extra code blocks.",
		Prompt: "generate a shell script for this task: " + prompt,
	}
	resp, err := provider.Generate(ctx, req)
	if err != nil {
		if firstRun && errors.Is(err, ollama.ErrUnreachable) {
			pullModel := FirstNonEmpty(model, ollama.DefaultModel)
			if !handleOllamaFirstRun(pullModel, ollama.DefaultBaseURL) {
				return nil
			}
			// Ollama is set up now — retry the request once.
			resp, err = provider.Generate(ctx, req)
		}
		if err != nil {
			return err
		}
	}

	script := shell.Extract(resp.Text)
	fmt.Println("\n--- generated script ---")
	fmt.Println(script)
	fmt.Println("------------------------")

	if opt.DryRun {
		fmt.Println("(dry run — not executing)")
		return nil
	}

	if !opt.AssumeYes {
		confirm := opt.Confirm
		if confirm == nil {
			confirm = stdinConfirm
		}
		ok, err := confirm()
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

	sh := FirstNonEmpty(opt.Shell, os.Getenv("PROMPTSHELL_SHELL"), shell.DefaultShell)
	if opt.Verbose {
		fmt.Fprintf(os.Stderr, "running: %s %s\n", sh, scriptPath)
	}
	out, runErr := shell.Execute(ctx, sh, scriptPath)
	if len(out) > 0 {
		fmt.Print(string(out))
	}
	if runErr != nil {
		return fmt.Errorf("running %s: %w", scriptPath, runErr)
	}
	return nil
}

// looksLikeFirstRun reports whether this is an unconfigured run that fell back
// to the default Ollama provider: the user did not pick a provider (no flag or
// env var) and nothing is saved in config.
func looksLikeFirstRun(cfg config.Config, opt Options, providerName string) bool {
	chosenByUser := opt.Provider != "" || os.Getenv("PROMPTSHELL_PROVIDER") != ""
	return !chosenByUser && providerName == ollama.Name && len(cfg.Providers) == 0
}

// handleOllamaFirstRun shows first-run guidance and, in an interactive
// terminal, offers to install and start Ollama. It returns true if Ollama is
// now reachable and the caller should retry.
func handleOllamaFirstRun(model, baseURL string) bool {
	interactive := stdinIsTTY()
	printFirstRunHelp(baseURL, model, interactive)
	if !interactive {
		return false
	}

	fmt.Println()
	ok, err := promptYesNo("Install and start Ollama now?")
	if err != nil || !ok {
		fmt.Println("No problem — set it up when you're ready, then re-run your command.")
		return false
	}
	if !setupOllama(defaultOllamaEnv(), model, baseURL) {
		return false
	}
	fmt.Println("\nOllama is ready — generating your script...")
	return true
}

// printFirstRunHelp explains the fresh-install state and the ways forward.
func printFirstRunHelp(baseURL, model string, interactive bool) {
	fmt.Printf(`promptshell isn't set up yet.

By default it uses Ollama — a model that runs locally on your machine with no
API key — but Ollama isn't reachable at %s.
`, baseURL)

	if interactive {
		fmt.Printf("\npromptshell can install and start Ollama for you (see the prompt below),\n"+
			"or you can set it up yourself: install from https://ollama.com, then `ollama pull %s`.\n", model)
	} else {
		fmt.Printf("\nTo use Ollama: install it from https://ollama.com, start it, then run:\n  ollama pull %s\n", model)
	}

	fmt.Println(`
Or use a cloud provider instead:
  promptshell config provider openai
  promptshell config key openai <your-api-key>

Run 'promptshell --help' for the full list of options.`)
}

// stdinIsTTY reports whether standard input is an interactive terminal.
func stdinIsTTY() bool {
	fi, err := os.Stdin.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// promptYesNo asks a yes/no question on stdin, defaulting to no.
func promptYesNo(prompt string) (bool, error) {
	fmt.Printf("%s [y/N] ", prompt)
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return YesNo(line), nil
}

// stdinConfirm prompts before running the generated script, defaulting to no.
func stdinConfirm() (bool, error) {
	return promptYesNo("Run this script?")
}

// YesNo interprets a line of input as an affirmative answer.
func YesNo(line string) bool {
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "y", "yes":
		return true
	default:
		return false
	}
}

// KeyRequired reports whether a provider needs an API key. Local providers do
// not.
func KeyRequired(provider string) bool {
	return provider != "ollama"
}

// ResolveKey returns the API key for a provider, preferring the provider-
// specific environment variable, then the legacy global one, then config.
func ResolveKey(provider string, ps config.ProviderSettings) string {
	if key := os.Getenv(KeyEnvVar(provider)); key != "" {
		return key
	}
	if key := os.Getenv("PROMPTSHELL_API_KEY"); key != "" {
		return key
	}
	return ps.APIKey
}

// KeyEnvVar returns the provider-specific API key environment variable name.
func KeyEnvVar(provider string) string {
	return "PROMPTSHELL_" + strings.ToUpper(provider) + "_API_KEY"
}

// FirstNonEmpty returns the first non-empty string, or "".
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
