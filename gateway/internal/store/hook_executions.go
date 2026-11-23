// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"sort"
	"strings"
	"time"
)

// HookExecution is one recorded run of a hook, the ledger GoClaw keeps in
// hook_executions. It is separate from the hook itself so a busy hook does not
// rewrite its own row.
type HookExecution struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	HookID     string `json:"hook_id,omitempty"`
	HookName   string `json:"hook_name,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	Event      string `json:"event"`
	InputHash  string `json:"input_hash,omitempty"`
	Decision   string `json:"decision"`
	DurationMS int    `json:"duration_ms"`
	Retry      int    `json:"retry"`
	DedupKey   string `json:"dedup_key,omitempty"`
	Error      string `json:"error,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// HookExecutionStore is the execution ledger capability.
type HookExecutionStore interface {
	InsertHookExecution(HookExecution) error
	ListHookExecutions(tenant, hookID, event, decision string, limit, offset int) ([]*HookExecution, int)
	HookExecutionStats(tenant string) map[string]int
}

// HookExecutions returns the execution ledger of st, or nil.
func HookExecutions(st StoreIface) HookExecutionStore {
	if x, ok := st.(HookExecutionStore); ok {
		return x
	}
	return nil
}

func (s *SQLiteStore) migrateHookExecutions() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS hook_executions (
			id TEXT PRIMARY KEY,
			tenant_id TEXT NOT NULL DEFAULT 'default',
			hook_id TEXT NOT NULL DEFAULT '',
			hook_name TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			event TEXT NOT NULL DEFAULT '',
			input_hash TEXT NOT NULL DEFAULT '',
			decision TEXT NOT NULL DEFAULT 'allow',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			retry INTEGER NOT NULL DEFAULT 0,
			dedup_key TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_hook_exec_tenant ON hook_executions(tenant_id, created_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

const hookExecColumns = `id, tenant_id, hook_id, hook_name, session_id, event, input_hash, decision, duration_ms, retry, dedup_key, error, created_at`

func (s *SQLiteStore) InsertHookExecution(e HookExecution) error {
	e.TenantID = NormalizeTenant(e.TenantID)
	if e.ID == "" {
		e.ID = "hx_" + newID()
	}
	if e.CreatedAt == "" {
		e.CreatedAt = formatTime(time.Now().UTC())
	}
	if e.Decision == "" {
		e.Decision = "allow"
	}
	_, err := s.db.Exec(`INSERT INTO hook_executions(`+hookExecColumns+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.TenantID, e.HookID, e.HookName, e.SessionID, e.Event, e.InputHash,
		e.Decision, e.DurationMS, e.Retry, e.DedupKey, e.Error, e.CreatedAt)
	return err
}

func (s *SQLiteStore) ListHookExecutions(tenant, hookID, event, decision string, limit, offset int) ([]*HookExecution, int) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	w := " WHERE tenant_id=?"
	args := []any{NormalizeTenant(tenant)}
	for _, pair := range [][2]string{{"hook_id", hookID}, {"event", event}, {"decision", decision}} {
		if strings.TrimSpace(pair[1]) != "" {
			w += " AND " + pair[0] + "=?"
			args = append(args, strings.TrimSpace(pair[1]))
		}
	}
	var total int
	_ = s.db.QueryRow(`SELECT count(*) FROM hook_executions`+w, args...).Scan(&total)
	rows, err := s.db.Query(`SELECT `+hookExecColumns+` FROM hook_executions`+w+` ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return []*HookExecution{}, total
	}
	defer rows.Close()
	out := []*HookExecution{}
	for rows.Next() {
		var e HookExecution
		if rows.Scan(&e.ID, &e.TenantID, &e.HookID, &e.HookName, &e.SessionID, &e.Event,
			&e.InputHash, &e.Decision, &e.DurationMS, &e.Retry, &e.DedupKey, &e.Error, &e.CreatedAt) != nil {
			continue
		}
		out = append(out, &e)
	}
	return out, total
}

// HookExecutionStats counts executions by decision.
func (s *SQLiteStore) HookExecutionStats(tenant string) map[string]int {
	rows, err := s.db.Query(`SELECT decision, count(*) FROM hook_executions WHERE tenant_id=? GROUP BY decision`, NormalizeTenant(tenant))
	if err != nil {
		return map[string]int{}
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if rows.Scan(&k, &n) == nil {
			out[k] = n
		}
	}
	return out
}

// Memory implementations keep the ledger available to the in-memory store used
// by tests and by the default single process mode.

func (s *Store) InsertHookExecution(e HookExecution) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e.ID == "" {
		e.ID = "hx_" + newID()
	}
	if e.CreatedAt == "" {
		e.CreatedAt = formatTime(time.Now().UTC())
	}
	if e.Decision == "" {
		e.Decision = "allow"
	}
	if s.hookExecutions == nil {
		s.hookExecutions = map[string]*HookExecution{}
	}
	copied := e
	s.hookExecutions[e.ID] = &copied
	return nil
}

func (s *Store) ListHookExecutions(tenant, hookID, event, decision string, limit, offset int) ([]*HookExecution, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	all := make([]*HookExecution, 0, len(s.hookExecutions))
	for _, e := range s.hookExecutions {
		if e.TenantID != NormalizeTenant(tenant) {
			continue
		}
		if hookID != "" && e.HookID != hookID {
			continue
		}
		if event != "" && e.Event != event {
			continue
		}
		if decision != "" && e.Decision != decision {
			continue
		}
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool { return all[i].CreatedAt > all[j].CreatedAt })
	total := len(all)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return all[offset:end], total
}

func (s *Store) HookExecutionStats(tenant string) map[string]int {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]int{}
	for _, e := range s.hookExecutions {
		if e.TenantID != NormalizeTenant(tenant) {
			continue
		}
		out[e.Decision]++
	}
	return out
}
