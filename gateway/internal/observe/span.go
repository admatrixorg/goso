// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

const (
	// KindAgent is the parent span of a chat run.
	KindAgent = "agent"
	// KindLLM is an LLM call child of the agent span.
	KindLLM = "llm"
	// KindTool is a tool invocation child of the agent span.
	KindTool = "tool"
)

type spanCtxKey struct{}
type currentSpanKey struct{}

// Span is one in-memory observability span (no prompts, no keys).
type Span struct {
	TraceID         string            `json:"trace_id"`
	SpanID          string            `json:"span_id"`
	ParentID        string            `json:"parent_id,omitempty"`
	Kind            string            `json:"kind"`
	Name            string            `json:"name"`
	Start           time.Time         `json:"start"`
	End             time.Time         `json:"end"`
	LatencyMS       int64             `json:"latency_ms"`
	Status          string            `json:"status,omitempty"`
	Error           string            `json:"error,omitempty"`
	CacheReadTokens int               `json:"cache_read_tokens"`
	Attributes      map[string]string `json:"attributes,omitempty"`
}

// SpanTree is one chat-run's nested spans (agent parent + llm/tool children).
type SpanTree struct {
	TraceID string `json:"trace_id"`
	Spans   []Span `json:"spans"`
}

// Collector holds spans for a single chat run.
type Collector struct {
	mu      sync.Mutex
	traceID string
	spans   []Span
}

// LiveSpan is an in-progress span that can be ended later.
type LiveSpan struct {
	col    *Collector
	idx    int
	spanID string
	ended  bool
}

// NewCollector allocates a collector with a fresh trace id.
func NewCollector() *Collector {
	return &Collector{traceID: newTraceID()}
}

// WithCollector stores col on ctx (tests / nested runs).
func WithCollector(ctx context.Context, col *Collector) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if col == nil {
		col = NewCollector()
	}
	return context.WithValue(ctx, spanCtxKey{}, col)
}

// CollectorFrom returns the collector stored by WithCollector / StartSpan.
func CollectorFrom(ctx context.Context) *Collector {
	if ctx == nil {
		return nil
	}
	c, _ := ctx.Value(spanCtxKey{}).(*Collector)
	return c
}

// StartSpan begins a span. The returned context has this span as current parent.
// LLM/tool siblings: start them from the agent context, not from the llm context.
func StartSpan(ctx context.Context, kind, name string) (context.Context, *LiveSpan) {
	if ctx == nil {
		ctx = context.Background()
	}
	col := CollectorFrom(ctx)
	if col == nil {
		col = NewCollector()
		ctx = WithCollector(ctx, col)
	}
	parent, _ := ctx.Value(currentSpanKey{}).(string)
	live := col.start(kind, name, parent)
	ctx = context.WithValue(ctx, currentSpanKey{}, live.spanID)
	return ctx, live
}

// SpansFrom snapshots spans collected on ctx.
func SpansFrom(ctx context.Context) []Span {
	col := CollectorFrom(ctx)
	if col == nil {
		return nil
	}
	return col.Snapshot()
}

func (c *Collector) start(kind, name, parent string) *LiveSpan {
	if kind == "" {
		kind = KindAgent
	}
	if name == "" {
		name = kind
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.traceID == "" {
		c.traceID = newTraceID()
	}
	sp := Span{
		TraceID:  c.traceID,
		SpanID:   newSpanID(),
		ParentID: parent,
		Kind:     kind,
		Name:     name,
		Start:    time.Now().UTC(),
		Status:   "ok",
	}
	c.spans = append(c.spans, sp)
	return &LiveSpan{col: c, idx: len(c.spans) - 1, spanID: sp.SpanID}
}

// Snapshot returns a copy of collected spans.
func (c *Collector) Snapshot() []Span {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]Span, len(c.spans))
	copy(out, c.spans)
	return out
}

