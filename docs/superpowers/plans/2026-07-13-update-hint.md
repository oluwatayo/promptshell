# Update Hint Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Print a one-line stderr hint when a newer promptshell release exists — after task runs and at REPL start — with at most one network check per day.

**Architecture:** All logic in a new `internal/update/hint.go` (same package as the update flow; reuses `Env`, `LatestVersion`, `normalize`, `isTTY`). A JSON cache file rations the network check. `cmd/promptshell/main.go` calls `update.Hint` at the two wire-up points. No changes to runner/repl packages.

**Tech Stack:** Go 1.24, stdlib + existing `golang.org/x/mod/semver` dep. No new dependencies.

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-13-update-hint-design.md`
- Hint text exactly: `A new version of promptshell is available: vCUR → vNEW — run "promptshell --update" to upgrade.`
- Suppressed (silently, and without network I/O) when: `PROMPTSHELL_NO_UPDATE_CHECK` set; stderr not a TTY; current version not bare `vX.Y.Z` semver (dev/pseudo-version builds — same gate as `Run`); already up to date; latest undeterminable
- Cache `~/.promptshell/cache/update-check.json` (`{"checkedAt":RFC3339,"latest":"vX.Y.Z"}`); fresh (≤24h) → zero network; live check uses a 2s-timeout client; check failure leaves the cache untouched; cache write failures are silent
- Hint never returns an error and never delays the confirm/preview flow (it runs after the task / before the REPL banner)
- Conventional Commits with trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`; feature commit type `feat:`
- Before each commit: `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass
- Branch: `feat/update-hint` (already created off main)

---

### Task 1: `internal/update/hint.go` — the hint logic with tests

**Files:**
- Create: `internal/update/hint.go`
- Test: `internal/update/hint_test.go` (create)

**Interfaces:**
- Consumes: `Env` (fields RepoURL/Client/Out/IsTTY), `LatestVersion(env)`, `normalize(v)`, and test helper `testEnv(srv)` — all existing in this package.
- Produces: `type HintOptions struct { CachePath string; MaxAge time.Duration; Now func() time.Time; Disabled bool }`, `func DefaultHintOptions() HintOptions`, `func Hint(env Env, opt HintOptions, currentVersion string)`. Task 2 calls `update.Hint(update.DefaultEnv(), update.DefaultHintOptions(), resolveVersion())`.

- [ ] **Step 1: Write the failing tests**

Create `internal/update/hint_test.go`:

```go
package update

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// countingLatest is fakeLatest plus a hit counter on the redirect endpoint.
func countingLatest(t *testing.T, tag string) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("/releases/tag/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &hits
}

func hintEnv(srv *httptest.Server, out *bytes.Buffer) Env {
	env := testEnv(srv)
	env.Out = out
	env.IsTTY = true
	return env
}

func hintOpts(t *testing.T) HintOptions {
	t.Helper()
	return HintOptions{
		CachePath: filepath.Join(t.TempDir(), "update-check.json"),
		MaxAge:    24 * time.Hour,
		Now:       time.Now,
	}
}

