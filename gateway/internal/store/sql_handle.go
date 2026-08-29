// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"database/sql"
	"strconv"
	"strings"
)

// txHandle is a dialect-aware transaction (SQLite ? or Postgres $n).
type txHandle interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
	Commit() error
	Rollback() error
}

// sqlHandle wraps *sql.DB. When pg is true, StoreIface SQL written with
// SQLite placeholders is rewritten for Postgres (no silent sqlite fallback).
type sqlHandle struct {
	db *sql.DB
	pg bool
}

func (h *sqlHandle) Exec(query string, args ...any) (sql.Result, error) {
	return h.db.Exec(h.q(query), args...)
}

func (h *sqlHandle) Query(query string, args ...any) (*sql.Rows, error) {
	return h.db.Query(h.q(query), args...)
}

func (h *sqlHandle) QueryRow(query string, args ...any) *sql.Row {
	return h.db.QueryRow(h.q(query), args...)
}

func (h *sqlHandle) Begin() (txHandle, error) {
	tx, err := h.db.Begin()
	if err != nil {
		return nil, err
	}
	return &sqlTx{tx: tx, pg: h.pg}, nil
}

func (h *sqlHandle) Close() error { return h.db.Close() }

func (h *sqlHandle) q(query string) string {
	if !h.pg {
		return query
	}
	return pgSQL(query)
}

type sqlTx struct {
	tx *sql.Tx
	pg bool
}

func (t *sqlTx) Exec(query string, args ...any) (sql.Result, error) {
	return t.tx.Exec(t.q(query), args...)
}

func (t *sqlTx) Query(query string, args ...any) (*sql.Rows, error) {
	return t.tx.Query(t.q(query), args...)
}

func (t *sqlTx) QueryRow(query string, args ...any) *sql.Row {
	return t.tx.QueryRow(t.q(query), args...)
}

func (t *sqlTx) Commit() error   { return t.tx.Commit() }
func (t *sqlTx) Rollback() error { return t.tx.Rollback() }

func (t *sqlTx) q(query string) string {
	if !t.pg {
		return query
	}
	return pgSQL(query)
}

// pgSQL maps SQLite-shaped DML onto Postgres: INSERT OR IGNORE, instr, and $n.
func pgSQL(q string) string {
	q = rewriteInsertOrIgnore(q)
	q = strings.ReplaceAll(q, "instr(", "strpos(")
	return rewritePlaceholders(q)
}

func rewriteInsertOrIgnore(q string) string {
	upper := strings.ToUpper(q)
	const needle = "INSERT OR IGNORE INTO"
	idx := strings.Index(upper, needle)
	if idx < 0 {
		return q
	}
	out := q[:idx] + "INSERT INTO" + q[idx+len(needle):]
	if strings.Contains(strings.ToUpper(out), "ON CONFLICT") {
		return out
	}
	return strings.TrimRight(out, " \t\n;") + " ON CONFLICT DO NOTHING"
}

func rewritePlaceholders(q string) string {
	var b strings.Builder
	b.Grow(len(q) + 8)
	n := 0
	for i := 0; i < len(q); i++ {
		if q[i] == '?' {
			n++
			b.WriteByte('$')
			b.WriteString(strconv.Itoa(n))
			continue
		}
		b.WriteByte(q[i])
	}
	return b.String()
}
