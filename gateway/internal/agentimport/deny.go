// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package agentimport

import (
	"os"
	"strings"
	"unicode"
)

// LoadDenylist reads one tenant id per line from a local file.
// A missing file, a file with no ids, and a malformed line are errors.
// The ids are not returned to logs or reports. Callers print only the count.
func LoadDenylist(path string) (map[string]struct{}, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, &ConfigError{Msg: "denylist is required"}
	}
	buf, err := os.ReadFile(path)
	if err != nil || len(buf) > 1<<20 {
		return nil, &ConfigError{Msg: "denylist is unreadable"}
	}
	out := map[string]struct{}{}
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !denylistToken(line) {
			return nil, &ConfigError{Msg: "denylist is malformed"}
		}
		out[strings.ToLower(line)] = struct{}{}
	}
	if len(out) == 0 {
		return nil, &ConfigError{Msg: "denylist is empty"}
	}
	return out, nil
}

func denylistToken(s string) bool {
	if s == "" || len(s) > 128 {
		return false
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// TenantDenied reports whether tenant is on the loaded list.
// The name is not included in errors.
func TenantDenied(list map[string]struct{}, tenant string) bool {
	return denied(list, tenant)
}

func denied(list map[string]struct{}, tenant string) bool {
	if len(list) == 0 {
		return false
	}
	_, ok := list[strings.ToLower(strings.TrimSpace(tenant))]
	return ok
}

// CountDenied is the number of distinct non-empty ids that are on the list.
func CountDenied(list map[string]struct{}, ids ...string) int {
	seen := map[string]struct{}{}
	for _, id := range ids {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" || !denied(list, id) {
			continue
		}
		seen[id] = struct{}{}
	}
	return len(seen)
}

func denyReady(list map[string]struct{}) error {
	if len(list) == 0 {
		return &ConfigError{Msg: "denylist is empty"}
	}
	return nil
}
