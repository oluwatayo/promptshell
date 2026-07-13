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
