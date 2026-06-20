package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeRawConfig(t *testing.T, home, contents string) {
	t.Helper()
	dir := filepath.Join(home, ".promptshell", "config")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadMissingFileDefaultsToOllama(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DefaultProvider != DefaultProvider {
		t.Fatalf("default provider = %q, want %q", c.DefaultProvider, DefaultProvider)
	}
	if c.Version != schemaVersion {
		t.Fatalf("version = %d, want %d", c.Version, schemaVersion)
	}
	if c.Providers == nil {
		t.Fatal("Providers map should not be nil")
	}
}

func TestLoadMigratesV1Config(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// A v1 file: single apiKey, no provider (v1 only supported Gemini).
	writeRawConfig(t, home, `{"apiKey":"legacy-key","provider":""}`)

	c, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.DefaultProvider != "gemini" {
		t.Fatalf("default provider = %q, want gemini (migrated)", c.DefaultProvider)
	}
	if got := c.Provider("gemini").APIKey; got != "legacy-key" {
		t.Fatalf("gemini api key = %q, want legacy-key", got)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	c.SetKey("openai", "sk-test")
	c.SetModel("openai", "gpt-4o")
	c.SetDefaultProvider("openai")
	if err := Save(c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.DefaultProvider != "openai" {
		t.Fatalf("default provider = %q, want openai", reloaded.DefaultProvider)
	}
	if got := reloaded.Provider("openai"); got.APIKey != "sk-test" || got.Model != "gpt-4o" {
		t.Fatalf("openai settings = %+v, want key sk-test model gpt-4o", got)
	}
}
