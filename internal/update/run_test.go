package update

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeRelease serves a complete fake GitHub release: the /releases/latest
// redirect, the platform archive, and checksums.txt.
func fakeRelease(t *testing.T, tag string, binary []byte) *httptest.Server {
	t.Helper()
	asset := fmt.Sprintf("promptshell_%s_linux_amd64.tar.gz", strings.TrimPrefix(tag, "v"))
	archive := makeArchive(t, "promptshell", binary)
	sums := fmt.Sprintf("%s  %s\n", sha256hex(archive), asset)

	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/"+tag, http.StatusFound)
	})
	mux.HandleFunc("/releases/tag/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/releases/download/"+tag+"/"+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/releases/download/"+tag+"/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sums))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// runEnv builds an Env pointing at srv with a real temp "binary" to replace.
func runEnv(t *testing.T, srv *httptest.Server) (Env, string) {
	t.Helper()
	execPath := filepath.Join(t.TempDir(), "promptshell")
	if err := os.WriteFile(execPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := testEnv(srv)
	env.ExecPath = func() (string, error) { return execPath, nil }
	return env, execPath
}

func TestRunUpToDate(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", []byte("new-binary"))
	env, execPath := runEnv(t, srv)
	var out bytes.Buffer
	env.Out = &out

	if err := Run(env, "0.5.0"); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !strings.Contains(out.String(), "no updates available") {
		t.Errorf("output = %q, want a 'no updates available' message", out.String())
	}
	got, _ := os.ReadFile(execPath)
	if string(got) != "old-binary" {
		t.Error("binary was replaced even though it was up to date")
	}
}

func TestRunNewerVersionInstalls(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", []byte("new-binary"))
	env, execPath := runEnv(t, srv)
	var out bytes.Buffer
	env.Out = &out

	if err := Run(env, "0.4.0"); err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-binary" {
		t.Errorf("binary content = %q, want %q", got, "new-binary")
	}
	if !strings.Contains(out.String(), "updated promptshell v0.4.0 → v0.5.0") {
		t.Errorf("output = %q, want an 'updated ... →' message", out.String())
	}
}

func TestRunRefusesDevBuild(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", []byte("new-binary"))
	env, execPath := runEnv(t, srv)

	err := Run(env, "dev")
	if err == nil {
		t.Fatal("Run() accepted a dev build, want refusal")
	}
	if !strings.Contains(err.Error(), "development build") {
		t.Errorf("error = %q, want it to mention a development build", err)
	}
	got, _ := os.ReadFile(execPath)
	if string(got) != "old-binary" {
		t.Error("dev binary was replaced")
	}
}

func TestRunChecksumMismatchAborts(t *testing.T) {
	srv := fakeRelease(t, "v0.5.0", []byte("new-binary"))
	env, execPath := runEnv(t, srv)

	// Point at a second server whose checksums.txt lies.
	asset := "promptshell_0.5.0_linux_amd64.tar.gz"
	archive := makeArchive(t, "promptshell", []byte("new-binary"))
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/releases/tag/v0.5.0", http.StatusFound)
	})
	mux.HandleFunc("/releases/tag/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/releases/download/v0.5.0/"+asset, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	})
	mux.HandleFunc("/releases/download/v0.5.0/checksums.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", sha256hex([]byte("tampered")), asset)
	})
	lying := httptest.NewServer(mux)
	t.Cleanup(lying.Close)
	env.RepoURL = lying.URL
	env.Client = lying.Client()

	err := Run(env, "0.4.0")
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("Run() = %v, want a checksum mismatch error", err)
	}
	got, _ := os.ReadFile(execPath)
	if string(got) != "old-binary" {
		t.Error("binary was replaced despite the checksum mismatch")
	}
}

func TestRunLookupFailure(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)
	env, _ := runEnv(t, srv)

	if err := Run(env, "0.4.0"); err == nil {
		t.Error("Run() succeeded with no releases, want error")
	}
}
