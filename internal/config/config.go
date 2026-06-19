// Package config loads and persists promptshell's user configuration,
// stored as JSON at ~/.promptshell/config/config.json.
//
// The current schema (version 2) supports multiple providers, each with its
// own API key, model, and base URL. Configuration files written by earlier
// versions (a single {apiKey, provider} object) are migrated automatically on
// load.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// schemaVersion is the current on-disk config version.
const schemaVersion = 2

// DefaultProvider is used when no provider is selected by flag, environment, or
// config. Ollama runs locally and needs no API key, so promptshell works
// offline out of the box.
const DefaultProvider = "ollama"

// ProviderSettings holds per-provider configuration. Not every field applies to
// every provider (local providers such as Ollama use BaseURL instead of
// APIKey).
type ProviderSettings struct {
	APIKey  string `json:"apiKey,omitempty"`
	Model   string `json:"model,omitempty"`
	BaseURL string `json:"baseURL,omitempty"`
}

// Config is promptshell's persisted configuration (schema v2).
type Config struct {
	Version         int                         `json:"version"`
	DefaultProvider string                      `json:"defaultProvider"`
	Providers       map[string]ProviderSettings `json:"providers"`
}

// rawConfig is the union of the v2 schema and the legacy v1 fields, used to
// detect and migrate old config files.
type rawConfig struct {
	Version         int                         `json:"version"`
	DefaultProvider string                      `json:"defaultProvider"`
	Providers       map[string]ProviderSettings `json:"providers"`

	// Legacy v1 fields.
	APIKey   string `json:"apiKey"`
	Provider string `json:"provider"`
}

func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".promptshell", "config", "config.json"), nil
}

// Load reads and normalizes the config file. A missing file yields a default
// config (Ollama as the default provider). A v1 file is migrated to v2 in
// memory; the migrated form is persisted the next time Save is called.
func Load() (Config, error) {
	p, err := path()
	if err != nil {
		return Config{}, err
	}

	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if mkErr := os.MkdirAll(filepath.Dir(p), 0o755); mkErr != nil {
				return Config{}, mkErr
			}
			return normalize(Config{}), nil
		}
		return Config{}, err
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return Config{}, err
	}
	return normalize(migrate(raw)), nil
}

// migrate converts a rawConfig into a v2 Config, translating legacy v1 fields
// when present.
func migrate(raw rawConfig) Config {
	c := Config{
		Version:         raw.Version,
		DefaultProvider: raw.DefaultProvider,
		Providers:       raw.Providers,
	}

	// A v1 file has no providers map but may carry a single apiKey/provider.
	if c.Providers == nil && (raw.APIKey != "" || raw.Provider != "") {
		provider := raw.Provider
		if provider == "" {
			// v1 only ever shipped Gemini support.
			provider = "gemini"
		}
		c.Providers = map[string]ProviderSettings{
			provider: {APIKey: raw.APIKey},
		}
		// Honor the provider the user was already using as their default.
		if c.DefaultProvider == "" {
			c.DefaultProvider = provider
		}
	}
	return c
}

// normalize fills in defaults so callers never deal with nil maps or empty
// required fields.
func normalize(c Config) Config {
	if c.Providers == nil {
		c.Providers = map[string]ProviderSettings{}
	}
	if c.DefaultProvider == "" {
		c.DefaultProvider = DefaultProvider
	}
	c.Version = schemaVersion
	return c
}

// Save writes the config to disk in the current schema version.
func Save(c Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	c = normalize(c)
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// Provider returns the settings for the named provider, or a zero value if the
// provider has no saved settings.
func (c *Config) Provider(name string) ProviderSettings {
	return c.Providers[name]
}

// SetKey sets the API key for the named provider.
func (c *Config) SetKey(name, key string) {
	c.set(name, func(p *ProviderSettings) { p.APIKey = key })
}

// SetModel sets the model for the named provider.
func (c *Config) SetModel(name, model string) {
	c.set(name, func(p *ProviderSettings) { p.Model = model })
}

// SetBaseURL sets the base URL for the named provider.
func (c *Config) SetBaseURL(name, baseURL string) {
	c.set(name, func(p *ProviderSettings) { p.BaseURL = baseURL })
}

// SetDefaultProvider sets the default provider.
func (c *Config) SetDefaultProvider(name string) {
	c.DefaultProvider = name
}

func (c *Config) set(name string, mutate func(*ProviderSettings)) {
	if c.Providers == nil {
		c.Providers = map[string]ProviderSettings{}
	}
	p := c.Providers[name]
	mutate(&p)
	c.Providers[name] = p
}
