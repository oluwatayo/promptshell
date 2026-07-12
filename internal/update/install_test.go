package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// makeArchive builds a tar.gz holding a single file entry, mimicking a
// GoReleaser release archive.
func makeArchive(t *testing.T, binaryName string, content []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: binaryName, Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func writeTemp(t *testing.T, name string, b []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestVerifyChecksumOK(t *testing.T) {
	asset := []byte("archive-bytes")
	sums := fmt.Sprintf("%s  my.tar.gz\n", sha256hex(asset))
	srv := serveBytes(t, "/checksums.txt", []byte(sums))

	path := writeTemp(t, "my.tar.gz", asset)
	if err := verifyChecksum(testEnv(srv), srv.URL+"/checksums.txt", path, "my.tar.gz"); err != nil {
		t.Errorf("verifyChecksum() error: %v", err)
	}
}

func TestVerifyChecksumMismatch(t *testing.T) {
	sums := fmt.Sprintf("%s  my.tar.gz\n", sha256hex([]byte("different")))
	srv := serveBytes(t, "/checksums.txt", []byte(sums))

	path := writeTemp(t, "my.tar.gz", []byte("archive-bytes"))
	if err := verifyChecksum(testEnv(srv), srv.URL+"/checksums.txt", path, "my.tar.gz"); err == nil {
		t.Error("verifyChecksum() passed on mismatch, want error")
	}
}

func TestVerifyChecksumMissingEntry(t *testing.T) {
	srv := serveBytes(t, "/checksums.txt", []byte("abc123  other.tar.gz\n"))
	path := writeTemp(t, "my.tar.gz", []byte("archive-bytes"))
	if err := verifyChecksum(testEnv(srv), srv.URL+"/checksums.txt", path, "my.tar.gz"); err == nil {
		t.Error("verifyChecksum() passed with no entry for the asset, want error")
	}
}

func TestVerifyChecksumMissingFile(t *testing.T) {
	srv := serveBytes(t, "/other", nil) // 404 for /checksums.txt
	path := writeTemp(t, "my.tar.gz", []byte("archive-bytes"))
	if err := verifyChecksum(testEnv(srv), srv.URL+"/checksums.txt", path, "my.tar.gz"); err == nil {
		t.Error("verifyChecksum() passed with no checksums.txt, want error")
	}
}

func TestReplaceBinary(t *testing.T) {
	newBin := []byte("#!/bin/sh\necho new\n")
	archive := writeTemp(t, "rel.tar.gz", makeArchive(t, "promptshell", newBin))

	execPath := filepath.Join(t.TempDir(), "promptshell")
	if err := os.WriteFile(execPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceBinary(archive, execPath); err != nil {
		t.Fatalf("replaceBinary() error: %v", err)
	}
	got, err := os.ReadFile(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, newBin) {
		t.Errorf("binary content = %q, want the new binary", got)
	}
	fi, err := os.Stat(execPath)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o755 {
		t.Errorf("binary mode = %v, want 0755", fi.Mode().Perm())
	}
}

func TestReplaceBinaryMissingEntry(t *testing.T) {
	archive := writeTemp(t, "rel.tar.gz", makeArchive(t, "README.md", []byte("docs")))
	execPath := filepath.Join(t.TempDir(), "promptshell")
	if err := replaceBinary(archive, execPath); err == nil {
		t.Error("replaceBinary() succeeded without a promptshell entry, want error")
	}
}

func TestReplaceBinaryUnwritableDir(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("running as root; directory permissions are not enforced")
	}
	archive := writeTemp(t, "rel.tar.gz", makeArchive(t, "promptshell", []byte("new")))

	dir := t.TempDir()
	execPath := filepath.Join(dir, "promptshell")
	if err := os.WriteFile(execPath, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	err := replaceBinary(archive, execPath)
	if err == nil {
		t.Fatal("replaceBinary() succeeded in an unwritable dir, want error")
	}
	got, readErr := os.ReadFile(execPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(got) != "old" {
		t.Error("target binary was modified despite the failure")
	}
}
