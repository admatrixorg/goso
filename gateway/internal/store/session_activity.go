// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package store

import (
	"strings"
	"time"
)

// SessionActivity is one session's last message time next to the time of its
// latest L1 episodic note. The idle memory flush (pipeline.IdleFlusher) uses
// it to find sessions that went quiet with turns the note does not cover yet.
type SessionActivity struct {
	SessionID     string
	LastMessageAt time.Time
	SummaryAt     time.Time // zero when the session has no episodic note
}

// ActivityLister is the optional fast path for the idle flush: one query
// instead of ListSessions + ListMessages per session. SQLiteStore,
// PostgresStore (embedded) and the memory Store implement it.
type ActivityLister interface {
	// SessionActivity lists sessions whose last message is at or after since.
	// Times are a prefilter; the caller re-checks a candidate from its
	// messages before writing.
	SessionActivity(since time.Time) ([]SessionActivity, error)
}

var (
	_ ActivityLister = (*Store)(nil)
	_ ActivityLister = (*SQLiteStore)(nil)
)

// activityRole keeps the activity prefilter on conversation rows: tool
// results and events never make a session look newer than its note. (The
// exact per-turn check is pipeline.IsConversationTurn.)
func activityRole(role string) bool { return role == "user" || role == "assistant" }

// SessionActivity implements ActivityLister for the memory store.
func (s *Store) SessionActivity(since time.Time) ([]SessionActivity, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []SessionActivity{}
	for id, msgs := range s.messages {
		if _, ok := s.sessions[id]; !ok || len(msgs) == 0 {
			continue
		}
		var last time.Time
		for _, m := range msgs {
			if m != nil && activityRole(m.Role) && m.CreatedAt.After(last) {
				last = m.CreatedAt
			}
		}
		if last.IsZero() || last.Before(since) {
			continue
		}
		a := SessionActivity{SessionID: id, LastMessageAt: last}
		for _, m := range s.memories[id] {
			if m != nil && m.Kind == KindEpisodic && m.CreatedAt.After(a.SummaryAt) {
				a.SummaryAt = m.CreatedAt
			}
		}
		out = append(out, a)
	}
	return out, nil
}

// SessionActivity implements ActivityLister for SQLite and Postgres. Times are
// RFC3339Nano text, so MAX() and the since bound compare as strings; that is
// exact except inside one second (trailing zeros are trimmed), which the
// caller's re-check absorbs.
func (s *SQLiteStore) SessionActivity(since time.Time) ([]SessionActivity, error) {
	rows, err := s.db.Query(`SELECT m.session_id, MAX(m.created_at),
			(SELECT MAX(e.created_at) FROM memories e WHERE e.session_id = m.session_id AND e.kind = ?)
		FROM messages m
		JOIN sessions sess ON sess.id = m.session_id
		WHERE m.role IN ('user', 'assistant')
		GROUP BY m.session_id
		HAVING MAX(m.created_at) >= ?`, KindEpisodic, formatTime(since.Add(-time.Second)))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SessionActivity{}
	for rows.Next() {
		var id, last string
		var sum *string
		if err := rows.Scan(&id, &last, &sum); err != nil {
			return nil, err
		}
		a := SessionActivity{SessionID: id, LastMessageAt: parseTime(last)}
		if sum != nil && strings.TrimSpace(*sum) != "" {
			a.SummaryAt = parseTime(*sum)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
