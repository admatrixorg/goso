// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"os"
	"sort"
	"strings"
)

// Registry holds available providers based on env.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry builds providers from environment.
// Native: GOSO_ANTHROPIC_API_KEY, GOSO_OPENAI_API_KEY.
// Named OpenAI-compat: see OpenAICompatProviders. Empty key → provider absent.
func NewRegistry() *Registry {
	m := make(map[string]Provider)
	if key := strings.TrimSpace(os.Getenv("GOSO_ANTHROPIC_API_KEY")); key != "" {
		m["anthropic"] = &Anthropic{APIKey: key, Model: os.Getenv("GOSO_ANTHROPIC_MODEL"), CacheMode: os.Getenv("GOSO_ANTHROPIC_CACHE_MODE")}
	}
	if key := strings.TrimSpace(os.Getenv("GOSO_OPENAI_API_KEY")); key != "" {
		m["openai"] = &OpenAI{APIKey: key, Model: os.Getenv("GOSO_OPENAI_MODEL")}
	}
	for _, spec := range OpenAICompatProviders() {
		key := strings.TrimSpace(os.Getenv(spec.EnvKey))
		if key == "" {
			continue
		}
		model := strings.TrimSpace(os.Getenv(spec.EnvModel))
		if model == "" {
			model = spec.Model
		}
		m[spec.Name] = &OpenAI{
			APIKey:  key,
			Model:   model,
			BaseURL: spec.BaseURL,
			Label:   spec.Name,
		}
	}
	m["echo"] = Echo{}
	return &Registry{providers: m}
}

// Get returns a provider by name, or echo fallback.
func (r *Registry) Get(name string) Provider {
	if r != nil {
		if p, ok := r.providers[name]; ok {
			return p
		}
		if p, ok := r.providers["echo"]; ok {
			return p
		}
	}
	return Echo{}
}

// List returns configured provider names (no secrets), sorted.
func (r *Registry) List() []string {
	if r == nil {
		return []string{"echo"}
	}
	out := make([]string, 0, len(r.providers))
	for k := range r.providers {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// HasReal returns true if any non-echo provider is configured.
func (r *Registry) HasReal() bool {
	if r == nil {
		return false
	}
	for k := range r.providers {
		if k != "echo" {
			return true
		}
	}
	return false
}

var preferredOrder = []string{
	"anthropic", "openai",
	"openrouter", "groq", "deepseek", "gemini", "mistral", "xai", "minimax", "dashscope",
}

// Preferred picks anthropic, else openai, else the first configured named compat, else echo.
func (r *Registry) Preferred() Provider {
	if r == nil {
		return Echo{}
	}
	for _, name := range preferredOrder {
		if p, ok := r.providers[name]; ok {
			return p
		}
	}
	return r.Get("echo")
}
