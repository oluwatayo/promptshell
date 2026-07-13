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
