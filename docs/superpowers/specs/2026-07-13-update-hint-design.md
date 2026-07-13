# Design: startup hint when a newer version is available

Date: 2026-07-13
Status: approved (feature requested by user; daily cadence chosen via Q&A)

## Goal

When a newer promptshell release exists, tell the user — passively, one line
on stderr — so people on old (possibly buggy) releases find out without
watching the repo. Complements `--update` (which does the installing) and
addresses the release-deletion lesson: users of deleted releases had no
signal to upgrade.

## Behavior

- Message (one line, stderr):
  `A new version of promptshell is available: v0.6.2 → v0.7.0 — run "promptshell --update" to upgrade.`
- Shown at the END of a one-shot task run (after the script runs or is
  declined — never interleaved with the preview/confirm flow), and at the
  START of a REPL session (before the banner).
- The hint appears on every eligible run while an update is pending — only
  the network check is rationed, not the message (same as `gh`).

### When the hint is suppressed (all silent)

- Not a release build: current version isn't bare `vX.Y.Z` semver (dev
  builds, VCS pseudo-versions — same rule as `--update`'s refusal).
- stderr is not a TTY (scripts, pipes, CI stay clean).
- `PROMPTSHELL_NO_UPDATE_CHECK` is set (any non-empty value) — the opt-out
  also skips the network check entirely.
- No newer version (current >= latest), or the latest couldn't be
  determined (network down, GitHub slow → quiet failure, never an error).
- The invocation is `--version`/`-v`, `--update`, `--help`, or a `config`
  subcommand — hint only wraps LLM task runs and REPL sessions.

## Check cadence: at most one network call per day

- Cache file `~/.promptshell/cache/update-check.json`:
  `{"checkedAt":"<RFC3339>","latest":"vX.Y.Z"}` (0644; dir 0755).
- Fresh cache (checkedAt within 24h): use `latest` from the file, zero
  network.
- Stale/missing/corrupt cache: call the existing
  `update.LatestVersion(env)` with a dedicated `http.Client{Timeout: 2s}`
  (bounded worst case, and it runs at end-of-task / REPL-start where ~150ms
  is imperceptible), then rewrite the cache best-effort (write failures are
  silent — worst case we check again next run).
- Check failure: cache is NOT updated (retry next run), no hint, no error.

## Implementation shape

New file `internal/update/hint.go` (same package as the update flow — it
reuses `Env`, `LatestVersion`, `normalize`, `isTTY`):

```go
// HintOptions carries the cadence knobs so tests can control time and disk.
type HintOptions struct {
    CachePath string                 // e.g. ~/.promptshell/cache/update-check.json
    MaxAge    time.Duration          // 24h in production
    Now       func() time.Time       // time.Now in production
    Disabled  bool                   // PROMPTSHELL_NO_UPDATE_CHECK
}

func DefaultHintOptions() HintOptions

// Hint prints the one-line upgrade hint to env.Out when a newer release is
// known (from cache or one rationed live check). All failures are silent.
func Hint(env Env, opt HintOptions, currentVersion string)
```

- `Hint` gates in order: `opt.Disabled` → version-shape check → `env.IsTTY`
  → cached-or-live latest → semver compare → print.
- Wire-up in `cmd/promptshell/main.go` only:
  - task path: call after `runner.Run` returns successfully (and also when
    the user declined — i.e. on any nil-error return).
  - REPL path: call before `repl.Run(cfg)`.
  - `update.DefaultEnv()` reused, with `Client` swapped for the 2s-timeout
    client inside `Hint`'s live-check path (Env's 5-minute download client
    must not bound a startup check).

Config-dir note: config lives at `~/.promptshell/config/config.json`; the
cache goes in a sibling `~/.promptshell/cache/` directory so config backup/
sync tooling can ignore it.

## Testing

- `internal/update/hint_test.go` (httptest + temp cache dir + fake clock):
  - newer version → hint printed with both versions and the `--update` command
  - up to date → silent
  - fresh cache → no network request made (assert zero server hits) and
    cached latest used
  - stale cache → exactly one request, cache rewritten with new checkedAt
  - corrupt cache file → treated as stale, no error
  - network failure → silent, cache left stale (next run retries)
  - Disabled / non-TTY / dev-or-pseudo version → silent AND no network hit
- Manual: build with an old `-ldflags -X main.version`, run a task against
  live GitHub, see the hint; run with `PROMPTSHELL_NO_UPDATE_CHECK=1`, see
  nothing.

## Non-goals

- No auto-install, no prompting to update (hint only).
- No mid-REPL-session re-checks (session start only).
- No hint-frequency backoff or "don't show again" persistence — the message
  repeats until they update (matches gh; it's one stderr line).
- No config-file knob (env var only) — can be added later if asked.

## Delivery

Branch `feat/update-hint` off main. Single PR, commit type `feat:` →
release-please cuts v0.7.0 carrying this plus the merged genai SDK
refactor.
