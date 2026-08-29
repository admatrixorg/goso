// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package config

import (
	"os"
	"strconv"
	"strings"
	"sync"
)

// Config holds gateway runtime configuration.
type Config struct {
	Port     int
	LogLevel string
	Env      string
}

// Load reads config from environment with sensible defaults.
func Load() Config {
	port := 8080
	if v := Lookup("GOSO_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	level := Lookup("GOSO_LOG_LEVEL")
	if level == "" {
		level = "info"
	}
	env := Lookup("GOSO_ENV")
	if env == "" {
		env = "development"
	}
	return Config{Port: port, LogLevel: level, Env: env}
}

// Editable maps public overlay keys to env names. Env wins when set.
var Editable = map[string]string{
	"log_level":  "GOSO_LOG_LEVEL",
	"quota_day":  "GOSO_QUOTA_DAY",
	"injection":  "GOSO_INJECTION",
	"ssrf":       "GOSO_SSRF",
	"heartbeat":  "GOSO_HEARTBEAT",
	"kg_extract": "GOSO_KG_EXTRACT",
	"cache_mode": "GOSO_ANTHROPIC_CACHE_MODE",
}

var envToField = func() map[string]string {
	out := make(map[string]string, len(Editable))
	for k, env := range Editable {
		out[env] = k
	}
	return out
}()

type overlayState struct {
	mu     sync.RWMutex
	values map[string]string
}

var overlay overlayState

// Lookup returns env if set, else a persisted overlay value for known keys.
func Lookup(envKey string) string {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		return v
	}
	overlay.mu.RLock()
	defer overlay.mu.RUnlock()
	if overlay.values == nil {
		return ""
	}
	if field, ok := envToField[envKey]; ok {
		return strings.TrimSpace(overlay.values[field])
	}
	return ""
}

// EnvOwned reports that the process environment currently owns envKey.
func EnvOwned(envKey string) bool {
	return strings.TrimSpace(os.Getenv(envKey)) != ""
}

// Overlay returns a copy of persisted overlay values.
func Overlay() map[string]string {
	overlay.mu.RLock()
	defer overlay.mu.RUnlock()
	return cloneValues(overlay.values)
}

// SetOverlay replaces the in-process overlay (env still wins on Lookup).
func SetOverlay(values map[string]string) {
	overlay.mu.Lock()
	defer overlay.mu.Unlock()
	overlay.values = cloneValues(values)
}

// ResetOverlay clears process overlay. Tests must call this to avoid leakage.
func ResetOverlay() {
	overlay.mu.Lock()
	defer overlay.mu.Unlock()
	overlay.values = nil
}

func cloneValues(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(v)
	}
	return out
}
