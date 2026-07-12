#!/bin/sh
# promptshell installer.
#
#   curl -fsSL https://raw.githubusercontent.com/oluwatayo/promptshell/main/install.sh | sh
#
# Downloads the latest release binary for your OS/arch, verifies its checksum,
# and installs it on your PATH.
#
# Environment overrides:
#   PROMPTSHELL_VERSION       install a specific tag (e.g. v0.1.0) instead of latest
#   PROMPTSHELL_INSTALL_DIR   install directory (default: /usr/local/bin, else ~/.local/bin)
#   PROMPTSHELL_BASE_URL      release download base (default: GitHub; for mirrors)

set -eu

REPO="oluwatayo/promptshell"
BIN="promptshell"
BASE_URL="${PROMPTSHELL_BASE_URL:-https://github.com/$REPO/releases/download}"

info() { printf '%s\n' "$*"; }
err() { printf 'promptshell: %s\n' "$*" >&2; }
die() { err "$*"; exit 1; }

command -v curl >/dev/null 2>&1 || die "curl is required"
command -v tar >/dev/null 2>&1 || die "tar is required"

# --- detect OS ---
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux | darwin) ;;
  *) die "unsupported OS '$os' (promptshell supports Linux and macOS only)" ;;
esac

# --- detect architecture ---
arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "unsupported architecture '$arch'" ;;
esac

# --- resolve version (latest unless pinned) ---
version="${PROMPTSHELL_VERSION:-}"
if [ -z "$version" ]; then
  # Follow the /releases/latest redirect to /releases/tag/<version> — no API
  # token needed and not subject to the GitHub API rate limit.
  version=$(curl -fsSLI -o /dev/null -w '%{url_effective}' \
    "https://github.com/$REPO/releases/latest" | sed -E 's#.*/tag/##')
fi

# Guard against a failed lookup producing a garbage URL.
case "$version" in
  v[0-9]* | [0-9]*) ;;
  *) die "could not determine the latest version (got '$version'); set PROMPTSHELL_VERSION (e.g. v0.1.0)" ;;
esac

ver_no_v="${version#v}"
asset="${BIN}_${ver_no_v}_${os}_${arch}.tar.gz"
base="$BASE_URL/$version"

# --- download into a temp dir that is always cleaned up ---
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

# Show a progress bar when stderr is a terminal; stay silent when piped (CI).
if [ -t 2 ]; then
  curl_progress="--progress-bar"
else
  curl_progress="-s"
fi

info "Downloading $asset ($version)..."
# shellcheck disable=SC2086 # $curl_progress is a single flag, not user data
curl -fSL $curl_progress "$base/$asset" -o "$tmp/$asset" || die "download failed: $base/$asset"

# --- verify checksum (best effort: only if checksums.txt and a tool exist) ---
if curl -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt" 2>/dev/null; then
  expected=$(awk -v a="$asset" '$2 == a {print $1}' "$tmp/checksums.txt")
  if [ -n "$expected" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
      actual=$(sha256sum "$tmp/$asset" | awk '{print $1}')
    elif command -v shasum >/dev/null 2>&1; then
      actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
    else
      actual=""
    fi
    if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
      die "checksum mismatch for $asset"
    fi
    [ -n "$actual" ] && info "Checksum verified."
  fi
fi

tar -xzf "$tmp/$asset" -C "$tmp" || die "failed to extract $asset"
[ -f "$tmp/$BIN" ] || die "archive did not contain a '$BIN' binary"

# --- choose install dir ---
dir="${PROMPTSHELL_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    dir=/usr/local/bin
  else
    dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$dir"

# --- install (sudo only if the target dir needs it) ---
if [ -w "$dir" ]; then
  mv "$tmp/$BIN" "$dir/$BIN"
elif command -v sudo >/dev/null 2>&1; then
  info "Installing to $dir (requires sudo)..."
  sudo mv "$tmp/$BIN" "$dir/$BIN"
else
  die "$dir is not writable and sudo is unavailable; set PROMPTSHELL_INSTALL_DIR"
fi
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
case ":$PATH:" in
  *":$dir:"*) ;;
  *) info "Note: $dir is not on your PATH. Add it, e.g.:"
     info "  export PATH=\"$dir:\$PATH\"" ;;
esac
