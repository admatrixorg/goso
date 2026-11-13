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
	plan, err := w.rollbackPlan()
	if err != nil {
		return nil, err
	}
	out.Links = len(plan.dropLinks)
	out.Sessions = len(plan.dropSessions)
	if opt.DryRun || (len(plan.dropMessages) == 0 && len(plan.dropSessions) == 0) {
		return out, nil
	}
	tx, err := w.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteFTSIDs(tx, w.pg, plan.dropMessages); err != nil {
		return nil, err
	}
	if err := deleteIDs(tx, w, `messages`, `id`, plan.dropMessages); err != nil {
		return nil, err
	}
	if err := deleteIDs(tx, w, `sessions`, `id`, plan.dropSessions); err != nil {
		return nil, err
	}
	if err := deleteIDs(tx, w, `session_import_links`, `session_id`, plan.dropLinks); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

type rollbackPlan struct {
	dropMessages []string
	dropSessions []string
	dropLinks    []string
}

func (w *writer) rollbackPlan() (rollbackPlan, error) {
	var plan rollbackPlan
	rows, err := w.db.Query(w.q(`SELECT session_id FROM session_import_links WHERE tenant_id=?`), w.dest)
	if err != nil {
		return plan, err
	}
	var sessions []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return plan, err
		}
		sessions = append(sessions, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return plan, err
	}
	rows.Close()
	if len(sessions) == 0 {
		return plan, nil
	}
	msgRows, err := w.db.Query(w.q(`SELECT id, session_id FROM messages WHERE session_id IN `+placeholders(len(sessions))), anys(sessions)...)
	if err != nil {
		return plan, err
	}
	real := map[string]int{}
	for _, id := range sessions {
		real[id] = 0
	}
	for msgRows.Next() {
		var id, sid string
		if err := msgRows.Scan(&id, &sid); err != nil {
			msgRows.Close()
			return plan, err
		}
		if importMessageID(sid, id) {
			plan.dropMessages = append(plan.dropMessages, id)
			continue
		}
		real[sid]++
	}
	if err := msgRows.Err(); err != nil {
		msgRows.Close()
		return plan, err
	}
	msgRows.Close()
	for _, id := range sessions {
		if real[id] > 0 {
			continue
		}
		plan.dropSessions = append(plan.dropSessions, id)
		plan.dropLinks = append(plan.dropLinks, id)
	}
	return plan, nil
}

func importMessageID(sessionID, id string) bool {
	prefix := sessionID + ":"
	if !strings.HasPrefix(id, prefix) {
		return false
	}
	rest := id[len(prefix):]
	if rest == "" {
		return false
	}
	for _, r := range rest {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func deleteIDs(tx *sql.Tx, w *writer, table, column string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	q := `DELETE FROM ` + table + ` WHERE ` + column + ` IN ` + placeholders(len(ids))
	if table == `session_import_links` {
		q += ` AND tenant_id=?`
		args := append(anys(ids), w.dest)
		_, err := tx.Exec(w.q(q), args...)
		return err
	}
	_, err := tx.Exec(w.q(q), anys(ids)...)
	return err
}

func deleteFTSIDs(tx *sql.Tx, pg bool, ids []string) error {
	if pg || len(ids) == 0 {
		return nil
	}
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='memory_fts'`).Scan(&n); err != nil || n == 0 {
		return err
	}
	_, err := tx.Exec(`DELETE FROM memory_fts WHERE id IN `+placeholders(len(ids)), anys(ids)...)
	return err
}
