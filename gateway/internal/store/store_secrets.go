// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import "strings"

// PutSecret upserts an encrypted blob by name (ciphertext only).
func (s *Store) PutSecret(row SecretRow) error {
	name := strings.TrimSpace(row.Name)
	if name == "" {
		return ErrNotFound
	}
	cp := SecretRow{
		Name:  name,
		Nonce: append([]byte(nil), row.Nonce...),
		CT:    append([]byte(nil), row.CT...),
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[name] = cp
	return nil
}

// GetSecret returns a copy of the encrypted blob.
func (s *Store) GetSecret(name string) (*SecretRow, error) {
	name = strings.TrimSpace(name)
	s.mu.RLock()
	defer s.mu.RUnlock()
	row, ok := s.secrets[name]
	if !ok {
		return nil, ErrNotFound
	}
	cp := SecretRow{
		Name:  row.Name,
		Nonce: append([]byte(nil), row.Nonce...),
		CT:    append([]byte(nil), row.CT...),
	}
	return &cp, nil
}
