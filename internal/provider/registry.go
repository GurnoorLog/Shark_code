package provider

import (
	"crypto/rand"
	"encoding/hex"

	"shark-agent/internal/config"
)

func NewRegistry(cfg *config.Config) *Registry {
	return &Registry{cfg: cfg, cache: map[string]Provider{}}
}

type Registry struct {
	cfg   *config.Config
	cache map[string]Provider
}

func (r *Registry) Get(name string) Provider {
	if name == "" {
		name = r.cfg.ActiveProvider
	}
	if p, ok := r.cache[name]; ok {
		return p
	}
	p := r.build(name)
	if p != nil {
		r.cache[name] = p
	}
	return p
}

func (r *Registry) build(name string) Provider {
	pc, ok := r.cfg.Providers[name]
	if !ok {
		return nil
	}
	switch name {
	case "anthropic":
		return NewAnthropic(pc)
	case "gemini":
		return NewGemini(pc)
	case "openai":
		return NewOpenAICompatible("openai", pc)
	case "fireworks":
		return NewOpenAICompatible("fireworks", pc)
	case "local":
		return NewOpenAICompatible("local", pc)
	default:
		return NewOpenAICompatible(name, pc)
	}
}

func randID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
