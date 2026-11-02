// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package jev

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultModel          = "jev-1.14.0"
	defaultURL            = "https://api.typesafe.ai/v1/systemone"
	defaultTimeout        = 1200 * time.Millisecond
	defaultMaxCalls       = 2
	maxCallsLimit         = 8
	defaultCacheTTL       = 24 * time.Hour
	defaultPerRequestTok  = 48000
	defaultPerQuestionTok = 24000
	cacheMaxEntries       = 1024
)

// Config is the process environment plus the question file.
// The API key is read from the environment only. It is not an overlay field
// and it is never returned by GET /api/config.
type Config struct {
	Enabled           bool
	APIKey            string
	Model             string
	URL               string
	Timeout           time.Duration
	MaxCalls          int
	CacheTTL          time.Duration
	PerRequestTokens  int
	PerQuestionTokens int
	ChannelContext    string
	Instructions      map[string]string
	CriteriaTrue      string
	CriteriaFalse     string
	YesAt             float64
	NoAt              float64
	QuestionsHash     string
	QuestionsErr      string
}

// EnabledFlag is GOSO_JEV_ENABLED, ignoring the key.
func EnabledFlag() bool {
	return envTruthy(os.Getenv("GOSO_JEV_ENABLED"))
}

// KeySet reports a non-empty GOSO_JEV_API_KEY. The value is never returned.
func KeySet() bool {
	return strings.TrimSpace(os.Getenv("GOSO_JEV_API_KEY")) != ""
}

// Model is GOSO_JEV_MODEL or the pinned default jev-1.14.0.
func Model() string {
	return modelFromEnv()
}

// Active is the reply-path switch: the flag is on and a key is present.
// Anything else leaves knowledge injection exactly as it is today.
func Active() bool {
	return EnabledFlag() && KeySet()
}

func modelFromEnv() string {
	m := strings.TrimSpace(os.Getenv("GOSO_JEV_MODEL"))
	if m == "" {
		return defaultModel
	}
	return m
}

// LoadConfig reads server env and the question file. A broken question file
// sets QuestionsErr; callers fall back instead of dropping passages.
func LoadConfig() Config {
	c := Config{
		Enabled:           Active(),
		APIKey:            strings.TrimSpace(os.Getenv("GOSO_JEV_API_KEY")),
		Model:             modelFromEnv(),
		URL:               defaultURL,
		Timeout:           defaultTimeout,
		MaxCalls:          defaultMaxCalls,
		CacheTTL:          defaultCacheTTL,
		PerRequestTokens:  defaultPerRequestTok,
		PerQuestionTokens: defaultPerQuestionTok,
	}
	if u := strings.TrimSpace(os.Getenv("GOSO_JEV_URL")); u != "" {
		c.URL = u
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("GOSO_JEV_TIMEOUT_MS"))); err == nil && n > 0 && n <= 10000 {
		c.Timeout = time.Duration(n) * time.Millisecond
	}
	if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("GOSO_JEV_MAX_CALLS"))); err == nil && n > 0 {
		if n > maxCallsLimit {
			n = maxCallsLimit
		}
		c.MaxCalls = n
	}
	if d, err := time.ParseDuration(strings.TrimSpace(os.Getenv("GOSO_JEV_CACHE_TTL"))); err == nil && d > 0 {
		c.CacheTTL = d
	}
	raw := questionsJSON
	if p := strings.TrimSpace(os.Getenv("GOSO_JEV_QUESTIONS_FILE")); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			c.QuestionsErr = "questions_file"
			return c
		}
		raw = b
	}
	sum := sha256.Sum256(raw)
	c.QuestionsHash = hex.EncodeToString(sum[:])
	if err := applyQuestions(&c, raw); err != nil {
		c.QuestionsErr = "questions_invalid"
	}
	return c
}

type questionFile struct {
	Channel struct {
		Context string `json:"context"`
	} `json:"channel"`
	Relevance struct {
		Instructions map[string]string `json:"instructions"`
		Criteria     struct {
			True  string `json:"true"`
			False string `json:"false"`
		} `json:"criteria"`
		YesAt float64 `json:"yes_at"`
		NoAt  float64 `json:"no_at"`
	} `json:"relevance"`
}

func applyQuestions(c *Config, raw []byte) error {
	var f questionFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	ctx := strings.TrimSpace(f.Channel.Context)
	if ctx == "" || strings.Contains(ctx, "{{content}}") || strings.Contains(ctx, "{{message}}") {
		return errQuestions
	}
	if len(f.Relevance.Instructions) == 0 {
		return errQuestions
	}
	joined := ""
	for _, v := range f.Relevance.Instructions {
		joined += v
	}
	if !strings.Contains(joined, "{{content}}") || !strings.Contains(joined, "{{message}}") {
		return errQuestions
	}
	if strings.TrimSpace(f.Relevance.Criteria.True) == "" || strings.TrimSpace(f.Relevance.Criteria.False) == "" {
		return errQuestions
	}
	yes, no := f.Relevance.YesAt, f.Relevance.NoAt
	if !(no > 0 && no < yes && yes < 1) {
		return errQuestions
	}
	c.ChannelContext = ctx
	c.Instructions = f.Relevance.Instructions
	c.CriteriaTrue = f.Relevance.Criteria.True
	c.CriteriaFalse = f.Relevance.Criteria.False
	c.YesAt = yes
	c.NoAt = no
	return nil
}

type questionError string

func (e questionError) Error() string { return string(e) }

const errQuestions questionError = "questions"

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
