// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/mqglobal/goso/gateway/internal/llm"
	"github.com/mqglobal/goso/gateway/internal/tenant"
)

// Trace is one LLM call (no prompts, no keys, no response body).
type Trace struct {
	Time            time.Time `json:"ts"`
	Provider        string    `json:"provider"`
	Model           string    `json:"model,omitempty"`
	LatencyMS       int64     `json:"latency_ms"`
	Error           string    `json:"error,omitempty"`
	RequestID       string    `json:"request_id,omitempty"`
	TraceID         string    `json:"trace_id,omitempty"`
	AgentID         string    `json:"agent_id,omitempty"`
	Status          string    `json:"status,omitempty"`
	Tokens          *int      `json:"tokens,omitempty"`
	InputTokens     int       `json:"input_tokens,omitempty"`
	OutputTokens    int       `json:"output_tokens,omitempty"`
	CacheReadTokens int       `json:"cache_read_tokens"`
	TenantID        string    `json:"-"`
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

// All returns every stored trace, newest first.
func (b *Buffer) All() []Trace {
	if b == nil {
		return []Trace{}
	}
	return b.Recent(b.cap)
}

// Record stores a trace, bumps the LLM counter, and writes a JSON log line.
func (o *Observer) Record(t Trace) {
	if o == nil {
		return
	}
	if t.Time.IsZero() {
		t.Time = time.Now().UTC()
	}
	if t.Error != "" {
		t.Status = "error"
		t.Error = redactText(t.Error)
	} else if t.Status == "" {
		t.Status = "ok"
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
	_, isTool := p.(llm.ToolChat)
	_, isStreamTool := p.(llm.StreamToolChat)
	if isTool && isStreamTool {
		return &tracedStreamToolProvider{tracedToolProvider: &tracedToolProvider{tracedProvider: tp}}
	}
	if isTool {
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
	t.record(ctx, start, err, usage)
	return reply, usage, err
}

func (t *tracedProvider) Chat(ctx context.Context, messages []llm.Message) (string, error) {
	reply, _, err := t.ChatUsage(ctx, messages)
	return reply, err
}

func (t *tracedProvider) ChatStream(ctx context.Context, messages []llm.Message, onDelta llm.StreamHandler) (string, error) {
	reply, _, err := t.ChatStreamUsage(ctx, messages, onDelta)
	return reply, err
}

func (t *tracedProvider) ChatStreamUsage(ctx context.Context, messages []llm.Message, onDelta llm.StreamHandler) (string, llm.Usage, error) {
	start := time.Now()
	reply, usage, err := llm.ChatStream(ctx, t.inner, messages, onDelta)
	t.record(ctx, start, err, usage)
	return reply, usage, err
}

func (t *tracedToolProvider) ChatTools(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec) (llm.Reply, error) {
	start := time.Now()
	inner, ok := t.inner.(llm.ToolChat)
	if !ok {
		text, usage, err := llm.ChatUsage(ctx, t.inner, messages)
		t.record(ctx, start, err, usage)
		return llm.Reply{Text: text, Usage: usage}, err
	}
	reply, err := inner.ChatTools(ctx, messages, tools)
	t.record(ctx, start, err, reply.Usage)
	return reply, err
}

type tracedStreamToolProvider struct {
	*tracedToolProvider
}

func (t *tracedStreamToolProvider) ChatStreamTools(ctx context.Context, messages []llm.Message, tools []llm.ToolSpec, onDelta llm.StreamHandler) (llm.Reply, error) {
	start := time.Now()
	inner, ok := t.inner.(llm.StreamToolChat)
	if !ok {
		text, usage, err := llm.ChatStream(ctx, t.inner, messages, onDelta)
		t.record(ctx, start, err, usage)
		return llm.Reply{Text: text, Usage: usage}, err
	}
	reply, err := inner.ChatStreamTools(ctx, messages, tools, onDelta)
	t.record(ctx, start, err, reply.Usage)
	return reply, err
}

func (t *tracedProvider) record(ctx context.Context, start time.Time, err error, usage llm.Usage) {
	cacheRead := usage.CacheReadTokens
	if cacheRead < 0 {
		cacheRead = 0
	}
	tr := Trace{
		Time:            time.Now().UTC(),
		Provider:        t.inner.Name(),
		Model:           modelOf(t.inner),
		LatencyMS:       time.Since(start).Milliseconds(),
		RequestID:       RequestIDFromContext(ctx),
		AgentID:         AgentFrom(ctx),
		TenantID:        TenantFrom(ctx),
		InputTokens:     usage.PromptTokens,
		OutputTokens:    usage.CompletionTokens,
		CacheReadTokens: cacheRead,
	}
	if col := CollectorFrom(ctx); col != nil {
		tr.TraceID = col.TraceID()
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

// HandleTraces serves GET /api/traces with search, filters, time range, and pagination.
func (o *Observer) HandleTraces(w http.ResponseWriter, r *http.Request) {
	q := parseListQuery(r)
	want := tenant.Resolve(r)
	traces := o.publicTraces(want)
	trees := o.publicTrees(want)
	items := filterItems(buildItems(trees, traces), q)
	groups := groupErrors(items)
	total := len(items)
	page := pageItems(items, q.Offset, q.Limit)
	tracesOut := pageTraces(filterTraceList(traces, q), q.Offset, q.Limit)
	treesOut := pageTrees(filterTreeList(trees, q), q.Offset, q.Limit)
	truncated := q.Offset+len(page) < total
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"items":        page,
		"total":        total,
		"offset":       q.Offset,
		"limit":        q.Limit,
		"truncated":    truncated,
		"error_groups": groups,
		"traces":       tracesOut,
		"spans":        treesOut,
	})
}

// HandleTraceDetail serves GET /api/traces/{id} with a redacted bounded tree.
func (o *Observer) HandleTraceDetail(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		http.NotFound(w, r)
		return
	}
	want := tenant.Resolve(r)
	tree, ok := o.lookupTree(id, want)
	if !ok {
		http.NotFound(w, r)
		return
	}
	pub, trunc := PublicTree(tree)
	item := summarizeTree(pub)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"trace_id":  pub.TraceID,
		"item":      item,
		"spans":     pub.Spans,
		"truncated": trunc,
	})
}

