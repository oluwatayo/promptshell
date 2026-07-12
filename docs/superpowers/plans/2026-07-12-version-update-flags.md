# Version & Update Flags Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `-v`/`--version` (with a `go install` build-info fallback) and `--update` (self-update from GitHub releases) to promptshell, and give install.sh a download progress bar.

**Architecture:** A new `internal/update` package owns the whole self-update flow (resolve latest tag via the GitHub `releases/latest` redirect, semver-compare, download with progress, mandatory sha256 verify, atomic binary replace). All OS/network touchpoints are injected through an `Env` struct (same pattern as `internal/runner/ollama_setup.go`) so everything is unit-testable with `httptest` and temp dirs. `cmd/promptshell/main.go` only gains flag wiring. install.sh gets a TTY-gated `--progress-bar` on the asset download.

**Tech Stack:** Go 1.24, stdlib (`net/http`, `archive/tar`, `compress/gzip`, `crypto/sha256`), plus one new dependency: `golang.org/x/mod` (semver only).

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-12-version-update-flags-design.md`
- Module path: `github.com/oluwatayo/promptshell`; repo URL `https://github.com/oluwatayo/promptshell`
- Release asset naming (GoReleaser): `promptshell_<version-no-v>_<os>_<arch>.tar.gz` + `checksums.txt` (`<sha256>  <filename>` lines); binary sits at the archive root named `promptshell`
- No `-u` short flag; no auto-sudo; no background update checks; darwin/linux only
- Checksum verification is MANDATORY in `--update` (missing file/entry or mismatch all abort)
- All errors are friendly one-liners (the CLI prints them as `error: <msg>`); exit 0 for both "updated" and "already up to date"
- Conventional Commits (`feat:`, `test:`, `docs:`); every commit ends with the trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- Before each commit: `gofmt -l .` must print nothing; `go vet ./...` and `go test ./...` must pass
- Branch: `feat/version-update-flags` (already created off main @ v0.4.0)

---

### Task 1: `-v` alias and build-info version fallback

**Files:**
- Modify: `cmd/promptshell/main.go` (flag block ~line 35; version print ~line 48; usage text ~line 71)
- Test: `cmd/promptshell/main_test.go` (create)

**Interfaces:**
- Produces: `resolveVersion() string` in package main — returns the ldflags-injected version, falling back to `debug.ReadBuildInfo()` for `go install` builds. Task 6 passes this to the updater.

- [ ] **Step 1: Write the failing test**

Create `cmd/promptshell/main_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/promptshell/ -run TestResolveVersion -v`
Expected: FAIL to compile with `undefined: resolveVersion`

- [ ] **Step 3: Implement**

In `cmd/promptshell/main.go`:

Add `"runtime/debug"` to the imports.

Replace the single line `showVersion := fs.Bool("version", false, "print the promptshell version and exit")` with:

```go
	var showVersion bool
	fs.BoolVar(&showVersion, "version", false, "print the promptshell version and exit")
	fs.BoolVar(&showVersion, "v", false, "print the promptshell version and exit (shorthand)")
```

Change the check `if *showVersion {` to `if showVersion {`, and inside it change `version` to `resolveVersion()`:

```go
	if showVersion {
		fmt.Printf("promptshell %s\n", resolveVersion())
		return nil
	}
```

Add below `printUsage`:

```go
// resolveVersion returns the release version injected via -ldflags, falling
// back to the Go module version for `go install` builds (which skip ldflags).
func resolveVersion() string {
	if version != "dev" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		if v := bi.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return version
}
```

In `printUsage`, add to the flags block (after the `--verbose` line):

