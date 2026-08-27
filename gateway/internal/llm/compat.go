// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package llm

import "time"

// OpenAICompat is a named OpenAI-compatible endpoint (SPEC 039 / 044).
type OpenAICompat struct {
	Name          string
	EnvKey        string
	EnvModel      string
	EnvURL        string // if set, construct when this env is non-empty; key may be empty
	BaseURL       string
	Model         string
	AllowEmptyKey bool
	Timeout       time.Duration
}

// OpenAICompatProviders is the named catalog: openrouter, groq, deepseek,
// gemini, mistral, xai, minimax, dashscope, router9.
// Default: construct only when EnvKey is set. router9 constructs when EnvURL is set.
func OpenAICompatProviders() []OpenAICompat {
	return []OpenAICompat{
		{Name: "openrouter", EnvKey: "GOSO_OPENROUTER_API_KEY", EnvModel: "GOSO_OPENROUTER_MODEL", BaseURL: "https://openrouter.ai/api", Model: "openai/gpt-4o-mini"},
		{Name: "groq", EnvKey: "GOSO_GROQ_API_KEY", EnvModel: "GOSO_GROQ_MODEL", BaseURL: "https://api.groq.com/openai", Model: "llama-3.3-70b-versatile"},
		{Name: "deepseek", EnvKey: "GOSO_DEEPSEEK_API_KEY", EnvModel: "GOSO_DEEPSEEK_MODEL", BaseURL: "https://api.deepseek.com", Model: "deepseek-chat"},
		{Name: "gemini", EnvKey: "GOSO_GEMINI_API_KEY", EnvModel: "GOSO_GEMINI_MODEL", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", Model: "gemini-2.0-flash"},
		{Name: "mistral", EnvKey: "GOSO_MISTRAL_API_KEY", EnvModel: "GOSO_MISTRAL_MODEL", BaseURL: "https://api.mistral.ai", Model: "mistral-small-latest"},
		{Name: "xai", EnvKey: "GOSO_XAI_API_KEY", EnvModel: "GOSO_XAI_MODEL", BaseURL: "https://api.x.ai", Model: "grok-2-1212"},
		{Name: "minimax", EnvKey: "GOSO_MINIMAX_API_KEY", EnvModel: "GOSO_MINIMAX_MODEL", BaseURL: "https://api.minimax.io", Model: "MiniMax-Text-01"},
		{Name: "dashscope", EnvKey: "GOSO_DASHSCOPE_API_KEY", EnvModel: "GOSO_DASHSCOPE_MODEL", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode", Model: "qwen-plus"},
		{
			Name: "router9", EnvKey: "GOSO_ROUTER9_API_KEY", EnvModel: "GOSO_ROUTER9_MODEL",
			EnvURL: "GOSO_ROUTER9_BASE_URL", BaseURL: "http://127.0.0.1:20127/v1",
			Model: "cx/gpt-5.6-sol", AllowEmptyKey: true, Timeout: 120 * time.Second,
		},
	}
}
