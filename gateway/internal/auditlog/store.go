// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package auditlog

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	MinCapacity     = 64
	DefaultCapacity = 4096
	DefaultLimit    = 50
	MaxLimit        = 200
	MaxMetaKeys     = 16
	MaxMetaBytes    = 400
)

// Record is an append-only administrative audit row. GET copies are redacted.
type Record struct {
	Seq      int64          `json:"seq"`
	ID       string         `json:"id"`
	Action   string         `json:"action"`
	Actor    string         `json:"actor"`
	Entity   string         `json:"entity"`
	EntityID string         `json:"entity_id,omitempty"`
	IP       string         `json:"ip,omitempty"`
	TS       time.Time      `json:"ts"`
	Before   map[string]any `json:"before,omitempty"`
	After    map[string]any `json:"after,omitempty"`
}

// Query selects records. Empty fields match any. BeforeSeq is exclusive (newer pages).
type Query struct {
	Action    string
	Actor     string
	Entity    string
	IP        string
	Since     time.Time
	Until     time.Time
	Limit     int
	BeforeSeq int64
}

// Page is a newest-first slice plus a stable cursor.
type Page struct {
	Records    []Record `json:"records"`
	Total      int      `json:"total"`
	Limit      int      `json:"limit"`
	Before     int64    `json:"before,omitempty"`
	NextBefore int64    `json:"next_before,omitempty"`
}

// Store is an in-memory append-only log. Existing rows are never updated or deleted.
// Overflow drops the oldest row only as a bound; remaining rows stay immutable.
type Store struct {
	mu   sync.Mutex
	cap  int
	seq  int64
	rows []Record
}

// New returns a log with the given capacity (min 64).
func New(capacity int) *Store {
	if capacity < MinCapacity {
		capacity = MinCapacity
	}
	return &Store{cap: capacity, rows: make([]Record, 0, capacity)}
}

func normalize(r Record) Record {
	if r.TS.IsZero() {
		r.TS = time.Now().UTC()
	} else {
		r.TS = r.TS.UTC()
	}
	if strings.TrimSpace(r.ID) == "" {
		r.ID = NewID()
	}
	r.Action = strings.TrimSpace(r.Action)
	r.Actor = strings.TrimSpace(r.Actor)
	r.Entity = strings.TrimSpace(r.Entity)
	r.EntityID = strings.TrimSpace(r.EntityID)
	r.IP = clipIP(r.IP)
	r.Before = PublicMeta(r.Before)
	r.After = PublicMeta(r.After)
	return r
}

// Append records a new row. Prior rows are not mutated.
func (s *Store) Append(r Record) Record {
	r = normalize(r)
	s.mu.Lock()
	if len(s.rows) >= s.cap {
		s.rows = s.rows[1:]
	}
	s.seq++
	r.Seq = s.seq
	s.rows = append(s.rows, r)
	s.mu.Unlock()
	return r
}

// Query lists newest-first public records matching q.
func (s *Store) Query(q Query) Page {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	out := make([]Record, 0, limit)
	var last int64
	for i := len(s.rows) - 1; i >= 0; i-- {
		e := s.rows[i]
		if !Match(e, q) {
			continue
		}
		total++
		if len(out) < limit {
			pub := Public(e)
			out = append(out, pub)
			last = pub.Seq
		}
	}
	page := Page{Records: out, Total: total, Limit: limit, Before: q.BeforeSeq}
	if total > len(out) && last > 0 {
		page.NextBefore = last
	}
	return page
}

// Match reports whether r satisfies q.
func Match(r Record, q Query) bool {
	if q.BeforeSeq > 0 && r.Seq >= q.BeforeSeq {
		return false
	}
	if q.Action != "" && r.Action != q.Action {
		return false
	}
	if q.Actor != "" && r.Actor != q.Actor {
		return false
	}
	if q.Entity != "" && r.Entity != q.Entity {
		return false
	}
	if q.IP != "" && r.IP != q.IP {
		return false
	}
	if !q.Since.IsZero() && r.TS.Before(q.Since) {
		return false
	}
	if !q.Until.IsZero() && !r.TS.Before(q.Until) {
		return false
	}
	return true
}

// Public returns a GET-safe copy: secrets stripped, metadata capped.
func Public(r Record) Record {
	r.Before = PublicMeta(r.Before)
	r.After = PublicMeta(r.After)
	r.IP = clipIP(r.IP)
	return r
}

var secretKeys = []string{
	"token", "password", "secret", "authorization", "api_key", "apikey", "bearer",
	"credential", "hmac", "private_key", "bot_token", "access_token", "hmac_key",
}

var payloadKeys = []string{
	"arguments", "args", "body", "content", "messages", "prompt", "result",
	"tool_input", "tool_result", "text", "input", "output", "message",
}

var tokenShape = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|Bearer\s+[A-Za-z0-9._\-+=/]+)`)

func hideKey(lk string) bool {
	if strings.HasSuffix(lk, "_set") {
		return false
	}
	for _, sk := range secretKeys {
		if lk == sk || strings.Contains(lk, sk) {
			return true
		}
	}
	for _, pk := range payloadKeys {
		if lk == pk {
			return true
		}
	}
	return false
}

// PublicMeta copies a before/after map with secret keys dropped and token shapes redacted.
func PublicMeta(in map[string]any) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	n := 0
	for k, v := range in {
		if n >= MaxMetaKeys {
			break
		}
		lk := strings.ToLower(strings.TrimSpace(k))
		if lk == "" || hideKey(lk) {
			continue
		}
		out[k] = publicValue(v)
		n++
	}
	if len(out) == 0 {
		return nil
	}
	b, err := json.Marshal(out)
	if err != nil {
		return map[string]any{"redacted": true}
	}
	if len(b) > MaxMetaBytes {
		return map[string]any{"truncated": true, "bytes": len(b)}
	}
	return out
}

func publicValue(v any) any {
	switch t := v.(type) {
	case nil:
		return nil
	case string:
		return redactTokenShapes(t)
	case bool, int, int64, float64, json.Number:
		return t
	case map[string]any:
		return PublicMeta(t)
	default:
		s, err := json.Marshal(t)
		if err != nil {
			return "[redacted]"
		}
		txt := redactTokenShapes(string(s))
		if len(txt) > 80 {
			txt = txt[:80] + "…"
		}
		return txt
	}
}

func redactTokenShapes(s string) string {
	return tokenShape.ReplaceAllString(s, "[redacted]")
}

func clipIP(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = strings.TrimSpace(s[:i])
	}
	if len(s) > 64 {
		s = s[:64]
	}
	return s
}

// NewID returns a random audit record id.
func NewID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "ar-" + hex.EncodeToString(b[:])
}
