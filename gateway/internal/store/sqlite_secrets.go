// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"strings"
)

// PutSecret upserts an encrypted blob by name.
func (s *SQLiteStore) PutSecret(row SecretRow) error {
	name := strings.TrimSpace(row.Name)
	if name == "" {
		return ErrNotFound
	}
	_, err := s.db.Exec(
		`INSERT INTO secrets(name, nonce, ct) VALUES(?,?,?)
		 ON CONFLICT(name) DO UPDATE SET nonce=excluded.nonce, ct=excluded.ct`,
		name, row.Nonce, row.CT,
	)
	return err
}

// GetSecret returns the encrypted blob.
func (s *SQLiteStore) GetSecret(name string) (*SecretRow, error) {
	name = strings.TrimSpace(name)
	var row SecretRow
	err := s.db.QueryRow(`SELECT name, nonce, ct FROM secrets WHERE name=?`, name).
		Scan(&row.Name, &row.Nonce, &row.CT)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
