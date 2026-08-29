// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package config

import (
	"strconv"
	"strings"
)

// PatchError is a validation or env-owned conflict on PUT /api/config.
type PatchError struct {
	Status  int
	Message string
}

func (e *PatchError) Error() string { return e.Message }

func badRequest(msg string) *PatchError {
	return &PatchError{Status: 400, Message: msg}
}

func conflict(msg string) *PatchError {
	return &PatchError{Status: 409, Message: msg}
}

var boolWords = map[string]string{
	"1": "1", "true": "1", "yes": "1", "on": "1",
	"0": "0", "false": "0", "no": "0", "off": "0",
}

func normalizeBoolWord(v string) (string, bool) {
	n, ok := boolWords[strings.ToLower(strings.TrimSpace(v))]
	return n, ok
}

func validateLogLevel(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "", nil
	}
	switch v {
	case "debug", "info", "warn", "warning", "error":
		if v == "warning" {
			return "warn", nil
		}
		return v, nil
	default:
		return "", badRequest("log_level must be debug, info, warn, or error")
	}
}

func validateQuotaDay(v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return "", badRequest("quota_day must be an integer >= 0")
	}
	return strconv.Itoa(n), nil
}

func validateInjection(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return "", nil
	}
	if v == "log" || v == "block" {
		return v, nil
	}
	return "", badRequest("injection must be log or block")
}

func validateCacheMode(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" || v == "none" || v == "full" {
		if v == "none" {
			return "", nil
		}
		return v, nil
	}
	return "", badRequest("cache_mode must be empty, none, or full")
}

func validateBoolFlag(name, v string) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", nil
	}
	n, ok := normalizeBoolWord(v)
	if !ok {
		return "", badRequest(name + " must be on or off")
	}
	return n, nil
}

func normalizeValue(key, raw string) (string, error) {
	switch key {
	case "log_level":
		return validateLogLevel(raw)
	case "quota_day":
		return validateQuotaDay(raw)
	case "injection":
		return validateInjection(raw)
	case "ssrf", "heartbeat", "kg_extract":
		return validateBoolFlag(key, raw)
	case "cache_mode":
		return validateCacheMode(raw)
	default:
		return "", badRequest("unknown field " + key)
	}
}

func isSecretKey(k string) bool {
	k = strings.ToLower(strings.TrimSpace(k))
	if k == "" {
		return false
	}
	if strings.Contains(k, "token") || strings.Contains(k, "secret") || strings.Contains(k, "password") {
		return true
	}
	if strings.Contains(k, "api_key") || strings.Contains(k, "master_key") || strings.Contains(k, "database_url") {
		return true
	}
	return false
}

// ApplyPatch validates values, refuses env-owned writes, and returns the merged overlay.
func ApplyPatch(current map[string]string, patch map[string]string) (map[string]string, error) {
	out := cloneValues(current)
	if out == nil {
		out = map[string]string{}
	}
	for key, raw := range patch {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, badRequest("empty field")
		}
		if isSecretKey(key) {
			return nil, badRequest("secret fields cannot be written")
		}
		envKey, ok := Editable[key]
		if !ok {
			return nil, badRequest("unknown field " + key)
		}
		if EnvOwned(envKey) {
			return nil, conflict("field is env-owned: " + key)
		}
		norm, err := normalizeValue(key, raw)
		if err != nil {
			return nil, err
		}
		if norm == "" {
			delete(out, key)
			continue
		}
		out[key] = norm
	}
	return out, nil
}
