// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	// HeaderRequestID is the HTTP request/correlation id header.
	HeaderRequestID = "X-Request-ID"
	// DefaultTraceCapacity is the in-memory LLM trace ring size (SPEC 008).
	DefaultTraceCapacity = 200
	// DefaultTraceLimit is the default GET /api/traces page size.
	DefaultTraceLimit = 20
)

type ctxKey int

const requestIDKey ctxKey = 1

// Observer is the gateway observability hub: access logs, LLM traces, nested spans, counters.
type Observer struct {
	started   time.Time
	out       io.Writer
	traces    *Buffer
	spanTrees *SpanTreeBuffer
	exporter  Exporter
	reqs      atomic.Int64
	llms      atomic.Int64
}

// New returns an Observer that logs JSON to stdout and keeps 200 LLM traces.
// OTLP export is enabled only when GOSO_OTEL_ENDPOINT is set; otherwise noop.
func New() *Observer {
	o := NewWithWriter(os.Stdout)
	o.exporter = ExporterFromEnv()
	return o
}

// NewWithWriter is like New but writes access/LLM logs to w (tests).
// Tests get a noop exporter; inject FakeExporter via SetExporter.
func NewWithWriter(w io.Writer) *Observer {
	if w == nil {
		w = os.Stdout
	}
	return &Observer{
		started:   time.Now().UTC(),
		out:       w,
		traces:    NewBuffer(DefaultTraceCapacity),
		spanTrees: NewSpanTreeBuffer(DefaultTraceCapacity),
		exporter:  NoopExporter{},
	}
}

// Traces returns the LLM trace ring buffer.
func (o *Observer) Traces() *Buffer { return o.traces }

// SpanTrees returns the nested-span ring buffer.
func (o *Observer) SpanTrees() *SpanTreeBuffer { return o.spanTrees }

// SetExporter replaces the OTLP exporter (tests). nil becomes noop.
func (o *Observer) SetExporter(e Exporter) {
	if o == nil {
		return
	}
	if e == nil {
		e = NoopExporter{}
	}
	o.exporter = e
}

// RecordSpans stores a chat-run span tree and exports it (noop when endpoint empty).
func (o *Observer) RecordSpans(spans []Span) {
	if o == nil || len(spans) == 0 {
		return
	}
	copied := make([]Span, len(spans))
	copy(copied, spans)
	tree := SpanTree{TraceID: copied[0].TraceID, Spans: copied}
	if o.spanTrees != nil {
		o.spanTrees.Add(tree)
	}
	o.dispatchExport(copied)
}

func (o *Observer) dispatchExport(spans []Span) {
	if o == nil || o.exporter == nil {
		return
	}
	if _, ok := o.exporter.(*HTTPExporter); ok {
		go o.doExport(spans)
		return
	}
	o.doExport(spans)
}

func (o *Observer) doExport(spans []Span) {
	if o == nil || o.exporter == nil {
		return
	}
	if err := o.exporter.Export(context.Background(), spans); err != nil {
		o.writeJSON(map[string]any{"level": "error", "msg": "otlp_export", "error": err.Error()})
	}
}

// Register mounts GET /api/traces, GET /api/stats, and GET /metrics.
func (o *Observer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/traces", o.HandleTraces)
	mux.HandleFunc("GET /api/stats", o.HandleStats)
	mux.HandleFunc("GET /api/metrics", o.HandleStats) // SPEC 018 alias
	mux.HandleFunc("GET /metrics", o.HandleMetrics)
}

// RequestIDFromContext returns the request id stored by Middleware, or "".
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(requestIDKey).(string)
	return v
}

// WithRequestID stores request id on ctx (tests / nested spans).
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
