package runner

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/oluwatayo/promptshell/internal/config"
	"github.com/oluwatayo/promptshell/internal/llm"
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
