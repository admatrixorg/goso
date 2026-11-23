// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"time"
)

// VaultVersion is one stored snapshot of a document body.
type VaultVersion struct {
	ID          string `json:"id"`
	TenantID    string `json:"tenant_id"`
	DocID       string `json:"doc_id"`
	Version     int    `json:"version"`
	Title       string `json:"title,omitempty"`
	Body        string `json:"body,omitempty"`
	ChangedBy   string `json:"changed_by,omitempty"`
	CustomScope string `json:"custom_scope,omitempty"`
	CreatedAt   string `json:"created_at"`
}

// VaultVersionStore is the document history capability.
type VaultVersionStore interface {
	InsertVaultVersion(VaultVersion) (*VaultVersion, error)
	ListVaultVersions(tenant, docID string) []*VaultVersion
	GetVaultVersion(tenant, docID string, version int) (*VaultVersion, error)
}

// VaultVersions returns the history capability of st, or nil.
func VaultVersions(st StoreIface) VaultVersionStore {
	if x, ok := st.(VaultVersionStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateVaultVersions() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS vault_versions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			doc_id TEXT NOT NULL,
			version INTEGER NOT NULL DEFAULT 1,
			title TEXT NOT NULL DEFAULT '',
			body TEXT NOT NULL DEFAULT '',
			changed_by TEXT NOT NULL DEFAULT '',
			custom_scope TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS ux_vault_version ON vault_versions(doc_id, version)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// InsertVaultVersion stores the next snapshot for a document. The version
// number is one past the newest snapshot, so an edit always appends.
func (s *SQLiteStore) InsertVaultVersion(v VaultVersion) (*VaultVersion, error) {
	v.TenantID = NormalizeTenant(v.TenantID)
	var maxVersion int
	_ = s.db.QueryRow(`SELECT coalesce(max(version),0) FROM vault_versions WHERE doc_id=? AND tenant_id=?`,
		strings.TrimSpace(v.DocID), v.TenantID).Scan(&maxVersion)
	v.Version = maxVersion + 1
	if v.ID == "" {
		v.ID = "vv_" + newID()
	}
	if v.CreatedAt == "" {
		v.CreatedAt = formatTime(time.Now().UTC())
	}
	_, err := s.db.Exec(`INSERT INTO vault_versions(id, tenant_id, doc_id, version, title, body, changed_by, custom_scope, created_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		v.ID, v.TenantID, v.DocID, v.Version, v.Title, v.Body, v.ChangedBy, v.CustomScope, v.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

const vaultVersionColumns = `id, tenant_id, doc_id, version, title, body, changed_by, custom_scope, created_at`

func (s *SQLiteStore) ListVaultVersions(tenant, docID string) []*VaultVersion {
	rows, err := s.db.Query(`SELECT ` + vaultVersionColumns + ` FROM vault_versions WHERE tenant_id=? AND doc_id=? ORDER BY version DESC`,
		NormalizeTenant(tenant), strings.TrimSpace(docID))
	if err != nil {
		return []*VaultVersion{}
	}
	defer rows.Close()
	out := []*VaultVersion{}
	for rows.Next() {
		var v VaultVersion
		if rows.Scan(&v.ID, &v.TenantID, &v.DocID, &v.Version, &v.Title, &v.Body, &v.ChangedBy, &v.CustomScope, &v.CreatedAt) != nil {
			continue
		}
		out = append(out, &v)
	}
	return out
}

func (s *SQLiteStore) GetVaultVersion(tenant, docID string, version int) (*VaultVersion, error) {
	var v VaultVersion
	err := s.db.QueryRow(`SELECT ` + vaultVersionColumns + ` FROM vault_versions WHERE tenant_id=? AND doc_id=? AND version=?`,
		NormalizeTenant(tenant), strings.TrimSpace(docID), version).
		Scan(&v.ID, &v.TenantID, &v.DocID, &v.Version, &v.Title, &v.Body, &v.ChangedBy, &v.CustomScope, &v.CreatedAt)
	if err != nil {
		return nil, ErrNotFound
	}
	return &v, nil
}
