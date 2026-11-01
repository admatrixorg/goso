// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode"
)

// RecallQuery selects L1 notes written in other sessions of the same tenant
// (and agent, when set) that lexically match Query. The pipeline uses it to
// inject prior-session memory; tenant scoping is mandatory at this layer.
//
// CustomerScoped is set for channel sessions: only notes stamped with the same
// non-empty CustomerKey (and the same agent) match. Unscoped notes never do,
// and an empty CustomerKey matches nothing (fail closed).
type RecallQuery struct {
	TenantID       string
	AgentID        string
	ExcludeSession string
	Query          string
	Limit          int
	CustomerScoped bool
	CustomerKey    string
}

// RecallHit is one candidate. Score is positive and higher is better, but it
// is only comparable within one result set (bm25 on FTS5, token overlap on
// the substring path).
type RecallHit struct {
	Memory Memory  `json:"memory"`
	Score  float64 `json:"score"`
}

const (
	recallDefaultLimit = 20
	recallMaxTokens    = 8
	recallScanCap      = 500
)

// RecallTokens lowercases q and splits it into lexical tokens of at least two
// runes (letters/digits only, diacritics kept), capped at recallMaxTokens.
func RecallTokens(q string) []string {
	var out []string
	seen := map[string]bool{}
	fields := strings.FieldsFunc(strings.ToLower(q), func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})
	for _, f := range fields {
		if len([]rune(f)) < 2 || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) >= recallMaxTokens {
			break
		}
	}
	return out
}

// lexicalScore is the fraction of toks present in body (0..1).
func lexicalScore(body string, toks []string) float64 {
	if len(toks) == 0 {
		return 0
	}
	lower := strings.ToLower(body)
	n := 0
	for _, t := range toks {
		if strings.Contains(lower, t) {
			n++
		}
	}
	return float64(n) / float64(len(toks))
}

func recallKindOK(kind string) bool {
	k := NormalizeMemoryKind(kind)
	return k == KindEpisodic || k == KindDurable
}

func sortRecall(hits []RecallHit, limit int) []RecallHit {
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].Memory.CreatedAt.After(hits[j].Memory.CreatedAt)
	})
	if limit > 0 && len(hits) > limit {
		hits = hits[:limit]
	}
	if hits == nil {
		hits = []RecallHit{}
	}
	return hits
}

