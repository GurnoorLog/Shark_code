package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type ProviderConfig struct {
	APIKey  string `json:"api_key,omitempty"`
	BaseURL string `json:"base_url,omitempty"`
	Model   string `json:"model"`
}

type Config struct {
	ActiveProvider string                     `json:"active_provider"`
	Providers      map[string]*ProviderConfig `json:"providers"`
	Cwd            string                     `json:"cwd,omitempty"`
}

func Default() *Config {
	return &Config{
		ActiveProvider: "openai",
		Providers: map[string]*ProviderConfig{
			"openai":    {BaseURL: "https://api.openai.com/v1", Model: "gpt-4o"},
			"anthropic": {BaseURL: "https://api.anthropic.com", Model: "claude-sonnet-4-20250514"},
			"gemini":    {BaseURL: "https://generativelanguage.googleapis.com/v1beta", Model: "gemini-2.0-flash"},
			"fireworks": {BaseURL: "https://api.fireworks.ai/inference/v1", Model: "accounts/fireworks/models/minimax-m3"},
			"local":     {BaseURL: "http://127.0.0.1:8080/v1", Model: "gemma-2-2b-it"},
		},
	}
}

func ConfigPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "sharkcode.json"
	}
	return filepath.Join(home, ".sharkcode", "config.json")
}

func Load() *Config {
	cfg := Default()
	p := ConfigPath()
	data, err := os.ReadFile(p)
	if err != nil {
		return cfg
	}
	json.Unmarshal(data, cfg)
	return cfg
}

func (c *Config) Save() error {
	p := ConfigPath()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o600)
}

func (c *Config) Active() *ProviderConfig {
	if p, ok := c.Providers[c.ActiveProvider]; ok {
		return p
	}
	return c.Providers["openai"]
}
