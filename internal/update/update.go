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
