// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package logstore

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	LevelDebug = "debug"
	LevelInfo  = "info"
	LevelWarn  = "warn"
	LevelError = "error"

	ComponentHTTP      = "http"
	ComponentLLM       = "llm"
	ComponentGateway   = "gateway"
	ComponentOTel      = "otel"
	ComponentChannel   = "channel"
	ComponentConnector = "connector"
	ComponentAgent     = "agent"
	ComponentCron      = "cron"
	ComponentAuth      = "auth"

	MaxMessageBytes = 400
	MaxFilterLimit  = 200
	DefaultLimit    = 50
	MinCapacity     = 32
)

// Entry is one redacted operator log line. Never include credentials.
type Entry struct {
	Seq       int64     `json:"seq,omitempty"`
	TS        time.Time `json:"ts"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
	RequestID string    `json:"request_id,omitempty"`
}

// Query selects entries. Empty fields match any. AfterSeq is exclusive.
type Query struct {
	Component string
	Level     string
	Q         string
	Limit     int
	AfterSeq  int64
}

// Store is an in-memory ring of log lines with live subscribers.
type Store struct {
	mu   sync.Mutex
	cap  int
	seq  int64
	ring []Entry
	subs map[int]chan Entry
	subn int
}

// New returns a ring with the given capacity (min 32).
func New(capacity int) *Store {
	if capacity < MinCapacity {
		capacity = MinCapacity
	}
	return &Store{cap: capacity, ring: make([]Entry, 0, capacity), subs: map[int]chan Entry{}}
}

func normalize(e Entry) Entry {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	} else {
		e.TS = e.TS.UTC()
	}
	e.Level = NormalizeLevel(e.Level)
	e.Component = NormalizeComponent(e.Component)
	e.RequestID = strings.TrimSpace(e.RequestID)
	e.Message = PublicMessage(e.Message)
	return e
}

// Append records a log line. Message is redacted and capped.
func (s *Store) Append(e Entry) Entry {
	e = normalize(e)
	s.mu.Lock()
	if len(s.ring) >= s.cap {
		s.ring = s.ring[1:]
	}
	s.seq++
	e.Seq = s.seq
	s.ring = append(s.ring, e)
	var dead []int
	for id, ch := range s.subs {
		select {
		case ch <- e:
		default:
			dead = append(dead, id)
		}
	}
	for _, id := range dead {
		if ch, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(ch)
		}
	}
	s.mu.Unlock()
	return e
}

// Subscribe receives live appends. Slow subscribers are dropped. cancel unsubscribes.
func (s *Store) Subscribe(buf int) (<-chan Entry, func()) {
	if buf <= 0 {
		buf = 16
	}
	ch := make(chan Entry, buf)
	s.mu.Lock()
	s.subn++
	id := s.subn
	if s.subs == nil {
		s.subs = map[int]chan Entry{}
	}
	s.subs[id] = ch
	s.mu.Unlock()
	var once sync.Once
	cancel := func() {
		once.Do(func() {
			s.mu.Lock()
			if c, ok := s.subs[id]; ok {
				delete(s.subs, id)
				close(c)
			}
			s.mu.Unlock()
		})
	}
	return ch, cancel
}

// Query lists newest-first public entries matching q.
func (s *Store) Query(q Query) []Entry {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxFilterLimit {
		limit = MaxFilterLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, 0, limit)
	for i := len(s.ring) - 1; i >= 0 && len(out) < limit; i-- {
		e := s.ring[i]
		if !Match(e, q) {
			continue
		}
		out = append(out, Public(e))
	}
	return out
}

// Components lists distinct component names currently in the ring.
func (s *Store) Components() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	for _, e := range s.ring {
		c := e.Component
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	return out
}

// Match reports whether e satisfies q.
func Match(e Entry, q Query) bool {
	if q.AfterSeq > 0 && e.Seq <= q.AfterSeq {
		return false
	}
	if q.Component != "" && e.Component != q.Component {
		return false
	}
	if q.Level != "" && !levelAllowed(e.Level, q.Level) {
		return false
	}
	if q.Q != "" && !textMatch(e, q.Q) {
		return false
	}
	return true
}

func levelAllowed(got, want string) bool {
	want = strings.TrimSpace(want)
	if want == "" {
		return true
	}
	got = NormalizeLevel(got)
	for _, part := range strings.Split(want, ",") {
		if NormalizeLevel(part) == got {
			return true
		}
	}
	return false
}

func textMatch(e Entry, q string) bool {
	needle := strings.ToLower(strings.TrimSpace(q))
	if needle == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{e.Message, e.Component, e.RequestID, e.Level}, " "))
	return strings.Contains(hay, needle)
}

// Public returns a copy safe for GET/stream.
func Public(e Entry) Entry {
	e.Message = PublicMessage(e.Message)
	e.Level = NormalizeLevel(e.Level)
	e.Component = NormalizeComponent(e.Component)
	return e
}

// PublicList maps Public over entries.
func PublicList(list []Entry) []Entry {
	if list == nil {
		return []Entry{}
	}
	out := make([]Entry, len(list))
	for i, e := range list {
		out[i] = Public(e)
	}
	return out
}

// NormalizeLevel maps aliases onto debug|info|warn|error.
func NormalizeLevel(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case LevelDebug, "dbg", "trace":
		return LevelDebug
	case LevelWarn, "warning":
		return LevelWarn
	case LevelError, "err", "fatal", "panic":
		return LevelError
	default:
		return LevelInfo
	}
}

// NormalizeComponent keeps a short, non-secret component label.
func NormalizeComponent(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ComponentGateway
	}
	if i := strings.IndexAny(s, "/\\ :"); i >= 0 {
		s = s[:i]
	}
	if utf8.RuneCountInString(s) > 32 {
		s = string([]rune(s)[:32])
	}
	return s
}

var secretKeys = []string{
	"token", "password", "secret", "authorization", "api_key", "apikey",
	"bearer", "credential", "hmac", "private_key", "bot_token", "access_token",
}

var tokenShape = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|gsk_[A-Za-z0-9]+|xai-[A-Za-z0-9]+|AIza[A-Za-z0-9_-]+|wh_[A-Za-z0-9]+|Bearer\s+[A-Za-z0-9._\-+=/]+)`)

