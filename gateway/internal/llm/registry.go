// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import (
	"net/http"
	"os"
	"sort"
	"strings"
)

// Provider types persisted / returned by GET /api/providers.
const (
	TypeOpenAICompat = "openai-compat"
	TypeAnthropic    = "anthropic"
	TypeEcho         = "echo"
	TypeRouter9      = "router9"
	SourceEnv        = "env"
	SourceSQLite     = "sqlite"
)

// ProviderInfo is a public listing row. Never includes api_key.
type ProviderInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	KeySet  bool   `json:"key_set"`
	Source  string `json:"source"`
}

// Registry holds available providers based on env.
type Registry struct {
	providers map[string]Provider
	infos     map[string]ProviderInfo
}

// NewRegistry builds providers from environment.
// Native: GOSO_ANTHROPIC_API_KEY, GOSO_OPENAI_API_KEY.
// Named OpenAI-compat: see OpenAICompatProviders. Empty key → provider absent
// except router9, which constructs when GOSO_ROUTER9_BASE_URL is non-empty.
func NewRegistry() *Registry {
	m := make(map[string]Provider)
	infos := make(map[string]ProviderInfo)
	put := func(name string, p Provider) {
		m[name] = p
		infos[name] = Describe(name, p, envType(name), SourceEnv)
	}
	if key := strings.TrimSpace(os.Getenv("GOSO_ANTHROPIC_API_KEY")); key != "" {
		put("anthropic", &Anthropic{APIKey: key, Model: os.Getenv("GOSO_ANTHROPIC_MODEL"), CacheMode: os.Getenv("GOSO_ANTHROPIC_CACHE_MODE")})
	}
	if key := strings.TrimSpace(os.Getenv("GOSO_OPENAI_API_KEY")); key != "" {
		put("openai", &OpenAI{APIKey: key, Model: os.Getenv("GOSO_OPENAI_MODEL")})
	}
	for _, spec := range OpenAICompatProviders() {
		key := strings.TrimSpace(os.Getenv(spec.EnvKey))
		base := spec.BaseURL
		if spec.EnvURL != "" {
			u := strings.TrimSpace(os.Getenv(spec.EnvURL))
			if u == "" {
				continue
			}
			base = u
		} else if key == "" {
			continue
		}
		model := strings.TrimSpace(os.Getenv(spec.EnvModel))
		if model == "" {
			model = spec.Model
		}
		o := &OpenAI{
			APIKey:        key,
			Model:         model,
			BaseURL:       base,
			Label:         spec.Name,
			AllowEmptyKey: spec.AllowEmptyKey,
		}
		if spec.Timeout > 0 {
			o.Client = guardedClient(&http.Client{Timeout: spec.Timeout}, spec.Timeout)
		}
		put(spec.Name, o)
	}
	put("echo", Echo{})
	return &Registry{providers: m, infos: infos}
}

func envType(name string) string {
	switch name {
	case "echo":
		return TypeEcho
	case "anthropic":
		return TypeAnthropic
	case "router9":
		return TypeRouter9
	default:
		return TypeOpenAICompat
	}
}

// Describe builds a public info row from a live provider (no secrets).
func Describe(name string, p Provider, typ, source string) ProviderInfo {
	info := ProviderInfo{Name: name, Type: typ, Source: source}
	switch v := p.(type) {
	case *OpenAI:
		info.BaseURL = v.BaseURL
		if info.BaseURL == "" {
			info.BaseURL = "https://api.openai.com"
		}
		info.Model = v.ModelName()
		info.KeySet = strings.TrimSpace(v.APIKey) != ""
	case *Anthropic:
		info.BaseURL = v.BaseURL
		if info.BaseURL == "" {
			info.BaseURL = "https://api.anthropic.com"
		}
		info.Model = v.ModelName()
		info.KeySet = strings.TrimSpace(v.APIKey) != ""
	case Echo:
		info.Model = v.ModelName()
	default:
		if mn, ok := p.(interface{ ModelName() string }); ok {
			info.Model = mn.ModelName()
		}
	}
	return info
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

// Has reports whether name is constructed (Get falls back to echo).
func (r *Registry) Has(name string) bool {
	if r == nil {
		return name == "echo"
	}
	_, ok := r.providers[name]
	return ok
}

// Infos returns public listing rows (no secrets), sorted by name.
func (r *Registry) Infos() []ProviderInfo {
	if r == nil {
		return []ProviderInfo{Describe("echo", Echo{}, TypeEcho, SourceEnv)}
	}
	out := make([]ProviderInfo, 0, len(r.infos))
	for _, inf := range r.infos {
		out = append(out, inf)
	}
	if len(out) == 0 {
		out = append(out, Describe("echo", Echo{}, TypeEcho, SourceEnv))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
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

// Preferred picks GOSO_LLM_PROVIDER if that name exists, else router9 if
// constructed, else anthropic, openai, first configured named compat, else echo.
func (r *Registry) Preferred() Provider {
	if r == nil {
		return Echo{}
	}
	if name := strings.TrimSpace(os.Getenv("GOSO_LLM_PROVIDER")); name != "" {
		if p, ok := r.providers[name]; ok {
			return p
		}
	}
	if p, ok := r.providers["router9"]; ok {
		return p
	}
	for _, name := range preferredOrder {
		if p, ok := r.providers[name]; ok {
			return p
		}
	}
	return r.Get("echo")
}
