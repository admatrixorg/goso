// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
)

// secretNameParts are matched against a flag name after the leading dashes
// are removed and '-' and '_' are stripped. "--api_key" and "--apikey" both
// become "apikey".
var secretNameParts = []string{
	"credential",
	"bearer",
	"token",
	"secret",
	"passw",
	"auth",
	"key",
	"pat",
}

// shortSecretFlags are single-letter switches that carry a credential.
// Other short flags, such as -n, stay usable.
var shortSecretFlags = map[string]struct{}{
	"t": {},
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
	if strings.Contains(low, "authorization") || strings.Contains(low, "bearer ") {
		return true
	}
	for _, field := range strings.Fields(t) {
		if fieldSecret(field) {
			return true
		}
	}
	return fieldSecret(t)
}

func fieldSecret(field string) bool {
	if jwtRe.MatchString(field) || telegramRe.MatchString(field) || looksLikeKey(field) || kvPair.MatchString(field) {
		return true
	}
	low := strings.ToLower(field)
	if strings.Contains(low, "authorization") || strings.Contains(low, "bearer ") {
		return true
	}
	name, ok := flagName(field)
	if !ok {
		return false
	}
	if _, short := shortSecretFlags[name]; short {
		return true
	}
	for _, part := range secretNameParts {
		if strings.Contains(name, part) {
			return true
		}
	}
	return false
}

// flagName returns the switch name with one leading dash group removed and
// with '-' and '_' stripped. "-t=abc" and "--api_key=abc" yield "t" and "apikey".
func flagName(field string) (string, bool) {
	f := strings.TrimSpace(field)
	if strings.HasPrefix(f, "--") {
		f = f[2:]
	} else if strings.HasPrefix(f, "-") && len(f) > 1 {
		f = f[1:]
	} else {
		return "", false
	}
	if i := strings.IndexByte(f, '='); i >= 0 {
		f = f[:i]
	}
	f = strings.ToLower(f)
	f = strings.ReplaceAll(f, "-", "")
	f = strings.ReplaceAll(f, "_", "")
	if f == "" {
		return "", false
	}
	return f, true
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

// sTokenPath matches /s/<segment>/ at any length. A short token in that
// position is still a credential and must not be stored.
var sTokenPath = regexp.MustCompile(`(?i)(?:^|/)s/[^/]+/`)

func pathHasKey(path string) bool {
	if sTokenPath.MatchString(path) || jwtRe.MatchString(path) || telegramRe.MatchString(path) {
		return true
	}
	for _, seg := range strings.Split(path, "/") {
		if seg != "" && argSecret(seg) {
			return true
		}
	}
	return false
}
