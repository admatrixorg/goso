// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package memdocimport

import (
	"bufio"
	"io"
	"strings"
)

// DeniedError is returned when the selected tenant, or a source tenant in the
// export, is on the local denylist. The text does not include a tenant id.
// Count is how many distinct tenants were denied. No row is written.
type DeniedError struct{ Count int }

func (e *DeniedError) Error() string { return "tenant is denied" }

// DenyList is a set of tenant ids and slugs read from a local file.
// The file is one token per line. Blank lines and lines starting with # are ignored.
// Matching is case-insensitive. This package does not name any tenant itself.
type DenyList struct {
	set map[string]struct{}
}

// ReadDenyList parses a denylist. A missing reader, a file with no tenant
// tokens, or a malformed line is an error. Blocking is the default: an empty
// file does not mean "deny nothing".
func ReadDenyList(r io.Reader) (*DenyList, error) {
	if r == nil {
		return nil, errConfig("denylist is missing")
	}
	d := &DenyList{set: map[string]struct{}{}}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.ContainsAny(line, " \t") || strings.ContainsRune(line, 0) {
			return nil, errConfig("denylist is malformed")
		}
		d.set[strings.ToLower(line)] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		return nil, errConfig("denylist is malformed")
	}
	if len(d.set) == 0 {
		return nil, errConfig("denylist is empty")
	}
	return d, nil
}

// Hit reports whether any value is on the list.
func (d *DenyList) Hit(values ...string) bool {
	if d == nil {
		return false
	}
	for _, v := range values {
		v = strings.ToLower(strings.TrimSpace(v))
		if v == "" {
			continue
		}
		if _, ok := d.set[v]; ok {
			return true
		}
	}
	return false
}
