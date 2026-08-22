// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package billing

import (
	"database/sql"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// Record is one LLM call's token usage.
type Record struct {
	ID               string    `json:"id"`
	AgentID          string    `json:"agent_id"`
	Provider         string    `json:"provider"`
	PromptTokens     int       `json:"prompt_tokens"`
	CompletionTokens int       `json:"completion_tokens"`
	TotalTokens      int       `json:"total_tokens"`
	Estimated        bool      `json:"estimated"`
	CreatedAt        time.Time `json:"created_at"`
}

// Query filters GET /api/usage. Zero From/To means unbounded.
// To is exclusive (handler converts YYYY-MM-DD to start of next day).
type Query struct {
	AgentID  string
	Provider string
	From     time.Time
	To       time.Time
}

// DayBucket is per-day aggregation (SPEC 010: tổng hợp theo agent/ngày).
type DayBucket struct {
	Date             string `json:"date"`
	Calls            int    `json:"calls"`
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
}

// Summary is the GET /api/usage payload.
type Summary struct {
	AgentID          string      `json:"agent_id,omitempty"`
	Provider         string      `json:"provider,omitempty"`
	From             string      `json:"from,omitempty"`
	To               string      `json:"to,omitempty"`
	Calls            int         `json:"calls"`
	PromptTokens     int         `json:"prompt_tokens"`
	CompletionTokens int         `json:"completion_tokens"`
	TotalTokens      int         `json:"total_tokens"`
	ByDay            []DayBucket `json:"by_day"`
}

// Store holds usage records in memory and optionally SQLite.
type Store struct {
	mu      sync.Mutex
	records []Record
	db      *sql.DB
}

var usageSeq int64

func nextID() string {
	n := atomic.AddInt64(&usageSeq, 1)
	return time.Now().UTC().Format("20060102") + "-u" + itoa(n)
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	b := make([]byte, 0, 20)
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// New returns an in-memory usage store (trace-buffer style).
func New() *Store {
	return &Store{}
}

// Open returns a memory store for empty/":memory:" path, else SQLite at path.
func Open(path string) (*Store, func() error, error) {
	if path == "" || path == ":memory:" {
		s := New()
		return s, func() error { return nil }, nil
	}
	s, err := OpenSQLite(path)
	if err != nil {
		return nil, nil, err
	}
	return s, s.Close, nil
}

// OpenSQLite opens (and migrates) a SQLite usage table at path.
func OpenSQLite(path string) (*Store, error) {
	if path == "" {
		path = ":memory:"
	}
	if path != ":memory:" {
		_ = filepath.Dir(path)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS usage (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			provider TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL,
			completion_tokens INTEGER NOT NULL,
			total_tokens INTEGER NOT NULL,
			estimated INTEGER NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_usage_agent ON usage(agent_id);
		CREATE INDEX IF NOT EXISTS idx_usage_provider ON usage(provider);
		CREATE INDEX IF NOT EXISTS idx_usage_created ON usage(created_at);
	`)
	return err
}

// Close closes the SQLite handle if any.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// Add records one LLM call. Nil-safe.
func (s *Store) Add(r Record) {
	if s == nil {
		return
	}
	if r.ID == "" {
		r.ID = nextID()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	} else {
		r.CreatedAt = r.CreatedAt.UTC()
	}
	r.TotalTokens = r.PromptTokens + r.CompletionTokens
	if s.db != nil {
		est := 0
		if r.Estimated {
			est = 1
		}
		_, _ = s.db.Exec(
			`INSERT INTO usage(id, agent_id, provider, prompt_tokens, completion_tokens, total_tokens, estimated, created_at) VALUES(?,?,?,?,?,?,?,?)`,
			r.ID, r.AgentID, r.Provider, r.PromptTokens, r.CompletionTokens, r.TotalTokens, est, r.CreatedAt.Format(time.RFC3339Nano),
		)
		return
	}
	s.mu.Lock()
	s.records = append(s.records, r)
	s.mu.Unlock()
}

// AddCall is a convenience wrapper around Add.
func (s *Store) AddCall(agentID, provider string, promptTokens, completionTokens int, estimated bool) {
	s.Add(Record{
		AgentID:          agentID,
		Provider:         provider,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		Estimated:        estimated,
	})
}

// Query aggregates matching records.
func (s *Store) Query(q Query) Summary {
	if s == nil {
		return emptySummary(q)
	}
	recs := s.list(q)
	return summarize(q, recs)
}

func (s *Store) list(q Query) []Record {
	if s.db != nil {
		return s.listSQL(q)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, 0, len(s.records))
	for _, r := range s.records {
		if match(q, r) {
			out = append(out, r)
		}
	}
	return out
}

func (s *Store) listSQL(q Query) []Record {
	rows, err := s.db.Query(`SELECT id, agent_id, provider, prompt_tokens, completion_tokens, total_tokens, estimated, created_at FROM usage`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []Record
	for rows.Next() {
		var r Record
		var ts string
		var est int
		if err := rows.Scan(&r.ID, &r.AgentID, &r.Provider, &r.PromptTokens, &r.CompletionTokens, &r.TotalTokens, &est, &ts); err != nil {
			continue
		}
		r.Estimated = est != 0
		t, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			t, _ = time.Parse(time.RFC3339, ts)
		}
		r.CreatedAt = t.UTC()
		if match(q, r) {
			out = append(out, r)
		}
	}
	return out
}

func match(q Query, r Record) bool {
	if q.AgentID != "" && r.AgentID != q.AgentID {
		return false
	}
	if q.Provider != "" && r.Provider != q.Provider {
		return false
	}
	if !q.From.IsZero() && r.CreatedAt.Before(q.From) {
		return false
	}
	if !q.To.IsZero() && !r.CreatedAt.Before(q.To) {
		return false
	}
	return true
}

func emptySummary(q Query) Summary {
	return summarize(q, nil)
}

func summarize(q Query, recs []Record) Summary {
	sum := Summary{
		AgentID:  q.AgentID,
		Provider: q.Provider,
		ByDay:    []DayBucket{},
	}
	if !q.From.IsZero() {
		sum.From = q.From.UTC().Format("2006-01-02")
	}
	if !q.To.IsZero() {
		// To is exclusive next-day; report the last included day.
		sum.To = q.To.UTC().Add(-time.Nanosecond).Format("2006-01-02")
	}
	days := map[string]*DayBucket{}
	var order []string
	for _, r := range recs {
		sum.Calls++
		sum.PromptTokens += r.PromptTokens
		sum.CompletionTokens += r.CompletionTokens
		sum.TotalTokens += r.TotalTokens
		day := r.CreatedAt.UTC().Format("2006-01-02")
		b, ok := days[day]
		if !ok {
			b = &DayBucket{Date: day}
			days[day] = b
			order = append(order, day)
		}
		b.Calls++
		b.PromptTokens += r.PromptTokens
		b.CompletionTokens += r.CompletionTokens
		b.TotalTokens += r.TotalTokens
	}
	for _, d := range order {
		sum.ByDay = append(sum.ByDay, *days[d])
	}
	return sum
}
