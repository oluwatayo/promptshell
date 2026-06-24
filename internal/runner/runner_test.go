package runner

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/oluwatayo/promptshell/internal/config"
	"github.com/oluwatayo/promptshell/internal/llm"
	"github.com/oluwatayo/promptshell/internal/llm/ollama"
)

// fakeProvider returns a fixed script that creates a marker file when run, so
// tests can detect whether the generated script was executed.
type fakeProvider struct{ script string }

func (fakeProvider) Name() string { return "fake" }

func (f fakeProvider) Generate(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{Text: f.script}, nil
}

func init() {
	llm.Register("fake", func(llm.Config) (llm.Provider, error) {
		return fakeProvider{script: "touch ran.marker\n"}, nil
	})
}

func fakeCfg() config.Config {
	return config.Config{
		DefaultProvider: "fake",
		Providers:       map[string]config.ProviderSettings{"fake": {APIKey: "x"}},
	}
}

func exists(t *testing.T, name string) bool {
	t.Helper()
	_, err := os.Stat(name)
	return err == nil
}

func TestRunDryRunDoesNotExecute(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := Run(context.Background(), fakeCfg(), Options{DryRun: true}, "task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exists(t, "ran.marker") {
		t.Error("dry run executed the script (marker created)")
	}
	if exists(t, "prompt.sh") {
		t.Error("dry run wrote prompt.sh")
	}
}

func TestRunAssumeYesExecutes(t *testing.T) {
	t.Chdir(t.TempDir())

	if err := Run(context.Background(), fakeCfg(), Options{AssumeYes: true}, "task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !exists(t, "ran.marker") {
		t.Error("script was not executed (no marker)")
	}
	if !exists(t, "prompt.sh") {
		t.Error("prompt.sh was not written")
	}
}

func TestRunConfirmDeclinedDoesNotExecute(t *testing.T) {
	t.Chdir(t.TempDir())

	opt := Options{Confirm: func() (bool, error) { return false, nil }}
	if err := Run(context.Background(), fakeCfg(), opt, "task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if exists(t, "ran.marker") {
		t.Error("declined confirmation still executed the script")
	}
}

func TestRunConfirmAcceptedExecutes(t *testing.T) {
	t.Chdir(t.TempDir())

	opt := Options{Confirm: func() (bool, error) { return true, nil }}
	if err := Run(context.Background(), fakeCfg(), opt, "task"); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !exists(t, "ran.marker") {
		t.Error("accepted confirmation did not execute the script")
	}
}

func TestRunMissingKeyErrors(t *testing.T) {
	t.Chdir(t.TempDir())
	// Ensure no environment key leaks in.
	t.Setenv("PROMPTSHELL_FAKE_API_KEY", "")
	t.Setenv("PROMPTSHELL_API_KEY", "")

	cfg := config.Config{
		DefaultProvider: "fake",
		Providers:       map[string]config.ProviderSettings{"fake": {}},
	}
	err := Run(context.Background(), cfg, Options{AssumeYes: true}, "task")
	if err == nil {
		t.Fatal("expected an error for missing api key, got nil")
	}
	if !strings.Contains(err.Error(), "api key") {
		t.Errorf("error = %q, want it to mention the missing api key", err)
	}
}

func TestResolveKeyPrefersProviderEnv(t *testing.T) {
	t.Setenv("PROMPTSHELL_FAKE_API_KEY", "from-env")
	got := ResolveKey("fake", config.ProviderSettings{APIKey: "from-config"})
	if got != "from-env" {
		t.Errorf("ResolveKey = %q, want from-env", got)
	}
}

func TestKeyRequired(t *testing.T) {
	if KeyRequired("ollama") {
		t.Error("ollama should not require a key")
	}
	if !KeyRequired("openai") {
		t.Error("openai should require a key")
	}
}

func TestLooksLikeFirstRun(t *testing.T) {
	empty := config.Config{DefaultProvider: "ollama", Providers: map[string]config.ProviderSettings{}}
	configured := config.Config{DefaultProvider: "ollama", Providers: map[string]config.ProviderSettings{"ollama": {}}}

	t.Run("fresh default ollama", func(t *testing.T) {
		t.Setenv("PROMPTSHELL_PROVIDER", "")
		if !looksLikeFirstRun(empty, Options{}, "ollama") {
			t.Error("want true for an unconfigured default-ollama run")
		}
	})
	t.Run("provider chosen by flag", func(t *testing.T) {
		t.Setenv("PROMPTSHELL_PROVIDER", "")
		if looksLikeFirstRun(empty, Options{Provider: "ollama"}, "ollama") {
			t.Error("want false when the user passed --provider")
		}
	})
	t.Run("provider chosen by env", func(t *testing.T) {
		t.Setenv("PROMPTSHELL_PROVIDER", "ollama")
		if looksLikeFirstRun(empty, Options{}, "ollama") {
			t.Error("want false when PROMPTSHELL_PROVIDER is set")
		}
	})
	t.Run("already configured", func(t *testing.T) {
		t.Setenv("PROMPTSHELL_PROVIDER", "")
		if looksLikeFirstRun(configured, Options{}, "ollama") {
			t.Error("want false when providers are configured")
		}
	})
	t.Run("non-ollama provider", func(t *testing.T) {
		t.Setenv("PROMPTSHELL_PROVIDER", "")
		if looksLikeFirstRun(empty, Options{}, "openai") {
			t.Error("want false for a non-ollama provider")
		}
	})
}

// unreachableOllama is a stand-in for the ollama provider that always reports
// the server is unreachable.
type unreachableOllama struct{}

func (unreachableOllama) Name() string { return "ollama" }
func (unreachableOllama) Generate(context.Context, llm.Request) (llm.Response, error) {
	return llm.Response{}, fmt.Errorf("boom: %w", ollama.ErrUnreachable)
}

func TestRunFirstRunShowsHelpNotError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PROMPTSHELL_PROVIDER", "")
	llm.Register("ollama", func(llm.Config) (llm.Provider, error) { return unreachableOllama{}, nil })

	cfg := config.Config{DefaultProvider: "ollama", Providers: map[string]config.ProviderSettings{}}
	if err := Run(context.Background(), cfg, Options{}, "task"); err != nil {
		t.Fatalf("want nil (help shown), got %v", err)
	}
	if exists(t, "ran.marker") || exists(t, "prompt.sh") {
		t.Error("first-run help should not generate or execute a script")
	}
}

func TestRunExplicitOllamaUnreachableReturnsError(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("PROMPTSHELL_PROVIDER", "")
	llm.Register("ollama", func(llm.Config) (llm.Provider, error) { return unreachableOllama{}, nil })

	// User explicitly chose ollama → they get the real error, not first-run help.
	cfg := config.Config{DefaultProvider: "ollama", Providers: map[string]config.ProviderSettings{}}
	err := Run(context.Background(), cfg, Options{Provider: "ollama"}, "task")
	if err == nil {
		t.Fatal("want an error when ollama was explicitly selected and is unreachable")
	}
}

func TestYesNo(t *testing.T) {
	for _, in := range []string{"y", "Y", "yes", "  YES  ", "Yes\n"} {
		if !YesNo(in) {
			t.Errorf("YesNo(%q) = false, want true", in)
		}
	}
	for _, in := range []string{"", "n", "no", "nope", "x"} {
		if YesNo(in) {
			t.Errorf("YesNo(%q) = true, want false", in)
		}
	}
}
