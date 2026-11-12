// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package sessionimport

import (
	"database/sql"
	"strings"

	"github.com/mqglobal/goso/gateway/internal/store"
)

// RollbackReport counts import-map rows for one destination tenant.
// It does not include tenant names, session ids, or message text.
type RollbackReport struct {
	DryRun   bool `json:"dry_run"`
	Links    int  `json:"links"`
	Sessions int  `json:"sessions"`
	Denied   int  `json:"denied"`
}

// Rollback removes sessions, messages, and session_import_links rows that this
// importer recorded for opt.Tenant. Dry-run counts them and writes nothing.
// Rows with no import-map entry, and rows for any other tenant, stay.
func Rollback(st SQLDB, opt Options) (*RollbackReport, error) {
	if st == nil {
		return nil, &ConfigError{Msg: "database is not open"}
	}
	db, pg := st.SQL()
	if db == nil {
		return nil, &ConfigError{Msg: "database is not open"}
	}
	opt.Tenant = strings.TrimSpace(opt.Tenant)
	if !store.TenantOK(opt.Tenant) {
		return nil, &ConfigError{Msg: "tenant is invalid"}
	}
	if _, rep, err := prepareDeny(opt.DenyFile, opt.Tenant, opt.Tenant); err != nil {
		out := &RollbackReport{DryRun: true}
		if rep != nil {
			out.Denied = rep.Denied
		}
		return out, err
	}
	w := &writer{db: db, pg: pg, dest: opt.Tenant, dry: opt.DryRun}
	ok, err := w.hasLinks()
	if err != nil {
		return nil, err
	}
	out := &RollbackReport{DryRun: opt.DryRun}
	if !ok {
		return out, nil
	}
	if err := w.db.QueryRow(w.q(`SELECT COUNT(*) FROM session_import_links WHERE tenant_id=?`), w.dest).Scan(&out.Links); err != nil {
		return nil, err
	}
	if err := w.db.QueryRow(w.q(`SELECT COUNT(*) FROM sessions WHERE tenant_id=? AND id IN (SELECT session_id FROM session_import_links WHERE tenant_id=?)`), w.dest, w.dest).Scan(&out.Sessions); err != nil {
		return nil, err
	}
	if opt.DryRun || out.Links == 0 {
		return out, nil
	}
	tx, err := w.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteImportFTS(tx, w.pg, w.dest); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(w.q(`DELETE FROM messages WHERE session_id IN (SELECT session_id FROM session_import_links WHERE tenant_id=?)`), w.dest); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(w.q(`DELETE FROM sessions WHERE tenant_id=? AND id IN (SELECT session_id FROM session_import_links WHERE tenant_id=?)`), w.dest, w.dest); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(w.q(`DELETE FROM session_import_links WHERE tenant_id=?`), w.dest); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

func deleteImportFTS(tx *sql.Tx, pg bool, tenant string) error {
	if pg {
		return nil
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='memory_fts'`).Scan(&n); err != nil || n == 0 {
		return err
	}
	_, err := tx.Exec(`DELETE FROM memory_fts WHERE session_id IN (SELECT session_id FROM session_import_links WHERE tenant_id=?)`, tenant)
	return err
}
