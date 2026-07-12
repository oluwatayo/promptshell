# Design: `pshell` alias + README command polish

Date: 2026-07-12
Status: approved (name and approach agreed in session)

## Goal

Give users a shorter command — `pshell` — for the `promptshell` binary, and
clean up the README so examples reflect an installed-on-PATH binary.

## Decisions

- Alias name: **`pshell`** (chosen over `pshl`/`psh`: guessable, no common
  conflicts, tab-completes from `psh<tab>`).
- Mechanism: **symlink created by install.sh**, not a second shipped binary
  (release archives stay single-binary) and not Go code changes (the binary
  ignores argv[0]; `--update` already resolves symlinks via
  `filepath.EvalSymlinks(os.Executable())`, so `pshell --update` correctly
  replaces the real `promptshell` file).

## 1. install.sh: create the symlink

After the existing `chmod +x "$dir/$BIN"` / before the "Installed" message:

- If `$dir/pshell` does not exist or is already a symlink: (re)create it as a
  **relative** symlink — `rm -f "$dir/pshell" && ln -s promptshell
  "$dir/pshell"` (`ln -n` is not POSIX, so remove-then-link; the relative
  target keeps the pair relocatable). Use sudo for the `rm`/`ln` only when
  the earlier install step needed sudo (same writability rule as the `mv`).
- If `$dir/pshell` exists and is NOT a symlink (a real file/dir owned by
  something else): do not touch it; print a warning
  (`Note: $dir/pshell already exists; skipping the pshell alias.`) and
  continue — the install itself still succeeds.
- Success message gains the alias: `Installed $BIN $version to $dir/$BIN
  (alias: pshell)` — the alias suffix only when the link was created.

## 2. README updates

- **Usage/Configuration examples:** replace every `./promptshell` invocation
  with `promptshell` (lines 74, 81, 106, 107, 116, 146–149 as of this
  writing) — the install script puts the binary on PATH, so `./` reads wrong.
- **Alias:** in the Installation section, note that the install script also
  creates a `pshell` alias, and everything works identically under either
  name. For `go install` / from-source users, show the one-liner:
  `ln -s "$(command -v promptshell)" <dir-on-PATH>/pshell` (or a shell
  alias).
- **From-source PATH note:** after the `go build` block, add that the built
  binary lands in the current directory — run it as `./promptshell` or move
  it somewhere on `PATH` (e.g. `mv promptshell ~/.local/bin/`). This is the
  one place `./promptshell` remains legitimate.

## Non-goals

- No second binary in release archives; no GoReleaser changes.
- No Go code changes and no new flags.
- No uninstaller work.

## Testing

- Extend the existing local mock-release verification for install.sh: after
  install, assert `$INSTALL_DIR/pshell` is a symlink pointing to
  `promptshell` and `$INSTALL_DIR/pshell` runs (prints the fake binary's
  output). Also cover the skip case: pre-create a regular file named
  `pshell` in the install dir, re-run, assert it is untouched and the
  warning appears.
- `sh -n install.sh` stays clean (POSIX).
- README changes are prose — reviewed by eye.

## Delivery

Branch `feat/pshell-alias`, stacked on `feat/version-update-flags` (PR #21)
because both touch install.sh's install section. PR base: `main` after #21
merges (GitHub retargets automatically if #21's branch is deleted on merge;
otherwise noted in the PR body to merge #21 first). Commit type `feat:` —
lands in v0.5.0 alongside the update flags.
