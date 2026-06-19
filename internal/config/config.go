// Package config loads and persists promptshell's user configuration,
// stored as JSON at ~/.promptshell/config/config.json.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config holds promptshell's persisted settings.
type Config struct {
	APIKey   string `json:"apiKey"`
	Provider string `json:"provider"`
}

// path returns the absolute path to the config file.
func path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".promptshell", "config", "config.json"), nil
}

// Load reads the config file. If it does not exist yet, Load ensures the
// config directory is present and returns a zero-value Config.
func Load() (Config, error) {
	var c Config
	p, err := path()
	if err != nil {
		return c, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, os.MkdirAll(filepath.Dir(p), 0o755)
		}
		return c, err
	}
	if err := json.Unmarshal(data, &c); err != nil {
		return c, err
	}
	return c, nil
}

// write persists the given config to disk.
func write(c Config) error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

// GetAPIKey returns the API key stored in the config file.
func GetAPIKey() (string, error) {
	c, err := Load()
	if err != nil {
		return "", err
	}
	return c.APIKey, nil
}

// UpdateAPIKey saves the given API key to the config file, preserving other
// settings.
func UpdateAPIKey(key string) error {
	c, err := Load()
	if err != nil {
		return err
	}
	c.APIKey = key
	return write(c)
}

// ResolveAPIKey returns the API key, preferring the PROMPTSHELL_API_KEY
// environment variable and falling back to the saved config file. It returns
// an empty string if no key can be found.
func ResolveAPIKey() string {
	if key := os.Getenv("PROMPTSHELL_API_KEY"); key != "" {
		return key
	}
	key, err := GetAPIKey()
	if err != nil {
		return ""
	}
	return key
}