func (o *Observer) publicTraces(want string) []Trace {
	if o == nil || o.traces == nil {
		return []Trace{}
	}
	raw := o.traces.All()
	out := make([]Trace, 0, len(raw))
	for _, t := range raw {
		if !sameTenantID(traceTenantID(t), want) {
			continue
		}
		out = append(out, PublicTrace(t))
	}
	return out
}

func (o *Observer) publicTrees(want string) []SpanTree {
	if o == nil || o.spanTrees == nil {
		return []SpanTree{}
	}
	raw := o.spanTrees.All()
	out := make([]SpanTree, 0, len(raw))
	for _, t := range raw {
		if !sameTenantID(treeTenant(t), want) {
			continue
		}
		pub, _ := PublicTree(t)
		out = append(out, pub)
	}
	return out
}

func (o *Observer) lookupTree(id, want string) (SpanTree, bool) {
	if o == nil {
		return SpanTree{}, false
	}
	if o.spanTrees != nil {
		if t, ok := o.spanTrees.Get(id); ok && sameTenantID(treeTenant(t), want) {
			return t, true
		}
	}
	if o.traces == nil {
		return SpanTree{}, false
	}
	for _, t := range o.traces.All() {
		if !sameTenantID(traceTenantID(t), want) {
			continue
		}
		if t.TraceID == id || t.RequestID == id {
			return SpanTree{
				TraceID: id,
				Spans: []Span{{
					TraceID:         id,
					Kind:            KindLLM,
					Name:            t.Provider,
					Start:           t.Time,
					LatencyMS:       t.LatencyMS,
					Status:          t.Status,
					Error:           t.Error,
					CacheReadTokens: t.CacheReadTokens,
					InputTokens:     t.InputTokens,
					OutputTokens:    t.OutputTokens,
					Attributes:      map[string]string{"model": t.Model, "agent_id": t.AgentID},
				}},
			}, true
		}
	}
	return SpanTree{}, false
}

func filterTraceList(traces []Trace, q ListQuery) []Trace {
	out := make([]Trace, 0, len(traces))
	for _, t := range traces {
		if matchItem(itemFromTrace(t), q) {
			out = append(out, t)
		}
	}
	return out
}

func filterTreeList(trees []SpanTree, q ListQuery) []SpanTree {
	out := make([]SpanTree, 0, len(trees))
	for _, t := range trees {
		if matchItem(summarizeTree(t), q) {
			out = append(out, t)
		}
	}
	return out
}

func pageTraces(in []Trace, offset, limit int) []Trace {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(in) {
		return []Trace{}
	}
	end := offset + limit
	if end > len(in) {
		end = len(in)
	}
	return in[offset:end]
}

func pageTrees(in []SpanTree, offset, limit int) []SpanTree {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(in) {
		return []SpanTree{}
	}
	end := offset + limit
	if end > len(in) {
		end = len(in)
	}
	return in[offset:end]
}
