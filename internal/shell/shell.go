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

// Extract pulls a shell script out of an LLM response. Chatty models wrap
// the script in prose, usage notes, and extra example blocks, so if the
// response contains a Markdown code fence anywhere, the contents of the
// first fenced block are returned (running the surrounding prose as shell
// is what we must never do); a response with no fence is returned trimmed.
func Extract(raw string) string {
	s := strings.TrimSpace(raw)
	lines := strings.Split(s, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			start = i
			break
		}
	}
	if start == -1 {
		return s
	}
	var body []string
	for _, line := range lines[start+1:] {
		if strings.TrimSpace(line) == "```" {
			break
		}
		body = append(body, line)
	}
	return strings.TrimSpace(strings.Join(body, "\n"))
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
