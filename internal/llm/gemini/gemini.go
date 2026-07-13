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
