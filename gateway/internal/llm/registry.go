// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import "os"

// Registry holds available providers based on env.
type Registry struct {
	providers map[string]Provider
}

// NewRegistry builds providers from environment.
// Env: GOSO_ANTHROPIC_API_KEY, GOSO_ANTHROPIC_MODEL, GOSO_OPENAI_API_KEY, GOSO_OPENAI_MODEL.
func NewRegistry() *Registry {
	m := make(map[string]Provider)
	if key := os.Getenv("GOSO_ANTHROPIC_API_KEY"); key != "" {
		m["anthropic"] = &Anthropic{APIKey: key, Model: os.Getenv("GOSO_ANTHROPIC_MODEL")}
	}
	if key := os.Getenv("GOSO_OPENAI_API_KEY"); key != "" {
		m["openai"] = &OpenAI{APIKey: key, Model: os.Getenv("GOSO_OPENAI_MODEL")}
	}
	// Always have echo as fallback.
	m["echo"] = Echo{}
	return &Registry{providers: m}
}

// Get returns a provider by name, or echo fallback.
func (r *Registry) Get(name string) Provider {
	if p, ok := r.providers[name]; ok {
		return p
	}
	return r.providers["echo"]
}

// List returns provider names (without exposing keys).
func (r *Registry) List() []string {
	out := make([]string, 0, len(r.providers))
	for k := range r.providers {
		out = append(out, k)
	}
	return out
}

// HasReal returns true if any non-echo provider is configured.
func (r *Registry) HasReal() bool {
	for k := range r.providers {
		if k != "echo" {
			return true
		}
	}
	return false
}
