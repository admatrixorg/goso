// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package eventstore

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	KindAttempt         = "attempt"
	KindSuccess         = "success"
	KindError           = "error"
	KindUnavailable     = "connector_unavailable"
	KindPendingApproval = "pending_approval"
	KindHumanFeedback   = "human_feedback"

	TypeConnector = "connector"
	TypeAgent     = "agent"
	TypeTeam      = "team"
	TypeTask      = "task"
	TypeMessage   = "message"
	TypeAgentLink = "agent_link"

	MaxSummaryBytes = 400
	MaxFilterLimit  = 500
	DefaultLimit    = 50
	MinCapacity     = 32
)

// Event is a redacted operator record. Never include credentials or message/tool payloads.
type Event struct {
	Seq       int64     `json:"seq,omitempty"`
	TraceID   string    `json:"trace_id"`
	Type      string    `json:"type,omitempty"`
	Kind      string    `json:"kind"`
	Action    string    `json:"action,omitempty"`
	Actor     string    `json:"actor,omitempty"`
	AgentID   string    `json:"agent_id,omitempty"`
	TeamID    string    `json:"team_id,omitempty"`
	Entity    string    `json:"entity,omitempty"`
	Connector string    `json:"connector,omitempty"`
	Tool      string    `json:"tool,omitempty"`
	TS        time.Time `json:"ts"`
	Summary   string    `json:"summary"`
}

// Query selects events. Empty fields match any. AfterSeq is exclusive.
type Query struct {
	Kind      string
	Connector string
	Type      string
	Actor     string
	Limit     int
	AfterSeq  int64
}

// Store is an in-memory ring of events with live subscribers.
type Store struct {
	mu   sync.Mutex
	cap  int
	seq  int64
	ring []Event
	subs map[int]chan Event
	subn int
}

// New returns a ring with the given capacity (min 32).
func New(capacity int) *Store {
	if capacity < MinCapacity {
		capacity = MinCapacity
	}
	return &Store{cap: capacity, ring: make([]Event, 0, capacity), subs: map[int]chan Event{}}
}

func normalize(e Event) Event {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	} else {
		e.TS = e.TS.UTC()
	}
	if e.TraceID == "" {
		e.TraceID = NewTraceID()
	}
	if strings.TrimSpace(e.Type) == "" {
		e.Type = TypeConnector
	}
	e.Summary = PublicSummary(e.Summary)
	return e
}

// Append records an event. Summary is redacted and payload keys are dropped.
func (s *Store) Append(e Event) Event {
	e = normalize(e)
	s.mu.Lock()
	if len(s.ring) >= s.cap {
		s.ring = s.ring[1:]
	}
	s.seq++
	e.Seq = s.seq
	s.ring = append(s.ring, e)
	for _, ch := range s.subs {
		select {
		case ch <- e:
		default:
		}
	}
	s.mu.Unlock()
	return e
}

// Subscribe receives live appends. Slow subscribers drop events. cancel unsubscribes.
func (s *Store) Subscribe(buf int) (<-chan Event, func()) {
	if buf <= 0 {
		buf = 16
	}
	ch := make(chan Event, buf)
	s.mu.Lock()
	s.subn++
	id := s.subn
	if s.subs == nil {
		s.subs = map[int]chan Event{}
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

// Filter lists newest-first events matching kind/connector (empty = any).
func (s *Store) Filter(kind, connector string, limit int) []Event {
	return s.Query(Query{Kind: kind, Connector: connector, Limit: limit})
}

// Query lists newest-first public events matching q.
func (s *Store) Query(q Query) []Event {
	limit := q.Limit
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxFilterLimit {
		limit = MaxFilterLimit
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0, limit)
	for i := len(s.ring) - 1; i >= 0 && len(out) < limit; i-- {
		e := s.ring[i]
		if !Match(e, q) {
			continue
		}
		out = append(out, Public(e))
	}
	return out
}

// Match reports whether e satisfies q. Actor matches actor, agent_id, or team_id.
func Match(e Event, q Query) bool {
	if q.AfterSeq > 0 && e.Seq <= q.AfterSeq {
		return false
	}
	if q.Kind != "" && e.Kind != q.Kind {
		return false
	}
	if q.Connector != "" && e.Connector != q.Connector {
		return false
	}
	if q.Type != "" && e.Type != q.Type {
		return false
	}
	if q.Actor != "" && e.Actor != q.Actor && e.AgentID != q.Actor && e.TeamID != q.Actor {
		return false
	}
	return true
}

// Public returns a copy safe for GET/stream: no payload secrets, capped summary.
func Public(e Event) Event {
	e.Summary = PublicSummary(e.Summary)
	return e
}

// PublicList maps Public over events.
func PublicList(list []Event) []Event {
	if list == nil {
		return []Event{}
	}
	out := make([]Event, len(list))
	for i, e := range list {
		out[i] = Public(e)
	}
	return out
}

// NewTraceID returns a random trace id.
func NewTraceID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return "tr-" + hex.EncodeToString(b[:])
}

var secretKeys = []string{"token", "password", "secret", "authorization", "api_key", "apikey", "bearer", "credential"}

var payloadKeys = []string{
	"arguments", "args", "body", "content", "messages", "prompt", "result",
	"tool_input", "tool_result", "text", "input", "output", "message",
}

var tokenShape = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|Bearer\s+[A-Za-z0-9._\-+=/]+)`)

func redactTokenShapes(s string) string {
	return tokenShape.ReplaceAllString(s, "[redacted]")
}

func hideKey(lk string, keys []string) bool {
	for _, sk := range keys {
		if lk == sk || strings.Contains(lk, sk) {
			return true
		}
	}
	return false
}

// Redact strips credential-like JSON keys and bearer/sk- token shapes from a summary.
func Redact(s string) string {
	if s == "" {
		return s
	}
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		redactValue(v)
		b, err := json.Marshal(v)
		if err == nil {
			return redactTokenShapes(string(b))
		}
	}
	return redactTokenShapes(s)
}

// PublicSummary redacts secrets, drops message/tool payload keys, and caps length.
func PublicSummary(s string) string {
	s = Redact(s)
	if s == "" {
		return s
	}
	var v any
	if json.Unmarshal([]byte(s), &v) == nil {
		dropPayload(v)
		b, err := json.Marshal(v)
		if err == nil {
			s = string(b)
		}
	}
	s = redactTokenShapes(s)
	if len(s) > MaxSummaryBytes {
		return s[:MaxSummaryBytes] + "…"
	}
	return s
}

func dropPayload(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			lk := strings.ToLower(k)
			if hideKey(lk, payloadKeys) || hideKey(lk, secretKeys) {
				delete(t, k)
				continue
			}
			dropPayload(child)
		}
	case []any:
		for _, child := range t {
			dropPayload(child)
		}
	}
}

func redactValue(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			lk := strings.ToLower(k)
			hide := hideKey(lk, secretKeys)
			if hide {
				t[k] = "[redacted]"
				continue
			}
			redactValue(child)
		}
	case []any:
		for _, child := range t {
			redactValue(child)
		}
	}
}

// SummarizeArgs builds a short JSON summary without credentials or payloads.
func SummarizeArgs(args map[string]any) string {
	if args == nil {
		return "{}"
	}
	cp := make(map[string]any, len(args))
	for k, v := range args {
		cp[k] = v
	}
	b, _ := json.Marshal(cp)
	return PublicSummary(string(b))
}

// SeqString formats a seq for Last-Event-ID.
func SeqString(seq int64) string {
	return strconv.FormatInt(seq, 10)
}
