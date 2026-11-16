// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/security"
)

// MaxAudioBytes caps one synthesized clip read from a provider.
const MaxAudioBytes = 25 << 20

var (
	// ErrUnsupported is a configured provider that has no synthesis path here
	// (google, azure, edge). Status stays not_configured for the tool.
	ErrUnsupported = errors.New("provider not supported for synthesis")
	// ErrTooLong is text over the configured max_chars.
	ErrTooLong = errors.New("text too long")
	// ErrEmptyText is blank input.
	ErrEmptyText = errors.New("text is required")
	// ErrVoiceRequired is ElevenLabs without a voice id.
	ErrVoiceRequired = errors.New("voice is required")
)

// SynthOptions are per-call overrides. Empty fields use the stored config.
type SynthOptions struct {
	Voice string
	Model string
}

// Audio is one synthesized clip.
type Audio struct {
	Data     []byte
	MIME     string
	Ext      string
	Provider string
}

// Configured reports whether Synthesize can run (provider set, key if needed,
// enabled, and a provider this package can call).
func (s *Service) Configured() bool {
	cfg, _, _ := s.merged()
	if !cfg.enabled || !isConfigured(cfg) {
		return false
	}
	return synthSupported(cfg.provider)
}

func synthSupported(p string) bool {
	switch NormalizeProvider(p) {
	case ProviderOpenAI, ProviderElevenLabs:
		return true
	}
	return false
}

// Synthesize turns text into MP3 audio with the configured provider.
// OpenAI (and OpenAI-compatible endpoints) use POST /v1/audio/speech;
// ElevenLabs uses POST /v1/text-to-speech/{voice}. The endpoint is checked by
// the SSRF guard and the provider error body is redacted.
func (s *Service) Synthesize(ctx context.Context, text string, opt SynthOptions) (Audio, error) {
	cfg, _, _ := s.merged()
	if NormalizeProvider(cfg.provider) == ProviderNone || (RequiresKey(cfg.provider) && cfg.apiKey == "") {
		return Audio{}, ErrNotConfigured
	}
	if !cfg.enabled {
		return Audio{}, ErrDisabled
	}
	if !synthSupported(cfg.provider) {
		return Audio{}, ErrUnsupported
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return Audio{}, ErrEmptyText
	}
	if utf8.RuneCountInString(text) > cfg.maxChars {
		return Audio{}, ErrTooLong
	}
	voice := strings.TrimSpace(opt.Voice)
	if voice == "" {
		voice = cfg.voice
	}
	model := strings.TrimSpace(opt.Model)
	if model == "" {
		model = cfg.model
	}
	var (
		endpoint string
		payload  map[string]any
	)
	switch cfg.provider {
	case ProviderElevenLabs:
		if voice == "" {
			return Audio{}, ErrVoiceRequired
		}
		if model == "" {
			model = "eleven_multilingual_v2"
		}
		base, err := elevenBase(cfg.endpoint)
		if err != nil {
			return Audio{}, err
		}
		endpoint = base + "/v1/text-to-speech/" + url.PathEscape(voice) + "?output_format=mp3_44100_128"
		payload = map[string]any{"text": text, "model_id": model}
		if cfg.language != "" {
			payload["language_code"] = cfg.language
		}
	default:
		if voice == "" {
			voice = "alloy"
		}
		if model == "" {
			model = "gpt-4o-mini-tts"
		}
		endpoint = openAISpeechURL(cfg.endpoint)
		payload = map[string]any{"model": model, "input": text, "voice": voice, "response_format": "mp3"}
	}
	if err := security.CheckURL(endpoint); err != nil {
		return Audio{}, errors.New("endpoint blocked")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Audio{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Audio{}, errors.New("invalid endpoint")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "audio/mpeg")
	if cfg.provider == ProviderElevenLabs {
		req.Header.Set("xi-api-key", cfg.apiKey)
	} else if cfg.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.apiKey)
	}
	c := s.client(time.Duration(cfg.timeoutMS) * time.Millisecond)
	security.GuardClient(c)
	res, err := c.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return Audio{}, ctx.Err()
		}
		return Audio{}, errors.New("synthesis request failed")
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 2048))
		if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
			return Audio{}, errors.New("unauthorized")
		}
		msg := strings.TrimSpace(Redact(string(b)))
		if msg == "" {
			return Audio{}, fmt.Errorf("http %d", res.StatusCode)
		}
		return Audio{}, fmt.Errorf("http %d: %s", res.StatusCode, msg)
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, MaxAudioBytes+1))
	if err != nil {
		return Audio{}, errors.New("read failed")
	}
	if len(data) > MaxAudioBytes {
		return Audio{}, errors.New("audio too large")
	}
	if !looksMP3(data) {
		return Audio{}, errors.New("provider did not return mp3 audio")
	}
	return Audio{Data: data, MIME: "audio/mpeg", Ext: ".mp3", Provider: cfg.provider}, nil
}

// looksMP3 accepts an ID3 tag or an MPEG audio frame sync.
func looksMP3(b []byte) bool {
	if len(b) < 3 {
		return false
	}
	if bytes.HasPrefix(b, []byte("ID3")) {
		return true
	}
	return b[0] == 0xff && b[1]&0xe0 == 0xe0
}

func openAISpeechURL(endpoint string) string {
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if base == "" {
		return "https://api.openai.com/v1/audio/speech"
	}
	if strings.HasSuffix(base, "/audio/speech") {
		return base
	}
	base = strings.TrimSuffix(base, "/models")
	if strings.HasSuffix(base, "/v1") {
		return base + "/audio/speech"
	}
	return base + "/v1/audio/speech"
}

func elevenBase(endpoint string) (string, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return "https://api.elevenlabs.io", nil
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "", errors.New("invalid endpoint")
	}
	return u.Scheme + "://" + u.Host, nil
}
