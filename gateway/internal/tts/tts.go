// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tts

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/security"
)

const (
	ProviderNone       = "none"
	ProviderOpenAI     = "openai"
	ProviderElevenLabs = "elevenlabs"
	ProviderGoogle     = "google"
	ProviderAzure      = "azure"
	ProviderEdge       = "edge"

	ApplyOff   = "off"
	ApplyReply = "reply"
	ApplyAll   = "all"

	DefaultMaxChars  = 4096
	MinMaxChars      = 1
	MaxMaxChars      = 10000
	DefaultTimeoutMS = 15000
	MinTimeoutMS     = 1000
	MaxTimeoutMS     = 60000
	maxErrorRunes    = 400
)

var (
	ErrNotConfigured = errors.New("not_configured")
	ErrDisabled      = errors.New("disabled")
	ErrEnvOwned      = errors.New("env overlay")
	ErrConfirm       = errors.New("confirm required")
	ErrProvider      = errors.New("unknown provider")
	ErrApply         = errors.New("unknown auto_apply")
)

// Public is the GET body. Secrets are never included.
type Public struct {
	Provider   string `json:"provider"`
	Enabled    bool   `json:"enabled"`
	Configured bool   `json:"configured"`
	KeySet     bool   `json:"key_set"`
	EnvOwned   bool   `json:"env_owned"`
	Source     string `json:"source"`
	Voice      string `json:"voice,omitempty"`
	Model      string `json:"model,omitempty"`
	Language   string `json:"language,omitempty"`
	Region     string `json:"region,omitempty"`
	Endpoint   string `json:"endpoint,omitempty"`
	AutoApply  string `json:"auto_apply"`
	MaxChars   int    `json:"max_chars"`
	TimeoutMS  int    `json:"timeout_ms"`
}

// Write is the PUT body. api_key is write-only; empty keeps the stored key.
type Write struct {
	Provider  string `json:"provider"`
	Enabled   *bool  `json:"enabled"`
	APIKey    string `json:"api_key"`
	Voice     string `json:"voice"`
	Model     string `json:"model"`
	Language  string `json:"language"`
	Region    string `json:"region"`
	Endpoint  string `json:"endpoint"`
	AutoApply string `json:"auto_apply"`
	MaxChars  int    `json:"max_chars"`
	TimeoutMS int    `json:"timeout_ms"`
}

// TestResult is POST /api/tts/test. Error text is already redacted.
type TestResult struct {
	OK         bool   `json:"ok"`
	Configured bool   `json:"configured"`
	Provider   string `json:"provider"`
	Kind       string `json:"kind,omitempty"`
	LatencyMS  int    `json:"latency_ms"`
	Error      string `json:"error,omitempty"`
}

type mem struct {
	provider  string
	enabled   bool
	apiKey    string
	voice     string
	model     string
	language  string
	region    string
	endpoint  string
	autoApply string
	maxChars  int
	timeoutMS int
	set       bool
}

// Service is in-memory TTS operator config with env overlay.
type Service struct {
	mu     sync.Mutex
	mem    mem
	Client *http.Client
}

// Default returns an empty TTS overlay.
func Default() *Service {
	return New()
}

func New() *Service {
	return &Service{}
}

func (s *Service) client(timeout time.Duration) *http.Client {
	if timeout < time.Second {
		timeout = time.Duration(DefaultTimeoutMS) * time.Millisecond
	}
	if s != nil && s.Client != nil {
		c := *s.Client
		c.Timeout = timeout
		if c.CheckRedirect == nil {
			c.CheckRedirect = func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}
		}
		return &c
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func envMem() mem {
	enabled := true
	if v := strings.TrimSpace(os.Getenv("GOSO_TTS_ENABLED")); v != "" {
		enabled = v == "1" || strings.EqualFold(v, "true")
	}
	return mem{
		provider:  strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_TTS_PROVIDER"))),
		enabled:   enabled,
		apiKey:    strings.TrimSpace(os.Getenv("GOSO_TTS_API_KEY")),
		voice:     strings.TrimSpace(os.Getenv("GOSO_TTS_VOICE")),
		model:     strings.TrimSpace(os.Getenv("GOSO_TTS_MODEL")),
		language:  strings.TrimSpace(os.Getenv("GOSO_TTS_LANGUAGE")),
		region:    strings.TrimSpace(os.Getenv("GOSO_TTS_REGION")),
		endpoint:  strings.TrimSpace(os.Getenv("GOSO_TTS_ENDPOINT")),
		autoApply: strings.ToLower(strings.TrimSpace(os.Getenv("GOSO_TTS_AUTO_APPLY"))),
		maxChars:  atoiEnv("GOSO_TTS_MAX_CHARS"),
		timeoutMS: atoiEnv("GOSO_TTS_TIMEOUT_MS"),
		set:       strings.TrimSpace(os.Getenv("GOSO_TTS_PROVIDER")) != "" || strings.TrimSpace(os.Getenv("GOSO_TTS_API_KEY")) != "",
	}
}

func atoiEnv(name string) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return 0
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0
	}
	return n
}