func writeCache(t *testing.T, path string, c hintCache) {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestHintNewerVersion(t *testing.T) {
	srv, _ := countingLatest(t, "v9.9.9")
	var out bytes.Buffer
	Hint(hintEnv(srv, &out), hintOpts(t), "0.6.2")
	got := out.String()
	if !strings.Contains(got, "v0.6.2") || !strings.Contains(got, "v9.9.9") ||
		!strings.Contains(got, `"promptshell --update"`) {
		t.Errorf("hint = %q, want both versions and the --update command", got)
	}
}

func TestHintUpToDate(t *testing.T) {
	srv, _ := countingLatest(t, "v0.6.2")
	var out bytes.Buffer
	Hint(hintEnv(srv, &out), hintOpts(t), "0.6.2")
	if out.Len() != 0 {
		t.Errorf("hint printed %q for an up-to-date binary, want silence", out.String())
	}
}

func TestHintFreshCacheSkipsNetwork(t *testing.T) {
	srv, hits := countingLatest(t, "v9.9.9")
	opt := hintOpts(t)
	writeCache(t, opt.CachePath, hintCache{CheckedAt: time.Now(), Latest: "v8.8.8"})

	var out bytes.Buffer
	Hint(hintEnv(srv, &out), opt, "0.6.2")
	if n := hits.Load(); n != 0 {
		t.Errorf("fresh cache still made %d network request(s)", n)
	}
	if !strings.Contains(out.String(), "v8.8.8") {
		t.Errorf("hint = %q, want the cached v8.8.8", out.String())
	}
}

func TestHintStaleCacheChecksOnceAndRewrites(t *testing.T) {
	srv, hits := countingLatest(t, "v9.9.9")
	opt := hintOpts(t)
	writeCache(t, opt.CachePath, hintCache{
		CheckedAt: time.Now().Add(-48 * time.Hour),
		Latest:    "v8.8.8",
	})

	var out bytes.Buffer
	Hint(hintEnv(srv, &out), opt, "0.6.2")
	if n := hits.Load(); n != 1 {
		t.Errorf("stale cache made %d request(s), want exactly 1", n)
	}
	if !strings.Contains(out.String(), "v9.9.9") {
		t.Errorf("hint = %q, want the freshly checked v9.9.9", out.String())
	}

	b, err := os.ReadFile(opt.CachePath)
	if err != nil {
		t.Fatal(err)
	}
	var c hintCache
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.Latest != "v9.9.9" {
		t.Errorf("cache latest = %q, want v9.9.9", c.Latest)
	}
	if time.Since(c.CheckedAt) > time.Minute {
		t.Errorf("cache checkedAt not refreshed: %v", c.CheckedAt)
	}
}

func TestHintCorruptCacheTreatedAsStale(t *testing.T) {
	srv, hits := countingLatest(t, "v9.9.9")
	opt := hintOpts(t)
	if err := os.WriteFile(opt.CachePath, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	Hint(hintEnv(srv, &out), opt, "0.6.2")
	if n := hits.Load(); n != 1 {
		t.Errorf("corrupt cache made %d request(s), want 1 (treated as stale)", n)
	}
	if !strings.Contains(out.String(), "v9.9.9") {
		t.Errorf("hint = %q, want v9.9.9 after re-check", out.String())
	}
}

func TestHintNetworkFailureSilent(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close() // connection refused from here on
	opt := hintOpts(t)

	var out bytes.Buffer
	env := testEnv(srv)
	env.Out = &out
	env.IsTTY = true
	Hint(env, opt, "0.6.2")
	if out.Len() != 0 {
		t.Errorf("hint printed %q on network failure, want silence", out.String())
	}
	if _, err := os.Stat(opt.CachePath); !os.IsNotExist(err) {
		t.Error("cache file written despite a failed check")
	}
}

func TestHintSuppressed(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(env *Env, opt *HintOptions)
		version string
	}{
		{"disabled", func(_ *Env, o *HintOptions) { o.Disabled = true }, "0.6.2"},
		{"non-tty", func(e *Env, _ *HintOptions) { e.IsTTY = false }, "0.6.2"},
		{"dev build", func(_ *Env, _ *HintOptions) {}, "dev"},
		{"pseudo-version", func(_ *Env, _ *HintOptions) {}, "v0.6.3-0.20260713120000-abcdef123456+dirty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, hits := countingLatest(t, "v9.9.9")
			opt := hintOpts(t)
			var out bytes.Buffer
			env := hintEnv(srv, &out)
			tt.mutate(&env, &opt)

			Hint(env, opt, tt.version)
			if out.Len() != 0 {
				t.Errorf("hint printed %q, want silence", out.String())
			}
			if n := hits.Load(); n != 0 {
				t.Errorf("suppressed case still made %d network request(s)", n)
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/update/ -run TestHint -v`
Expected: FAIL to compile with `undefined: HintOptions` / `undefined: Hint` / `undefined: hintCache`

- [ ] **Step 3: Write the implementation**

Create `internal/update/hint.go`:

```go
package update

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/mod/semver"
)

// HintOptions carries the update-hint cadence knobs so tests can control
// time and disk.
type HintOptions struct {
	CachePath string           // where the last-check record lives
	MaxAge    time.Duration    // how long a cached check stays fresh
	Now       func() time.Time // injectable clock
	Disabled  bool             // PROMPTSHELL_NO_UPDATE_CHECK opt-out
}

// DefaultHintOptions wires HintOptions to production values. The cache is a
// sibling of the config dir so config backup tooling can ignore it.
func DefaultHintOptions() HintOptions {
	opt := HintOptions{
		MaxAge:   24 * time.Hour,
		Now:      time.Now,
		Disabled: os.Getenv("PROMPTSHELL_NO_UPDATE_CHECK") != "",
	}
	if home, err := os.UserHomeDir(); err == nil {
		opt.CachePath = filepath.Join(home, ".promptshell", "cache", "update-check.json")
	}
	return opt
}

// hintCache is the on-disk record of the last successful version check.
type hintCache struct {
	CheckedAt time.Time `json:"checkedAt"`
	Latest    string    `json:"latest"`
}

// Hint prints a one-line upgrade hint to env.Out when a newer release is
// known — from the cache, or from at most one live check per MaxAge. Every
// failure is silent: a hint is never worth an error or a slow startup.
func Hint(env Env, opt HintOptions, currentVersion string) {
	if opt.Disabled || opt.CachePath == "" || !env.IsTTY {
		return
	}
	cur := normalize(currentVersion)
	// Same gate as Run: only bare vX.Y.Z release builds get update handling.
	if !semver.IsValid(cur) || semver.Prerelease(cur) != "" || semver.Build(cur) != "" {
		return
	}

	latest := cachedLatest(opt)
	if latest == "" {
		latest = liveLatest(env, opt)
	}
	if latest == "" || semver.Compare(cur, latest) >= 0 {
		return
	}
	_, _ = fmt.Fprintf(env.Out,
		"A new version of promptshell is available: %s → %s — run %q to upgrade.\n",
		cur, latest, "promptshell --update")
}

// cachedLatest returns the cached latest version, or "" when the cache is
// missing, corrupt, stale, or holds an invalid version.
func cachedLatest(opt HintOptions) string {
	b, err := os.ReadFile(opt.CachePath)
	if err != nil {
		return ""
	}
	var c hintCache
	if err := json.Unmarshal(b, &c); err != nil {
		return ""
	}
	if opt.Now().Sub(c.CheckedAt) > opt.MaxAge {
		return ""
	}
	latest := normalize(c.Latest)
	if !semver.IsValid(latest) {
		return ""
	}
	return latest
}

// liveLatest checks GitHub once with a short-timeout client (Env's download
// client allows minutes; a startup hint must not) and rewrites the cache on
// success. Failures return "" and leave the cache untouched so the next run
// retries.
func liveLatest(env Env, opt HintOptions) string {
	env.Client = &http.Client{Timeout: 2 * time.Second}
	latest, err := LatestVersion(env)
	if err != nil {
		return ""
	}
	if b, err := json.Marshal(hintCache{CheckedAt: opt.Now(), Latest: latest}); err == nil {
		if err := os.MkdirAll(filepath.Dir(opt.CachePath), 0o755); err == nil {
			_ = os.WriteFile(opt.CachePath, b, 0o644)
		}
	}
	return latest
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/update/ -v`
Expected: all tests PASS (hint tests plus the existing update-flow tests)

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add internal/update/hint.go internal/update/hint_test.go
git commit -m "feat: hint when a newer release is available

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: wire the hint into the CLI and document it

**Files:**
- Modify: `cmd/promptshell/main.go` (the `switch` in `run()`)
- Modify: `README.md` (after the flags-examples paragraph; the env-var paragraph near the end of Configuration)

**Interfaces:**
- Consumes: `update.Hint`, `update.DefaultHintOptions`, `update.DefaultEnv` (Task 1), `resolveVersion()` (existing).

- [ ] **Step 1: Wire main.go**

In `cmd/promptshell/main.go`, change the final `switch` in `run()`:

```go
	switch {
	case len(args) == 0:
		// No task given: start the interactive shell.
		update.Hint(update.DefaultEnv(), update.DefaultHintOptions(), resolveVersion())
		return repl.Run(cfg)
	case args[0] == "config":
		return runConfig(cfg, args[1:])
	default:
		if err := runner.Run(context.Background(), cfg, opt, args[0]); err != nil {
			return err
		}
		update.Hint(update.DefaultEnv(), update.DefaultHintOptions(), resolveVersion())
		return nil
	}
```

(`--version`/`--update` returned earlier in `run()`, and `config` deliberately gets no hint — matches the spec's suppression list. The `update` package is already imported.)

- [ ] **Step 2: Verify the wiring compiles and existing tests pass**

Run: `go test ./cmd/promptshell/ ./internal/update/ -v`
Expected: all PASS (the existing `run()` tests use dev builds, whose hint path is a silent no-op with no network I/O)

- [ ] **Step 3: README**

After the flags examples paragraph (the code block ending `promptshell --provider gemini --yes "list the 5 largest files here"`), add:

```markdown
When a newer release is available, promptshell prints a one-line hint after a
run (checked at most once per day; shown only on a terminal). Disable it with
`PROMPTSHELL_NO_UPDATE_CHECK=1`.
```

In the Configuration section's environment-variable paragraph (`API keys can also be supplied via environment variables: ...`), append to the same paragraph:

```markdown
`PROMPTSHELL_NO_UPDATE_CHECK=1` disables the daily new-version check.
```

- [ ] **Step 4: Manual verification with a pseudo-TTY**

stderr in an agent session is a pipe, so use `script(1)` to fake a terminal, and a deliberately old version stamp against live GitHub:

```bash
go build -ldflags "-X main.version=0.1.0" -o /tmp/ps-hint ./cmd/promptshell
rm -f ~/.promptshell/cache/update-check.json
script -q /dev/null /tmp/ps-hint --provider gemini --dry-run "print hello" | tail -3
```

Expected: the dry-run preview followed by `A new version of promptshell is available: v0.1.0 → v0.6.2 — run "promptshell --update" to upgrade.` (v0.6.2 or whatever is latest). Then:

```bash
cat ~/.promptshell/cache/update-check.json   # cache written with checkedAt + latest
PROMPTSHELL_NO_UPDATE_CHECK=1 script -q /dev/null /tmp/ps-hint --provider gemini --dry-run "print hello" | tail -1
```

Expected: cache file present and valid; the opted-out run's last line is the dry-run footer, no hint. Clean up: `rm -f /tmp/ps-hint ~/.promptshell/cache/update-check.json` (leave the user's real cache to regenerate).

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go vet ./... && go test ./...
git add cmd/promptshell/main.go README.md
git commit -m "feat: show the update hint after task runs and at REPL start

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: full verification and PR

**Files:** none new; checks only.

- [ ] **Step 1: CI-parity gate**

```bash
gofmt -l .            # expect: no output
go build ./...        # expect: clean
go vet ./...          # expect: clean
go test -race ./...   # expect: all packages ok
golangci-lint run     # expect: 0 issues
```

- [ ] **Step 2: Push and open the PR**

```bash
git push -u origin feat/update-hint
gh pr create --title "feat: hint when a newer release is available" --body "$(cat <<'EOF'
## Summary
- After a task run (and at REPL start), promptshell prints one stderr line when a newer release exists: `A new version of promptshell is available: v0.6.2 → v0.7.0 — run "promptshell --update" to upgrade.`
- Network-frugal: the check result is cached in `~/.promptshell/cache/update-check.json`, at most one 2s-bounded HTTPS check per day; every failure mode is silent
- Suppressed for non-TTY stderr (scripts/CI), dev/pseudo-version builds, `--version`/`--update`/`config` invocations, and `PROMPTSHELL_NO_UPDATE_CHECK=1` (which also skips the check itself)

Spec: docs/superpowers/specs/2026-07-13-update-hint-design.md

## Testing
- Unit tests with a hit-counting fake GitHub: newer/up-to-date, fresh cache → zero requests, stale cache → exactly one request + cache rewrite, corrupt cache, network failure silent + no cache poisoning, and a suppression table (disabled / non-TTY / dev / pseudo-version) asserting silence AND zero network hits
- Manual live check under `script(1)` (pseudo-TTY): a binary stamped v0.1.0 printed the hint against real GitHub; `PROMPTSHELL_NO_UPDATE_CHECK=1` silenced it; cache file written correctly
- gofmt / vet / `go test -race ./...` / golangci-lint clean

Releases as v0.7.0 together with the already-merged genai SDK migration.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Report**

Tell the user the PR is open; merging it and the Release PR ships v0.7.0 (hint + SDK migration), after which older-release users start seeing upgrade hints.
