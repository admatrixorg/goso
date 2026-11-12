// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"errors"
	"strings"
)

func (s *SQLiteStore) migrateImportSource() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS import_source_ids (
		tenant_id TEXT NOT NULL,
		kind TEXT NOT NULL,
		source_id TEXT NOT NULL,
		goso_id TEXT NOT NULL,
		PRIMARY KEY (tenant_id, kind, source_id)
	)`)
	return err
}

func (s *SQLiteStore) GetImportSource(tenant, kind, sourceID string) (string, bool, error) {
	tenant = strings.TrimSpace(tenant)
	kind = strings.TrimSpace(kind)
	sourceID = strings.TrimSpace(sourceID)
	if tenant == "" || kind == "" || sourceID == "" {
		return "", false, ErrNotFound
	}
	var id string
	err := s.db.QueryRow(
		`SELECT goso_id FROM import_source_ids WHERE tenant_id=? AND kind=? AND source_id=?`,
		tenant, kind, sourceID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

func (s *SQLiteStore) PutImportSource(tenant, kind, sourceID, gosoID string) error {
	tenant = strings.TrimSpace(tenant)
	kind = strings.TrimSpace(kind)
	sourceID = strings.TrimSpace(sourceID)
	gosoID = strings.TrimSpace(gosoID)
	if tenant == "" || kind == "" || sourceID == "" || gosoID == "" {
		return ErrNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var existing string
	err = tx.QueryRow(
		`SELECT goso_id FROM import_source_ids WHERE tenant_id=? AND kind=? AND source_id=?`,
		tenant, kind, sourceID,
	).Scan(&existing)
	if err == nil {
		return ErrExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.Exec(
		`INSERT INTO import_source_ids(tenant_id, kind, source_id, goso_id) VALUES(?,?,?,?)`,
		tenant, kind, sourceID, gosoID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListImportSources(tenant string) ([]ImportSource, error) {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return nil, ErrNotFound
	}
	rows, err := s.db.Query(
		`SELECT kind, source_id, goso_id FROM import_source_ids WHERE tenant_id=? ORDER BY kind, source_id`,
		tenant,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ImportSource
	for rows.Next() {
		var row ImportSource
		row.TenantID = tenant
		if err := rows.Scan(&row.Kind, &row.SourceID, &row.GosoID); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []ImportSource{}
	}
	return out, nil
}

func (s *SQLiteStore) DeleteImportSources(tenant string) (int, error) {
	tenant = strings.TrimSpace(tenant)
	if tenant == "" {
		return 0, ErrNotFound
	}
	res, err := s.db.Exec(`DELETE FROM import_source_ids WHERE tenant_id=?`, tenant)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *SQLiteStore) DeleteChannelConfig(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrNotFound
	}
	res, err := s.db.Exec(`DELETE FROM channel_config WHERE name=?`, name)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
