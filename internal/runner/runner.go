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

	fmt.Printf("generating with %s...\n", provider.Name())
	resp, err := provider.Generate(ctx, llm.Request{
		Prompt: "generate a shell script for this task: " + prompt,
	})
	if err != nil {
		return err
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

// stdinConfirm prompts the user for a yes/no answer on stdin, defaulting to no.
func stdinConfirm() (bool, error) {
	fmt.Print("Run this script? [y/N] ")
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return YesNo(line), nil
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
