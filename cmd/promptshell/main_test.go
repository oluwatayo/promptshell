package main

import (
	"strings"
	"testing"
)

func TestResolveVersionUsesInjectedVersion(t *testing.T) {
	old := version
	defer func() { version = old }()

	version = "1.2.3"
	if got := resolveVersion(); got != "1.2.3" {
		t.Errorf("resolveVersion() = %q, want %q", got, "1.2.3")
	}
}

func TestResolveVersionDevBuildFallsBack(t *testing.T) {
	old := version
	defer func() { version = old }()

	// In a `go test` binary ReadBuildInfo reports no usable main version,
	// so the fallback chain must land back on "dev" — never empty.
	version = "dev"
	if got := resolveVersion(); got == "" {
		t.Error("resolveVersion() returned empty string for a dev build")
	}
}

func TestVersionFlagAliases(t *testing.T) {
	for _, argv := range [][]string{{"-v"}, {"--version"}, {"-version"}} {
		if err := run(argv); err != nil {
			t.Errorf("run(%v) returned error: %v", argv, err)
		}
	}
}

func TestUpdateFlagIsRegistered(t *testing.T) {
	// A dev build refuses to self-update before touching the network, so
	// --update on a test binary must return the refusal — not a flag error.
	err := run([]string{"--update"})
	if err == nil {
		t.Fatal("run(--update) on a dev build should refuse, got nil")
	}
	if !strings.Contains(err.Error(), "development build") {
		t.Errorf("error = %q, want a development-build refusal", err)
	}
}
