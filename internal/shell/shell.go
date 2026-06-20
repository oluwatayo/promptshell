// Package shell extracts shell scripts from LLM responses and runs them.
package shell

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// DefaultShell is used to run generated scripts when none is configured.
const DefaultShell = "bash"

// Extract pulls a shell script out of an LLM response. If the response is
// wrapped in a single Markdown code fence (optionally tagged with a language
// such as `sh` or `bash`), the surrounding fence is removed; otherwise the
// trimmed text is returned unchanged.
func Extract(raw string) string {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	lines = lines[1:] // drop the opening ```lang line
	if n := len(lines); n > 0 && strings.TrimSpace(lines[n-1]) == "```" {
		lines = lines[:n-1] // drop the closing fence
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// Write writes the script to path with owner-executable permissions, ensuring
// it ends with a newline.
func Write(path, script string) error {
	if !strings.HasSuffix(script, "\n") {
		script += "\n"
	}
	return os.WriteFile(path, []byte(script), 0o755)
}

// Execute runs the script file with the given shell and returns its combined
// stdout and stderr.
func Execute(ctx context.Context, shell, path string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, shell, path)
	return cmd.CombinedOutput()
}
