// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package observe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mqglobal/goso/gateway/internal/logstore"
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

const (
	requestIDKey ctxKey = 1
	tenantKey    ctxKey = 2
	agentKey     ctxKey = 3
)

// Observer is the gateway observability hub: access logs, LLM traces, nested spans, counters.
type Observer struct {
	started   time.Time
	out       io.Writer
	traces    *Buffer
	spanTrees *SpanTreeBuffer
	logs      *logstore.Store
	exporter  Exporter
	reqs      atomic.Int64
	llms      atomic.Int64
	lastHB    atomic.Value // string RFC3339 UTC; empty until first stamp
	wsUp      atomic.Bool
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

// SetLogs attaches the operator log tail ring. Nil disables tailing.
func (o *Observer) SetLogs(s *logstore.Store) {
	if o == nil {
		return
	}
	o.logs = s
}

// Logs returns the operator log tail ring, or nil.
func (o *Observer) Logs() *logstore.Store {
	if o == nil {
		return nil
	}
	return o.logs
}

func (o *Observer) recordTail(v any) {
	if o == nil || o.logs == nil || v == nil {
		return
	}
	e := logstore.Entry{Component: logstore.ComponentGateway, Level: logstore.LevelInfo}
	switch t := v.(type) {
	case accessLog:
		e.Component = logstore.ComponentHTTP
		e.RequestID = t.RequestID
		switch {
		case t.Status >= 500:
			e.Level = logstore.LevelError
		case t.Status >= 400:
			e.Level = logstore.LevelWarn
		default:
			e.Level = logstore.LevelInfo
		}
		e.Message = fmt.Sprintf("%s %s %d %dms", t.Method, t.Path, t.Status, t.LatencyMS)
	case map[string]any:
		if s, _ := t["level"].(string); s != "" {
			e.Level = s
		}
		if s, _ := t["request_id"].(string); s != "" {
			e.RequestID = s
		}
		msg, _ := t["msg"].(string)
		switch msg {
		case "llm":
			e.Component = logstore.ComponentLLM
			provider, _ := t["provider"].(string)
			model, _ := t["model"].(string)
			errStr, _ := t["error"].(string)
			e.Message = strings.TrimSpace(strings.Join([]string{provider, model, fmt.Sprint(t["latency_ms"]) + "ms", errStr}, " "))
		case "otlp_export":
			e.Component = logstore.ComponentOTel
			errStr, _ := t["error"].(string)
			e.Message = strings.TrimSpace("otlp_export " + errStr)
		default:
			if msg == "" {
				msg = "log"
			}
			e.Message = msg
		}
	default:
		return
	}
	o.logs.Append(e)
}

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
	for i, s := range spans {
		copied[i] = PublicSpan(s)
	}
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

// RecordHeartbeat stamps last_heartbeat as RFC3339 UTC (application-level, not WS ping).
func (o *Observer) RecordHeartbeat(at time.Time) {
	if o == nil {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	o.lastHB.Store(at.UTC().Format(time.RFC3339))
}

// LastHeartbeat returns the RFC3339 UTC stamp, or "" if never fired.
func (o *Observer) LastHeartbeat() string {
	if o == nil {
		return ""
	}
	v, _ := o.lastHB.Load().(string)
	return v
}

// SetWsUp records that GET /ws is mounted.
func (o *Observer) SetWsUp(v bool) {
	if o == nil {
		return
	}
	o.wsUp.Store(v)
}

// WsUp reports WebSocket route readiness (not a connected client).
func (o *Observer) WsUp() bool {
	if o == nil {
		return false
	}
	return o.wsUp.Load()
}

// Register mounts GET /api/traces (and /v1/traces), GET /api/stats, and GET /metrics.
func (o *Observer) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/traces", o.HandleTraces)
	mux.HandleFunc("GET /v1/traces", o.HandleTraces)
	mux.HandleFunc("GET /api/traces/{id}", o.HandleTraceDetail)
	mux.HandleFunc("GET /v1/traces/{id}", o.HandleTraceDetail)
	mux.HandleFunc("GET /api/stats", o.HandleStats)
	mux.HandleFunc("GET /api/metrics", o.HandleStats) // SPEC 018 alias
	mux.HandleFunc("GET /metrics", o.HandleMetrics)
	mux.HandleFunc("POST /api/system/heartbeat", o.HandleHeartbeat)
	mux.HandleFunc("POST /v1/system/heartbeat", o.HandleHeartbeat)
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

// WithTenant stores the recording tenant on ctx.
func WithTenant(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, tenantKey, id)
}

// TenantFrom returns the tenant stored by WithTenant, or "".
func TenantFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(tenantKey).(string)
	return v
}

// WithAgent stores the recording agent id on ctx.
func WithAgent(ctx context.Context, id string) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	return context.WithValue(ctx, agentKey, id)
}

// AgentFrom returns the agent id stored by WithAgent, or "".
func AgentFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	v, _ := ctx.Value(agentKey).(string)
	return v
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b[:])
}
