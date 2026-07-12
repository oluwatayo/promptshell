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
