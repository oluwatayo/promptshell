package update

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// verifyChecksum fetches the release checksums.txt and requires a matching
// sha256 for assetName. Unlike install.sh's best-effort check this is
// mandatory: we are about to replace the running binary.
func verifyChecksum(env Env, checksumsURL, archivePath, assetName string) error {
	resp, err := env.Client.Get(checksumsURL)
	if err != nil {
		return fmt.Errorf("fetching checksums: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching checksums: %s returned %s", checksumsURL, resp.Status)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("fetching checksums: %w", err)
	}

	var want string
	for _, line := range strings.Split(string(body), "\n") {
		if fields := strings.Fields(line); len(fields) == 2 && fields[1] == assetName {
			want = fields[0]
			break
		}
	}
	if want == "" {
		return fmt.Errorf("no checksum for %s in checksums.txt; aborting update", assetName)
	}

	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != want {
		return fmt.Errorf("checksum mismatch for %s; aborting update", assetName)
	}
	return nil
}

// replaceBinary extracts the promptshell binary from the tar.gz archive and
// atomically renames it over execPath. The temp file is created in execPath's
// directory so the rename never crosses filesystems.
func replaceBinary(archivePath, execPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to extract %s: %w", filepath.Base(archivePath), err)
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("release archive did not contain a promptshell binary")
		}
		if err != nil {
			return fmt.Errorf("failed to extract %s: %w", filepath.Base(archivePath), err)
		}
		if hdr.Typeflag == tar.TypeReg && filepath.Base(hdr.Name) == "promptshell" {
			return installFile(tr, execPath)
		}
	}
}

// installFile writes src to a temp file beside execPath, then renames it into
// place. Failures suggest the two recovery paths instead of auto-escalating.
func installFile(src io.Reader, execPath string) error {
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, ".promptshell-update-*")
	if err != nil {
		return fmt.Errorf("%s is not writable (%v); try `sudo promptshell --update` or reinstall with install.sh", dir, err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }() // no-op once the rename succeeds

	if _, err := io.Copy(tmp, src); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), execPath); err != nil {
		return fmt.Errorf("could not replace %s (%v); try `sudo promptshell --update` or reinstall with install.sh", execPath, err)
	}
	return nil
}
