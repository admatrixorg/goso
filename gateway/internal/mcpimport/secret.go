// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// secretFlags are argv switches whose presence means the command carries a
// credential. The value is not kept. Longer names are listed before shorter
// ones so a scan of a whole command can match --access-token before --token.
var secretFlags = []string{
	"--access-token",
	"--api-key",
	"--token",
	"--secret",
	"--password",
	"--auth",
	"--key",
}

// kvPair is KEY=VALUE. The value is not kept.
var kvPair = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=.+`)

// jwtRe matches a three-segment JWT, including the dots.
var jwtRe = regexp.MustCompile(`[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}`)

// telegramRe matches a numeric bot id, a colon, and a token.
var telegramRe = regexp.MustCompile(`\d{5,}:[A-Za-z0-9_-]{20,}`)

// looksLikeKey reports a single token that is long enough to be a credential.
// Dots and colons are included so a JWT or a Telegram token is not stored.
func looksLikeKey(s string) bool {
	if len(s) < 24 {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '_' || r == '-' || r == '+' || r == '/' || r == '=' || r == '.' || r == ':':
		default:
			return false
		}
	}
	return true
}

func argSecret(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	if jwtRe.MatchString(t) || telegramRe.MatchString(t) {
		return true
	}
	low := strings.ToLower(t)
	for _, flag := range secretFlags {
		if low == flag || strings.HasPrefix(low, flag+"=") || strings.Contains(low, " "+flag) || strings.Contains(low, " "+flag+"=") {
			return true
		}
	}
	for _, field := range strings.Fields(t) {
		flow := strings.ToLower(field)
		for _, flag := range secretFlags {
			if flow == flag || strings.HasPrefix(flow, flag+"=") {
				return true
			}
		}
		if kvPair.MatchString(field) || looksLikeKey(field) {
			return true
		}
	}
	return looksLikeKey(t)
}

// classifyArgs returns the joined safe arguments. A secret argument returns
// hold=true and an empty join so the value is not retained.
func classifyArgs(raw json.RawMessage) (joined string, hold bool, bad bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte("[]")) {
		return "", false, false
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return "", false, true
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		var s string
		if err := json.Unmarshal(bytes.TrimSpace(item), &s); err != nil {
			return "", false, true
		}
		if argSecret(s) {
			return "", true, false
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, " "), false, false
}

func pathHasKey(path string) bool {
	if jwtRe.MatchString(path) || telegramRe.MatchString(path) {
		return true
	}
	for _, seg := range strings.Split(path, "/") {
		if argSecret(seg) {
			return true
		}
	}
	return false
}
