// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package skillimport

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"strings"
	"unicode/utf8"
)

// loadDenyFile reads the local denylist. The path is required. A missing,
// empty, or malformed file is an error. Names from the file are not returned
// in the error text.
func loadDenyFile(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errConfig("deny file is missing")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, errConfig("deny file is missing")
		}
		return nil, errConfig("deny file is unreadable")
	}
	if !utf8.Valid(raw) || bytes.Contains(raw, []byte{0}) {
		return nil, errConfig("deny file is malformed")
	}
	var ids []string
	sc := bufio.NewScanner(bytes.NewReader(raw))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !validDenyID(line) {
			return nil, errConfig("deny file is malformed")
		}
		ids = append(ids, line)
	}
	if err := sc.Err(); err != nil {
		return nil, errConfig("deny file is malformed")
	}
	if len(ids) == 0 {
		return nil, errConfig("deny file is empty")
	}
	return ids, nil
}

func validDenyID(s string) bool {
	if s == "" || len(s) > 200 || strings.ContainsAny(s, " \t") {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == ':' || r == '-':
		default:
			return false
		}
	}
	return true
}

func listed(ids []string, tenant string) bool {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return false
	}
	for _, id := range ids {
		if strings.EqualFold(strings.TrimSpace(id), tenant) {
			return true
		}
	}
	return false
}

// deniedCount is how many of the explicit tenant values are on the list.
// The same value is counted once. The names are not included.
func deniedCount(ids []string, tenants ...string) int {
	seen := map[string]struct{}{}
	n := 0
	for _, tenant := range tenants {
		key := strings.ToLower(strings.TrimSpace(tenant))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if listed(ids, tenant) {
			n++
		}
	}
	return n
}
