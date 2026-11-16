// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package apikey

import (
	"errors"
	"log"
	"strconv"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// ErrStore is returned when a key could not be written to the database. The
// key is then not issued (create) or stays active (revoke).
var ErrStore = errors.New("api key store failed")

// ErrLoad is returned when stored keys cannot be read back at startup.
var ErrLoad = errors.New("api key load failed")

type keySaver interface {
	PutAPIKey(store.APIKeyRow) error
}

type keyDB interface {
	keySaver
	ListAPIKeys() ([]store.APIKeyRow, error)
}

func (r *Registry) persist(row *record) error {
	if r.save == nil {
		return nil
	}
	err := r.save.PutAPIKey(store.APIKeyRow{
		ID:        row.ID,
		Name:      row.Name,
		Prefix:    row.Prefix,
		Hash:      row.Hash,
		TenantID:  row.TenantID,
		Scopes:    row.Scopes,
		CreatedAt: row.CreatedAt,
		ExpiresAt: row.ExpiresAt,
		RevokedAt: row.RevokedAt,
	})
	if err != nil {
		log.Printf("goso api keys: save failed")
		return ErrStore
	}
	return nil
}

// Load reads issued keys from a SQLite or Postgres store and returns a
// registry that writes through to it, so keys survive a restart. A nil or
// in-memory store returns (nil, nil): the caller keeps the memory registry.
func Load(st store.StoreIface) (*Registry, error) {
	db := keySQL(st)
	if db == nil {
		return nil, nil
	}
	rows, err := db.ListAPIKeys()
	if err != nil {
		return nil, ErrLoad
	}
	return fromRows(rows, db)
}

func keySQL(st store.StoreIface) keyDB {
	switch s := st.(type) {
	case *store.SQLiteStore:
		if s == nil {
			return nil
		}
		return s
	case *store.PostgresStore:
		if s == nil || s.SQLiteStore == nil {
			return nil
		}
		return s.SQLiteStore
	default:
		return nil
	}
}

func fromRows(rows []store.APIKeyRow, db keySaver) (*Registry, error) {
	r := New()
	r.save = db
	var maxSeq int64
	for _, row := range rows {
		if row.ID == "" || row.Hash == "" || r.rows[row.ID] != nil {
			return nil, ErrLoad
		}
		rec := &record{
			ID:        row.ID,
			Name:      row.Name,
			Prefix:    row.Prefix,
			Hash:      row.Hash,
			TenantID:  store.NormalizeTenant(row.TenantID),
			Scopes:    append([]string(nil), row.Scopes...),
			CreatedAt: row.CreatedAt,
			ExpiresAt: row.ExpiresAt,
			RevokedAt: row.RevokedAt,
		}
		r.rows[rec.ID] = rec
		if rec.RevokedAt.IsZero() {
			r.byHash[rec.Hash] = rec.ID
		}
		if n, err := strconv.ParseInt(strings.TrimPrefix(rec.ID, "ak_"), 10, 64); err == nil && n > maxSeq {
			maxSeq = n
		}
	}
	r.seq.Store(maxSeq)
	return r, nil
}
