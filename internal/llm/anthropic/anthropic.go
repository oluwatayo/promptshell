// Package anthropic implements the llm.Provider interface for Anthropic's
// Claude models via the official anthropic-sdk-go. Importing it for its side
// effects registers the "anthropic" provider.
package anthropic

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/oluwatayo/promptshell/internal/llm"
)

// Name is the registered provider name.
const Name = "anthropic"

const (
	defaultModel = "claude-opus-4-8"
	maxTokens    = 4096
)

func init() {
	llm.Register(Name, New)
}

type provider struct {
	apiKey  string
	model   string
	baseURL string
}

// New builds an Anthropic provider. An API key is required.
func New(cfg llm.Config) (llm.Provider, error) {
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: api key is required")
	}
	model := cfg.Model
	if model == "" {
		model = defaultModel
	}
	return &provider{apiKey: cfg.APIKey, model: model, baseURL: cfg.BaseURL}, nil
}

func (p *provider) Name() string { return Name }

// Generate sends the request to the Anthropic Messages API and returns the
// concatenated text of the response.
func (p *provider) Generate(ctx context.Context, req llm.Request) (llm.Response, error) {
	opts := []option.RequestOption{option.WithAPIKey(p.apiKey)}
	if p.baseURL != "" {
		opts = append(opts, option.WithBaseURL(p.baseURL))
	}
	client := anthropic.NewClient(opts...)

	model := p.model
	if req.Model != "" {
		model = req.Model
	}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: maxTokens,
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(req.Prompt)),
		},
	}
	if req.System != "" {
		params.System = []anthropic.TextBlockParam{{Text: req.System}}
	}

	resp, err := client.Messages.New(ctx, params)
	if err != nil {
		return llm.Response{}, err
	}

	var sb strings.Builder
	for _, block := range resp.Content {
		if t, ok := block.AsAny().(anthropic.TextBlock); ok {
			sb.WriteString(t.Text)
		}
	}
	return llm.Response{Text: sb.String()}, nil
}
