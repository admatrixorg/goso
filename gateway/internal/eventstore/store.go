// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package eventstore

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
	KindAttempt         = "attempt"
	KindSuccess         = "success"
	KindError           = "error"
	KindUnavailable     = "connector_unavailable"
	KindPendingApproval = "pending_approval"
	KindHumanFeedback   = "human_feedback"
)

// Event is a redacted audit record. Never include credentials.
type Event struct {
	TraceID   string    `json:"trace_id"`
	Connector string    `json:"connector"`
	Tool      string    `json:"tool"`
	Kind      string    `json:"kind"`
	TS        time.Time `json:"ts"`
	Summary   string    `json:"summary"`
}

// Store is an in-memory ring of events.
type Store struct {
	mu   sync.Mutex
	cap  int
	seq  int
	ring []Event
}

// New returns a ring with the given capacity (min 32).
func New(capacity int) *Store {
	if capacity < 32 {
		capacity = 32
	}
	return &Store{cap: capacity, ring: make([]Event, 0, capacity)}
}

// Append records an event. Summary is redacted.
func (s *Store) Append(e Event) Event {
	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	} else {
		e.TS = e.TS.UTC()
	}
	if e.TraceID == "" {
		e.TraceID = NewTraceID()
	}
	e.Summary = Redact(e.Summary)
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.ring) >= s.cap {
		s.ring = s.ring[1:]
	}
	s.ring = append(s.ring, e)
	s.seq++
	return e
}

// Filter lists newest-first events matching kind/connector (empty = any).
func (s *Store) Filter(kind, connector string, limit int) []Event {
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, 0, limit)
	for i := len(s.ring) - 1; i >= 0 && len(out) < limit; i-- {
		e := s.ring[i]
		if kind != "" && e.Kind != kind {
			continue
		}
		if connector != "" && e.Connector != connector {
			continue
		}
		out = append(out, e)
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

var tokenShape = regexp.MustCompile(`(?i)(sk-[A-Za-z0-9_-]{8,}|Bearer\s+[A-Za-z0-9._\-+=/]+)`)

func redactTokenShapes(s string) string {
	return tokenShape.ReplaceAllString(s, "[redacted]")
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

func redactValue(v any) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			lk := strings.ToLower(k)
			hide := false
			for _, sk := range secretKeys {
				if strings.Contains(lk, sk) {
					hide = true
					break
				}
			}
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

// SummarizeArgs builds a short JSON summary without credentials.
func SummarizeArgs(args map[string]any) string {
	if args == nil {
		return "{}"
	}
	cp := make(map[string]any, len(args))
	for k, v := range args {
		cp[k] = v
	}
	b, _ := json.Marshal(cp)
	return Redact(string(b))
}
