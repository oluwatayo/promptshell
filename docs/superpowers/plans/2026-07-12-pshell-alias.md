# pshell Alias Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** install.sh creates a `pshell` symlink alias for the installed binary, and the README stops saying `./promptshell` for installed usage.

**Architecture:** Shell + docs only — no Go changes. The symlink lives beside the binary with a relative target (`pshell -> promptshell`), so `--update` (which resolves symlinks before replacing) and reinstalls keep working. A non-symlink occupying the `pshell` name is never clobbered.

**Tech Stack:** POSIX sh (install.sh), Markdown (README.md).

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-12-pshell-alias-design.md`
- Alias name is exactly `pshell`; created ONLY by install.sh (no second release binary, no Go changes)
- install.sh must remain POSIX sh (`sh -n install.sh` clean); no clobbering of a non-symlink `$dir/pshell` (warn + continue, install still succeeds)
- Conventional Commits with trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- Branch: `feat/pshell-alias` (already created, stacked on `feat/version-update-flags` / PR #21)
- Scratchpad for mock testing: `/private/tmp/claude-501/-Users-oluwatayo-Development-go-promptshell/c4dc03ab-500a-4d96-ba1d-40b2e88873f8/scratchpad`

---

### Task 1: install.sh — create the `pshell` symlink

**Files:**
- Modify: `install.sh` (install section, after `chmod +x "$dir/$BIN"`, currently ~line 117; the `info "Installed ..."` line is replaced)

**Interfaces:**
- Consumes: existing variables `$dir`, `$BIN`, `$version`, and helper `info` from install.sh.
- Produces: `$dir/pshell` relative symlink; final install message gains ` (alias: pshell)` when the link was created.

- [ ] **Step 1: Make the change**

In `install.sh`, replace:

```sh
chmod +x "$dir/$BIN"

info "Installed $BIN $version to $dir/$BIN"
```

with:

```sh
chmod +x "$dir/$BIN"

# --- create the pshell alias (a relative symlink; never clobber a real file) ---
alias_name=pshell
alias_note=""
run_in_dir=""
[ -w "$dir" ] || run_in_dir="sudo"
if [ ! -e "$dir/$alias_name" ] || [ -h "$dir/$alias_name" ]; then
  # shellcheck disable=SC2086 # $run_in_dir is empty or the single word "sudo"
  $run_in_dir rm -f "$dir/$alias_name"
  # shellcheck disable=SC2086
  if $run_in_dir ln -s "$BIN" "$dir/$alias_name"; then
    alias_note=" (alias: $alias_name)"
  fi
else
  info "Note: $dir/$alias_name already exists and is not a symlink; skipping the $alias_name alias."
fi

info "Installed $BIN $version to $dir/$BIN$alias_note"
```

Notes on why this shape: `[ -h ]` is the POSIX symlink test (covers broken
symlinks too, which `[ -e ]` misses); `rm -f` + `ln -s` instead of `ln -sfn`
because `-n` is not POSIX; the `if $run_in_dir ln -s ...` form keeps a failed
`ln` from killing the whole install under `set -e`; `sudo` only when the
earlier `mv` branch needed it (same `-w` test).

- [ ] **Step 2: Verify against a local mock release (happy, idempotent, and skip cases)**

```bash
cd /private/tmp/claude-501/-Users-oluwatayo-Development-go-promptshell/c4dc03ab-500a-4d96-ba1d-40b2e88873f8/scratchpad
rm -rf mockrel2 bin2 && mkdir -p mockrel2/v9.9.9 bin2
printf '#!/bin/sh\necho promptshell v9.9.9\n' > promptshell && chmod +x promptshell
tar -czf mockrel2/v9.9.9/promptshell_9.9.9_$(uname -s | tr '[:upper:]' '[:lower:]')_arm64.tar.gz promptshell
(cd mockrel2/v9.9.9 && shasum -a 256 *.tar.gz > checksums.txt)
python3 -m http.server 8932 --directory mockrel2 &
sleep 1
run_install() {
  PROMPTSHELL_VERSION=v9.9.9 \
    PROMPTSHELL_BASE_URL=http://localhost:8932 \
    PROMPTSHELL_INSTALL_DIR=$PWD/bin2 \
    sh /Users/oluwatayo/Development/go/promptshell/install.sh
}
run_install                    # 1) happy path
[ -h bin2/pshell ] && echo "SYMLINK: $(readlink bin2/pshell)"   # expect: SYMLINK: promptshell
./bin2/pshell                  # expect: promptshell v9.9.9
run_install                    # 2) idempotent re-install (symlink recreated, no error)
[ -h bin2/pshell ] && echo "STILL SYMLINK"
rm bin2/pshell && echo "real file" > bin2/pshell
run_install                    # 3) skip case: expect the "skipping the pshell alias" note
cat bin2/pshell                # expect: real file   (untouched)
kill %1
```

Expected: run 1 ends `Installed promptshell v9.9.9 to .../bin2/promptshell (alias: pshell)` and `./bin2/pshell` prints `promptshell v9.9.9`; run 2 succeeds identically; run 3 prints the skip note, ends WITHOUT `(alias: pshell)`, and `bin2/pshell` still contains `real file`.

Also: `sh -n /Users/oluwatayo/Development/go/promptshell/install.sh && echo OK` → `OK`.

- [ ] **Step 3: Commit**

```bash
git add install.sh
git commit -m "feat: create a pshell alias symlink in install.sh

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: README — alias docs and installed-binary command polish