func envEnabledSet() bool {
	return strings.TrimSpace(os.Getenv("GOSO_TTS_ENABLED")) != ""
}

func applyEnv(base mem, env mem) mem {
	out := base
	if env.provider != "" {
		out.provider = env.provider
	}
	if envEnabledSet() {
		out.enabled = env.enabled
	}
	if env.apiKey != "" {
		out.apiKey = env.apiKey
		// Env key never rides a memory endpoint.
		out.endpoint = env.endpoint
	} else if env.endpoint != "" {
		out.endpoint = env.endpoint
	}
	if env.voice != "" {
		out.voice = env.voice
	}
	if env.model != "" {
		out.model = env.model
	}
	if env.language != "" {
		out.language = env.language
	}
	if env.region != "" {
		out.region = env.region
	}
	if env.autoApply != "" {
		out.autoApply = env.autoApply
	}
	if env.maxChars > 0 {
		out.maxChars = env.maxChars
	}
	if env.timeoutMS > 0 {
		out.timeoutMS = env.timeoutMS
	}
	out.provider = NormalizeProvider(out.provider)
	out.autoApply = NormalizeApply(out.autoApply)
	out.maxChars = ClampMaxChars(out.maxChars)
	out.timeoutMS = ClampTimeout(out.timeoutMS)
	return out
}

func (s *Service) merged() (mem, bool, bool) {
	env := envMem()
	m := mem{provider: ProviderNone, enabled: true, autoApply: ApplyOff, maxChars: DefaultMaxChars, timeoutMS: DefaultTimeoutMS}
	if s != nil {
		s.mu.Lock()
		stored := s.mem
		s.mu.Unlock()
		if stored.set {
			m = stored
		}
	}
	out := applyEnv(m, env)
	return out, env.apiKey != "", env.set || envEnabledSet()
}

// Public returns non-secret TTS status.
func (s *Service) Public() Public {
	cfg, envOwned, overlay := s.merged()
	return publicOf(cfg, envOwned, overlay)
}

func publicOf(cfg mem, envOwned, overlay bool) Public {
	keySet := cfg.apiKey != ""
	src := "none"
	if overlay {
		src = "env"
	} else if cfg.set {
		src = "memory"
	}
	return Public{
		Provider:   cfg.provider,
		Enabled:    cfg.enabled,
		Configured: isConfigured(cfg),
		KeySet:     keySet,
		EnvOwned:   envOwned,
		Source:     src,
		Voice:      cfg.voice,
		Model:      cfg.model,
		Language:   cfg.language,
		Region:     cfg.region,
		Endpoint:   cfg.endpoint,
		AutoApply:  cfg.autoApply,
		MaxChars:   cfg.maxChars,
		TimeoutMS:  cfg.timeoutMS,
	}
}

func isConfigured(cfg mem) bool {
	p := NormalizeProvider(cfg.provider)
	if p == ProviderNone {
		return false
	}
	if RequiresKey(p) && cfg.apiKey == "" {
		return false
	}
	return true
}

// RequiresKey reports whether the provider stores a write-only credential.
func RequiresKey(provider string) bool {
	switch NormalizeProvider(provider) {
	case ProviderOpenAI, ProviderElevenLabs, ProviderGoogle, ProviderAzure:
		return true
	default:
		return false
	}
}

// NormalizeProvider maps unknown values to none.
func NormalizeProvider(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case ProviderOpenAI, ProviderElevenLabs, ProviderGoogle, ProviderAzure, ProviderEdge:
		return strings.ToLower(strings.TrimSpace(p))
	default:
		return ProviderNone
	}
}

// KnownProvider reports whether p is a selectable id (including none).
func KnownProvider(p string) bool {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case ProviderNone, ProviderOpenAI, ProviderElevenLabs, ProviderGoogle, ProviderAzure, ProviderEdge:
		return true
	default:
		return false
	}
}

