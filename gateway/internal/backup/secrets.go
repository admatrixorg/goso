// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package backup

import (
	"encoding/json"
	"regexp"
	"strings"
)

var secretKeySet = map[string]struct{}{
	"token": {}, "secret": {}, "password": {}, "hmac": {}, "hmac_key": {},
	"bot_token": {}, "access_token": {}, "api_key": {}, "authorization": {},
	"private_key": {}, "bearer": {}, "credential": {}, "access_key": {},
	"secret_access_key": {}, "access_key_id": {},
}

var secretVal = regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|gk_[0-9a-f]{16,}|Bearer\s+[A-Za-z0-9._\-+=/]{8,}|ghp_[A-Za-z0-9]+|token=)`)

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
