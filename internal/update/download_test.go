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

func serveBytes(t *testing.T, path string, body []byte) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
		_, _ = w.Write(body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestDownloadWritesFile(t *testing.T) {
	body := bytes.Repeat([]byte("promptshell!"), 1000)
	srv := serveBytes(t, "/asset.tar.gz", body)

	dest := filepath.Join(t.TempDir(), "asset.tar.gz")
	env := testEnv(srv)
	if err := download(env, srv.URL+"/asset.tar.gz", dest, "v0.5.0"); err != nil {
		t.Fatalf("download() error: %v", err)
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, body) {
		t.Errorf("downloaded %d bytes, want %d identical bytes", len(got), len(body))
	}
}

func TestDownloadShowsProgressOnTTY(t *testing.T) {
	body := bytes.Repeat([]byte("x"), 10_000)
	srv := serveBytes(t, "/asset.tar.gz", body)

	var out bytes.Buffer
	env := testEnv(srv)
	env.Out = &out
	env.IsTTY = true

	dest := filepath.Join(t.TempDir(), "asset.tar.gz")
	if err := download(env, srv.URL+"/asset.tar.gz", dest, "v0.5.0"); err != nil {
		t.Fatalf("download() error: %v", err)
	}
	if !strings.Contains(out.String(), "100%") {
		t.Errorf("progress output missing 100%%, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "v0.5.0") {
		t.Errorf("progress output missing version label, got: %q", out.String())
	}
}

func TestDownloadNonTTYPrintsSingleLine(t *testing.T) {
	srv := serveBytes(t, "/asset.tar.gz", []byte("data"))

	var out bytes.Buffer
	env := testEnv(srv)
	env.Out = &out

	dest := filepath.Join(t.TempDir(), "asset.tar.gz")
	if err := download(env, srv.URL+"/asset.tar.gz", dest, "v0.5.0"); err != nil {
		t.Fatalf("download() error: %v", err)
	}
	if strings.Contains(out.String(), "\r") {
		t.Errorf("non-TTY output must not use carriage returns, got: %q", out.String())
	}
	if !strings.Contains(out.String(), "Downloading") {
		t.Errorf("expected a Downloading line, got: %q", out.String())
	}
}

func TestDownload404(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(srv.Close)

	dest := filepath.Join(t.TempDir(), "asset.tar.gz")
	if err := download(testEnv(srv), srv.URL+"/nope", dest, "v0.5.0"); err == nil {
		t.Error("download() succeeded on 404, want error")
	}
}

func TestFormatBytes(t *testing.T) {
	for n, want := range map[int64]string{
		-1:        "size unknown",
		1024:      "1 KB",
		4_400_000: "4.2 MB",
	} {
		if got := formatBytes(n); got != want {
			t.Errorf("formatBytes(%d) = %q, want %q", n, got, want)
		}
	}
}
