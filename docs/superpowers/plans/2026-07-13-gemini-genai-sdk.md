# Gemini genai SDK Migration Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the deprecated `github.com/google/generative-ai-go` SDK with `google.golang.org/genai` in the gemini provider, add the provider's first unit tests, and shed the old dependency tree.

**Architecture:** `internal/llm/gemini` is the only consumer of the old SDK, so this is a one-package rewrite behind the unchanged `llm.Provider` interface. New capabilities used: `HTTPOptions.BaseURL` (honors `cfg.BaseURL`, enables httptest-backed tests) and native `SystemInstruction`.

**Tech Stack:** Go 1.24, `google.golang.org/genai` v1.63.0 (verified: its go.mod requires go 1.24; `ClientConfig` has `APIKey`, `Backend`, `HTTPOptions HTTPOptions` fields; `Models.GenerateContent(ctx, model string, contents []*Content, config *GenerateContentConfig)`; helpers `genai.Text(string) []*Content`, `genai.NewContentFromText(text, role) *Content`, `(*GenerateContentResponse).Text() string`).

## Global Constraints

- Spec: `docs/superpowers/specs/2026-07-13-gemini-genai-sdk-design.md`
- No user-visible behavior change: provider name `gemini`, default model `gemini-flash-latest`, API key required, `req.Model` override wins over configured model
- `go` directive in go.mod must remain `1.24.x` after the dependency swap
- After `go mod tidy`: `github.com/google/generative-ai-go` and `google.golang.org/api` must be GONE from go.mod
- Conventional Commits with trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`; commit type `refactor:` (no release cut — ships with the next feat)
- Before each commit: `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass
- Branch: `refactor/gemini-genai-sdk` (already created off main)

---

### Task 1: rewrite the gemini provider on google.golang.org/genai, with tests

**Files:**
- Modify: `internal/llm/gemini/gemini.go` (full rewrite of the SDK-facing parts)
- Create: `internal/llm/gemini/gemini_test.go`
- Modify: `go.mod` / `go.sum` (via `go get` + `go mod tidy`)

**Interfaces:**
- Consumes: `llm.Provider`, `llm.Config{APIKey, Model, BaseURL}`, `llm.Request{Prompt, System, Model}`, `llm.Response{Text}`, `llm.Register` — all unchanged.
- Produces: same registered provider; `provider` struct gains a `baseURL` field.

- [ ] **Step 1: Add the new dependency**

```bash
go get google.golang.org/genai@v1.63.0
grep '^go ' go.mod
```

Expected: dependency added; `go` directive still `1.24` / `1.24.0`.

- [ ] **Step 2: Write the failing tests**

Create `internal/llm/gemini/gemini_test.go`:

