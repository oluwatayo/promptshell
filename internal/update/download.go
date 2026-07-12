package update

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

// download streams url into dest. On a TTY with a known Content-Length it
// renders a single-line percentage bar on env.Out; otherwise it prints one
// "Downloading..." line so logs stay clean.
func download(env Env, url, dest, tag string) error {
	resp, err := env.Client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: %s returned %s", url, resp.Status)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}

	label := "Downloading promptshell " + tag
	var src io.Reader = resp.Body
	showBar := env.IsTTY && resp.ContentLength > 0
	if showBar {
		src = io.TeeReader(resp.Body, &progress{out: env.Out, label: label, total: resp.ContentLength})
	} else {
		_, _ = fmt.Fprintf(env.Out, "%s (%s)...\n", label, formatBytes(resp.ContentLength))
	}

	_, copyErr := io.Copy(f, src)
	if showBar {
		_, _ = fmt.Fprintln(env.Out) // terminate the \r progress line
	}
	if err := f.Close(); err != nil {
		return err
	}
	if copyErr != nil {
		return fmt.Errorf("download failed: %w", copyErr)
	}
	return nil
}

// progress is an io.Writer that renders download progress as bytes flow
// through it via io.TeeReader.
type progress struct {
	out     io.Writer
	label   string
	total   int64
	written int64
	lastPct int64
}

func (p *progress) Write(b []byte) (int, error) {
	p.written += int64(len(b))
	if pct := p.written * 100 / p.total; pct != p.lastPct {
		p.lastPct = pct
		_, _ = fmt.Fprintf(p.out, "\r%s... %d%% (%s / %s)", p.label, pct, formatBytes(p.written), formatBytes(p.total))
	}
	return len(b), nil
}

// formatBytes renders a byte count for humans; n < 0 means unknown.
func formatBytes(n int64) string {
	const mb = 1024 * 1024
	switch {
	case n < 0:
		return "size unknown"
	case n >= mb:
		return fmt.Sprintf("%.1f MB", float64(n)/mb)
	default:
		return fmt.Sprintf("%.0f KB", float64(n)/1024)
	}
}
