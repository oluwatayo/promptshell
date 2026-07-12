package runner

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

// ollamaEnv abstracts the OS interactions the Ollama setup needs, so the
// orchestration can be unit-tested without installing or starting anything.
type ollamaEnv struct {
	goos       string
	lookPath   func(string) (string, error)
	reachable  func(baseURL string) bool
	run        func(name string, args ...string) error // interactive, wired to the terminal
	runShell   func(script string) error               // sh -c script, wired to the terminal
	startServe func() error                            // start `ollama serve` detached
	sleep      func()
	out        io.Writer
}

// defaultOllamaEnv wires ollamaEnv to the real OS.
func defaultOllamaEnv() ollamaEnv {
	return ollamaEnv{
		goos:       runtime.GOOS,
		lookPath:   exec.LookPath,
		reachable:  ollamaReachable,
		run:        runInteractive,
		runShell:   runShellInteractive,
		startServe: startOllamaServe,
		sleep:      func() { time.Sleep(time.Second) },
		out:        os.Stdout,
	}
}

// printf and println narrate progress to the environment's writer. Write errors
// on a progress stream are not actionable, so they are ignored.
func (e ollamaEnv) printf(format string, a ...any) { _, _ = fmt.Fprintf(e.out, format, a...) }
func (e ollamaEnv) println(a ...any)               { _, _ = fmt.Fprintln(e.out, a...) }

// setupOllama installs Ollama (if missing), starts it (if not running), and
// pulls the model. It returns true if Ollama ends up reachable so the caller
// can retry the request. Each step is narrated to env.out.
func setupOllama(env ollamaEnv, model, baseURL string) bool {
	if _, err := env.lookPath("ollama"); err != nil {
		if !installOllama(env) {
			return false
		}
	} else {
		env.println("Ollama is already installed.")
	}

	if !env.reachable(baseURL) {
		env.println("Starting Ollama...")
		if err := env.startServe(); err != nil {
			env.printf("Could not start Ollama: %v\n", err)
			env.println("Start it manually with: ollama serve")
			return false
		}
		if !waitReachable(env, baseURL, 30) {
			env.println("Ollama didn't come up in time. Start it with `ollama serve`, then re-run.")
			return false
		}
	}

	env.printf("Pulling model %q (this can take a few minutes)...\n", model)
	if err := env.run("ollama", "pull", model); err != nil {
		env.printf("Failed to pull %s: %v\n", model, err)
		return false
	}
	return env.reachable(baseURL)
}

// installOllama runs the platform-appropriate installer. It shows the exact
// command before running it.
func installOllama(env ollamaEnv) bool {
	switch env.goos {
	case "linux":
		env.println("Installing Ollama (running: curl -fsSL https://ollama.com/install.sh | sh)...")
		if err := env.runShell("curl -fsSL https://ollama.com/install.sh | sh"); err != nil {
			env.printf("Install failed: %v\n", err)
			return false
		}
		return true
	case "darwin":
		if _, err := env.lookPath("brew"); err == nil {
			env.println("Installing Ollama via Homebrew (running: brew install ollama)...")
			if err := env.run("brew", "install", "ollama"); err != nil {
				env.printf("Install failed: %v\n", err)
				return false
			}
			return true
		}
		env.println("Automatic install on macOS needs Homebrew.")
		env.println("Download Ollama from https://ollama.com/download, then re-run your command.")
		return false
	default:
		env.println("Automatic install isn't supported on this OS. See https://ollama.com/download")
		return false
	}
}

func waitReachable(env ollamaEnv, baseURL string, tries int) bool {
	for i := 0; i < tries; i++ {
		if env.reachable(baseURL) {
			return true
		}
		env.sleep()
	}
	return false
}

// ollamaReachable reports whether the Ollama server answers at baseURL.
func ollamaReachable(baseURL string) bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(baseURL)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// runInteractive runs a command wired to the current terminal (so sudo prompts
// and progress bars work).
func runInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func runShellInteractive(script string) error {
	cmd := exec.Command("sh", "-c", script)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// startOllamaServe launches `ollama serve` detached from promptshell so it
// keeps running after promptshell exits. Its output goes to /dev/null.
func startOllamaServe() error {
	cmd := exec.Command("ollama", "serve")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd.Start()
}