```
  --version, -v  print the promptshell version and exit
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/promptshell/ -v`
Expected: all three tests PASS (note: `TestVersionFlagAliases` prints `promptshell dev` lines — that's fine)

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add cmd/promptshell/main.go cmd/promptshell/main_test.go
git commit -m "feat: add -v shorthand and go-install version fallback

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: `internal/update` — latest-version lookup and comparison

**Files:**
- Create: `internal/update/update.go`
- Test: `internal/update/update_test.go` (create)
- Modify: `go.mod` / `go.sum` (via `go get`)

**Interfaces:**
- Produces:
  - `type Env struct { RepoURL string; Client *http.Client; ExecPath func() (string, error); GOOS, GOARCH string; Out io.Writer; IsTTY bool }`
  - `func DefaultEnv() Env` — production wiring (GitHub URL, `os.Stderr`, real executable path)
  - `func LatestVersion(env Env) (string, error)` — returns the latest release tag, always `v`-prefixed valid semver
  - `func normalize(v string) string` — trims space, ensures a single leading `v`
- Consumes: nothing from other tasks.

- [ ] **Step 1: Add the dependency**

```bash
go get golang.org/x/mod@latest
```

Expected: `go: added golang.org/x/mod v0.x.y` (exact version varies)

- [ ] **Step 2: Write the failing test**

Create `internal/update/update_test.go`:

```go
package update

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeLatest serves the GitHub /releases/latest redirect shape: a redirect
// from /releases/latest to /releases/tag/<tag>.
func fakeLatest(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("/releases/tag/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func testEnv(srv *httptest.Server) Env {
	return Env{
		RepoURL: srv.URL,
		Client:  srv.Client(),
		GOOS:    "linux",
		GOARCH:  "amd64",
		Out:     io.Discard,
	}
}

func TestLatestVersion(t *testing.T) {
	srv := fakeLatest(t, "v0.5.0")
	got, err := LatestVersion(testEnv(srv))
	if err != nil {
		t.Fatalf("LatestVersion() error: %v", err)
	}
	if got != "v0.5.0" {
		t.Errorf("LatestVersion() = %q, want %q", got, "v0.5.0")
	}
}

func TestLatestVersionRejectsGarbageTag(t *testing.T) {
	srv := fakeLatest(t, "not-a-version")
	if _, err := LatestVersion(testEnv(srv)); err == nil {
		t.Error("LatestVersion() accepted a garbage tag, want error")
	}
}

func TestLatestVersionNoRedirect(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	if _, err := LatestVersion(testEnv(srv)); err == nil {
		t.Error("LatestVersion() succeeded with no releases, want error")
	}
}

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"0.4.0":   "v0.4.0",
		"v0.4.0":  "v0.4.0",
		" v0.4.0": "v0.4.0",
	} {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/update/ -v`
Expected: FAIL to compile with `undefined: Env` / `undefined: LatestVersion`

- [ ] **Step 4: Write the implementation**

Create `internal/update/update.go`:

```go
// Package update implements self-updating from GitHub releases: it resolves
// the latest release tag, downloads the right archive for this platform,
// verifies its checksum, and atomically replaces the running binary.
package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Env abstracts everything the update flow needs from the outside world so it
// can be unit-tested against fake release servers and temp binaries.
type Env struct {
	RepoURL  string // repository page URL, no trailing slash
	Client   *http.Client
	ExecPath func() (string, error) // path of the binary to replace
	GOOS     string
	GOARCH   string
	Out      io.Writer // status and progress output
	IsTTY    bool      // whether Out is a terminal (enables the progress bar)
}

// DefaultEnv wires Env to GitHub and the real process.
func DefaultEnv() Env {
	return Env{
		RepoURL:  "https://github.com/oluwatayo/promptshell",
		Client:   &http.Client{Timeout: 5 * time.Minute},
		ExecPath: executablePath,
		GOOS:     runtime.GOOS,
		GOARCH:   runtime.GOARCH,
		Out:      os.Stderr,
		IsTTY:    isTTY(os.Stderr),
	}
}

// LatestVersion resolves the newest release tag by following the
// /releases/latest redirect — no GitHub API, so no rate limit or token.
func LatestVersion(env Env) (string, error) {
	req, err := http.NewRequest(http.MethodHead, env.RepoURL+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	resp, err := env.Client.Do(req)
	if err != nil {
		return "", err
	}
	_ = resp.Body.Close()

	final := resp.Request.URL.Path
	i := strings.LastIndex(final, "/tag/")
	if i < 0 {
		return "", fmt.Errorf("no releases found at %s", env.RepoURL)
	}
	tag := normalize(final[i+len("/tag/"):])
	if !semver.IsValid(tag) {
		return "", fmt.Errorf("could not determine the latest version (got %q)", tag)
	}
	return tag, nil
}

// normalize trims whitespace and guarantees a single leading "v" so versions
// are comparable with the semver package.
func normalize(v string) string {
	return "v" + strings.TrimPrefix(strings.TrimSpace(v), "v")
}

func executablePath() (string, error) {
	p, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(p)
}

// isTTY reports whether f is attached to a terminal.
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/update/ -v`
Expected: all 4 tests PASS

- [ ] **Step 6: Tidy and commit**

```bash
go mod tidy && gofmt -l . && go vet ./...
git add internal/update/ go.mod go.sum
git commit -m "feat: resolve latest release version for self-update

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: download with progress display

**Files:**
- Create: `internal/update/download.go`
- Test: `internal/update/download_test.go` (create)

**Interfaces:**
- Consumes: `Env` from Task 2.
- Produces:
  - `func download(env Env, url, dest, tag string) error` — streams url to dest; progress bar on `env.Out` when `env.IsTTY` and Content-Length is known, single "Downloading..." line otherwise
  - `func formatBytes(n int64) string` — "4.2 MB" / "312 KB" / "size unknown" for n < 0

- [ ] **Step 1: Write the failing test**

Create `internal/update/download_test.go`:

```go
package update

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func serveBytes(t *testing.T, path string, body []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadWritesFile(t *testing.T) {
	body := bytes.Repeat([]byte("promptshell!"), 1000)
	srv := serveBytes(t, "/asset.tar.gz", body)

	dest := filepath.Join(t.TempDir(), "asset.tar.gz")
	env := testEnv(srv)
	if err := download(env, srv.URL+"/asset.tar.gz", dest, "v0.5.0"); err != nil {
		t.Fatalf("download() error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("downloaded %d bytes, want %d identical bytes", len(got), len(body))
	}
}

func TestDownloadShowsProgressOnTTY(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 10_000)
	srv := serveBytes(t, "/asset.tar.gz", body)

	var out bytes.Buffer
	env := testEnv(srv)
	env.Out = &out
	env.IsTTY = true

	dest := filepath.Join(t.TempDir(), "asset.tar.gz")
	if err := download(env, srv.URL+"/asset.tar.gz", dest, "v0.5.0"); err != nil {
		t.Fatalf("download() error: %v", err)
	}
	if !strings.Contains(out.String(), "100%") {
		t.Errorf("progress output missing 100%%, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "v0.5.0") {
		t.Errorf("progress output missing version label, got: %q", out.String())
	}
}

func TestDownloadNonTTYPrintsSingleLine(t *testing.T) {
	srv := serveBytes(t, "/asset.tar.gz", []byte("data"))

	var out bytes.Buffer
	env := testEnv(srv)
	env.Out = &out

	dest := filepath.Join(t.TempDir(), "asset.tar.gz")
	if err := download(env, srv.URL+"/asset.tar.gz", dest, "v0.5.0"); err != nil {
		t.Fatalf("download() error: %v", err)
	}
	if strings.Contains(out.String(), "\r") {
		t.Errorf("non-TTY output must not use carriage returns, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "Downloading") {
		t.Errorf("expected a Downloading line, got: %q", out.String())
	}
}

func TestDownload404(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "asset.tar.gz")
	if err := download(testEnv(srv), srv.URL+"/nope", dest, "v0.5.0"); err == nil {
		t.Error("download() succeeded on 404, want error")
	}
}

func TestFormatBytes(t *testing.T) {
	for n, want := range map[int64]string{
		-1:              "size unknown",
		1024:            "1 KB",
		4_400_000:       "4.2 MB",
	} {
		if got := formatBytes(n); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/update/ -v`
Expected: FAIL to compile with `undefined: download` / `undefined: formatBytes`

- [ ] **Step 3: Write the implementation**

Create `internal/update/download.go`:

```go
package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

// download streams url into dest. On a TTY with a known Content-Length it
// renders a single-line percentage bar on env.Out; otherwise it prints one
// "Downloading..." line so logs stay clean.
func download(env Env, url, dest, tag string) error {
	resp, err := env.Client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s returned %s", url, resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}

	label := "Downloading promptshell " + tag
	var src io.Reader = resp.Body
	showBar := env.IsTTY && resp.ContentLength > 0
	if showBar {
		src = io.TeeReader(resp.Body, &progress{out: env.Out, label: label, total: resp.ContentLength})
	} else {
		_, _ = fmt.Fprintf(env.Out, "%s (%s)...\n", label, formatBytes(resp.ContentLength))
	}

	_, copyErr := io.Copy(f, src)
	if showBar {
		_, _ = fmt.Fprintln(env.Out) // terminate the \r progress line
	}
	if err := f.Close(); err != nil {
		return err
	}
	if copyErr != nil {
		return fmt.Errorf("download failed: %w", copyErr)
	}
	return nil
}

// progress is an io.Writer that renders download progress as bytes flow
// through it via io.TeeReader.
type progress struct {
	out     io.Writer
	label   string
	total   int64
	written int64
	lastPct int64
}

func (p *progress) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if pct := p.written * 100 / p.total; pct != p.lastPct {
		p.lastPct = pct
		_, _ = fmt.Fprintf(p.out, "\r%s... %d%% (%s / %s)", p.label, pct, formatBytes(p.written), formatBytes(p.total))
	}
	return len(b), nil
}

// formatBytes renders a byte count for humans; n < 0 means unknown.
func formatBytes(n int64) string {
	const mb = 1024 * 1024
	switch {
	case n < 0:
		return "size unknown"
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	default:
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/update/ -v`
Expected: all tests PASS

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add internal/update/download.go internal/update/download_test.go
git commit -m "feat: download release assets with progress display

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 4: checksum verification and atomic binary replace

**Files:**
- Create: `internal/update/install.go`
- Test: `internal/update/install_test.go` (create)

**Interfaces:**
- Consumes: `Env` and `testEnv` from Task 2; test helper `serveBytes` from Task 3.
- Produces:
  - `func verifyChecksum(env Env, checksumsURL, archivePath, assetName string) error` — mandatory sha256 check; missing file/entry or mismatch all error
  - `func replaceBinary(archivePath, execPath string) error` — extracts the `promptshell` entry from the tar.gz and atomically renames it over execPath (mode 0755)
  - Test helper `makeArchive(t *testing.T, binaryName string, content []byte) []byte` — builds an in-memory tar.gz (reused by Task 5's tests)

- [ ] **Step 1: Write the failing test**

Create `internal/update/install_test.go`:

```go
package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// makeArchive builds a tar.gz holding a single file entry, mimicking a
// GoReleaser release archive.
func makeArchive(t *testing.T, binaryName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func writeTemp(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVerifyChecksumOK(t *testing.T) {
	asset := []byte("archive-bytes")
	sums := fmt.Sprintf("%s  my.tar.gz\n", sha256hex(asset))
	srv := serveBytes(t, "/checksums.txt", []byte(sums))

	path := writeTemp(t, "my.tar.gz", asset)
	if err := verifyChecksum(testEnv(srv), srv.URL+"/checksums.txt", path, "my.tar.gz"); err != nil {
		t.Errorf("verifyChecksum() error: %v", err)
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	sums := fmt.Sprintf("%s  my.tar.gz\n", sha256hex([]byte("different")))
	srv := serveBytes(t, "/checksums.txt", []byte(sums))

	path := writeTemp(t, "my.tar.gz", []byte("archive-bytes"))
	if err := verifyChecksum(testEnv(srv), srv.URL+"/checksums.txt", path, "my.tar.gz"); err == nil {
		t.Error("verifyChecksum() passed on mismatch, want error")
	}
}

func TestVerifyChecksumMissingEntry(t *testing.T) {
	srv := serveBytes(t, "/checksums.txt", []byte("abc123  other.tar.gz\n"))
	path := writeTemp(t, "my.tar.gz", []byte("archive-bytes"))
	if err := verifyChecksum(testEnv(srv), srv.URL+"/checksums.txt", path, "my.tar.gz"); err == nil {
		t.Error("verifyChecksum() passed with no entry for the asset, want error")
	}
}

func TestVerifyChecksumMissingFile(t *testing.T) {
	srv := serveBytes(t, "/other", nil) // 404 for /checksums.txt
	path := writeTemp(t, "my.tar.gz", []byte("archive-bytes"))
	if err := verifyChecksum(testEnv(srv), srv.URL+"/checksums.txt", path, "my.tar.gz"); err == nil {
		t.Error("verifyChecksum() passed with no checksums.txt, want error")
	}
}

func TestReplaceBinary(t *testing.T) {
	newBin := []byte("#!/bin/sh\necho new\n")
	archive := writeTemp(t, "rel.tar.gz", makeArchive(t, "promptshell", newBin))

	execPath := filepath.Join(t.TempDir(), "promptshell")
	if err := os.WriteFile(execPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceBinary(archive, execPath); err != nil {
		t.Fatalf("replaceBinary() error: %v", err)
	}
	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBin) {
		t.Errorf("binary content = %q, want the new binary", got)
	}
	fi, err := os.Stat(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("binary mode = %v, want 0755", fi.Mode().Perm())
	}
}

func TestReplaceBinaryMissingEntry(t *testing.T) {
	archive := writeTemp(t, "rel.tar.gz", makeArchive(t, "README.md", []byte("docs")))
	execPath := filepath.Join(t.TempDir(), "promptshell")
	if err := replaceBinary(archive, execPath); err == nil {
		t.Error("replaceBinary() succeeded without a promptshell entry, want error")
	}
}

func TestReplaceBinaryUnwritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; directory permissions are not enforced")
	}
	archive := writeTemp(t, "rel.tar.gz", makeArchive(t, "promptshell", []byte("new")))

	dir := t.TempDir()
	execPath := filepath.Join(dir, "promptshell")
	if err := os.WriteFile(execPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := replaceBinary(archive, execPath)
	if err == nil {
		t.Fatal("replaceBinary() succeeded in an unwritable dir, want error")
	}
	got, readErr := os.ReadFile(execPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "old" {
		t.Error("target binary was modified despite the failure")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/update/ -v`
Expected: FAIL to compile with `undefined: verifyChecksum` / `undefined: replaceBinary`

- [ ] **Step 3: Write the implementation**

Create `internal/update/install.go`:

```go
package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// verifyChecksum fetches the release checksums.txt and requires a matching
// sha256 for assetName. Unlike install.sh's best-effort check this is
// mandatory: we are about to replace the running binary.
func verifyChecksum(env Env, checksumsURL, archivePath, assetName string) error {
	resp, err := env.Client.Get(checksumsURL)
	if err != nil {
		return fmt.Errorf("fetching checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching checksums: %s returned %s", checksumsURL, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("fetching checksums: %w", err)
	}

	var want string
	for _, line := range strings.Split(string(body), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum for %s in checksums.txt; aborting update", assetName)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s; aborting update", assetName)
	}
	return nil
}

// replaceBinary extracts the promptshell binary from the tar.gz archive and
// atomically renames it over execPath. The temp file is created in execPath's
// directory so the rename never crosses filesystems.
func replaceBinary(archivePath, execPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to extract %s: %w", filepath.Base(archivePath), err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("release archive did not contain a promptshell binary")
		}
		if err != nil {
			return fmt.Errorf("failed to extract %s: %w", filepath.Base(archivePath), err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == "promptshell" {
			return installFile(tr, execPath)
		}
	}
}

// installFile writes src to a temp file beside execPath, then renames it into
// place. Failures suggest the two recovery paths instead of auto-escalating.
func installFile(src io.Reader, execPath string) error {
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, ".promptshell-update-*")
	if err != nil {
		return fmt.Errorf("%s is not writable (%v); try `sudo promptshell --update` or reinstall with install.sh", dir, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op once the rename succeeds

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), execPath); err != nil {
		return fmt.Errorf("could not replace %s (%v); try `sudo promptshell --update` or reinstall with install.sh", execPath, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/update/ -v`
Expected: all tests PASS (the unwritable-dir test may SKIP when run as root — that's fine)

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./...
git add internal/update/install.go internal/update/install_test.go
git commit -m "feat: verify checksums and atomically replace the binary

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 5: `Run` — the full update flow

**Files:**
- Modify: `internal/update/update.go` (add `Run`)
- Test: `internal/update/run_test.go` (create)

**Interfaces:**
- Consumes: `Env`, `LatestVersion`, `normalize` (Task 2); `download` (Task 3); `verifyChecksum`, `replaceBinary` (Task 4); test helpers `fakeLatest`/`testEnv` (Task 2) and `makeArchive`/`sha256hex` (Task 4).
- Produces: `func Run(env Env, currentVersion string) error` — nil on both "updated" and "already up to date"; error on dev builds, lookup/download/verify/replace failures. Task 6 calls this from main.

- [ ] **Step 1: Write the failing test**

Create `internal/update/run_test.go`:

```go
package update

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRelease serves a complete fake GitHub release: the /releases/latest
// redirect, the platform archive, and checksums.txt.
func fakeRelease(t *testing.T, tag string, binary []byte) *httptest.Server {
	t.Helper()
	asset := fmt.Sprintf("promptshell_%s_linux_amd64.tar.gz", strings.TrimPrefix(tag, "v"))
	archive := makeArchive(t, "promptshell", binary)
	sums := fmt.Sprintf("%s  %s\n", sha256hex(archive), asset)

	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("/releases/tag/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/releases/download/"+tag+"/"+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/releases/download/"+tag+"/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runEnv builds an Env pointing at srv with a real temp "binary" to replace.
func runEnv(t *testing.T, srv *httptest.Server) (Env, string) {
	t.Helper()
	execPath := filepath.Join(t.TempDir(), "promptshell")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := testEnv(srv)
	env.ExecPath = func() (string, error) { return execPath, nil }
	return env, execPath
}

func TestRunUpToDate(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", []byte("new-binary"))
	env, execPath := runEnv(t, srv)
	var out bytes.Buffer
	env.Out = &out

	if err := Run(env, "0.5.0"); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !strings.Contains(out.String(), "no updates available") {
		t.Errorf("output = %q, want a 'no updates available' message", out.String())
	}
	got, _ := os.ReadFile(execPath)
	if string(got) != "old-binary" {
		t.Error("binary was replaced even though it was up to date")
	}
}

func TestRunNewerVersionInstalls(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", []byte("new-binary"))
	env, execPath := runEnv(t, srv)
	var out bytes.Buffer
	env.Out = &out

	if err := Run(env, "0.4.0"); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-binary" {
		t.Errorf("binary content = %q, want %q", got, "new-binary")
	}
	if !strings.Contains(out.String(), "updated promptshell v0.4.0 → v0.5.0") {
		t.Errorf("output = %q, want an 'updated ... →' message", out.String())
	}
}

func TestRunRefusesDevBuild(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", []byte("new-binary"))
	env, execPath := runEnv(t, srv)

	err := Run(env, "dev")
	if err == nil {
		t.Fatal("Run() accepted a dev build, want refusal")
	}
	if !strings.Contains(err.Error(), "development build") {
		t.Errorf("error = %q, want it to mention a development build", err)
	}
	got, _ := os.ReadFile(execPath)
	if string(got) != "old-binary" {
		t.Error("dev binary was replaced")
	}
}

func TestRunChecksumMismatchAborts(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", []byte("new-binary"))
	env, execPath := runEnv(t, srv)

	// Point at a second server whose checksums.txt lies.
	asset := "promptshell_0.5.0_linux_amd64.tar.gz"
	archive := makeArchive(t, "promptshell", []byte("new-binary"))
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v0.5.0", http.StatusFound)
	})
	mux.HandleFunc("/releases/tag/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/releases/download/v0.5.0/"+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/releases/download/v0.5.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", sha256hex([]byte("tampered")), asset)
	})
	lying := httptest.NewServer(mux)
	t.Cleanup(lying.Close)
	env.RepoURL = lying.URL
	env.Client = lying.Client()

	err := Run(env, "0.4.0")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Run() = %v, want a checksum mismatch error", err)
	}
	got, _ := os.ReadFile(execPath)
	if string(got) != "old-binary" {
		t.Error("binary was replaced despite the checksum mismatch")
	}
}

func TestRunLookupFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	env, _ := runEnv(t, srv)

	if err := Run(env, "0.4.0"); err == nil {
		t.Error("Run() succeeded with no releases, want error")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/update/ -v`
Expected: FAIL to compile with `undefined: Run`

- [ ] **Step 3: Write the implementation**

Append to `internal/update/update.go`:

```go
// Run checks for a newer release and installs it over the current binary.
// It returns nil both when an update was installed and when none was needed.
func Run(env Env, currentVersion string) error {
	cur := normalize(currentVersion)
	if !semver.IsValid(cur) {
		return fmt.Errorf("this is a development build (version %q); update from source or reinstall with install.sh", currentVersion)
	}

	latest, err := LatestVersion(env)
	if err != nil {
		return fmt.Errorf("could not check for updates: %w", err)
	}

	if semver.Compare(cur, latest) >= 0 {
		_, _ = fmt.Fprintf(env.Out, "promptshell %s is up to date — no updates available\n", strings.TrimPrefix(cur, "v"))
		return nil
	}

	execPath, err := env.ExecPath()
	if err != nil {
		return fmt.Errorf("locating the current binary: %w", err)
	}

	asset := fmt.Sprintf("promptshell_%s_%s_%s.tar.gz", strings.TrimPrefix(latest, "v"), env.GOOS, env.GOARCH)
	base := env.RepoURL + "/releases/download/" + latest

	tmpDir, err := os.MkdirTemp("", "promptshell-update-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archive := filepath.Join(tmpDir, asset)
	if err := download(env, base+"/"+asset, archive, latest); err != nil {
		return err
	}
	if err := verifyChecksum(env, base+"/checksums.txt", archive, asset); err != nil {
		return err
	}
	_, _ = fmt.Fprintln(env.Out, "Checksum verified.")

	if err := replaceBinary(archive, execPath); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(env.Out, "updated promptshell %s → %s\n", cur, latest)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/update/ -v`
Expected: all tests PASS

- [ ] **Step 5: Run the full suite and commit**

```bash
go test ./... && gofmt -l . && go vet ./...
git add internal/update/
git commit -m "feat: self-update flow for promptshell --update

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 6: wire `--update` into the CLI, usage text, and README

**Files:**
- Modify: `cmd/promptshell/main.go` (flag block; dispatch after the version check; `printUsage`)
- Modify: `README.md` (Flags table, line 92-99)
- Test: `cmd/promptshell/main_test.go` (extend)

**Interfaces:**
- Consumes: `update.Run(update.DefaultEnv(), resolveVersion())` (Tasks 2/5), `resolveVersion()` (Task 1).

- [ ] **Step 1: Write the failing test**

Add to `cmd/promptshell/main_test.go`:

```go
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
```

Add `"strings"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/promptshell/ -run TestUpdateFlag -v`
Expected: FAIL — `flag provided but not defined: -update`

- [ ] **Step 3: Implement**

In `cmd/promptshell/main.go`:

Add the import `"github.com/oluwatayo/promptshell/internal/update"`.

In the flag block, after the `showVersion` lines, add:

```go
	var doUpdate bool
	fs.BoolVar(&doUpdate, "update", false, "check for a newer release and install it")
```

After the `if showVersion { ... }` block, add:

```go
	if doUpdate {
		return update.Run(update.DefaultEnv(), resolveVersion())
	}
```

In `printUsage`, add these lines to the usage list (after the `config model` line):

```
  promptshell --version | -v              print the version
  promptshell --update                    update promptshell to the latest release
```

and this to the flags block (after the `--version, -v` line from Task 1):

```
  --update       check for a newer release and install it
```

In `README.md`, add two rows to the Flags table after the `--verbose` row (line 99):

```markdown
| `--version`, `-v` | print the promptshell version and exit |
| `--update` | check for a newer release and install it |
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./cmd/promptshell/ -v`
Expected: all tests PASS

- [ ] **Step 5: Verify by hand**

```bash
go build -o /tmp/ps-dev ./cmd/promptshell && /tmp/ps-dev --update; echo "exit: $?"
```

Expected: `error: this is a development build (version "dev"); update from source or reinstall with install.sh` and `exit: 1`

```bash
/tmp/ps-dev -v
```

Expected: `promptshell dev`

- [ ] **Step 6: Commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add cmd/promptshell/main.go cmd/promptshell/main_test.go README.md
git commit -m "feat: add --update flag to self-update from GitHub releases

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 7: install.sh download progress bar

**Files:**
- Modify: `install.sh` (download section, lines 61-66)

**Interfaces:**
- Consumes: nothing from other tasks (shell-only change).

- [ ] **Step 1: Make the change**

In `install.sh`, replace:

```sh
info "Downloading $asset ($version)..."
curl -fsSL "$base/$asset" -o "$tmp/$asset" || die "download failed: $base/$asset"
```

with:

```sh
# Show a progress bar when stderr is a terminal; stay silent when piped (CI).
if [ -t 2 ]; then
  curl_progress="--progress-bar"
else
  curl_progress="-s"
fi

info "Downloading $asset ($version)..."
# shellcheck disable=SC2086 # $curl_progress is a single flag, not user data
curl -fSL $curl_progress "$base/$asset" -o "$tmp/$asset" || die "download failed: $base/$asset"
```

(The checksums.txt fetch on line 69 keeps its silent `-fsSL` — it's a few hundred bytes.)

- [ ] **Step 2: Verify against a local mock release**

```bash
cd /private/tmp/claude-501/-Users-oluwatayo-Development-go-promptshell/c4dc03ab-500a-4d96-ba1d-40b2e88873f8/scratchpad
rm -rf mockrel && mkdir -p mockrel/v9.9.9 bin
printf '#!/bin/sh\necho promptshell v9.9.9\n' > promptshell && chmod +x promptshell
tar -czf mockrel/v9.9.9/promptshell_9.9.9_$(uname -s | tr '[:upper:]' '[:lower:]')_arm64.tar.gz promptshell
(cd mockrel/v9.9.9 && shasum -a 256 *.tar.gz > checksums.txt)
python3 -m http.server 8931 --directory mockrel &
sleep 1
PROMPTSHELL_VERSION=v9.9.9 \
  PROMPTSHELL_BASE_URL=http://localhost:8931 \
  PROMPTSHELL_INSTALL_DIR=$PWD/bin \
  sh /Users/oluwatayo/Development/go/promptshell/install.sh
./bin/promptshell
kill %1
```

Expected: installer output ends with `Installed promptshell v9.9.9 to .../bin/promptshell`, and `./bin/promptshell` prints `promptshell v9.9.9`. (Progress bar won't render here — stderr is piped in this session; that's the correct silent behavior. The TTY progress bar gets eyeballed by the user after merge.)

Also confirm the script is still POSIX-clean:

```bash
sh -n /Users/oluwatayo/Development/go/promptshell/install.sh && echo OK
```

Expected: `OK`

- [ ] **Step 3: Commit**

```bash
git add install.sh
git commit -m "feat: show a download progress bar in install.sh on a TTY

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 8: full verification and PR

**Files:**
- No new files; final checks only.

- [ ] **Step 1: Run everything CI runs**

```bash
gofmt -l .            # expect: no output
go build ./...        # expect: clean
go vet ./...          # expect: clean
go test -race ./...   # expect: all packages ok
golangci-lint run     # expect: 0 issues (brew golangci-lint v2.x, same as CI)
```

- [ ] **Step 2: Push and open the PR**

```bash
git push -u origin feat/version-update-flags
gh pr create --title "feat: version/update flags and installer download progress" --body "$(cat <<'EOF'
## Summary
- `-v` shorthand for `--version`, plus a `debug.ReadBuildInfo()` fallback so `go install` builds report a real version instead of `dev`
- `--update`: self-update from GitHub releases — resolves the latest tag via the releases/latest redirect (no API rate limit), downloads the platform tarball with a TTY progress display, **mandatorily** verifies the sha256 against checksums.txt, then atomically replaces the running binary; prints "no updates available" when already newest; refuses on dev builds; suggests sudo/reinstall when the install dir isn't writable
- install.sh now shows curl's progress bar during the asset download when stderr is a TTY (still silent when piped/CI)

Spec: docs/superpowers/specs/2026-07-12-version-update-flags-design.md

## Testing
- `internal/update` unit tests against httptest fake release servers: up-to-date, install happy path, checksum mismatch/missing, unwritable dir, dev-build refusal, garbage-tag guard
- install.sh re-verified against a local mock release (download → checksum → install → run)
- NOT tested live: `--update` against a real GitHub release needs a manual smoke test once v0.5.0 exists (same caveat install.sh had)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Report**

Tell the user the PR is open, remind them the live `--update` smoke test can only happen after this merges and v0.5.0 is released (update from a v0.4.0 install), and that the TTY progress bar in install.sh should be eyeballed once merged to main.
