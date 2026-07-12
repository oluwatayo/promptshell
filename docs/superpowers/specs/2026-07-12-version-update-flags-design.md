# Design: version & update flags + installer download progress

Date: 2026-07-12
Status: approved

## Goal

Give installed users a way to see what version they're running and to update
in place, and make `install.sh` show download progress instead of downloading
silently.

- `-v` / `--version` — print the current version and exit.
- `--update` — check GitHub releases for a newer version; if one exists,
  download and install it over the running binary; otherwise print that no
  updates are available.
- `install.sh` — show a progress bar while the release tarball downloads.

## Non-goals

- No `-u` short form for `--update` (keep the short-flag namespace clean).
- No auto-sudo or privilege escalation inside `--update`.
- No background/automatic update checks — updates only run when asked.
- No Windows support (build matrix is darwin/linux only).

## 1. Flags & UX (`cmd/promptshell/main.go`)

- Bind a new `v` flag to the same bool as the existing `version` flag, so
  `-v`, `-version`, and `--version` are all equivalent (Go's `flag` package
  treats single and double dash identically).
- Add a `--update` bool flag. When set, run the updater and exit before any
  config/provider work, same as `--version`.
- Version fallback: builds installed via `go install` don't get the
  `-ldflags -X main.version` override, so they currently print `dev`. When
  `version == "dev"`, fall back to `debug.ReadBuildInfo()` main-module
  version (e.g. `v0.4.0`); keep `dev` only if that's also unset/`(devel)`.
- Update `printUsage` and the README flags section with both flags.

Output examples:

```
$ promptshell -v
promptshell 0.4.0

$ promptshell --update        # already newest
promptshell 0.4.0 is up to date — no updates available

$ promptshell --update        # newer release exists
Downloading promptshell v0.5.0... 100% (4.2 MB)
Checksum verified.
updated promptshell v0.4.0 → v0.5.0
```

## 2. Self-update (`internal/update` package)

Flow, in order:

1. **Resolve latest version** the same way `install.sh` does: follow the
   `https://github.com/oluwatayo/promptshell/releases/latest` redirect and
   read the tag from the final URL. No GitHub API, so no rate limit and no
   token. Guard against a non-`vX.Y.Z`-shaped result.
2. **Compare versions** with `golang.org/x/mod/semver` (small official
   module). Normalise both sides to a leading `v`.
   - Current ≥ latest → print "up to date — no updates available", exit 0.
   - Current version unknown (not valid semver after the build-info
     fallback, i.e. a from-source dev build) → refuse with a clear message
     ("development build; update from source or reinstall via install.sh"),
     exit non-zero. Never clobber a dev build.
3. **Download** `promptshell_<ver-no-v>_<GOOS>_<GOARCH>.tar.gz` from
   `https://github.com/<repo>/releases/download/<tag>/`. Stream to a temp
   file, showing progress on stderr (percent + downloaded/total bytes from
   `Content-Length`) only when stderr is a TTY; print a single
   "Downloading..." line otherwise.
4. **Verify checksum** against `checksums.txt` from the same release.
   Mandatory: missing checksums file, missing entry, or mismatch all abort
   the update. (Stricter than install.sh's best-effort check because we are
   replacing the running binary.)
5. **Replace the binary**: resolve `os.Executable()` through symlinks,
   extract the `promptshell` binary from the tarball into a temp file in the
   same directory (same filesystem → atomic rename), chmod 0755, then
   `os.Rename` over the target. Renaming over a running binary is safe on
   Unix.
   - If the target directory isn't writable, fail cleanly and suggest
     `sudo promptshell --update` or re-running install.sh. No auto-sudo.
6. **Report**: `updated promptshell vOLD → vNEW`.

Testability: the package takes an injectable base URL (for `httptest` fake
release servers) and target executable path (temp dir), mirroring the
injectable-environment pattern used by `internal/runner/ollama_setup.go`.

Dependency added: `golang.org/x/mod` (semver only).

## 3. install.sh download progress

- Asset download: use curl's progress bar (`--progress-bar`) when stderr is
  a TTY (`[ -t 2 ]`), keep the current silent `-s` when not (CI logs, piped
  runs stay clean). Keep `-f -S -L` in both cases.
- The `checksums.txt` fetch stays silent — it's a few hundred bytes.

## 4. Error handling

- All network/lookup failures print a friendly one-line error (matching the
  tone of the existing first-run UX), never a stack trace or raw dump.
- `--update` exit codes: 0 for "updated" and "already up to date"; non-zero
  for refusals and failures.

## 5. Testing

Unit tests (`internal/update`, `httptest` + temp dirs; no live network):

- already up to date → "no updates available", no download attempted
- newer version → binary downloaded, checksum verified, file replaced,
  old content gone, mode 0755
- checksum mismatch / missing checksum entry → abort, target untouched
- unwritable target directory → clean error with suggestion
- dev/unknown version → refusal message
- version-resolution redirect parsing (including garbage tag guard)
- `cmd/promptshell`: `-v` and `--version` print the same thing;
  build-info fallback covered where feasible

install.sh: re-verify against the local mock-release harness used for
PR #13; progress bar behaviour checked manually (TTY vs piped).

Manual post-merge smoke test (like install.sh had): run `--update` on a
machine with an older release installed, against live GitHub.

## 6. Delivery

Single PR from `feat/version-update-flags` off `main` (independent of the
merged Ollama auto-setup work). Conventional commit type `feat:` →
release-please will cut v0.5.0 (current latest release: v0.4.0).