// NormalizeApply maps unknown values to off.
func NormalizeApply(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case ApplyReply, ApplyAll:
		return strings.ToLower(strings.TrimSpace(v))
	default:
		return ApplyOff
	}
}

func ClampMaxChars(n int) int {
	if n <= 0 {
		return DefaultMaxChars
	}
	if n < MinMaxChars {
		return MinMaxChars
	}
	if n > MaxMaxChars {
		return MaxMaxChars
	}
	return n
}

func ClampTimeout(n int) int {
	if n <= 0 {
		return DefaultTimeoutMS
	}
	if n < MinTimeoutMS {
		return MinTimeoutMS
	}
	if n > MaxTimeoutMS {
		return MaxTimeoutMS
	}
	return n
}

// Put stores write-only credentials in process memory. Env-owned secrets are refused.
func (s *Service) Put(in Write) (Public, error) {
	if s == nil {
		return Public{}, ErrNotConfigured
	}
	_, envOwned, _ := s.merged()
	key := strings.TrimSpace(in.APIKey)
	if envOwned && key != "" {
		return s.Public(), ErrEnvOwned
	}
	if in.Provider != "" && !KnownProvider(in.Provider) {
		return Public{}, ErrProvider
	}
	if in.AutoApply != "" {
		switch strings.ToLower(strings.TrimSpace(in.AutoApply)) {
		case ApplyOff, ApplyReply, ApplyAll:
		default:
			return Public{}, ErrApply
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.mem.set {
		s.mem = mem{provider: ProviderNone, enabled: true, autoApply: ApplyOff, maxChars: DefaultMaxChars, timeoutMS: DefaultTimeoutMS, set: true}
	}
	if in.Provider != "" {
		s.mem.provider = NormalizeProvider(in.Provider)
	}
	if in.Enabled != nil {
		s.mem.enabled = *in.Enabled
	}
	if key != "" {
		s.mem.apiKey = key
	}
	s.mem.voice = strings.TrimSpace(in.Voice)
	s.mem.model = strings.TrimSpace(in.Model)
	s.mem.language = strings.TrimSpace(in.Language)
	s.mem.region = strings.TrimSpace(in.Region)
	if envOwned {
		s.mem.endpoint = ""
	} else {
		s.mem.endpoint = strings.TrimSpace(in.Endpoint)
	}
	if in.AutoApply != "" {
		s.mem.autoApply = NormalizeApply(in.AutoApply)
	}
	if in.MaxChars != 0 {
		s.mem.maxChars = ClampMaxChars(in.MaxChars)
	}
	if in.TimeoutMS != 0 {
		s.mem.timeoutMS = ClampTimeout(in.TimeoutMS)
	}
	s.mem.set = true
	return publicOf(applyEnv(s.mem, envMem()), envOwned, envMem().set || envEnabledSet()), nil
}

// ConfirmMatch accepts the current provider id or the literal "tts".
func ConfirmMatch(typed, provider string) bool {
	got := strings.ToLower(strings.TrimSpace(typed))
	if got == "" {
		return false
	}
	if got == "tts" {
		return true
	}
	return got == NormalizeProvider(provider) || got == strings.ToLower(strings.TrimSpace(provider))
}

// Clear wipes in-memory config. Env overlay is unchanged. Env-owned keys refuse secret clear.
func (s *Service) Clear(confirm string) (Public, error) {
	if s == nil {
		return Public{}, ErrNotConfigured
	}
	pub := s.Public()
	if !ConfirmMatch(confirm, pub.Provider) {
		return pub, ErrConfirm
	}
	_, envOwned, _ := s.merged()
	if envOwned {
		return pub, ErrEnvOwned
	}
	s.mu.Lock()
	s.mem = mem{}
	s.mu.Unlock()
	return s.Public(), nil
}

// Test validates local config and optionally probes endpoint. Failures are redacted.
func (s *Service) Test() TestResult {
	cfg, _, _ := s.merged()
	start := time.Now()
	out := TestResult{Provider: cfg.provider, Configured: isConfigured(cfg)}
	if NormalizeProvider(cfg.provider) == ProviderNone {
		out.Error = ErrNotConfigured.Error()
		out.LatencyMS = int(time.Since(start).Milliseconds())
		return out
	}
	if !cfg.enabled {
		out.Error = ErrDisabled.Error()
		out.LatencyMS = int(time.Since(start).Milliseconds())
		return out
	}
	if RequiresKey(cfg.provider) && cfg.apiKey == "" {
		out.Error = ErrNotConfigured.Error()
		out.LatencyMS = int(time.Since(start).Milliseconds())
		return out
	}
	if cfg.endpoint == "" {
		out.OK = true
		out.Kind = "local"
		out.LatencyMS = int(time.Since(start).Milliseconds())
		return out
	}
	err := s.probe(cfg)
	out.LatencyMS = int(time.Since(start).Milliseconds())
	if err != nil {
		out.Error = Redact(err.Error())
		return out
	}
	out.OK = true
	out.Kind = "http"
	return out
}

func (s *Service) probe(cfg mem) error {
	if err := security.CheckURL(cfg.endpoint); err != nil {
		return errors.New("probe failed")
	}
	req, err := http.NewRequest(http.MethodGet, cfg.endpoint, nil)
	if err != nil {
		return errors.New("invalid endpoint")
	}
	switch cfg.provider {
	case ProviderElevenLabs:
		req.Header.Set("xi-api-key", cfg.apiKey)
	case ProviderAzure:
		req.Header.Set("Ocp-Apim-Subscription-Key", cfg.apiKey)
	default:
		if cfg.apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
		}
	}
	c := s.client(time.Duration(cfg.timeoutMS) * time.Millisecond)
	security.GuardClient(c)
	res, err := c.Do(req)
	if err != nil {
		return fmt.Errorf("probe failed")
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		msg := strings.TrimSpace(string(body))
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			return errors.New("unauthorized")
		}
		if msg == "" {
			return fmt.Errorf("http %d", res.StatusCode)
		}
		return errors.New(Redact(msg))
	}
	return nil
}

