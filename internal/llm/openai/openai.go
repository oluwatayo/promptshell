// Package openai implements the llm.Provider interface for OpenAI's chat
// models via the official openai-go SDK. Importing it for its side effects
// registers the "openai" provider.
package openai

import (
	"context"
	"fmt"
	"strings"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/oluwatayo/promptshell/internal/llm"
)

// Name is the registered provider name.
const Name = "openai"

const defaultModel = "gpt-4o"

func init() {
	llm.Register(Name, New)
}

type provider struct {
	apiKey  string
	model   string
	baseURL string
}

// New builds an OpenAI provider. An API key is required.
func New(cfg llm.Config) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai: api key is required")
	}
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}
	return &provider{apiKey: cfg.APIKey, model: model, baseURL: cfg.BaseURL}, nil
}

func (p *provider) Name() string { return Name }

// Generate sends the request to the OpenAI Chat Completions API and returns the
// text of the first choice.
func (p *provider) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	opts := []option.RequestOption{option.WithAPIKey(p.apiKey)}
	if p.baseURL != "" {
		opts = append(opts, option.WithBaseURL(p.baseURL))
	}
	client := openai.NewClient(opts...)

	model := p.model
	if req.Model != "" {
		model = req.Model
	}

	messages := []openai.ChatCompletionMessageParamUnion{}
	if req.System != "" {
		messages = append(messages, openai.SystemMessage(req.System))
	}
	messages = append(messages, openai.UserMessage(req.Prompt))

	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(model),
		Messages: messages,
	})
	if err != nil {
		return llm.Response{}, err
	}
	if len(resp.Choices) == 0 {
		return llm.Response{}, fmt.Errorf("openai: empty response")
	}

	return llm.Response{Text: strings.TrimSpace(resp.Choices[0].Message.Content)}, nil
}
