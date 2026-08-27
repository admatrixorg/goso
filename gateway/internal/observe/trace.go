// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
)

// Trace is one LLM call (no prompts, no keys, no response body).
type Trace struct {
	Time            time.Time `json:"ts"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model,omitempty"`
	LatencyMS       int64     `json:"latency_ms"`
	Error           string    `json:"error,omitempty"`
	RequestID       string    `json:"request_id,omitempty"`
	Tokens          *int      `json:"tokens,omitempty"`
	CacheReadTokens int       `json:"cache_read_tokens"`
}

// Buffer is a fixed-size in-memory ring of LLM traces (N=200).
type Buffer struct {
	mu   sync.Mutex
	cap  int
	buf  []Trace
	next int
	full bool
}

// NewBuffer creates a ring buffer that retains at most n traces.
func NewBuffer(n int) *Buffer {
	if n < 1 {
		n = DefaultTraceCapacity
	}
	return &Buffer{cap: n, buf: make([]Trace, n)}
}

// Add appends t, dropping the oldest entry when full.
func (b *Buffer) Add(t Trace) {
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

// Len returns how many traces are currently stored.
func (b *Buffer) Len() int {
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

// Recent returns up to limit traces, newest first. limit<=0 uses DefaultTraceLimit.
func (b *Buffer) Recent(limit int) []Trace {
	if b == nil {
		return []Trace{}
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
	out := make([]Trace, 0, limit)
	for i := 0; i < limit; i++ {
		idx := b.next - 1 - i
		if idx < 0 {
			idx += b.cap
		}
		out = append(out, b.buf[idx])
	}
	return out
}

// Record stores a trace, bumps the LLM counter, and writes a JSON log line.
func (o *Observer) Record(t Trace) {
	if o == nil {
		return
	}
	if t.Time.IsZero() {
		t.Time = time.Now().UTC()
	}
	o.llms.Add(1)
	o.traces.Add(t)
	level := "info"
	if t.Error != "" {
		level = "error"
	}
	entry := map[string]any{
		"level":      level,
		"msg":        "llm",
		"request_id": t.RequestID,
		"provider":   t.Provider,
		"model":      t.Model,
		"latency_ms": t.LatencyMS,
	}
	if t.Error != "" {
		entry["error"] = t.Error
	}
	o.writeJSON(entry)
}

// Wrap returns a provider that records a trace on every Chat call.
// ToolChat is preserved only when the inner provider implements it, so
// Anthropic stays on the ChatUsage path and cache_read_tokens reach llm spans.
func (o *Observer) Wrap(p llm.Provider) llm.Provider {
	if p == nil {
		p = llm.Echo{}
	}
	tp := &tracedProvider{inner: p, obs: o}
	if _, ok := p.(llm.ToolChat); ok {
		return &tracedToolProvider{tracedProvider: tp}
	}
	return tp
}

type tracedProvider struct {
	inner llm.Provider
	obs   *Observer
}

type tracedToolProvider struct {
	*tracedProvider
}

func (t *tracedProvider) Name() string { return t.inner.Name() }

func (t *tracedProvider) ChatUsage(ctx context.Context, messages []llm.Message) (string, llm.Usage, error) {
	start := time.Now()
	reply, usage, err := llm.ChatUsage(ctx, t.inner, messages)
	t.record(ctx, start, err, usage.CacheReadTokens)
	return reply, usage, err
}

func (t *tracedProvider) Chat(ctx context.Context, messages []llm.Message) (string, error) {
	reply, _, err := t.ChatUsage(ctx, messages)
	return reply, err
}

func (t *tracedToolProvider) ChatTools(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Reply, error) {
	start := time.Now()
	inner, ok := t.inner.(llm.ToolChat)
	if !ok {
		text, usage, err := llm.ChatUsage(ctx, t.inner, messages)
		t.record(ctx, start, err, usage.CacheReadTokens)
		return llm.Reply{Text: text, Usage: usage}, err
	}
	reply, err := inner.ChatTools(ctx, messages, tools)
	t.record(ctx, start, err, reply.Usage.CacheReadTokens)
	return reply, err
}

func (t *tracedProvider) record(ctx context.Context, start time.Time, err error, cacheRead int) {
	if cacheRead < 0 {
		cacheRead = 0
	}
	tr := Trace{
		Time:            time.Now().UTC(),
		Provider:        t.inner.Name(),
		Model:           modelOf(t.inner),
		LatencyMS:       time.Since(start).Milliseconds(),
		RequestID:       RequestIDFromContext(ctx),
		CacheReadTokens: cacheRead,
	}
	if err != nil {
		tr.Error = err.Error()
	}
	t.obs.Record(tr)
}

type namedModel interface {
	ModelName() string
}

func modelOf(p llm.Provider) string {
	if m, ok := p.(namedModel); ok {
		name := m.ModelName()
		if name != "" {
			return name
		}
	}
	if p != nil {
		return p.Name()
	}
	return ""
}

// HandleTraces serves GET /api/traces?limit=20.
func (o *Observer) HandleTraces(w http.ResponseWriter, r *http.Request) {
	limit := DefaultTraceLimit
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > DefaultTraceCapacity {
		limit = DefaultTraceCapacity
	}
	traces := o.traces.Recent(limit)
	if traces == nil {
		traces = []Trace{}
	}
	trees := []SpanTree{}
	if o.spanTrees != nil {
		trees = o.spanTrees.Recent(limit)
		if trees == nil {
			trees = []SpanTree{}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"traces": traces, "spans": trees})
}
