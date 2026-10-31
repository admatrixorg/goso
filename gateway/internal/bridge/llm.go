// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package bridge

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/llm"
)

// Completion is one tenant-scoped model call. APIKey is not logged.
type Completion struct {
	Type        string
	BaseURL     string
	APIKey      string
	Model       string
	Temperature *float64
	MaxTokens   *int
	Messages    []llm.Message
	OnDelta     func(string)
}

// Result is a model reply plus token counts.
type Result struct {
	Text             string
	PromptTokens     int
	CompletionTokens int
	StopReason       string
}

// Completer generates a reply. Tests inject a fake; production uses DefaultComplete.
type Completer func(ctx context.Context, in Completion) (Result, error)

// DefaultComplete calls the tenant provider through the existing LLM clients.
func DefaultComplete(ctx context.Context, in Completion) (Result, error) {
	typ := mapProviderType(in.Type)
	base := normalizeBase(in.Type, in.BaseURL)
	if typ != llm.TypeEcho && typ != llm.TypeRouter9 && strings.TrimSpace(in.APIKey) == "" {
		return Result{}, unavailable("provider api key is not set")
	}
	p, err := llm.Build("omni", typ, base, in.Model, in.APIKey)
	if err != nil {
		return Result{}, unavailable("provider is not usable")
	}
	applySampling(p, in.Temperature, in.MaxTokens)
	var text string
	var usage llm.Usage
	if in.OnDelta != nil {
		text, usage, err = llm.ChatStream(ctx, p, in.Messages, in.OnDelta)
	} else {
		text, usage, err = llm.ChatUsage(ctx, p, in.Messages)
	}
	if err != nil {
		var up *llm.UpstreamError
		if errors.As(err, &up) {
			return Result{}, upstreamFailure(up)
		}
		if strings.Contains(err.Error(), "empty content") || strings.Contains(err.Error(), "no choices") {
			return Result{}, &Error{Status: http.StatusBadGateway, Code: "empty_reply", Msg: "upstream returned empty text"}
		}
		return Result{}, unavailable(redact(err.Error(), in.APIKey))
	}
	if strings.TrimSpace(text) == "" {
		return Result{}, &Error{Status: http.StatusBadGateway, Code: "empty_reply", Msg: "upstream returned empty text"}
	}
	return Result{
		Text:             text,
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		StopReason:       "stop",
	}, nil
}

func mapProviderType(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "anthropic":
		return llm.TypeAnthropic
	case "echo":
		return llm.TypeEcho
	case "router9":
		return llm.TypeRouter9
	default:
		// openai, openai_compatible, custom, opencode, openrouter, deepseek, gemini, goso.
		return llm.TypeOpenAICompat
	}
}

func normalizeBase(typ, base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	t := strings.ToLower(strings.TrimSpace(typ))
	if base == "" {
		if v := strings.TrimRight(strings.TrimSpace(firstNonEmpty(os.Getenv("GOSO_LLM_BASE_URL"), os.Getenv("GOSO_DEFAULT_BASE_URL"))), "/"); v != "" && openAICompatType(t) {
			return v
		}
		switch t {
		case "anthropic":
			return "https://api.anthropic.com"
		case "gemini":
			return "https://generativelanguage.googleapis.com/v1beta"
		case "deepseek":
			return "https://api.deepseek.com/v1"
		case "openrouter":
			return "https://openrouter.ai/api/v1"
		case "echo", "router9":
			return ""
		default:
			// opencode is an alias of openai: same default host, no dedicated vendor URL.
			return "https://api.openai.com/v1"
		}
	}
	// llm.Anthropic appends /v1/messages. Core stores https://api.anthropic.com/v1.
	if t == "anthropic" && strings.HasSuffix(base, "/v1") {
		return strings.TrimSuffix(base, "/v1")
	}
	return base
}

func openAICompatType(t string) bool {
	switch t {
	case "", "openai", "openai-compat", "openai_compatible", "openai-compatible",
		"opencode", "zen", "opencode-zen", "openrouter", "deepseek", "gemini", "custom", "goso":
		return true
	default:
		return false
	}
}

// upstreamFailure maps a provider HTTP status onto a stable bridge error.
// The provider body is not copied into the client message.
func upstreamFailure(up *llm.UpstreamError) *Error {
	if up == nil {
		return unavailable("upstream provider failed")
	}
	code, status, msg := classifyStatus(up.Status)
	return &Error{Status: status, Code: code, Upstream: up.Status, Msg: msg}
}

func classifyStatus(status int) (code string, httpStatus int, msg string) {
	switch status {
	case http.StatusUnauthorized:
		return "upstream_unauthorized", http.StatusUnauthorized, "upstream unauthorized"
	case http.StatusPaymentRequired:
		return "insufficient_funds", http.StatusPaymentRequired, "upstream insufficient funds"
	case http.StatusForbidden:
		return "upstream_forbidden", http.StatusForbidden, "upstream forbidden"
	case http.StatusNotFound:
		return "upstream_not_found", http.StatusNotFound, "upstream not found"
	case http.StatusTooManyRequests:
		return "rate_limited", http.StatusTooManyRequests, "upstream rate limit"
	default:
		if status >= 500 {
			return "upstream_unavailable", http.StatusBadGateway, "upstream unavailable"
		}
		return "upstream_rejected", http.StatusBadGateway, "upstream rejected"
	}
}

// envDefaultProvider is the process-wide OpenAI-compatible fallback used when
// the tenant has no provider for the requested model. Core compose sets
// GOSO_LLM_PROVIDER, GOSO_LLM_API_KEY, GOSO_LLM_BASE_URL, and GOSO_LLM_MODEL.
// The model string is kept as given, including slashes. GOSO_DEFAULT_* is an alias.
func envDefaultProvider() (Provider, bool) {
	key := firstNonEmpty(os.Getenv("GOSO_LLM_API_KEY"), os.Getenv("GOSO_DEFAULT_API_KEY"))
	if key == "" {
		return Provider{}, false
	}
	typ := firstNonEmpty(os.Getenv("GOSO_LLM_PROVIDER"), os.Getenv("GOSO_DEFAULT_PROVIDER_TYPE"))
	if typ == "" {
		typ = "openai"
	}
	base := strings.TrimRight(firstNonEmpty(os.Getenv("GOSO_LLM_BASE_URL"), os.Getenv("GOSO_DEFAULT_BASE_URL")), "/")
	if base == "" {
		base = normalizeBase(typ, "")
	}
	model := firstNonEmpty(os.Getenv("GOSO_LLM_MODEL"), os.Getenv("GOSO_DEFAULT_MODEL"))
	var models []string
	if model != "" {
		models = []string{model}
	}
	return Provider{
		ID:      "env-default",
		Name:    "env-default",
		Type:    typ,
		BaseURL: base,
		Models:  models,
		Active:  true,
		APIKey:  key,
		KeySet:  true,
	}, true
}

func applySampling(p llm.Provider, temp *float64, maxTok *int) {
	switch v := p.(type) {
	case *llm.OpenAI:
		v.Temperature = temp
		v.MaxTokens = maxTok
	case *llm.Anthropic:
		v.Temperature = temp
		v.MaxTokens = maxTok
	}
}

func redact(msg, key string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return "upstream provider failed"
	}
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "[redacted]")
	}
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}
