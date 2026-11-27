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
	// ChannelContext is the English context JSON sent as `state`.
	ChannelContext   string
	ContextJSON      []byte
	ContextVersion   string
	ContextHash      string
	QuestionsVersion string
	Instructions     map[string]string
	CriteriaTrue     string
	CriteriaFalse    string
	YesAt            float64
	NoAt             float64
	QuestionsHash    string
	ThresholdVersion string
	ConfigErr        string
	// QuestionsErr is set when the question file cannot be loaded.
	// Kept so a questions-only failure is visible in logs.
	QuestionsErr string
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
	ctxRaw, ctxErr := loadFile("GOSO_JEV_CONTEXT_FILE", contextJSON)
	qRaw, qErr := loadFile("GOSO_JEV_QUESTIONS_FILE", questionsJSON)
	thRaw, thErr := loadFile("GOSO_JEV_THRESHOLDS_FILE", thresholdsJSON)
	if ctxErr != "" || qErr != "" || thErr != "" {
		c.ConfigErr = firstNonEmpty(ctxErr, qErr, thErr)
		c.QuestionsErr = c.ConfigErr
		return c
	}
	c.ContextHash = shortHash(ctxRaw)
	c.QuestionsHash = shortHash(qRaw)
	if err := applyContext(&c, ctxRaw); err != nil {
		c.ConfigErr = "context_invalid"
		c.QuestionsErr = c.ConfigErr
		return c
	}
	if err := applyQuestions(&c, qRaw); err != nil {
		c.ConfigErr = "questions_invalid"
		c.QuestionsErr = c.ConfigErr
		return c
	}
	if err := applyThresholds(&c, thRaw); err != nil {
		c.ConfigErr = "thresholds_invalid"
		c.QuestionsErr = c.ConfigErr
		return c
	}
	return c
}

func loadFile(envKey string, embedded []byte) ([]byte, string) {
	if p := strings.TrimSpace(os.Getenv(envKey)); p != "" {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, envKey
		}
		return b, ""
	}
	return embedded, ""
}

func shortHash(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func applyContext(c *Config, raw []byte) error {
	var f struct {
		Version  string `json:"version"`
		Company  string `json:"company"`
		Audience string `json:"audience"`
		Task     string `json:"task"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	if strings.TrimSpace(f.Version) == "" || strings.TrimSpace(f.Company) == "" || strings.TrimSpace(f.Audience) == "" || strings.TrimSpace(f.Task) == "" {
		return errQuestions
	}
	text := string(raw)
	if strings.Contains(text, "{{content}}") || strings.Contains(text, "{{message}}") {
		return errQuestions
	}
	c.ContextVersion = strings.TrimSpace(f.Version)
	c.ContextJSON = raw
	c.ChannelContext = text
	return nil
}

type questionFile struct {
	Version   string `json:"version"`
	Relevance struct {
		Instructions map[string]string `json:"instructions"`
		Criteria     struct {
			True  string `json:"true"`
			False string `json:"false"`
		} `json:"criteria"`
	} `json:"relevance"`
}

func applyQuestions(c *Config, raw []byte) error {
	var f questionFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	if strings.TrimSpace(f.Version) == "" || len(f.Relevance.Instructions) == 0 {
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
	c.QuestionsVersion = strings.TrimSpace(f.Version)
	c.Instructions = f.Relevance.Instructions
	c.CriteriaTrue = f.Relevance.Criteria.True
	c.CriteriaFalse = f.Relevance.Criteria.False
	return nil
}

func applyThresholds(c *Config, raw []byte) error {
	var f struct {
		Version string  `json:"version"`
		YesAt   float64 `json:"yes_at"`
		NoAt    float64 `json:"no_at"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	if strings.TrimSpace(f.Version) == "" || !(f.NoAt > 0 && f.NoAt < f.YesAt && f.YesAt < 1) {
		return errQuestions
	}
	c.ThresholdVersion = strings.TrimSpace(f.Version)
	c.YesAt = f.YesAt
	c.NoAt = f.NoAt
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
