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
// Other short flags, such as -n, stay usable. -k and -p are the common
// spellings for an API key and a password on an MCP command line.
var shortSecretFlags = map[string]struct{}{
	"t": {},
	"k": {},
	"p": {},
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
	if name, ok := headerName(field); ok {
		return nameSecret(name)
	}
	// "--header=X-Api-Key: value" carries the header name after the '='.
	// The switch is named "header", which is not itself a credential, so the
	// value side has to be classified too.
	if i := strings.IndexByte(field, '='); i >= 0 {
		if name, ok := headerName(field[i+1:]); ok {
			return nameSecret(name)
		}
	}
	// getopt allows the value to be glued to a short switch: "-kabc" is
	// "-k abc". Only a known credential switch is read that way, so "-n3"
	// keeps working.
	if attachedShortSecret(field) {
		return true
	}
	name, ok := flagName(field)
	if !ok {
		return false
	}
	return nameSecret(name)
}

// nameSecret reports a credential-shaped switch or header name. The caller
// strips the dashes or the colon first, so "x-api-key" and "-k" both land
// here as lowercase letters and digits.
func nameSecret(name string) bool {
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

// attachedShortSecret reports "-KVALUE" where K is a known credential switch.
// The glued form is one dash, a credential letter, and at least one more byte.
func attachedShortSecret(field string) bool {
	f := strings.TrimSpace(field)
	if len(f) < 3 || f[0] != '-' || f[1] == '-' {
		return false
	}
	_, short := shortSecretFlags[string(f[1])]
	return short
}

// headerName returns a lowercase header name with "-" and "_" stripped
// when the field is a header line. A header line is a name, a colon, and a
// value: "X-Api-Key: abc", "PRIVATE-TOKEN:abc", or the bare "x-api-key:".
// The colon is required. Without it a dashed *value* such as an api_key of
// "placeholder-minh-an-api-key" would read as a header name and every such
// row would be held, which is not what the field is.
func headerName(field string) (string, bool) {
	f := strings.TrimSpace(field)
	if f == "" || strings.HasPrefix(f, "-") {
		return "", false
	}
	k := strings.IndexByte(f, ':')
	if k < 0 {
		return "", false
	}
	f = strings.TrimSpace(f[:k])
	if strings.ContainsAny(f, " \t") || strings.ContainsAny(f, "{}\"[]") {
		return "", false
	}
	name := strings.ToLower(f)
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, "_", "")
	if name == "" {
		return "", false
	}
	return name, true
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
