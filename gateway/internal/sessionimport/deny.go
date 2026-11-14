// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package sessionimport

import (
	"bufio"
	"os"
	"strings"
	"unicode"
)

// DenyGate checks the denylist before any database open. The report, when
// non-nil, has a denied count and no tenant names.
func DenyGate(path, tenant, source string) (*Report, error) {
	_, rep, err := prepareDeny(path, tenant, source)
	return rep, err
}

// prepareDeny loads the local denylist. A missing, empty, or malformed file
// is an error before any write. When tenant or source is listed, the report
// carries only the count. The error text does not include a tenant id.
func prepareDeny(path, tenant, source string) (map[string]struct{}, *Report, error) {
	list, err := loadDeny(path)
	if err != nil {
		return nil, emptyReport(), err
	}
	n := 0
	if denyListed(list, tenant) {
		n++
	}
	if denyListed(list, source) && !strings.EqualFold(strings.TrimSpace(tenant), strings.TrimSpace(source)) {
		n++
	}
	if n > 0 {
		rep := emptyReport()
		rep.Denied = n
		return list, rep, &ConfigError{Msg: "tenant is denied"}
	}
	return list, nil, nil
}

func emptyReport() *Report {
	return &Report{DryRun: true, Tenants: []TenantCount{}, NotImported: []string{}}
}

func loadDeny(path string) (map[string]struct{}, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, &ConfigError{Msg: "denylist is missing"}
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, &ConfigError{Msg: "denylist is missing"}
	}
	defer f.Close()
	out := map[string]struct{}{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if !denyTokenOK(line) {
			return nil, &ConfigError{Msg: "denylist is malformed"}
		}
		out[strings.ToLower(line)] = struct{}{}
	}
	if err := sc.Err(); err != nil {
		return nil, &ConfigError{Msg: "denylist is malformed"}
	}
	if len(out) == 0 {
		return nil, &ConfigError{Msg: "denylist is empty"}
	}
	return out, nil
}

func denyTokenOK(s string) bool {
	if s == "" || len(s) > maxIDLen {
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

func denyListed(list map[string]struct{}, id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "" || list == nil {
		return false
	}
	_, ok := list[id]
	return ok
}
