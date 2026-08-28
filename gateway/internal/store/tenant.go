// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"os"
	"strings"
	"unicode"
)

// DefaultTenant is the single-tenant / demo id (GoClaw Mode 1 "master" analogue).
const DefaultTenant = "default"

const maxTenantLen = 64

// ErrPostgresUnsupported is returned when a postgres DSN is supplied. StoreIface
// stays SQLite; do not half-open a broken PG driver.
var ErrPostgresUnsupported = errors.New("postgres is not supported in this build: SQLite only (see docs/qa/071-pgvector-path.md)")

// NormalizeTenant maps empty or invalid ids to DefaultTenant.
func NormalizeTenant(id string) string {
	id = strings.TrimSpace(id)
	if !TenantOK(id) {
		return DefaultTenant
	}
	return id
}

// TenantOK reports a conservative id: letters, digits, . _ - ; 1–64 chars.
func TenantOK(id string) bool {
	if id == "" || len(id) > maxTenantLen {
		return false
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.' {
			continue
		}
		return false
	}
	return true
}

// SameTenant compares after NormalizeTenant (empty == default).
func SameTenant(a, b string) bool {
	return NormalizeTenant(a) == NormalizeTenant(b)
}

// IsPostgresDSN reports postgres:// / postgresql:// (and postgres: URI forms).
func IsPostgresDSN(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return false
	}
	return strings.HasPrefix(s, "postgres://") ||
		strings.HasPrefix(s, "postgresql://") ||
		strings.HasPrefix(s, "postgres:") ||
		strings.HasPrefix(s, "postgresql:")
}

// RefusePostgres returns ErrPostgresUnsupported when GOSO_DATABASE_URL or path
// looks like Postgres. Fail closed; never open sqlite as a silent fallback.
func RefusePostgres(path string) error {
	if IsPostgresDSN(os.Getenv("GOSO_DATABASE_URL")) || IsPostgresDSN(path) {
		return ErrPostgresUnsupported
	}
	return nil
}