// RecallMemories (in-memory store): token overlap over notes of other sessions.
func (s *Store) RecallMemories(q RecallQuery) ([]RecallHit, error) {
	toks := RecallTokens(q.Query)
	if len(toks) == 0 || (q.CustomerScoped && (q.CustomerKey == "" || strings.TrimSpace(q.AgentID) == "")) {
		return []RecallHit{}, nil
	}
	tenant := NormalizeTenant(q.TenantID)
	agent := strings.TrimSpace(q.AgentID)
	limit := q.Limit
	if limit <= 0 {
		limit = recallDefaultLimit
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var hits []RecallHit
	for sid, list := range s.memories {
		if sid == q.ExcludeSession {
			continue
		}
		sess, ok := s.sessions[sid]
		if !ok || sess == nil || !SameTenant(sess.TenantID, tenant) {
			continue
		}
		if agent != "" && sess.AgentID != agent {
			continue
		}
		for _, m := range list {
			if m == nil || !SameTenant(m.TenantID, tenant) || !recallKindOK(m.Kind) {
				continue
			}
			if q.CustomerScoped && m.CustomerKey != q.CustomerKey {
				continue
			}
			if sc := lexicalScore(m.Body, toks); sc > 0 {
				hits = append(hits, RecallHit{Memory: *m, Score: sc})
			}
		}
	}
	return sortRecall(hits, limit), nil
}

// PruneEpisodic (in-memory): drops episodic notes created before `before`.
// Durable notes are never pruned.
func (s *Store) PruneEpisodic(before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := 0
	for sid, list := range s.memories {
		kept := make([]*Memory, 0, len(list))
		for _, m := range list {
			if m != nil && NormalizeMemoryKind(m.Kind) == KindEpisodic && m.CreatedAt.Before(before) {
				n++
				continue
			}
			kept = append(kept, m)
		}
		s.memories[sid] = kept
	}
	return n, nil
}

// RecallMemories (SQL stores): bm25 over memory_fts when FTS5 is available,
// otherwise a bounded scan scored by token overlap (Postgres path too).
func (s *SQLiteStore) RecallMemories(q RecallQuery) ([]RecallHit, error) {
	toks := RecallTokens(q.Query)
	if len(toks) == 0 || (q.CustomerScoped && (q.CustomerKey == "" || strings.TrimSpace(q.AgentID) == "")) {
		return []RecallHit{}, nil
	}
	tenant := NormalizeTenant(q.TenantID)
	agent := strings.TrimSpace(q.AgentID)
	limit := q.Limit
	if limit <= 0 {
		limit = recallDefaultLimit
	}
	if s.fts {
		if hits, err := s.recallFTS(q, tenant, agent, toks, limit); err == nil {
			return hits, nil
		}
	}
	return s.recallScan(q, tenant, agent, toks, limit)
}

// recallScope appends the agent and customer filters shared by both paths.
func recallScope(query string, args []any, q RecallQuery, agent string) (string, []any) {
	if agent != "" {
		query += ` AND sess.agent_id = ?`
		args = append(args, agent)
	}
	if q.CustomerScoped {
		query += ` AND m.customer_key = ?`
		args = append(args, q.CustomerKey)
	}
	return query, args
}

func (s *SQLiteStore) recallFTS(q RecallQuery, tenant, agent string, toks []string, limit int) ([]RecallHit, error) {
	exclude := q.ExcludeSession
	parts := make([]string, 0, len(toks))
	for _, t := range toks {
		parts = append(parts, `"`+strings.ReplaceAll(t, `"`, `""`)+`"`)
	}
	match := strings.Join(parts, " OR ")
	query := `SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key, bm25(memory_fts)
		FROM memory_fts
		JOIN memories m ON m.id = memory_fts.id
		JOIN sessions sess ON sess.id = m.session_id
		WHERE memory_fts MATCH ?
		AND m.tenant_id = ? AND sess.tenant_id = ?
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`
	args := []any{match, tenant, tenant, KindEpisodic, KindDurable, exclude}
	query, args = recallScope(query, args, q, agent)
	query += ` ORDER BY bm25(memory_fts) LIMIT ?`
	args = append(args, limit)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []RecallHit
	for rows.Next() {
		var m Memory
		var ts, tn, ck string
		var rank float64
		if err := rows.Scan(&m.ID, &m.SessionID, &m.Kind, &m.Body, &ts, &tn, &ck, &rank); err != nil {
			continue
		}
		m.TenantID = NormalizeTenant(tn)
		m.CustomerKey = ck
		m.CreatedAt = parseTime(ts)
		hits = append(hits, RecallHit{Memory: m, Score: -rank})
	}
	return sortRecall(hits, limit), nil
}

func (s *SQLiteStore) recallScan(q RecallQuery, tenant, agent string, toks []string, limit int) ([]RecallHit, error) {
	exclude := q.ExcludeSession
	query := `SELECT m.id, m.session_id, m.kind, m.body, m.created_at, m.tenant_id, m.customer_key
		FROM memories m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE m.tenant_id = ? AND sess.tenant_id = ?
		AND m.kind IN (?, ?)
		AND m.session_id <> ?`
	args := []any{tenant, tenant, KindEpisodic, KindDurable, exclude}
	query, args = recallScope(query, args, q, agent)
	query += ` ORDER BY m.created_at DESC LIMIT ?`
	args = append(args, recallScanCap)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []RecallHit
	for rows.Next() {
		m, err := scanMemory(rows)
		if err != nil || m == nil {
			continue
		}
		if sc := lexicalScore(m.Body, toks); sc > 0 {
			hits = append(hits, RecallHit{Memory: *m, Score: sc})
		}
	}
	return sortRecall(hits, limit), nil
}

// PruneEpisodic (SQL stores): deletes episodic notes created before `before`.
// The memories_ad trigger keeps memory_fts in sync.
func (s *SQLiteStore) PruneEpisodic(before time.Time) (int, error) {
	if before.IsZero() {
		return 0, errors.New("before is required")
	}
	res, err := s.db.Exec(`DELETE FROM memories WHERE kind=? AND created_at < ?`, KindEpisodic, formatTime(before))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
