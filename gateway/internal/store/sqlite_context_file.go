// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"time"
)

func (s *SQLiteStore) agentContextOwner(f ContextFile) error {
	if f.Scope != ContextScopeAgent {
		return nil
	}
	a, err := s.GetAgent(f.OwnerID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ErrNotFound
		}
		return err
	}
	if a == nil || !SameTenant(a.TenantID, f.TenantID) {
		return ErrNotFound
	}
	return nil
}

func (s *SQLiteStore) PutContextFile(in ContextFile) (*ContextFile, error) {
	f, err := normalizeContextFile(in)
	if err != nil {
		return nil, err
	}
	if err := s.agentContextOwner(f); err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	rows, err := tx.Query(`SELECT name, body FROM context_files WHERE tenant_id=? AND scope=? AND owner_id=?`,
		f.TenantID, f.Scope, f.OwnerID)
	if err != nil {
		return nil, err
	}
	var existing []ContextFile
	for rows.Next() {
		var name, body string
		if err := rows.Scan(&name, &body); err != nil {
			_ = rows.Close()
			return nil, err
		}
		existing = append(existing, ContextFile{Name: name, Body: body})
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := contextOwnerFits(existing, f.Name, f.Body); err != nil {
		return nil, err
	}
	exists := false
	for _, row := range existing {
		if row.Name == f.Name {
			exists = true
			break
		}
	}
	now := time.Now().UTC()
	updated := now
	if !f.UpdatedAt.IsZero() {
		updated = f.UpdatedAt.UTC()
	}
	// An empty created_at on conflict keeps the stored clock. A new row
	// still needs a value. The HTTP handler leaves CreatedAt zero; the
	// importer sets the source clock.
	created := ""
	if !f.CreatedAt.IsZero() {
		created = formatTime(f.CreatedAt.UTC())
	} else if !exists {
		created = formatTime(now)
	}
	id := newID()
	if _, err := tx.Exec(`INSERT INTO context_files(id, tenant_id, scope, owner_id, name, body, created_at, updated_at)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(tenant_id, scope, owner_id, name) DO UPDATE SET
			body=excluded.body,
			updated_at=excluded.updated_at,
			created_at=CASE WHEN excluded.created_at = '' THEN context_files.created_at ELSE excluded.created_at END`,
		id, f.TenantID, f.Scope, f.OwnerID, f.Name, f.Body, created, formatTime(updated)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	committed = true
	return s.GetContextFile(f.TenantID, f.Scope, f.OwnerID, f.Name)
}

func (s *SQLiteStore) GetContextFile(tenant, scope, ownerID, name string) (*ContextFile, error) {
	f, err := normalizeContextFile(ContextFile{
		TenantID: tenant, Scope: scope, OwnerID: ownerID, Name: name, Body: "x",
	})
	if err != nil {
		return nil, err
	}
	row := s.db.QueryRow(`SELECT id, tenant_id, scope, owner_id, name, body, created_at, updated_at
		FROM context_files WHERE tenant_id=? AND scope=? AND owner_id=? AND name=?`,
		f.TenantID, f.Scope, f.OwnerID, f.Name)
	got, err := scanContextFile(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !SameTenant(got.TenantID, f.TenantID) {
		return nil, ErrNotFound
	}
	return presentContext(got), nil
}

func (s *SQLiteStore) ListContextFiles(tenant, scope, ownerID string) ([]*ContextFile, error) {
	f, err := normalizeContextFile(ContextFile{
		TenantID: tenant, Scope: scope, OwnerID: ownerID, Name: "list.md", Body: "x",
	})
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, tenant_id, scope, owner_id, name, body, created_at, updated_at
		FROM context_files WHERE tenant_id=? AND scope=? AND owner_id=? ORDER BY name`,
		f.TenantID, f.Scope, f.OwnerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*ContextFile{}
	for rows.Next() {
		got, err := scanContextFile(rows)
		if err != nil {
			return nil, err
		}
		if !SameTenant(got.TenantID, f.TenantID) || got.OwnerID != f.OwnerID || got.Scope != f.Scope {
			continue
		}
		out = append(out, presentContext(got))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLiteStore) DeleteContextFile(tenant, scope, ownerID, name string) error {
	f, err := normalizeContextFile(ContextFile{
		TenantID: tenant, Scope: scope, OwnerID: ownerID, Name: name, Body: "x",
	})
	if err != nil {
		return err
	}
	res, err := s.db.Exec(`DELETE FROM context_files WHERE tenant_id=? AND scope=? AND owner_id=? AND name=?`,
		f.TenantID, f.Scope, f.OwnerID, f.Name)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func scanContextFile(sc scanner) (*ContextFile, error) {
	var f ContextFile
	var created, updated string
	if err := sc.Scan(&f.ID, &f.TenantID, &f.Scope, &f.OwnerID, &f.Name, &f.Body, &created, &updated); err != nil {
		return nil, err
	}
	f.TenantID = NormalizeTenant(f.TenantID)
	f.CreatedAt = parseTime(created)
	f.UpdatedAt = parseTime(updated)
	return &f, nil
}
