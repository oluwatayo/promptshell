package runner

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// fakeEnv is a scriptable ollamaEnv for testing the setup orchestration without
// touching the real system.
type fakeEnv struct {
	goos         string
	installed    bool
	hasBrew      bool
	reachableSeq []bool // successive reachable() results; last value repeats
	reachIdx     int
	installErr   error
	serveErr     error
	pullErr      error
	calls        []string
	out          bytes.Buffer
}

func (f *fakeEnv) env() ollamaEnv {
	notFound := errors.New("not found")
	return ollamaEnv{
		goos: f.goos,
		lookPath: func(name string) (string, error) {
			switch name {
			case "ollama":
				if f.installed {
					return "/usr/bin/ollama", nil
				}
			case "brew":
				if f.hasBrew {
					return "/opt/homebrew/bin/brew", nil
				}
			}
			return "", notFound
		},
		reachable: func(string) bool {
			r := false
			switch {
			case f.reachIdx < len(f.reachableSeq):
				r = f.reachableSeq[f.reachIdx]
			case len(f.reachableSeq) > 0:
				r = f.reachableSeq[len(f.reachableSeq)-1]
			}
			f.reachIdx++
			return r
		},
		run: func(name string, args ...string) error {
			f.calls = append(f.calls, "run:"+name+" "+strings.Join(args, " "))
			if name == "brew" {
				f.installed = true
				return f.installErr
			}
			return f.pullErr
		},
		runShell: func(string) error {
			f.calls = append(f.calls, "runShell")
			f.installed = true
			return f.installErr
		},
		startServe: func() error {
			f.calls = append(f.calls, "startServe")
			return f.serveErr
		},
		sleep: func() {},
		out:   &f.out,
	}
}

func called(calls []string, substr string) bool {
	for _, c := range calls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

func TestSetupOllamaLinuxFullPath(t *testing.T) {
	f := &fakeEnv{goos: "linux", installed: false, reachableSeq: []bool{false, true}}
	if !setupOllama(f.env(), "llama3", "http://x") {
		t.Fatalf("want success; output:\n%s", f.out.String())
	}
	if !called(f.calls, "runShell") {
		t.Error("expected the Linux installer to run")
	}
	if !called(f.calls, "startServe") {
		t.Error("expected the server to be started")
	}
	if !called(f.calls, "run:ollama pull llama3") {
		t.Error("expected the model to be pulled")
	}
}

func TestSetupOllamaAlreadyInstalledAndRunning(t *testing.T) {
	f := &fakeEnv{goos: "linux", installed: true, reachableSeq: []bool{true}}
	if !setupOllama(f.env(), "llama3", "http://x") {
		t.Fatal("want success")
	}
	if called(f.calls, "runShell") {
		t.Error("should not re-install when already installed")
	}
	if called(f.calls, "startServe") {
		t.Error("should not start the server when already reachable")
	}
	if !called(f.calls, "run:ollama pull llama3") {
		t.Error("expected the model to be pulled")
	}
}

func TestSetupOllamaServeNeverComesUp(t *testing.T) {
	f := &fakeEnv{goos: "linux", installed: true, reachableSeq: []bool{false}}
	if setupOllama(f.env(), "llama3", "http://x") {
		t.Fatal("want failure when the server never becomes reachable")
	}
	if called(f.calls, "run:ollama pull") {
		t.Error("should not pull a model when the server never came up")
	}
}

func TestSetupOllamaMacWithoutBrew(t *testing.T) {
	f := &fakeEnv{goos: "darwin", installed: false, hasBrew: false}
	if setupOllama(f.env(), "llama3", "http://x") {
		t.Fatal("want failure when Homebrew is unavailable on macOS")
	}
	if !strings.Contains(f.out.String(), "ollama.com/download") {
		t.Errorf("expected a download pointer; got:\n%s", f.out.String())
	}
}

func TestSetupOllamaMacWithBrew(t *testing.T) {
	f := &fakeEnv{goos: "darwin", installed: false, hasBrew: true, reachableSeq: []bool{false, true}}
	if !setupOllama(f.env(), "llama3", "http://x") {
		t.Fatalf("want success; output:\n%s", f.out.String())
	}
	if !called(f.calls, "run:brew install ollama") {
		t.Error("expected brew install to run")
	}
}

func TestSetupOllamaInstallFails(t *testing.T) {
	f := &fakeEnv{goos: "linux", installed: false, installErr: errors.New("boom")}
	if setupOllama(f.env(), "llama3", "http://x") {
		t.Fatal("want failure when the installer fails")
	}
	if called(f.calls, "startServe") {
		t.Error("should not start the server after a failed install")
	}
}
