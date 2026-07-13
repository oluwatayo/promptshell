// Package gemini implements the llm.Provider interface for Google's Gemini
// models. Importing it for its side effects registers the "gemini" provider.
package gemini

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"

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
	apiKey string
	model  string
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
	return &provider{apiKey: cfg.APIKey, model: model}, nil
}

func (p *provider) Name() string { return Name }

// Generate sends the request to Gemini and returns the model's raw text.
func (p *provider) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(p.apiKey))
	if err != nil {
		return llm.Response{}, err
	}
	defer func() { _ = client.Close() }()

	modelName := p.model
	if req.Model != "" {
		modelName = req.Model
	}

	prompt := req.Prompt
	if req.System != "" {
		prompt = req.System + "\n\n" + req.Prompt
	}

	model := client.GenerativeModel(modelName)
	resp, err := model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return llm.Response{}, err
	}
	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil ||
		len(resp.Candidates[0].Content.Parts) == 0 {
		return llm.Response{}, fmt.Errorf("gemini: empty response")
	}
	text := fmt.Sprintf("%v", resp.Candidates[0].Content.Parts[0])
	return llm.Response{Text: text}, nil
}