var assignmentShape = regexp.MustCompile(`(?i)\b(token|password|secret|authorization|api[_-]?key|bearer|credential|hmac|private[_-]?key|bot[_-]?token|access[_-]?token)\s*[:=]\s*\S+`)

func redactTokenShapes(s string) string {
	s = tokenShape.ReplaceAllString(s, "[redacted]")
	return assignmentShape.ReplaceAllString(s, "${1}=[redacted]")
}

func hideKey(lk string, keys []string) bool {
	for _, sk := range keys {
		if lk == sk || strings.Contains(lk, sk) {
			return true
		}
	}
	return false
}

func dropSecrets(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			lk := strings.ToLower(k)
			if hideKey(lk, secretKeys) {
				delete(t, k)
				continue
			}
			dropSecrets(child)
		}
	case []any:
		for _, child := range t {
			dropSecrets(child)
		}
	}
}

// PublicMessage redacts credential shapes, drops secret JSON keys, and caps length.
func PublicMessage(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = redactTokenShapes(s)
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		dropSecrets(v)
		if b, err := json.Marshal(v); err == nil {
			s = string(b)
		}
	}
	s = redactTokenShapes(s)
	if utf8.RuneCountInString(s) > MaxMessageBytes {
		r := []rune(s)
		return string(r[:MaxMessageBytes]) + "…"
	}
	return s
}

// SeqString formats a seq for Last-Event-ID.
func SeqString(seq int64) string {
	return strconv.FormatInt(seq, 10)
}
