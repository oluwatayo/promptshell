// Package llm defines the provider abstraction promptshell uses to talk to
// language models, plus a name-keyed registry so providers can be selected at
// runtime from configuration.
package llm

import (
	"context"
	"fmt"
	"sort"
)

// Request is a single generation request to a provider.
type Request struct {
	// System is an optional system/instruction message. Providers that lack a
	// dedicated system role may prepend it to the prompt.
	System string
	// Prompt is the user prompt to generate from.
	Prompt string
	// Model optionally overrides the provider's default model.
	Model string
}

// Response is the result of a generation request.
type Response struct {
	// Text is the generated text, as returned by the provider.
	Text string
}

// Provider is an LLM backend capable of generating text.
type Provider interface {
	// Name returns the provider's registered name (e.g. "gemini").
	Name() string
	// Generate produces a response for the given request.
	Generate(ctx context.Context, req Request) (Response, error)
}

// Config carries the settings needed to construct a Provider. Not every field
// applies to every provider (e.g. local providers use BaseURL instead of
// APIKey).
type Config struct {
	APIKey  string
	Model   string
	BaseURL string
}

// Factory constructs a Provider from a Config.
type Factory func(cfg Config) (Provider, error)

var registry = map[string]Factory{}

// Register adds a provider factory under the given name. It is intended to be
// called from provider packages' init functions. Registering the same name
// twice overwrites the earlier registration.
func Register(name string, f Factory) {
	registry[name] = f
}

// New constructs the named provider with the given config.
func New(name string, cfg Config) (Provider, error) {
	f, ok := registry[name]
	if !ok {
		if len(registry) == 0 {
			return nil, fmt.Errorf("unknown provider %q (no providers registered)", name)
		}
		return nil, fmt.Errorf("unknown provider %q (available: %v)", name, Available())
	}
	return f(cfg)
}

// Available returns the sorted names of all registered providers.
func Available() []string {
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