```go
package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/oluwatayo/promptshell/internal/llm"
)

// fakeGemini serves the Gemini generateContent wire format and captures the
// last request it saw.
type fakeGemini struct {
	srv      *httptest.Server
	lastPath string
	lastBody []byte
	status   int
	reply    string
}

func newFakeGemini(t *testing.T, reply string) *fakeGemini {
	t.Helper()
	f := &fakeGemini{status: http.StatusOK, reply: reply}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.lastPath = r.URL.Path
		f.lastBody, _ = io.ReadAll(r.Body)
		if f.status != http.StatusOK {
			http.Error(w, `{"error":{"code":500,"message":"boom","status":"INTERNAL"}}`, f.status)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(f.reply))
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func textReply(text string) string {
	return `{"candidates":[{"content":{"role":"model","parts":[{"text":"` + text + `"}]}}]}`
}

func newTestProvider(t *testing.T, baseURL, model string) llm.Provider {
	t.Helper()
	p, err := New(llm.Config{APIKey: "test-key", Model: model, BaseURL: baseURL})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestNewRequiresAPIKey(t *testing.T) {
	if _, err := New(llm.Config{}); err == nil {
		t.Error("New() without an API key succeeded, want error")
	}
}

func TestGenerateHappyPath(t *testing.T) {
	f := newFakeGemini(t, textReply("echo hi"))
	p := newTestProvider(t, f.srv.URL, "")

	resp, err := p.Generate(context.Background(), llm.Request{Prompt: "say hi"})
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if resp.Text != "echo hi" {
		t.Errorf("Text = %q, want %q", resp.Text, "echo hi")
	}
	if !strings.Contains(f.lastPath, defaultModel) || !strings.Contains(f.lastPath, "generateContent") {
		t.Errorf("request path = %q, want the default model's generateContent endpoint", f.lastPath)
	}
	if !strings.Contains(string(f.lastBody), "say hi") {
		t.Errorf("request body missing the prompt: %s", f.lastBody)
	}
}

func TestGenerateSendsNativeSystemInstruction(t *testing.T) {
	f := newFakeGemini(t, textReply("ok"))
	p := newTestProvider(t, f.srv.URL, "")

	_, err := p.Generate(context.Background(), llm.Request{System: "only scripts", Prompt: "task"})
	if err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	var body struct {
		SystemInstruction *struct {
			Parts []struct{ Text string }
		} `json:"systemInstruction"`
		Contents []struct {
			Parts []struct{ Text string }
		} `json:"contents"`
	}
	if err := json.Unmarshal(f.lastBody, &body); err != nil {
		t.Fatalf("unmarshal request body: %v", err)
	}
	if body.SystemInstruction == nil || len(body.SystemInstruction.Parts) == 0 ||
		body.SystemInstruction.Parts[0].Text != "only scripts" {
		t.Errorf("system text not sent as native systemInstruction; body: %s", f.lastBody)
	}
	if len(body.Contents) != 1 || len(body.Contents[0].Parts) != 1 ||
		body.Contents[0].Parts[0].Text != "task" {
		t.Errorf("user prompt should carry only the task (no concatenated system text); body: %s", f.lastBody)
	}
}

func TestGenerateModelSelection(t *testing.T) {
	f := newFakeGemini(t, textReply("ok"))
	p := newTestProvider(t, f.srv.URL, "gemini-2.5-pro")

	if _, err := p.Generate(context.Background(), llm.Request{Prompt: "x"}); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}
	if !strings.Contains(f.lastPath, "gemini-2.5-pro") {
		t.Errorf("path = %q, want configured model gemini-2.5-pro", f.lastPath)
	}

	if _, err := p.Generate(context.Background(), llm.Request{Prompt: "x", Model: "gemini-3-flash-preview"}); err != nil {
		t.Fatalf("Generate() with override error: %v", err)
	}
	if !strings.Contains(f.lastPath, "gemini-3-flash-preview") {
		t.Errorf("path = %q, want per-request override gemini-3-flash-preview", f.lastPath)
	}
}

func TestGenerateAPIError(t *testing.T) {
	f := newFakeGemini(t, "")
	f.status = http.StatusInternalServerError
	p := newTestProvider(t, f.srv.URL, "")

	if _, err := p.Generate(context.Background(), llm.Request{Prompt: "x"}); err == nil {
		t.Error("Generate() succeeded on a 500, want error")
	}
}

func TestGenerateEmptyResponse(t *testing.T) {
	f := newFakeGemini(t, `{"candidates":[]}`)
	p := newTestProvider(t, f.srv.URL, "")

	_, err := p.Generate(context.Background(), llm.Request{Prompt: "x"})
	if err == nil || !strings.Contains(err.Error(), "empty response") {
		t.Errorf("Generate() = %v, want an empty-response error", err)
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/llm/gemini/ -v`
Expected: FAIL — the current provider ignores `cfg.BaseURL`, so every Generate test dials the real Google endpoint (connection/auth failures), and the old SDK is still imported.

- [ ] **Step 4: Rewrite the provider**

Replace the contents of `internal/llm/gemini/gemini.go` with:

```go
// Package gemini implements the llm.Provider interface for Google's Gemini
// models. Importing it for its side effects registers the "gemini" provider.
package gemini

import (
	"context"
	"fmt"

	"google.golang.org/genai"

	"github.com/oluwatayo/promptshell/internal/llm"
)

// Name is the registered provider name.
const Name = "gemini"

// defaultModel is a rolling alias so the default keeps working as Google
// retires pinned model versions (the pinned "gemini-pro" now 404s).
const defaultModel = "gemini-flash-latest"

func init() {
	llm.Register(Name, New)
}

type provider struct {
	apiKey  string
	model   string
	baseURL string
}

// New builds a Gemini provider from the given config. An API key is required.
func New(cfg llm.Config) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("gemini: api key is required")
	}
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}
	return &provider{apiKey: cfg.APIKey, model: model, baseURL: cfg.BaseURL}, nil
}

func (p *provider) Name() string { return Name }

// Generate sends the request to Gemini and returns the model's raw text.
func (p *provider) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  p.apiKey,
		Backend: genai.BackendGeminiAPI,
		// An empty BaseURL means the SDK's default public endpoint.
		HTTPOptions: genai.HTTPOptions{BaseURL: p.baseURL},
	})
	if err != nil {
		return llm.Response{}, err
	}

	modelName := p.model
	if req.Model != "" {
		modelName = req.Model
	}

	var genCfg *genai.GenerateContentConfig
	if req.System != "" {
		genCfg = &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(req.System, ""),
		}
	}

	resp, err := client.Models.GenerateContent(ctx, modelName, genai.Text(req.Prompt), genCfg)
	if err != nil {
		return llm.Response{}, err
	}
	if text := resp.Text(); text != "" {
		return llm.Response{Text: text}, nil
	}
	return llm.Response{}, fmt.Errorf("gemini: empty response")
}
```

