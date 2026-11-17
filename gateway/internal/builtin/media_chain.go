// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package builtin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/mqglobal/goso/gateway/internal/connector"
	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/secrets"
	"github.com/mqglobal/goso/gateway/internal/security"
	"github.com/mqglobal/goso/gateway/internal/skill"
	"github.com/mqglobal/goso/gateway/internal/store"
	"github.com/mqglobal/goso/gateway/internal/tts"
	"github.com/mqglobal/goso/gateway/internal/vault"
)

// Media provider chains. A media tool's tenant settings hold an ordered list
// of providers (the GoClaw builtin_tools settings shape):
//
//	{"providers":[{"provider":"gemini","model":"gemini-2.5-flash",
//	  "params":{},"timeout":120,"max_retries":2,"enabled":true}, ...]}
//
// "provider" names an env provider (GOSO_*_API_KEY registry) or a saved
// llm_providers row of the caller's tenant; its key stays in secrets and is
// never part of the settings. Entries run in order: each gets max_retries
// attempts (default 1) of at most timeout seconds (default 600), then the
// next entry is tried; only 429, 5xx and network errors are retried on the
// same provider. A missing "enabled" means on. With no tenant settings the
// tool falls back to its GOSO_* env config.

const (
	// MaxChainProviders caps one tool's provider list.
	MaxChainProviders = 10
	// MaxChainTimeout caps one attempt, in seconds.
	MaxChainTimeout = 900
	// MaxChainRetries caps attempts per provider.
	MaxChainRetries = 5

	// defaultChainTimeout matches GoClaw: image generation can take minutes.
	defaultChainTimeout = 600
	maxParamsBytes      = 8 << 10
	// geminiInlineBytes keeps one inline part under Gemini's 20MB request cap
	// after base64 (+33%). Larger media goes through the Gemini File API.
	geminiInlineBytes = 14 << 20
	// compatInlineBytes caps media sent base64-inline to an OpenAI-compatible
	// chat endpoint.
	compatInlineBytes = 20 << 20

	apiOpenAI    = "openai"
	apiGemini    = "gemini"
	apiAnthropic = "anthropic"
)

// MediaProviderEntry is one provider in a media tool's chain.
type MediaProviderEntry struct {
	// ProviderID is accepted for GoClaw compatibility and ignored: goso
	// resolves the provider by name.
	ProviderID string         `json:"provider_id,omitempty"`
	Provider   string         `json:"provider"`
	Model      string         `json:"model,omitempty"`
	Enabled    *bool          `json:"enabled,omitempty"`
	Timeout    int            `json:"timeout,omitempty"`
	MaxRetries int            `json:"max_retries,omitempty"`
	Params     map[string]any `json:"params,omitempty"`
}

// On reports whether the entry runs. A missing enabled means on.
func (e MediaProviderEntry) On() bool { return e.Enabled == nil || *e.Enabled }

// MediaToolSettings is the settings document of a chain tool.
type MediaToolSettings struct {
	Providers []MediaProviderEntry `json:"providers"`
}

// ChainTools are the builtin tools configured by a provider chain.
var ChainTools = []string{ToolReadImage, ToolReadDocument, ToolReadVideo, ToolReadAudio, ToolCreateImage}

// SettingsTool maps an alias to the tool that owns the settings
// (stt → read_audio, image_gen → create_image). "" when not a chain tool.
func SettingsTool(name string) string {
	switch strings.TrimSpace(name) {
	case ToolSTT, ToolReadAudio:
		return ToolReadAudio
	case ToolImageGen, ToolCreateImage:
		return ToolCreateImage
	case ToolReadImage, ToolReadDocument, ToolReadVideo:
		return strings.TrimSpace(name)
	}
	return ""
}