var (
	secretKeySet = map[string]struct{}{
		"token": {}, "secret": {}, "password": {}, "hmac": {}, "hmac_key": {},
		"bot_token": {}, "access_token": {}, "api_key": {}, "authorization": {},
		"private_key": {}, "bearer": {}, "credential": {}, "xi-api-key": {},
		"xi_api_key": {}, "ocp-apim-subscription-key": {}, "subscription_key": {},
	}
	secretVal  = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|xi-[A-Za-z0-9]+|token=)`)
	jsonSecret = regexp.MustCompile(`(?i)"(authorization|api[_-]?key|secret|token|password|xi-api-key|subscription_key)"\s*:\s*"(?:\\.|[^"\\])*"`)
	kvSecret   = regexp.MustCompile(`(?i)(authorization|api[_-]?key|secret|token|password|xi-api-key|ocp-apim-subscription-key)\s*[:=]\s*\S+`)
)

// ContainsSecrets reports whether a JSON-shaped value still carries credentials.
func ContainsSecrets(v any) bool {
	return walkSecrets(v)
}

func walkSecrets(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			lk := strings.ToLower(k)
			if _, ok := secretKeySet[lk]; ok {
				if s, ok := val.(string); ok && strings.TrimSpace(s) != "" {
					return true
				}
			}
			if walkSecrets(val) {
				return true
			}
		}
	case []any:
		for _, item := range t {
			if walkSecrets(item) {
				return true
			}
		}
	case string:
		return secretVal.MatchString(t)
	}
	return false
}

// Redact strips authorization material from operator-visible errors.
func Redact(s string) string {
	out := jsonSecret.ReplaceAllString(s, `"$1":"[redacted]"`)
	out = kvSecret.ReplaceAllString(out, "$1=[redacted]")
	out = regexp.MustCompile(`(?i)Bearer\s+[^\s"'\\]+`).ReplaceAllString(out, "Bearer [redacted]")
	out = secretVal.ReplaceAllStringFunc(out, func(m string) string {
		lower := strings.ToLower(m)
		switch {
		case strings.HasPrefix(lower, "bearer"):
			return "Bearer [redacted]"
		case strings.HasPrefix(lower, "sk-"):
			return "sk-[redacted]"
		case strings.HasPrefix(lower, "gsk_"):
			return "gsk_[redacted]"
		case strings.HasPrefix(lower, "xai-"):
			return "xai-[redacted]"
		case strings.HasPrefix(lower, "aiza"):
			return "AIza[redacted]"
		default:
			return "[redacted]"
		}
	})
	if utf8.RuneCountInString(out) > maxErrorRunes {
		r := []rune(out)
		out = string(r[:maxErrorRunes]) + "…"
	}
	return out
}

// AsPublicJSON round-trips v and refuses secret-shaped payloads.
func AsPublicJSON(v any) (any, bool) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, false
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, false
	}
	if walkSecrets(decoded) {
		return nil, false
	}
	return decoded, true
}