// SetCacheReadTokens records prompt-cache read tokens (default 0).
func (s *LiveSpan) SetCacheReadTokens(n int) {
	if s == nil || s.col == nil {
		return
	}
	if n < 0 {
		n = 0
	}
	s.col.mu.Lock()
	defer s.col.mu.Unlock()
	if s.idx < 0 || s.idx >= len(s.col.spans) {
		return
	}
	s.col.spans[s.idx].CacheReadTokens = n
}

// SetAttr sets a non-secret attribute.
func (s *LiveSpan) SetAttr(k, v string) {
	if s == nil || s.col == nil || k == "" {
		return
	}
	s.col.mu.Lock()
	defer s.col.mu.Unlock()
	if s.idx < 0 || s.idx >= len(s.col.spans) {
		return
	}
	if s.col.spans[s.idx].Attributes == nil {
		s.col.spans[s.idx].Attributes = map[string]string{}
	}
	s.col.spans[s.idx].Attributes[k] = v
}

// SetStatus records a non-error status (e.g. pending_approval).
func (s *LiveSpan) SetStatus(status, errMsg string) {
	if s == nil || s.col == nil {
		return
	}
	s.col.mu.Lock()
	defer s.col.mu.Unlock()
	if s.idx < 0 || s.idx >= len(s.col.spans) {
		return
	}
	if status != "" {
		s.col.spans[s.idx].Status = status
	}
	if errMsg != "" {
		s.col.spans[s.idx].Error = errMsg
	}
}

// End finishes the span. Safe to call more than once.
func (s *LiveSpan) End(err error) {
	if s == nil || s.ended || s.col == nil {
		return
	}
	s.ended = true
	now := time.Now().UTC()
	s.col.mu.Lock()
	defer s.col.mu.Unlock()
	if s.idx < 0 || s.idx >= len(s.col.spans) {
		return
	}
	sp := &s.col.spans[s.idx]
	sp.End = now
	sp.LatencyMS = now.Sub(sp.Start).Milliseconds()
	if sp.LatencyMS < 0 {
		sp.LatencyMS = 0
	}
	if err != nil {
		sp.Status = "error"
		sp.Error = err.Error()
	} else if sp.Status == "" {
		sp.Status = "ok"
	}
}

func newTraceID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405")))
	}
	return hex.EncodeToString(b[:])
}

func newSpanID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().UTC().Format("15040500")))
	}
	return hex.EncodeToString(b[:])
}

// SpanTreeBuffer is a fixed-size ring of span trees (N=200).
type SpanTreeBuffer struct {
	mu   sync.Mutex
	cap  int
	buf  []SpanTree
	next int
	full bool
}

// NewSpanTreeBuffer retains at most n trees.
func NewSpanTreeBuffer(n int) *SpanTreeBuffer {
	if n < 1 {
		n = DefaultTraceCapacity
	}
	return &SpanTreeBuffer{cap: n, buf: make([]SpanTree, n)}
}

// Add appends t, dropping the oldest when full.
func (b *SpanTreeBuffer) Add(t SpanTree) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf[b.next] = t
	b.next = (b.next + 1) % b.cap
	if b.next == 0 {
		b.full = true
	}
}

// Len returns how many trees are stored.
func (b *SpanTreeBuffer) Len() int {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.full {
		return b.cap
	}
	return b.next
}

// Recent returns up to limit trees, newest first.
func (b *SpanTreeBuffer) Recent(limit int) []SpanTree {
	if b == nil {
		return []SpanTree{}
	}
	if limit <= 0 {
		limit = DefaultTraceLimit
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	n := b.next
	if b.full {
		n = b.cap
	}
	if limit > n {
		limit = n
	}
	out := make([]SpanTree, 0, limit)
	for i := 0; i < limit; i++ {
		idx := b.next - 1 - i
		if idx < 0 {
			idx += b.cap
		}
		out = append(out, b.buf[idx])
	}
	return out
}