**Files:**
- Modify: `README.md` (Installation section ~lines 40–69; Usage ~74–116; Configuration ~146–149)

**Interfaces:**
- Consumes: nothing from Task 1 (text-only), but describes its behavior — wording below matches Task 1's actual output.

- [ ] **Step 1: Make the edits**

All line numbers are as of branch `feat/pshell-alias`; anchor on the quoted text, not the numbers.

a) In the install-script paragraph (after "...and installs it (to `/usr/local/bin`, or `~/.local/bin` otherwise)."), append to the same paragraph:

```markdown
It also creates a shorter `pshell` alias (a symlink beside the binary) — `pshell` and `promptshell` are interchangeable in every command below.
```

b) After the **With Go:** code block (`go install github.com/...@latest`), add:

```markdown
`go install` doesn't create the `pshell` alias; add it yourself with
`ln -s promptshell "$(go env GOPATH)/bin/pshell"` (or use a shell alias).
```

c) After the **From source:** code block (`go build -o promptshell ./cmd/promptshell`), add:

```markdown
This leaves the binary in the current directory — run it as `./promptshell`,
or move it onto your `PATH` (e.g. `mv promptshell ~/.local/bin/`) to invoke
it as `promptshell` like the examples below.
```

d) Replace `./promptshell` with `promptshell` in ALL usage/config examples — the Usage synopsis code block, the compress-logs example, the two flags examples (`--dry-run`, `--provider gemini --yes`), the interactive-shell example (`$ ./promptshell`), and the four `config` examples. The ONLY `./promptshell` that remains in the file is the one added in (c). Verify with:

```bash
grep -n '\./promptshell' README.md
```

Expected: exactly one hit, inside the from-source note.

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: document the pshell alias and drop ./ from installed examples

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 3: verification and PR

**Files:** none new; final checks only.

- [ ] **Step 1: Full gate**

```bash
sh -n install.sh && echo SH_OK     # expect SH_OK
gofmt -l .                          # expect: no output (nothing Go changed)
go test ./...                       # expect: all ok (unchanged, sanity)
grep -c '\./promptshell' README.md  # expect: 1
```

- [ ] **Step 2: Push and open the PR (stacked on #21)**

```bash
git push -u origin feat/pshell-alias
gh pr create --base feat/version-update-flags --title "feat: pshell alias symlink and README command polish" --body "$(cat <<'EOF'
## Summary
- install.sh now creates a `pshell` symlink alias beside the binary (relative target, sudo only if the install needed it, never clobbers a non-symlink occupying the name — warns and skips instead)
- `pshell --update` already works: the updater resolves symlinks before replacing the binary
- README: examples now say `promptshell` instead of `./promptshell` (the installer puts it on PATH); documented the alias, a `go install` alias one-liner, and a PATH note for from-source builds

Spec: docs/superpowers/specs/2026-07-12-pshell-alias-design.md

**Stacked on #21** (both touch install.sh) — merge #21 first; this PR retargets to `main` automatically when #21's branch is deleted on merge.

## Testing
- Mock-release install verified: symlink created + resolves + runs, idempotent re-install, and skip case (pre-existing real file named `pshell` left untouched with a warning)
- `sh -n install.sh` clean; no Go changes (`go test ./...` still green)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Report**

Tell the user the PR is open and stacked on #21 (merge order: #21 → this), and that the alias should be eyeballed after the next real release install.
