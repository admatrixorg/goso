// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"strings"
	"time"
)

// APIKeyRow is one issued gateway key as stored. Only the SHA-256 hash and
// the display prefix are kept; the secret itself is never written.
type APIKeyRow struct {
	ID        string
	Name      string
	Prefix    string
	Hash      string
	TenantID  string
	Scopes    []string
	CreatedAt time.Time
	ExpiresAt time.Time
	RevokedAt time.Time
}

// ErrAPIKeyStore is the only error text callers see from the key table, so a
// driver message never reaches a response or a log line.
var ErrAPIKeyStore = errors.New("api key store failed")

func (s *SQLiteStore) migrateAPIKeys() error {
	if s == nil || s.db == nil {
		return ErrAPIKeyStore
	}
	for _, q := range []string{
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			prefix TEXT NOT NULL,
			hash TEXT NOT NULL UNIQUE,
			tenant_id TEXT NOT NULL,
			scopes TEXT NOT NULL,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL DEFAULT '',
			revoked_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_tenant ON api_keys(tenant_id)`,
	} {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// ListAPIKeys returns every stored key, revoked ones included.
func (s *SQLiteStore) ListAPIKeys() ([]APIKeyRow, error) {
	if s == nil || s.db == nil {
		return nil, ErrAPIKeyStore
	}
	rows, err := s.db.Query(`SELECT id, name, prefix, hash, tenant_id, scopes, created_at, expires_at, revoked_at
		FROM api_keys ORDER BY id`)
	if err != nil {
		return nil, ErrAPIKeyStore
	}
	defer rows.Close()
	out := []APIKeyRow{}
	for rows.Next() {
		var row APIKeyRow
		var scopes, created, expires, revoked string
		if err := rows.Scan(&row.ID, &row.Name, &row.Prefix, &row.Hash, &row.TenantID, &scopes, &created, &expires, &revoked); err != nil {
			return nil, ErrAPIKeyStore
		}
		if scopes != "" {
			row.Scopes = strings.Split(scopes, ",")
		}
		var ok bool
		if row.CreatedAt, ok = parseKeyTime(created); !ok || row.CreatedAt.IsZero() {
			return nil, ErrAPIKeyStore
		}
		if row.ExpiresAt, ok = parseKeyTime(expires); !ok {
			return nil, ErrAPIKeyStore
		}
		if row.RevokedAt, ok = parseKeyTime(revoked); !ok {
			return nil, ErrAPIKeyStore
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, ErrAPIKeyStore
	}
	return out, nil
}

// PutAPIKey inserts or updates one key row (create and revoke).
func (s *SQLiteStore) PutAPIKey(row APIKeyRow) error {
	if s == nil || s.db == nil {
		return ErrAPIKeyStore
	}
	if row.ID == "" || row.Hash == "" || strings.TrimSpace(row.TenantID) == "" {
		return ErrAPIKeyStore
	}
	_, err := s.db.Exec(`INSERT INTO api_keys(id, name, prefix, hash, tenant_id, scopes, created_at, expires_at, revoked_at)
		VALUES(?,?,?,?,?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET
			name=excluded.name,
			revoked_at=excluded.revoked_at`,
		row.ID, row.Name, row.Prefix, row.Hash, row.TenantID, strings.Join(row.Scopes, ","),
		formatTime(row.CreatedAt), keyTime(row.ExpiresAt), keyTime(row.RevokedAt))
	if err != nil {
		return ErrAPIKeyStore
	}
	return nil
}

func keyTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return formatTime(t)
}

func parseKeyTime(v string) (time.Time, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return time.Time{}, true
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}
