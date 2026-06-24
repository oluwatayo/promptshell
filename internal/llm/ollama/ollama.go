// Package ollama implements the llm.Provider interface for a local Ollama
// server (https://ollama.com). Importing it for its side effects registers the
// "ollama" provider. Ollama runs locally and needs no API key.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/oluwatayo/promptshell/internal/llm"
)

// Name is the registered provider name.
const Name = "ollama"

// Defaults for a local Ollama install.
const (
	DefaultModel   = "llama3"
	DefaultBaseURL = "http://localhost:11434"
)

// ErrUnreachable indicates the Ollama server could not be contacted (e.g. it is
// not installed or not running). Callers can detect it with errors.Is.
var ErrUnreachable = errors.New("ollama: could not reach the server")

func init() {
	llm.Register(Name, New)
}

type provider struct {
	model   string
	baseURL string
	client  *http.Client
}

// New builds an Ollama provider. No API key is required; the model and base URL
// fall back to sensible local defaults.
func New(cfg llm.Config) (llm.Provider, error) {
	model := cfg.Model
	if model == "" {
		model = DefaultModel
	}
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &provider{model: model, baseURL: baseURL, client: http.DefaultClient}, nil
}

func (p *provider) Name() string { return Name }

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	System string `json:"system,omitempty"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
}

// Generate sends the request to the local Ollama server and returns the model's
// raw text.
func (p *provider) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	model := p.model
	if req.Model != "" {
		model = req.Model
	}

	body, err := json.Marshal(generateRequest{
		Model:  model,
		Prompt: req.Prompt,
		System: req.System,
		Stream: false,
	})
	if err != nil {
		return llm.Response{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return llm.Response{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return llm.Response{}, fmt.Errorf("%w at %s — is Ollama running? (https://ollama.com): %v", ErrUnreachable, p.baseURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return llm.Response{}, fmt.Errorf("ollama: server returned status %s (is the model %q pulled? try `ollama pull %s`)", resp.Status, model, model)
	}

	var out generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return llm.Response{}, fmt.Errorf("ollama: decoding response: %w", err)
	}
	return llm.Response{Text: out.Response}, nil
}
