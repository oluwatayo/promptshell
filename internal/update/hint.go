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
