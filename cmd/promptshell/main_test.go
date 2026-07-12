package main

import "testing"

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
