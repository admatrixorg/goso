// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package contextimport

import (
	"bytes"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// DenyError stops a run whose source or destination tenant is listed.
// Count is the number of distinct listed tenants on this run. The text
// does not include a tenant id.
type DenyError struct{ Count int }

func (e *DenyError) Error() string { return "tenant denied" }

// FileError stops a run before any work because the denylist cannot be used.
type FileError struct{ Kind string }

func (e *FileError) Error() string {
	switch e.Kind {
	case "missing":
		return "denylist is missing"
	case "empty":
		return "denylist is empty"
	default:
		return "denylist is malformed"
	}
}

// loadDenylist reads a local denylist. A missing, empty, or malformed file
// is an error. Names from the file are not returned to logs.
func loadDenylist(path string) (map[string]struct{}, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, &FileError{Kind: "missing"}
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &FileError{Kind: "missing"}
		}
		return nil, &FileError{Kind: "malformed"}
	}
	if !utf8.Valid(buf) || bytes.Contains(buf, []byte{0}) {
		return nil, &FileError{Kind: "malformed"}
	}
	ids := map[string]struct{}{}
	for _, line := range strings.Split(string(buf), "\n") {
		line = strings.TrimSpace(strings.TrimRight(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !store.TenantOK(line) {
			return nil, &FileError{Kind: "malformed"}
		}
		ids[strings.ToLower(line)] = struct{}{}
	}
	if len(ids) == 0 {
		return nil, &FileError{Kind: "empty"}
	}
	return ids, nil
}

// CheckDeny loads the denylist and counts listed tenants among source and dest.
// A missing, empty, or malformed file returns FileError. The count never
// includes the tenant text.
func CheckDeny(path, source, dest string) (int, error) {
	ids, err := loadDenylist(path)
	if err != nil {
		return 0, err
	}
	return deniedCount(ids, source, dest), nil
}

func deniedCount(ids map[string]struct{}, tenants ...string) int {
	seen := map[string]struct{}{}
	n := 0
	for _, tenant := range tenants {
		key := strings.ToLower(strings.TrimSpace(tenant))
		if key == "" {
			continue
		}
		if _, ok := ids[key]; !ok {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		n++
	}
	return n
}