If the SDK's actual behavior differs from a test's assumption (e.g. it
retries 5xx internally, or `resp.Text()` panics on zero candidates), adapt
the TEST to the SDK's real contract — keeping the provider's externally
visible behavior per the Global Constraints — and record the deviation in
your report. Do not weaken the two invariants: system text travels as
`systemInstruction` (never concatenated into the prompt), and empty model
output yields the `gemini: empty response` error.

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/llm/gemini/ -v`
Expected: all 6 tests PASS

- [ ] **Step 6: Tidy the module and verify the old tree is gone**

```bash
go mod tidy
grep -E 'generative-ai-go|google.golang.org/api ' go.mod && echo "OLD DEPS STILL PRESENT" || echo "old deps gone"
grep '^go ' go.mod
go build ./... && go test ./... > /dev/null && echo "full suite ok"
```

Expected: `old deps gone`; go directive still 1.24.x; full suite ok.

- [ ] **Step 7: Commit**

```bash
gofmt -l . && go vet ./...
git add internal/llm/gemini/ go.mod go.sum
git commit -m "refactor: migrate gemini provider to google.golang.org/genai

The generative-ai-go SDK is deprecated. The new unified SDK also lets the
provider honor cfg.BaseURL (like ollama already does), send req.System as
a native systemInstruction instead of concatenating it into the prompt,
and read all response text parts. BaseURL support makes the provider unit-
testable, so this adds its first test suite. Drops the old SDK's
cloud.google.com/go and google.golang.org/api dependency tree.

Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>"
```

---

### Task 2: full verification and PR

**Files:** none new; checks only.

- [ ] **Step 1: CI-parity gate**

```bash
gofmt -l .            # expect: no output
go build ./...        # expect: clean
go vet ./...          # expect: clean
go test -race ./...   # expect: all packages ok
golangci-lint run     # expect: 0 issues
```

- [ ] **Step 2: Live verification with the real key**

```bash
go run ./cmd/promptshell --provider gemini --dry-run "print hello"
```

Expected: `generating with gemini...` then a clean fenced-free script preview (e.g. `echo "hello"`) and `(dry run — not executing)`. This exercises the new SDK end-to-end: real auth, default model `gemini-flash-latest`, native system instruction.

- [ ] **Step 3: Push and open the PR**

```bash
git push -u origin refactor/gemini-genai-sdk
gh pr create --title "refactor: migrate gemini provider to google.golang.org/genai" --body "$(cat <<'EOF'
## Summary
- Replaces the deprecated `github.com/google/generative-ai-go` SDK with Google's current `google.golang.org/genai` (v1.63.0) in the gemini provider — no user-visible behavior change
- Gemini now honors `cfg.BaseURL` (config has had the field since v2; ollama already honors it)
- `req.System` travels as a native `systemInstruction` instead of being concatenated into the user prompt
- Response text now reads all parts (`resp.Text()`), not just the first
- go.mod sheds the old SDK's entire dependency tree (`cloud.google.com/go/*`, `google.golang.org/api`, grpc/otel indirects); `go` floor stays 1.24

Spec: docs/superpowers/specs/2026-07-13-gemini-genai-sdk-design.md

## Testing
- First-ever unit tests for the gemini provider (httptest server speaking the generateContent wire format): happy path, native systemInstruction placement, configured-model + per-request override, API error surfaced, empty response error, missing-key constructor error
- Live-verified with a real API key: `--provider gemini --dry-run "print hello"` works end-to-end on the new SDK
- gofmt / vet / `go test -race ./...` / golangci-lint clean

Note: `refactor:` commit — release-please won't cut a release for this alone; it ships with the upcoming update-hint feature (v0.7.0).

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 4: Report**

Tell the user the PR is open and that the SDK migration rides to users in v0.7.0 alongside the update-hint feature.
