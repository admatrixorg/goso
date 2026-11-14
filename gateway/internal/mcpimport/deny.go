// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package mcpimport

import (
	"bufio"
	"os"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// LoadDenyFile reads one tenant id per line. Blank lines and lines starting
// with # are ignored. The file is local. This package does not ship a list.
func LoadDenyFile(path string) (map[string]struct{}, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errConfig("deny file")
	}
	defer f.Close()
	out := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !store.TenantOK(line) {
			return nil, errConfig("deny file")
		}
		out[strings.ToLower(line)] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		return nil, errConfig("deny file")
	}
	if len(out) == 0 {
		return nil, errConfig("deny file")
	}
	return out, nil
}

// Denied reports whether id is on the list. Comparison is case-insensitive.
func Denied(list map[string]struct{}, id string) bool {
	if len(list) == 0 {
		return false
	}
	_, ok := list[strings.ToLower(strings.TrimSpace(id))]
	return ok
}