// SecretParamKey reports a params key that names a credential (api_key,
// access_token, secret, ...). "max_tokens" is not one.
func SecretParamKey(k string) bool {
	for _, w := range strings.FieldsFunc(strings.ToLower(k), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		switch w {
		case "key", "apikey", "secret", "token", "password", "passwd", "authorization", "auth", "bearer", "credential", "credentials":
			return true
		}
	}
	return false
}

// ParseMediaSettings decodes and validates a chain settings document. It is
// strict so a typo is reported instead of silently ignored.
func ParseMediaSettings(raw []byte) (*MediaToolSettings, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var s MediaToolSettings
	if err := dec.Decode(&s); err != nil {
		return nil, fmt.Errorf("settings: %s", strings.TrimPrefix(err.Error(), "json: "))
	}
	if err := ValidateMediaSettings(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

// ValidateMediaSettings checks bounds and that no params value looks like a
// credential: keys belong in the provider's secret, never in settings.
func ValidateMediaSettings(s *MediaToolSettings) error {
	if s == nil {
		return errors.New("settings are required")
	}
	if len(s.Providers) > MaxChainProviders {
		return fmt.Errorf("at most %d providers", MaxChainProviders)
	}
	for i, e := range s.Providers {
		at := fmt.Sprintf("providers[%d]", i)
		name := strings.TrimSpace(e.Provider)
		if name == "" || len(name) > 128 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
			return fmt.Errorf("%s.provider is required", at)
		}
		if len(e.Model) > 200 || strings.IndexFunc(e.Model, unicode.IsControl) >= 0 {
			return fmt.Errorf("%s.model is invalid", at)
		}
		if e.Timeout < 0 || e.Timeout > MaxChainTimeout {
			return fmt.Errorf("%s.timeout must be 0..%d seconds", at, MaxChainTimeout)
		}
		if e.MaxRetries < 0 || e.MaxRetries > MaxChainRetries {
			return fmt.Errorf("%s.max_retries must be 0..%d", at, MaxChainRetries)
		}
		if len(e.Params) > 0 {
			b, err := json.Marshal(e.Params)
			if err != nil || len(b) > maxParamsBytes {
				return fmt.Errorf("%s.params is too large", at)
			}
			for k := range e.Params {
				if SecretParamKey(k) {
					return fmt.Errorf("%s.params.%s: keys belong in the provider secret, not in tool settings", at, k)
				}
			}
		}
	}
	return nil
}

func toolSettingsStore() store.ToolSettingsStore {
	if ProviderStore == nil {
		return nil
	}
	ts, _ := ProviderStore.(store.ToolSettingsStore)
	return ts
}

func callerTenant(ctx context.Context) string {
	return store.NormalizeTenant(skill.TenantFrom(ctx))
}

// tenantChain returns the enabled entries of the caller tenant's settings
// for tool (aliases resolved). nil means "no tenant chain": use env.
func tenantChain(ctx context.Context, tool string) []MediaProviderEntry {
	return tenantChainFor(callerTenant(ctx), tool)
}

func tenantChainFor(tenant, tool string) []MediaProviderEntry {
	ts := toolSettingsStore()
	tool = SettingsTool(tool)
	if ts == nil || tool == "" {
		return nil
	}
	row, err := ts.GetToolSettings(tenant, tool)
	if err != nil || row == nil {
		return nil
	}
	s, err := ParseMediaSettings(row.Settings)
	if err != nil {
		return nil
	}
	var out []MediaProviderEntry
	for _, e := range s.Providers {
		if e.On() {
			e.Provider = strings.TrimSpace(e.Provider)
			e.Model = strings.TrimSpace(e.Model)
			out = append(out, e)
		}
	}
	return out
}

// ConfiguredForTenant is Configured plus the tenant's provider chain.
func ConfiguredForTenant(tenant, name string) bool {
	if Configured(name) {
		return true
	}
	if SettingsTool(name) == "" || len(tenantChainFor(store.NormalizeTenant(tenant), name)) == 0 {
		return false
	}
	switch name {
	case ToolCreateImage, ToolImageGen, ToolReadDocument:
		return workspaceConfigured()
	}
	return true
}

// ChainProviderStatus says how name resolves for tenant: "env" (env
// registry), "saved" (an enabled llm_providers row of that tenant),
// "disabled" (a disabled row of that tenant) or "" (not usable here). A row
// of another tenant reads as "".
func ChainProviderStatus(st store.StoreIface, tenant, name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if reg := llm.NewRegistry(); reg.Has(name) {
		switch reg.Get(name).(type) {
		case *llm.OpenAI, *llm.Anthropic:
			return "env"
		}
		return ""
	}
	if st == nil {
		return ""
	}
	row, err := st.GetLLMProvider(name)
	if err != nil || row == nil || !store.SameTenant(row.TenantID, tenant) {
		return ""
	}
	if !row.Enabled {
		return "disabled"
	}
	return "saved"
}

// EnvConfigured reports whether the GOSO_* env fallback of a chain tool is
// set (read_video has none).
func EnvConfigured(tool string) bool {
	switch SettingsTool(tool) {
	case ToolReadImage:
		return visionConfigured()
	case ToolReadAudio:
		return sttConfigured()
	case ToolCreateImage:
		return imageGenConfigured()
	case ToolReadDocument:
		return vault.DoclingEnabled() && vault.DoclingURL() != ""
	}
	return false
}

// ---- resolving one entry ----

type mediaTarget struct {
	provider string
	model    string
	api      string // apiOpenAI | apiGemini | apiAnthropic
	base     string // compat base, or the Gemini native base (…/v1beta)
	key      string
	llm      llm.Provider
	params   map[string]any
	timeout  time.Duration
	retries  int
}

func (t *mediaTarget) param(key string) string {
	if t == nil || t.params == nil {
		return ""
	}
	if v, ok := t.params[key].(string); ok {
		return strings.TrimSpace(v)
	}
	return ""
}

func (t *mediaTarget) paramInt(key string, def int) int {
	if t == nil || t.params == nil {
		return def
	}
	return intArg(t.params, key, def)
}

// permanentError is not retried on the same provider (bad input, wrong
// provider type); the chain still moves on to the next provider.
type permanentError struct{ msg string }

func (e *permanentError) Error() string { return e.msg }

func permanent(format string, a ...any) error { return &permanentError{msg: fmt.Sprintf(format, a...)} }

// providerHTTPError is a non-2xx provider reply. Detail is redacted.
type providerHTTPError struct {
	status int
	detail string
}

func (e *providerHTTPError) Error() string {
	if e.status == http.StatusUnauthorized || e.status == http.StatusForbidden {
		return "unauthorized"
	}
	if e.detail == "" {
		return fmt.Sprintf("http %d", e.status)
	}
	return fmt.Sprintf("http %d: %s", e.status, e.detail)
}

func retryable(err error) bool {
	if err == nil {
		return false
	}
	var pe *permanentError
	if errors.As(err, &pe) {
		return false
	}
	var he *providerHTTPError
	if errors.As(err, &he) {
		return he.status == http.StatusTooManyRequests || he.status >= 500
	}
	return true
}

func resolveMediaTarget(ctx context.Context, e MediaProviderEntry) (*mediaTarget, error) {
	name := strings.TrimSpace(e.Provider)
	var p llm.Provider
	reg := llm.NewRegistry()
	if reg.Has(name) {
		p = reg.Get(name)
	} else if ProviderStore != nil {
		row, err := ProviderStore.GetLLMProvider(name)
		if err != nil || row == nil || !store.SameTenant(row.TenantID, callerTenant(ctx)) {
			return nil, permanent("provider %q not found for this tenant", name)
		}
		if !row.Enabled {
			return nil, permanent("provider %q is disabled", name)
		}
		key := ""
		if b, gerr := secrets.Get(ProviderStore, llm.APIKeySecretName(name)); gerr == nil {
			key = string(b)
		}
		built, err := llm.Build(row.Name, row.Type, row.BaseURL, row.Model, key)
		if err != nil {
			return nil, permanent("provider %q cannot be built", name)
		}
		p = built
	} else {
		return nil, permanent("provider %q not found", name)
	}
	t := &mediaTarget{provider: name, params: e.Params}
	timeout := e.Timeout
	if timeout <= 0 {
		timeout = defaultChainTimeout
	}
	t.timeout = time.Duration(timeout) * time.Second
	t.retries = e.MaxRetries
	if t.retries <= 0 {
		t.retries = 1
	}
	switch v := p.(type) {
	case *llm.OpenAI:
		cp := *v
		t.base, t.key, t.model = cp.BaseURL, cp.APIKey, cp.Model
		if t.base == "" {
			t.base = "https://api.openai.com"
		}
		if e.Model != "" {
			cp.Model = e.Model
		}
		t.llm = &cp
		t.api = apiOpenAI
		if looksGemini(t.base) {
			t.api = apiGemini
		}
	case *llm.Anthropic:
		cp := *v
		t.base, t.key, t.model = cp.BaseURL, cp.APIKey, cp.Model
		if e.Model != "" {
			cp.Model = e.Model
		}
		cp.Client = mediaClient(t.timeout)
		t.llm = &cp
		t.api = apiAnthropic
	default:
		return nil, permanent("provider %q type cannot serve media", name)
	}
	if e.Model != "" {
		t.model = e.Model
	}
	switch strings.ToLower(t.param("api")) {
	case "":
	case apiGemini:
		if t.api == apiAnthropic {
			return nil, permanent("provider %q: api=gemini needs an OpenAI-compatible provider", name)
		}
		t.api = apiGemini
	case apiOpenAI, "openai-compat", "chat":
		if t.api == apiAnthropic {
			return nil, permanent("provider %q: api=openai needs an OpenAI-compatible provider", name)
		}
		t.api = apiOpenAI
	case "images", "transcriptions":
		// endpoint hints read by the tool, not an API family
	default:
		return nil, permanent("provider %q: unknown params.api", name)
	}
	if t.api == apiGemini {
		t.base = geminiNativeBase(t.base)
	}
	if t.model == "" {
		return nil, permanent("provider %q has no model; set model in the tool settings", name)
	}
	return t, nil
}

func looksGemini(base string) bool {
	u, err := url.Parse(strings.TrimSpace(base))
	return err == nil && strings.EqualFold(u.Hostname(), "generativelanguage.googleapis.com")
}

// geminiNativeBase turns the OpenAI-compatible base
// (https://generativelanguage.googleapis.com/v1beta/openai) into the native
// one (…/v1beta).
func geminiNativeBase(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	base = strings.TrimSuffix(base, "/openai")
	u, err := url.Parse(base)
	if err == nil && (u.Path == "" || u.Path == "/") {
		return base + "/v1beta"
	}
	return base
}

// geminiModel strips "models/" and an OpenRouter-style "google/" prefix.
func geminiModel(m string) string {
	m = strings.TrimPrefix(strings.TrimSpace(m), "models/")
	return strings.TrimPrefix(m, "google/")
}

var versionSeg = regexp.MustCompile(`^v[0-9]+[a-z0-9]*$`)

// compatURL joins an OpenAI-compatible base and an endpoint path
// ("chat/completions"). A base ending in a version segment (/v1, /api/v1)
// or Gemini's /openai gets the path directly; a bare host gets /v1/.
func compatURL(base, path string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if strings.HasSuffix(base, "/"+path) {
		return base
	}
	last := base[strings.LastIndex(base, "/")+1:]
	if versionSeg.MatchString(last) || last == "openai" {
		return base + "/" + path
	}
	return base + "/v1/" + path
}

// ---- running the chain ----

// chainAttempt is one failed provider in the report. It never has a key.
type chainAttempt struct {
	Provider string `json:"provider"`
	Model    string `json:"model,omitempty"`
	Error    string `json:"error"`
}

type chainCall func(ctx context.Context, t *mediaTarget) (map[string]any, error)

// runMediaChain tries each entry in order and returns the first success.
func runMediaChain(ctx context.Context, entries []MediaProviderEntry, call chainCall) (map[string]any, []chainAttempt, error) {
	var attempts []chainAttempt
	var last error
	for _, e := range entries {
		t, err := resolveMediaTarget(ctx, e)
		if err != nil {
			attempts = append(attempts, chainAttempt{Provider: e.Provider, Model: e.Model, Error: err.Error()})
			last = err
			continue
		}
		for try := 1; try <= t.retries; try++ {
			callCtx, cancel := context.WithTimeout(ctx, t.timeout)
			out, err := call(callCtx, t)
			cancel()
			if err == nil {
				if out == nil {
					out = map[string]any{}
				}
				out["provider"] = t.provider
				out["model"] = t.model
				if len(attempts) > 0 {
					out["fallback_from"] = attempts
				}
				return out, attempts, nil
			}
			last = errors.New(redactKey(err.Error(), t.key))
			if ctx.Err() != nil {
				attempts = append(attempts, chainAttempt{Provider: t.provider, Model: t.model, Error: last.Error()})
				return nil, attempts, ctx.Err()
			}
			if !retryable(err) {
				break
			}
		}
		attempts = append(attempts, chainAttempt{Provider: t.provider, Model: t.model, Error: last.Error()})
	}
	if last == nil {
		last = errors.New("no providers configured")
	}
	return nil, attempts, last
}

func chainFailed(name, what string, attempts []chainAttempt, err error) *connector.InvokeResult {
	msg := what + " failed"
	if err != nil {
		msg += ": " + err.Error()
	}
	c := map[string]any{"error": msg}
	if len(attempts) > 0 {
		c["attempts"] = attempts
	}
	return &connector.InvokeResult{Tool: name, Connector: ConnectorName, Status: "error", Content: c}
}

func redactKey(s, key string) string {
	if key != "" {
		s = strings.ReplaceAll(s, key, "[redacted]")
	}
	return tts.Redact(s)
}

// ---- HTTP ----

// providerJSON sends one JSON (or raw) request through the SSRF guard and
// decodes a capped JSON reply into out (when non-nil). Errors never carry
// the key; non-2xx bodies are redacted and clipped.
func providerJSON(ctx context.Context, t *mediaTarget, method, endpoint string, headers map[string]string, body []byte, limit int64, out any) (http.Header, error) {
	if err := security.CheckURL(endpoint); err != nil {
		return nil, permanent("endpoint blocked")
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, rd)
	if err != nil {
		return nil, permanent("invalid endpoint")
	}
	if body != nil && headers["Content-Type"] == "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if t != nil && t.key != "" {
		switch t.api {
		case apiGemini:
			req.Header.Set("x-goog-api-key", t.key)
		default:
			req.Header.Set("Authorization", "Bearer "+t.key)
		}
	}
	timeout := mediaCallTimeout
	if t != nil && t.timeout > 0 {
		timeout = t.timeout
	}
	resp, err := mediaClient(timeout).Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, errors.New("timed out")
		}
		var ne net.Error
		if errors.As(err, &ne) && ne.Timeout() {
			return nil, errors.New("timed out")
		}
		if strings.Contains(strings.ToLower(err.Error()), "ssrf") {
			return nil, permanent("endpoint blocked")
		}
		return nil, errors.New("request failed")
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, errors.New("read failed")
	}
	if int64(len(b)) > limit {
		return nil, permanent("response too large")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		key := ""
		if t != nil {
			key = t.key
		}
		detail := strings.TrimSpace(redactKey(string(b), key))
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return nil, &providerHTTPError{status: resp.StatusCode, detail: detail}
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return nil, errors.New("bad provider response")
		}
	}
	return resp.Header, nil
}
